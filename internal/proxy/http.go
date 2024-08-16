package proxy

import (
	"bufio"
	"bytes"
	"fmt"
	"hashrouter/internal/globals"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	// defaultHttpRequestHeadersMaxSizeBytes is the default maximum size of HTTP request headers
	defaultHttpRequestHeadersMaxSizeBytes = 4096
)

// Custom writer that captures only headers
type headerCaptureWriter struct {
	io.Writer
	headerFound bool
}

// headerCaptureWriter.Write is a writer that captures only the headers of an HTTP message
// It stops writing after the headers are found
func (w *headerCaptureWriter) Write(p []byte) (n int, err error) {
	if !w.headerFound {

		// Look for the end of headers (double CRLF)
		if i := bytes.Index(p, []byte("\r\n\r\n")); i != -1 {
			w.headerFound = true

			// Write headers part to buffer
			n, err = w.Writer.Write(p[:i+4])
			if err != nil {
				return n, err
			}

			// Pretend to have written all bytes to avoid short write
			return len(p), nil
		}
	}

	// If headers are found, discard any additional data
	if w.headerFound {
		return len(p), nil
	}
	return w.Writer.Write(p)
}

// sendErrorResponse send a static error response to the client
func sendErrorResponse(conn net.Conn, statusCode int, message string) error {
	// Create the HTTP response header
	response := fmt.Sprintf(
		"HTTP/1.1 %d %s\r\n"+
			"Content-Type: text/plain\r\n"+
			"Content-Length: %d\r\n"+
			"\r\n"+
			"%s",
		statusCode,
		http.StatusText(statusCode),
		len(message),
		message,
	)

	// Send the response through the connection
	_, err := conn.Write([]byte(response))
	return err
}

// peekHeadersOnly get a connection reader and reads only the headers of an HTTP request
// without consuming the reader buffer
func peekHeadersOnly(connectionReader *bufio.Reader, maxHeadersSizeBytes int) (*http.Request, error) {
	var peekBuffer []byte
	for {
		peeked, err := connectionReader.Peek(len(peekBuffer) + 1)
		if err != nil && err != bufio.ErrBufferFull {
			return nil, err
		}
		peekBuffer = append(peekBuffer, peeked[len(peekBuffer):]...)

		// Check if we have read all headers
		if bytes.Contains(peekBuffer, []byte("\r\n\r\n")) {
			break
		}

		// Limit the buffer size to avoid consuming too much memory
		if len(peekBuffer) > maxHeadersSizeBytes {
			return nil, fmt.Errorf("headers too large")
		}
	}

	// Create a new reader for the peeked data
	peekedReader := bufio.NewReader(bytes.NewReader(peekBuffer))

	// Read the first line of the request
	// It will be parsed later to obtain request data
	requestLine, err := peekedReader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	// Read the headers of the request
	requestHeaders := make(http.Header)
	for {
		line, err := peekedReader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		// Remember headers are separated from the body by a double CRLF
		// so when found, we stop reading headers
		if line == "\r\n" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed header line: %s", line)
		}
		requestHeaders.Add(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
	}

	// Parse the request line to get the method, URL and protocol version
	requestLineParts := strings.Split(requestLine, " ")
	if len(requestLineParts) != 3 {
		return nil, fmt.Errorf("malformed request line: %s", requestLine)
	}
	requestMethod := requestLineParts[0]
	requestUrl := requestLineParts[1]
	requestProto := strings.TrimSpace(requestLineParts[2])

	// Create an HTTP request using the read headers
	req := &http.Request{
		Method:     requestMethod,
		URL:        &url.URL{Path: requestUrl},
		Proto:      requestProto,
		Header:     requestHeaders,
		Host:       requestHeaders.Get("Host"),
		RequestURI: requestUrl,
	}
	return req, nil
}

// handleConnection handles an incoming client connection
func (p *Proxy) handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	// Create an HTTP data reader from the client.
	// Created here as headers are needed to determine the backend server
	// Remember once a reader is linked to a connection, io.Copy can not read directly from the connection pointer
	// and needs to read the stream from this reader
	clientConnectionReader := bufio.NewReader(clientConn)

	// Peek the headers of the HTTP request
	httpRequestHeadersMaxSizeBytes := defaultHttpRequestHeadersMaxSizeBytes
	if p.Config.Options.HttpRequestMaxHeadersSizeBytes > 0 {
		httpRequestHeadersMaxSizeBytes = p.Config.Options.HttpRequestMaxHeadersSizeBytes
	}

	httpRequestHeaders, err := peekHeadersOnly(clientConnectionReader, httpRequestHeadersMaxSizeBytes)
	if err != nil {
		globals.Application.Logger.Errorf("error reading HTTP request headers: %v", err.Error())
		sendErrorResponse(clientConn, http.StatusBadRequest, "Bad Request")
		return
	}

	// Connect hashring-assigned backend server
	hashKey := ReplaceRequestTagsString(httpRequestHeaders, p.Config.HashKey.Pattern)
	hashKey = p.Hashring.GetServer(hashKey)

	serverConn, err := net.Dial("tcp", hashKey)
	if err != nil {
		globals.Application.Logger.Errorf("error connecting to server: %v", err.Error())
		sendErrorResponse(clientConn, http.StatusServiceUnavailable, "Service Unavailable")
		return
	}
	defer serverConn.Close()

	//
	var wg sync.WaitGroup
	wg.Add(2)

	// Read the server response and forward it to the client
	var bufferResponse bytes.Buffer
	go func() {
		defer wg.Done()

		headerWriter := &headerCaptureWriter{Writer: &bufferResponse}
		multiWriter := io.MultiWriter(clientConn, headerWriter)

		_, err = io.Copy(multiWriter, serverConn)
		if err != nil {
			globals.Application.Logger.Errorf("error copying from server to client: %v", err)
		}

	}()

	// Read the client request and forward it to the server
	go func() {
		defer wg.Done()

		_, err = io.Copy(serverConn, clientConnectionReader)
		if err != nil {
			globals.Application.Logger.Errorf("error copying from client to server: %s", err.Error())
		}
	}()

	// Wait for both goroutines to finish
	wg.Wait()

	// Read bufferResponse as an HTTP response
	// At this point bufferResponse contains only the headers of the response
	bufferResponseReader := bufio.NewReader(&bufferResponse)
	httpResponse, err := http.ReadResponse(bufferResponseReader, httpRequestHeaders)
	if err != nil {
		fmt.Printf("error reading HTTP request: %s\n", err.Error())
		return
	}

	// TODO: Refine this log a bit
	if globals.Application.Config.Logs.ShowAccessLogs {
		logFields := BuildLogFields(httpRequestHeaders, httpResponse, globals.Application.Config.Logs.AccessLogsFields)
		globals.Application.Logger.Infow("received request", logFields...)
	}
}

func (p *Proxy) RunHttp() (err error) {

	// Launch the listener
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP(p.Config.Listener.Address), Port: p.Config.Listener.Port})
	if err != nil {
		return fmt.Errorf("error launching the listener: %v", err.Error())
	}
	defer listener.Close()

	globals.Application.Logger.Infof("proxy listening on %s", listener.Addr().String())

	// Handle incoming connections
	for {
		clientConn, err := listener.Accept()
		if err != nil {
			globals.Application.Logger.Infof("error accepting connection: %v", err)
			continue
		}
		go p.handleConnection(clientConn.(*net.TCPConn))
	}
}
