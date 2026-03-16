// @guardian-project: guardian
// @guardian-path: internal/api/server.go
// Guardian HTTP API server on 127.0.0.1:8085 (ADR-013).
package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Harshmaury/Guardian/internal/api/handler"
)

// Server is the Guardian HTTP server.
type Server struct {
	http   *http.Server
	logger *log.Logger
}

// NewServer creates the Guardian HTTP server and registers routes.
func NewServer(addr string, store *handler.ReportStore, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}

	findingsH := handler.NewFindingsHandler(store)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health",                       handleHealth)
	mux.HandleFunc("GET /guardian/findings",             findingsH.All)
	mux.HandleFunc("GET /guardian/findings/{rule_id}",   findingsH.ByRule)

	return &Server{
		http: &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		logger: logger,
	}
}

// Run starts the server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Printf("Guardian API listening on %s", s.http.Addr)
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("guardian http: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.logger.Println("Guardian API shutting down...")
	return s.http.Shutdown(shutdownCtx)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"ok":true,"status":"healthy","service":"guardian"}`)) //nolint:errcheck
}
