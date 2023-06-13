package main

import (
	"context"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	flag "github.com/spf13/pflag"
)

type logger struct {
	info *log.Logger
	err  *log.Logger
}

type conns struct {
	holder map[*net.TCPConn]struct{}
	mu     sync.Mutex
}

func (cs *conns) removeAndCloseAll() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for conn := range cs.holder {
		conn.Close()
	}
}

func (cs *conns) add(conn *net.TCPConn) {
	cs.mu.Lock()
	cs.holder[conn] = struct{}{}
	cs.mu.Unlock()
}

func (cs *conns) removeAndClose(conn *net.TCPConn) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.holder, conn)
	conn.Close()
}

func pipe(ctx context.Context, forwardToConn *net.TCPConn,
	clientConn *net.TCPConn, logger logger) {
	// pipe
	ctx, cancel := context.WithCancel(ctx)
	copy := func(dst, src *net.TCPConn) {
		_, err := io.Copy(dst, src)
		if err != nil {
			logger.err.Println("on copy:", err)
		}
		cancel()
	}
	go copy(forwardToConn, clientConn)
	go copy(clientConn, forwardToConn)
	<-ctx.Done()
}

func main() {
	// logger
	logger := logger{
		info: log.New(os.Stdout, "INFO: ", log.Ltime|log.Lmsgprefix),
		err:  log.New(os.Stdout, "ERROR: ", log.Ltime|log.Lmsgprefix),
	}
	// defaults
	listenAt := "localhost:8000"
	forwardTo := ""
	verbose := true

	// get cli args
	flag.BoolVarP(&verbose, "verbose", "v", true, "Verbose")
	flag.StringVarP(&listenAt, "listen-at", "l", listenAt, "Address to listen at incoming connections")
	flag.StringVarP(&forwardTo, "forward-to", "f", "", "Address to forward incoming connections")
	flag.Parse()

	if !verbose {
		logger.info.SetOutput(ioutil.Discard)
	}
	if forwardTo == "" {
		logger.err.Fatal("arg `forward-to` for the address to forward to is empty")
	}

	// resolve addresses
	forwardToAddr, err := net.ResolveTCPAddr("tcp", forwardTo)
	if err != nil {
		logger.err.Fatal(err)
	}

	listenAtAddr, err := net.ResolveTCPAddr("tcp", listenAt)
	if err != nil {
		logger.err.Fatal(err)
	}

	logger.info.Printf("forwarding connections to: %v\n", forwardToAddr)
	logger.info.Printf("listening for connection at: %v\n", listenAtAddr)

	// create listener
	listener, err := net.ListenTCP("tcp", listenAtAddr)
	if err != nil {
		logger.err.Fatal(err)
	}

	// ctx
	ctx, cancel := context.WithCancel(context.Background())
	conns := conns{
		holder: make(map[*net.TCPConn]struct{}),
	}

	// handle close signal
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		logger.info.Printf("received signal: %s\n", sig.String())
		cancel()
		listener.Close()
	}()

	// listen for connections
loop:
	for {
		clientConn, err := listener.AcceptTCP()
		if err != nil {
			logger.err.Println(err)
			break loop
		}
		conns.add(clientConn)
		logger.info.Printf("receieved connection from: %v\n", clientConn.RemoteAddr())

		// handle conn
		go func() {
			forwardToConn, err := net.DialTCP("tcp", nil, forwardToAddr)
			if err != nil {
				logger.err.Println("on connect to forward addr:", err)
				conns.removeAndClose(clientConn)
				return
			}
			conns.add(forwardToConn)
			pipe(ctx, forwardToConn, clientConn, logger)
			conns.removeAndClose(forwardToConn)
			conns.removeAndClose(clientConn)
		}()
	}

	cancel()
	conns.removeAndCloseAll()
	logger.info.Println("exit")
}
