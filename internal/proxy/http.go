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

	"k8s.io/utils/strings/slices"
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
	if err != nil {
		return fmt.Errorf("error sending response: %s", err.Error())
	}

	err = conn.Close()
	if err != nil {
		return fmt.Errorf("client connection close error: %s", err)
	}

	return nil
}

// accumulateHeaders reads the headers of an HTTP request from a connection reader.
// It uses peek to avoid consuming the reader buffer
func accumulateHeaders(connectionReader *bufio.Reader, maxHeadersSizeBytes int) ([]byte, error) {
	var peekBuffer []byte

	for {
		peeked, err := connectionReader.Peek(len(peekBuffer) + 1)
		if err != nil && err != bufio.ErrBufferFull {

			// EOF is expected when the connection is closed
			if err == io.EOF {
				err = fmt.Errorf("unexpected EOF found while reading headers. Connection closed")
			}

			return peekBuffer, err
		}
		peekBuffer = append(peekBuffer, peeked[len(peekBuffer):]...)

		// Check if we have read all headers
		if bytes.Contains(peekBuffer, []byte("\r\n\r\n")) {
			break
		}

		// Limit the buffer size to avoid consuming too much memory
		if len(peekBuffer) > maxHeadersSizeBytes {
			return peekBuffer, fmt.Errorf("headers too large")
		}
	}

	return peekBuffer, nil
}

// parseRequestFromHeaders parses an HTTP request from the headers read from a connection
func parseRequestFromHeaders(peekBuffer []byte) (*http.Request, error) {
	peekedReader := bufio.NewReader(bytes.NewReader(peekBuffer))

	// Read the first line of the request (request line)
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

		// Headers are separated from the body by a double CRLF
		if line == "\r\n" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed header line: %s", line)
		}
		requestHeaders.Add(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
	}

	//
	requestLineParts := strings.Split(requestLine, " ")
	if len(requestLineParts) != 3 {
		return nil, fmt.Errorf("malformed request line: %s", requestLine)
	}
	requestMethod := requestLineParts[0]
	requestUrl := requestLineParts[1]
	requestProto := strings.TrimSpace(requestLineParts[2])

	//
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

// peekHeadersOnly get a connection reader and reads only the headers of an HTTP request
// without consuming the reader buffer
func peekHeadersOnly(connectionReader *bufio.Reader, maxHeadersSizeBytes int) (*http.Request, []byte, error) {
	peekBuffer, err := accumulateHeaders(connectionReader, maxHeadersSizeBytes)
	if err != nil {
		return nil, peekBuffer, err
	}

	req, err := parseRequestFromHeaders(peekBuffer)
	if err != nil {
		return nil, peekBuffer, err
	}

	return req, peekBuffer, nil
}

