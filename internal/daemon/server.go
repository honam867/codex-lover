package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"codex-lover/internal/service"
)

type Server struct {
	address string
	svc     *service.Service
}

func New(address string, svc *service.Service) *Server {
	return &Server{
		address: address,
		svc:     svc,
	}
}

func (s *Server) Run(ctx context.Context, pollInterval time.Duration) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", s.handleStatus)
	mux.HandleFunc("/v1/refresh", s.handleRefresh)

	server := &http.Server{
		Addr:    s.address,
		Handler: mux,
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = s.svc.RefreshAll()
			}
		}
	}()

	_, _ = s.svc.RefreshAll()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("daemon listen: %w", err)
	}
	return nil
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	statuses, err := s.svc.ProfileStatuses()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, statuses)
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	statuses, err := s.svc.RefreshAll()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, statuses)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
