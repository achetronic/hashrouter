package proxy

import (
	"bufio"
	"bytes"
	"fmt"
	"hashrouter/internal/globals"
	"io"
	"net"
	"net/http"
	"sync"
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

// handleConnection handles an incoming client connection
func (p *Proxy) handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	//var bufferRequest bytes.Buffer
	var bufferResponse bytes.Buffer

	// Create an HTTP data reader from the client
	// It's size limited as only the path is needed
	// requestReader := io.LimitReader(clientConn, 1024)
	httpRequest, err := http.ReadRequest(bufio.NewReader(clientConn))
	if err != nil {
		globals.Application.Logger.Errorf("error reading request: %v", err)
		return
	}

	// Connect hashring-assigned backend server
	hashKey := ReplaceRequestTagsString(httpRequest, p.Config.HashKey.Pattern)
	hashKey = p.Hashring.GetServer(hashKey)

	serverConn, err := net.Dial("tcp", hashKey)
	if err != nil {
		globals.Application.Logger.Errorf("error connecting to server: %v", err)
		sendErrorResponse(clientConn, http.StatusServiceUnavailable, "Service Unavailable")
		return
	}
	defer serverConn.Close()

	// Write the request to the backend server as previously
	// we empty the buffer for reading the headers
	err = httpRequest.Write(serverConn)
	if err != nil {
		globals.Application.Logger.Errorf("error writing request to server: %v", err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(1)

	// Read the server response and forward it to the client
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
	// go func() {
	// 	defer wg.Done()

	// 	// TODO: Decide if we want to capture the headers here or on top of the function
	// 	headerWriter := &headerCaptureWriter{Writer: &bufferRequest}
	// 	multiWriter := io.MultiWriter(serverConn, headerWriter)

	// 	_, err = io.Copy(multiWriter, clientConn)
	// 	if err != nil {
	// 		globals.Application.Logger.Errorf("error copying from client to server: %s", err.Error())
	// 	}
	// }()

	// Wait for both goroutines to finish
	wg.Wait()

	// // Read bufferRequest as an HTTP request
	// bufferRequestReader := bufio.NewReader(&bufferRequest)
	// httpRequest, err := http.ReadRequest(bufferRequestReader)
	// if err != nil {
	// 	fmt.Printf("error reading HTTP request: %s\n", err.Error())
	// 	return
	// }

	// Read bufferResponse as an HTTP response
	bufferResponseReader := bufio.NewReader(&bufferResponse)
	httpResponse, err := http.ReadResponse(bufferResponseReader, httpRequest)
	if err != nil {
		fmt.Printf("error reading HTTP request: %s\n", err.Error())
		return
	}

	// TODO: Refine this log a bit
	if globals.Application.Config.Logs.ShowAccessLogs {
		logFields := BuildLogFields(httpRequest, httpResponse, globals.Application.Config.Logs.AccessLogsFields)
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
