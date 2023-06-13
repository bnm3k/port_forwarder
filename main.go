package main

import (
	"context"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

type logger struct {
	info *log.Logger
	err  *log.Logger
}

func handleConn(ctx context.Context, forwardToAddr *net.TCPAddr,
	clientConn *net.TCPConn, logger logger) {
	forwardToConn, err := net.DialTCP("tcp", nil, forwardToAddr)
	if err != nil {
		logger.err.Println("on connect to forward addr:", err)
		clientConn.Close()
		return
	}

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
	clientConn.Close()
	forwardToConn.Close()
}

func main() {
	// config
	listenAt := ":8000"
	forwardTo := "localhost:9000"
	verbose := true

	// logger
	infoOut := ioutil.Discard
	if verbose {
		infoOut = os.Stdout
	}
	logger := logger{
		info: log.New(infoOut, "INFO: ", log.Ltime|log.Lmsgprefix),
		err:  log.New(os.Stdout, "ERROR: ", log.Ltime|log.Lmsgprefix),
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
		logger.info.Printf("receieved connection from: %v\n", clientConn.RemoteAddr())
		go handleConn(ctx, forwardToAddr, clientConn, logger)
	}

	cancel()
	logger.info.Println("exit")
}
