package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() { os.Exit(run()) }

func run() int {
	path := flag.String("config", "config.json", "configuration file")
	addr := flag.String("listen", "127.0.0.1:8080", "listen address")
	flag.Parse()
	logger := log.New(os.Stderr, "metasearch: ", log.LstdFlags)
	c, err := loadConfig(*path)
	if err != nil {
		logger.Print(err) // Configuration errors are fixed strings, never input values.
		return 1
	}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		logger.Print("cannot open listener")
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := newServer(*addr, newHandler(c, logger), logger)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	logger.Print("started")
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Print("server stopped unexpectedly")
			return 1
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			logger.Print("graceful shutdown timed out")
			server.Close()
		}
		<-done
	}
	logger.Print("stopped")
	return 0
}
