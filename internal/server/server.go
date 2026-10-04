// Package server merangkai HTTP server JenderalRouter dengan graceful
// shutdown (NFR-11): request streaming aktif diberi waktu selesai.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Server membungkus http.Server dengan batas graceful shutdown.
type Server struct {
	http        *http.Server
	drainPeriod time.Duration
}

// New membuat server pada addr dengan handler dan batas waktu aman.
func New(addr string, h http.Handler) *Server {
	return &Server{
		http: &http.Server{
			Addr:              addr,
			Handler:           h,
			ReadHeaderTimeout: 15 * time.Second,
			// WriteTimeout sengaja tidak diset: streaming SSE berdurasi panjang.
			IdleTimeout:    120 * time.Second,
			MaxHeaderBytes: 1 << 20,
		},
		drainPeriod: 30 * time.Second, // NFR-11
	}
}

// ListenAndServe menjalankan server sampai ctx dibatalkan.
func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.http.Addr)
	if err != nil {
		return err
	}
	slog.Info("server mendengarkan", "addr", ln.Addr().String())
	errCh := make(chan error, 1)
	go func() { errCh <- s.http.Serve(ln) }()
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		slog.Info("shutdown dimulai; menyelesaikan request aktif", "drain", s.drainPeriod)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.drainPeriod)
		defer cancel()
		if err := s.http.Shutdown(shutdownCtx); err != nil {
			slog.Warn("shutdown dengan sisa koneksi", "err", err)
			_ = s.http.Close()
		}
		return nil
	}
}
