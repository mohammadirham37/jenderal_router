// Package api menyatukan seluruh handler HTTP: endpoint inferensi /v1/*,
// API manajemen /api/admin/*, sesi dashboard, dan health endpoint.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jenderal/jenderalrouter/internal/apigate"
	"github.com/jenderal/jenderalrouter/internal/config"
	"github.com/jenderal/jenderalrouter/internal/provider"
	"github.com/jenderal/jenderalrouter/internal/router"
	"github.com/jenderal/jenderalrouter/internal/store"
	"github.com/jenderal/jenderalrouter/internal/usage"
	"github.com/jenderal/jenderalrouter/internal/web"
)

// App memegang seluruh dependensi aplikasi (store, gate, client, dst).
type App struct {
	cfg       *config.Config
	st        *store.Store
	gate      *apigate.Gate
	client    *provider.Client
	breakers  *router.Breakers
	rec       *usage.Recorder
	resolver  router.Resolver
	localGate *localGate
	mux       *http.ServeMux
}

// NewApp melakukan inisialisasi penuh: db, migrasi, store, dan routing.
func NewApp(cfg *config.Config) (*App, error) {
	st, err := store.New(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	store.SetMasterKey(cfg.MasterKey)
	a := &App{
		cfg:       cfg,
		st:        st,
		gate:      apigate.NewGate(st),
		client:    provider.NewClient(),
		breakers:  router.NewBreakers(),
		rec:       usage.NewRecorder(st, 4096),
		resolver:  storeResolver{st: st},
		localGate: newLocalGate(),
		mux:       http.NewServeMux(),
	}
	a.routes()
	return a, nil
}

func (a *App) routes() {
	// kesehatan (NFR-13)
	a.mux.HandleFunc("GET /healthz", a.handleHealthz)
	a.mux.HandleFunc("GET /readyz", a.handleReadyz)
	a.mux.HandleFunc("GET /metrics", a.handleMetrics)

	// inferensi (semua wajib API key, NFR-01)
	a.mux.HandleFunc("POST /v1/chat/completions", a.chatInference)
	a.mux.HandleFunc("POST /v1/messages", a.messagesInference)
	a.mux.HandleFunc("GET /v1/models", a.modelsInference)
	a.mux.HandleFunc("GET /v1/usage/me", a.usageMe)

	// dashboard: setup wizard, sesi, admin API, playground (terdaftar di file lain)
	a.routesDashboard()

	// UI SvelteKit (embed, SPA)
	a.mux.Handle("/", web.Handler())
}

// Close membersihkan resource.
func (a *App) Close() {
	if a.rec != nil {
		a.rec.Close()
	}
	if a.st != nil {
		_ = a.st.Close()
	}
}

// RunBackground menjalankan worker latar: reset kuota, purge sesi, backup.
func (a *App) RunBackground(ctx context.Context) {
	stop := make(chan struct{})
	go usage.ResetWorker(a.st, a.cfg.TZ, stop)

	purge := time.NewTicker(10 * time.Minute)
	backup := time.NewTicker(time.Hour)
	circuit := time.NewTicker(30 * time.Second)
	defer purge.Stop()
	defer backup.Stop()
	defer circuit.Stop()
	lastBackupDay := ""
	for {
		select {
		case <-ctx.Done():
			close(stop)
			return
		case <-circuit.C:
			providers, err := a.st.ListProviders()
			if err == nil {
				for _, p := range providers {
					metrics.setCircuit(p.ID, a.breakers.For(p.ID).IsOpen())
				}
			}
		case <-purge.C:
			if n, err := a.st.PurgeExpiredSessions(); err == nil && n > 0 {
				slog.Info("sesi kedaluwarsa dibuang", "count", n)
			}
		case <-backup.C:
			if !a.cfg.BackupEnabled {
				continue
			}
			day := time.Now().Format("2006-01-02")
			if day != lastBackupDay && a.cfg.BackupHour == time.Now().Hour() {
				lastBackupDay = day
				if err := a.backupDatabase(); err != nil {
					slog.Error("backup harian gagal", "err", err)
				} else {
					slog.Info("backup harian selesai", "day", day)
				}
			}
		}
	}
}

func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": Version})
}

func (a *App) handleReadyz(w http.ResponseWriter, r *http.Request) {
	// siap bila DB bisa menjawab
	if err := a.st.DB.Ping(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "db tidak siap", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}

func (a *App) Handler() http.Handler {
	return a.mux
}
