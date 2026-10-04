package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jenderal/jenderalrouter/internal/store"
)

// Instalasi LlamaStash dari dashboard (FR-6.8): unduh installer, jalankan
// `llamastash init --recommended --json`, lalu daftarkan provider lokal
// beserta modelnya secara otomatis. Hanya mungkin di mode binary (aplikasi
// berjalan langsung di host); di container diberi petunjuk instalasi host.

const (
	llamastashInstallURL = "https://llamastash.dev/install.sh"
	llamastashInitCmd    = "init"
)

var (
	llamastashMu   sync.Mutex
	llamastashBusy bool
)

// findLlamastashBin mencari binary llamastash: PATH, lalu lokasi umum.
func findLlamastashBin() string {
	if p, err := exec.LookPath("llamastash"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		filepath.Join(home, ".local", "bin", "llamastash"),
		filepath.Join(home, "bin", "llamastash"),
		"/usr/local/bin/llamastash",
		"/usr/bin/llamastash",
		"/opt/llamastash/bin/llamastash",
	} {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

// llamastashVersion versi ringkas dari `llamastash --version`.
func llamastashVersion(bin string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// handleLocalInstallStatusStatus digabung ke handleLocalStatus (admin.go):
// enrichment installed/version/in_container dilakukan di localStatusExtra.

// localStatusExtra metadata tambahan untuk halaman Status LlamaStash.
func localStatusExtra() map[string]any {
	bin := findLlamastashBin()
	extra := map[string]any{
		"installed":    bin != "",
		"in_container": inContainer(),
		"busy":         llamastashBusy,
	}
	if bin != "" {
		extra["bin"] = bin
		if v := llamastashVersion(bin); v != "" {
			extra["version"] = v
		}
	}
	if inContainer() {
		extra["hint"] = "Aplikasi berjalan di container — LlamaStash harus dipasang di HOST. " +
			"SSH ke server lalu: curl -fsSL " + llamastashInstallURL + " | sh && llamastash init --recommended --json"
	}
	return extra
}

// handleLocalInstall POST /api/admin/local/install (FR-6.8).
func (a *App) handleLocalInstall(w http.ResponseWriter, r *http.Request) {
	if inContainer() {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "aplikasi berjalan di container — LlamaStash dipasang di HOST. Jalankan via SSH: " +
				"curl -fsSL " + llamastashInstallURL + " | sh && llamastash init --recommended --json",
		})
		return
	}
	llamastashMu.Lock()
	if llamastashBusy {
		llamastashMu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "instalasi sedang berjalan"})
		return
	}
	llamastashBusy = true
	llamastashMu.Unlock()
	defer func() {
		llamastashMu.Lock()
		llamastashBusy = false
		llamastashMu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()
	var log strings.Builder
	result := map[string]any{"ok": false, "log": ""}
	fail := func(status int, msg string) {
		log.WriteString("\n✗ " + msg + "\n")
		result["log"] = log.String()
		result["error"] = msg
		a.audit(r, "local.install", "llamastash", nil, map[string]any{"ok": false, "error": msg})
		writeJSON(w, status, result)
	}

	// 1) binary — unduh bila belum ada
	bin := findLlamastashBin()
	if bin == "" {
		log.WriteString("mengunduh installer dari " + llamastashInstallURL + " …\n")
		tmp := filepath.Join(os.TempDir(), "llamastash-install.sh")
		if err := runWithLog(ctx, &log, "curl", "-fsSL", llamastashInstallURL, "-o", tmp); err != nil {
			fail(http.StatusBadGateway, "unduh installer gagal: "+err.Error())
			return
		}
		log.WriteString("menjalankan installer…\n")
		if err := runWithLog(ctx, &log, "sh", tmp); err != nil {
			fail(http.StatusInternalServerError, "installer gagal: "+err.Error())
			return
		}
		_ = os.Remove(tmp)
		bin = findLlamastashBin()
		if bin == "" {
			fail(http.StatusInternalServerError, "installer selesai tetapi binary llamastash tidak ditemukan. "+log.String())
			return
		}
	} else {
		log.WriteString("llamastash sudah terpasang: " + bin + "\n")
	}
	if v := llamastashVersion(bin); v != "" {
		log.WriteString("versi: " + v + "\n")
		result["version"] = v
	}
	result["bin"] = bin

	// 2) init --recommended --json (auto setup tanpa interaksi, FR-6.8)
	if _, err := exec.CommandContext(ctx, bin, "list", "--json").CombinedOutput(); err != nil {
		log.WriteString("menjalankan '" + llamastashInitCmd + " --recommended --json'…\n")
		out, err := exec.CommandContext(ctx, bin, llamastashInitCmd, "--recommended", "--json").CombinedOutput()
		log.WriteString(strings.TrimSpace(string(out)) + "\n")
		if err != nil {
			fail(http.StatusInternalServerError, "init gagal: "+err.Error())
			return
		}
		// bearer key best-effort dari output JSON (proxy memakai key otomatis)
		if key := extractBearerLike(out); key != "" {
			if p, err := a.st.GetProviderByPrefix("local"); err == nil {
				if creds, _ := a.st.ListCredentials(p.ID); len(creds) == 0 {
					if _, err := a.st.AddCredential(p.ID, "auto", key, 1); err == nil {
						log.WriteString("bearer key terdeteksi & disimpan sebagai credential ✓\n")
					}
				}
			}
		}
	} else {
		log.WriteString("daemon sudah ter-inisialisasi ✓\n")
	}

	// 3) daftarkan provider lokal (idempoten) + sinkron model
	var providerRegistered bool
	p, err := a.st.GetProviderByPrefix("local")
	if err != nil {
		tpl := store.FindTemplate("local")
		if tpl == nil {
			fail(http.StatusInternalServerError, "template lokal tidak ditemukan")
			return
		}
		p, err = a.st.SeedProvidersFromTemplate(tpl)
		if err != nil {
			fail(http.StatusInternalServerError, "daftarkan provider gagal: "+err.Error())
			return
		}
		providerRegistered = true
	} else if !p.Enabled {
		enabled := true
		_ = a.st.UpdateProviderFields(p.ID, nil, nil, nil, &enabled)
		providerRegistered = true
	}
	result["provider_registered"] = providerRegistered

	// sinkron model dari proxy (FR-6.2)
	names, source, err := a.fetchUpstreamModels(p, "")
	if err == nil {
		for _, n := range names {
			_ = a.st.UpsertModelSinkron(p.ID, n, 0)
		}
		result["models"] = len(names)
		result["model_source"] = source
		log.WriteString(fmt.Sprintf("model tersinkron: %d (via %s)\n", len(names), source))
	}

	a.audit(r, "local.install", "llamastash", nil, map[string]any{
		"ok": true, "bin": bin, "provider_registered": providerRegistered,
	})
	result["ok"] = true
	result["log"] = log.String()
	writeJSON(w, http.StatusOK, result)
}

// extractBearerLike mencari nilai mirip API key pada output JSON init
// (kunci bernama key/token/bearer/secret, string ≥ 16 char). Best-effort.
func extractBearerLike(data []byte) string {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return ""
	}
	var found string
	var walk func(m map[string]any)
	walk = func(m map[string]any) {
		for k, val := range m {
			kl := strings.ToLower(k)
			if s, ok := val.(string); ok && len(s) >= 16 &&
				(strings.Contains(kl, "key") || strings.Contains(kl, "token") ||
					strings.Contains(kl, "bearer") || strings.Contains(kl, "secret")) {
				found = s
				return
			}
			if sub, ok := val.(map[string]any); ok {
				walk(sub)
				if found != "" {
					return
				}
			}
		}
	}
	if m, ok := v.(map[string]any); ok {
		walk(m)
	}
	return found
}