// handleConnection handles an incoming client connection
func (p *Proxy) handleConnection(clientConn net.Conn) {
	var err error

	// Closing client connection in the end only for premature errored responses
	// The connection closing is cleanly managed in the transfer goroutines
	defer clientConn.Close()

	// Set the connection reader buffer size
	httpRequestHeadersMaxSizeBytes := defaultHttpRequestHeadersMaxSizeBytes
	if p.Config.Options.HttpRequestMaxHeadersSizeBytes > 0 {
		httpRequestHeadersMaxSizeBytes = p.Config.Options.HttpRequestMaxHeadersSizeBytes
	}

	// Create an HTTP data reader from the client.
	// Created here as headers are needed to determine the backend server
	// Remember once a reader is linked to a connection, and has consumed bytes from it,
	// those bytes are not available for io.Copy or other readers
	clientConnectionReader := bufio.NewReaderSize(clientConn, httpRequestHeadersMaxSizeBytes)

	// Peek the headers of the HTTP request
	httpRequestHeaders, httpRequestHeadersBuffer, err := peekHeadersOnly(clientConnectionReader, httpRequestHeadersMaxSizeBytes)
	if err != nil {
		globals.Application.Logger.Errorf("error reading HTTP request headers: %v", err.Error())
		err = sendErrorResponse(clientConn, http.StatusBadRequest, "Bad Request")
		if err != nil {
			globals.Application.Logger.Errorf("error sending direct response to client: %v", err.Error())
		}
		return
	}

	// Figure out the backend server to connect to, according to the users' configured 'hask_key.pattern'
	// When the pattern can not be parsed the connection is not established as it's impossible to determine the backend
	// in a consistent way
	hashKey, err := ReplaceRequestTagsString(httpRequestHeaders, p.Config.HashKey.Pattern)
	if err != nil {
		globals.Application.Logger.Errorf("error calculating hash_key: %v", err.Error())
		err = sendErrorResponse(clientConn, http.StatusInternalServerError, "Internal Server Error")
		if err != nil {
			globals.Application.Logger.Errorf("error sending direct response to client: %v", err.Error())
		}
		return
	}

	dueBackend := p.Hashring.GetServer(hashKey)

	// Backend connection is always performed to the hashring-assigned backend server.
	// When the user enabled 'options.try_another_backend_on_failure', the proxy will try to connect
	// to the next server in the hashring until a connection is established or all servers are tried.
	hashringServerPool := p.Hashring.GetServerList()
	dueBackendPoolIndex := slices.Index(hashringServerPool, dueBackend)

	var serverConn net.Conn
	for i := 0; i < len(hashringServerPool); i++ {

		indexToTry := (dueBackendPoolIndex + i)

		// When the loop is in last item, start from the beginning
		if indexToTry >= len(hashringServerPool) {
			indexToTry = indexToTry - len(hashringServerPool)
		}

		// Establish connection to the server
		serverConn, err = net.Dial("tcp", hashringServerPool[indexToTry])
		if err == nil {
			break
		}

		// TODO: Discuss this message usefulness with more people
		globals.Application.Logger.Debugf("failed connecting to server '%s': %v", hashringServerPool[indexToTry], err.Error())

		// There is an error but user does not want to try another backend
		if !p.Config.Options.TryAnotherBackendOnFailure {
			globals.Application.Logger.Infof("'options.try_another_backend_on_failure' is disabled, skip trying another backend.")
			break
		}
	}

	if len(hashringServerPool) == 0 || err != nil {
		globals.Application.Logger.Errorf("failed connecting to all backend servers: %v", err.Error())
		err = sendErrorResponse(clientConn, http.StatusServiceUnavailable, "Service Unavailable")
		if err != nil {
			globals.Application.Logger.Errorf("error sending direct response to client: %v", err.Error())
		}
		return
	}

	/////////////////////////////////////////////////////////

	clientConnClosed := make(chan struct{}, 1)
	serverConnClosed := make(chan struct{}, 1)

	// Read the server response and forward it to the client
	// This routine is launched before, to keep the returning communication channel open
	var bufferResponse bytes.Buffer
	go func() {

		// Capture the headers of the response while it's transmitted
		headerWriter := &headerCaptureWriter{Writer: &bufferResponse}
		multiWriter := io.MultiWriter(clientConn, headerWriter)

		//
		_, err = io.Copy(multiWriter, serverConn)
		if err != nil {
			globals.Application.Logger.Errorf("error copying from server to client: %v", err)
		}

		if err := serverConn.Close(); err != nil {
			globals.Application.Logger.Errorf("server connection close error: %s", err)
		}
		serverConnClosed <- struct{}{}
	}()

	// Read the client request and forward it to the server
	go func() {
		// As headers are already read, we need to craft a new composed reader
		// to join the headers and the rest of the request
		requestHeadersReader := bytes.NewReader(httpRequestHeadersBuffer)
		composedReader := io.MultiReader(requestHeadersReader, clientConn)

		//
		_, err = io.Copy(serverConn, composedReader)
		if err != nil {
			globals.Application.Logger.Errorf("error copying from client to server: %s", err.Error())
		}

		if err := clientConn.Close(); err != nil {
			globals.Application.Logger.Errorf("client connection close error: %s", err)
		}
		clientConnClosed <- struct{}{}
	}()

	// wait for one half of the proxy to exit, then trigger a shutdown of the
	// other half by calling CloseRead(). This will break the read loop in the
	// broker and allow us to fully close the connection cleanly without a
	// "use of closed network connection" error.
	var waitFor chan struct{}

	select {

	case <-clientConnClosed:
		// The client (browser, curl, whatever) closed first and the packets from the backend
		// are not useful anymore, so close the connection with the backend using SetLinger(0) to
		// recycle the port faster
		serverConn.(*net.TCPConn).SetLinger(0)
		serverConn.(*net.TCPConn).CloseRead()
		waitFor = serverConnClosed

	case <-serverConnClosed:
		// Backend server closed first. This means backend could be down unexpectedly,
		// so close the connection to let the user try again
		clientConn.(*net.TCPConn).CloseRead()
		waitFor = clientConnClosed
	}

	// Wait for the other connection to close.
	// This "waitFor" pattern isn't required, but gives us a way to track the
	// connection and ensure all copies terminate correctly; we can trigger
	// stats on entry and deferred exit of this function.
	<-waitFor

	// Read bufferResponse as an HTTP response
	// At this point bufferResponse contains only the headers of the response
	bufferResponseReader := bufio.NewReader(&bufferResponse)
	httpResponse, err := http.ReadResponse(bufferResponseReader, httpRequestHeaders)
	if err != nil {
		globals.Application.Logger.Errorf("error reading HTTP request: %s\n", err.Error())
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

		//
		tcpConn, connOk := clientConn.(*net.TCPConn)
		if !connOk {
			globals.Application.Logger.Errorf("unexpected connection type: %T", clientConn)
			clientConn.Close()
			continue
		}

		go p.handleConnection(tcpConn)
	}
}
