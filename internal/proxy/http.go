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
	//headerCaptured bool

	//headerCaptured chan<- struct{}
	headerFound bool
}

// TODO
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

// OLD
func (p *Proxy) handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	var bufferRequest bytes.Buffer
	var bufferResponse bytes.Buffer

	// Create an HTTP data reader from the client
	// It's size limited as only the path is needed
	// requestReader := io.LimitReader(clientConn, 1024)
	// req, err := http.ReadRequest(bufio.NewReader(requestReader))
	// if err != nil {
	// 	globals.Application.Logger.Errorf("error reading request: %v", err)
	// 	return
	// }

	// Connect hashring-assigned backend server
	//p.Config.HashKey.Pattern = req.URL.Path
	//hashKey := p.Hashring.GetServer(req.URL.Path)

	//p.ReplaceRequestTagsString(req, p.Config.HashKey.Pattern)

	serverConn, err := net.Dial("tcp", p.Hashring.GetServer("/text")) // construir el key
	if err != nil {
		globals.Application.Logger.Errorf("error connecting to server: %v", err)
		return
	}
	defer serverConn.Close()

	// Send the request to the backend server
	// err = req.Write(serverConn)
	// if err != nil {
	// 	globals.Application.Logger.Errorf("error writing request to server: %v", err)
	// 	return
	// }

	//resp := &http.Response{}
	var wg sync.WaitGroup
	wg.Add(2)

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
	go func() {
		defer wg.Done()

		headerWriter := &headerCaptureWriter{Writer: &bufferRequest}
		multiWriter := io.MultiWriter(serverConn, headerWriter)

		_, err = io.Copy(multiWriter, clientConn)
		if err != nil {
			globals.Application.Logger.Errorf("error copying from client to server: %s", err.Error())
		}
	}()

	// Wait for both goroutines to finish
	wg.Wait()

	// Leer el bufferRequest como una solicitud HTTP
	bufferRequestReader := bufio.NewReader(&bufferRequest)
	httpRequest, err := http.ReadRequest(bufferRequestReader)
	if err != nil {
		fmt.Printf("error reading HTTP request: %s\n", err.Error())
		return
	}

	// Leer el bufferResponse como una solicitud HTTP
	bufferResponseReader := bufio.NewReader(&bufferResponse)
	httpResponse, err := http.ReadResponse(bufferResponseReader, httpRequest)
	if err != nil {
		fmt.Printf("error reading HTTP request: %s\n", err.Error())
		return
	}

	// TODO: Refine this log
	if globals.Application.Config.Logs.ShowAccessLogs {
		logFields := BuildLogFields(httpRequest, httpResponse, globals.Application.Config.Logs.AccessLogsFields)
		globals.Application.Logger.Infow("received request", logFields...)
	}
}

func (p *Proxy) RunHttp() (err error) {

	//listenAddr := strings.Join([]string{p.Config.Listener.Address, strconv.Itoa(p.Config.Listener.Port)}, ":")

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
