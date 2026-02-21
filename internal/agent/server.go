// Package agent implements the DevOpsCtl agent daemon.
// It listens on a TCP (or UNIX) socket and handles commands from the controller.
package agent

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

// Server is the agent TCP server.
type Server struct {
	Addr string
}

// ListenAndServe starts the agent and blocks until a SIGTERM/SIGINT is received.
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("agent listen %s: %w", s.Addr, err)
	}
	log.Printf("[agent] listening on %s", s.Addr)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		<-ctx.Done()
		log.Printf("[agent] shutting down")
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			// After context cancellation the listener is closed — normal exit.
			select {
			case <-ctx.Done():
				return nil
			default:
				return fmt.Errorf("agent accept: %w", err)
			}
		}
		go handleConn(conn)
	}
}
