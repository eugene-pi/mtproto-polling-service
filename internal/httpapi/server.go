// Package httpapi exposes the current working proxy over a small local HTTP API.
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/eugene-pi/mtproto-polling-service/internal/proxy"
)

// Provider supplies the proxy currently being served.
type Provider interface {
	Current() (proxy.Proxy, bool)
}

// Server serves the current proxy as JSON.
type Server struct {
	srv      *http.Server
	provider Provider
}

// New builds a Server listening on addr (e.g. "127.0.0.1:8080").
func New(addr string, provider Provider) *Server {
	s := &Server{provider: provider}

	mux := http.NewServeMux()
	mux.HandleFunc("/proxy", s.handleProxy)
	mux.HandleFunc("/healthz", s.handleHealth)

	s.srv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

// Addr returns the configured listen address.
func (s *Server) Addr() string { return s.srv.Addr }

// Start runs the HTTP server until Shutdown is called. http.ErrServerClosed is
// returned as nil so a clean shutdown is not reported as an error.
func (s *Server) Start() error {
	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	p, ok := s.provider.Current()
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "no working proxy available yet",
		})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	_, ok := s.provider.Current()
	status := "searching"
	if ok {
		status = "ok"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    status,
		"hasProxy":  ok,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
