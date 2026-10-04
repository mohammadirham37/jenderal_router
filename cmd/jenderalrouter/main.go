// Command jenderalrouter adalah gateway LLM self-hosted multi-user.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jenderal/jenderalrouter/internal/api"
	"github.com/jenderal/jenderalrouter/internal/config"
	"github.com/jenderal/jenderalrouter/internal/server"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "-version" || os.Args[1] == "--version") {
		fmt.Println("jenderalrouter", api.Version)
		return
	}

	// mode healthcheck container (dipakai Dockerfile HEALTHCHECK)
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		addr := os.Getenv("JR_ADDR")
		if addr == "" {
			addr = "127.0.0.1:20130"
		}
		url := "http://" + addr + "/healthz"
		resp, err := http.Get(url)
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		resp.Body.Close()
		os.Exit(0)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("konfigurasi gagal", "err", err)
		os.Exit(1)
	}

	app, err := api.NewApp(cfg)
	if err != nil {
		slog.Error("inisialisasi aplikasi gagal", "err", err)
		os.Exit(1)
	}
	defer app.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := server.New(cfg.Addr, app.Handler())
	go app.RunBackground(ctx)

	if err := srv.ListenAndServe(ctx); err != nil && err != http.ErrServerClosed {
		slog.Error("server berhenti", "err", err)
		os.Exit(1)
	}
	slog.Info("jenderalrouter berhenti dengan bersih")
}
