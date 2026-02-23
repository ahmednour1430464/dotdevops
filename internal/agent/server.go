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

	agentcontext "devopsctl/internal/agent/context"
)

// Server is the agent TCP server.
type Server struct {
	Addr         string
	ContextsPath string
	AuditLogPath string

	contexts    map[string]*agentcontext.ExecutionContext
	auditLogger *agentcontext.AuditLogger
}

// LoadConfiguration loads execution contexts and initializes the audit logger.
func (s *Server) LoadConfiguration() error {
	// Load contexts
	contexts, err := agentcontext.LoadContexts(s.ContextsPath)
	if err != nil {
		return fmt.Errorf("load contexts: %w", err)
	}
	s.contexts = contexts

	// Initialize audit logger
	if s.AuditLogPath != "" {
		logger, err := agentcontext.NewAuditLogger(s.AuditLogPath)
		if err != nil {
			return fmt.Errorf("init audit logger: %w", err)
		}
		s.auditLogger = logger
	}

	log.Printf("[agent] loaded %d execution contexts", len(s.contexts))
	return nil
}

// ListenAndServe starts the agent and blocks until a SIGTERM/SIGINT is received.
func (s *Server) ListenAndServe() error {
	// Load configuration first
	if err := s.LoadConfiguration(); err != nil {
		return err
	}
	defer func() {
		if s.auditLogger != nil {
			s.auditLogger.Close()
		}
	}()

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
		go handleConn(conn, s.contexts, s.auditLogger)
	}
}
