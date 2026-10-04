package api

import (
	"bufio"
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

// Instalasi LlamaStash dari dashboard (FR-6.8) — BERJALAN ASINKRON:
// `init --recommended` dapat mengunduh model berukuran gigabyte, sehingga
// pekerjaan dilakukan di latar dengan log live yang dipoll dashboard.

const (
	llamastashInstallURL = "https://llamastash.dev/install.sh"
	llamastashInitCmd    = "init"
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

// ---- job asinkron ----

type installJob struct {
	mu       sync.Mutex
	running  bool
	phase    string
	log      []string
	started  time.Time
	finished time.Time
	done     bool
	ok       bool
	err      string
}

var installJobState installJobStateT

type installJobStateT struct {
	mu  sync.Mutex
	job *installJob
}

type installJobSnapshot struct {
	Running  bool     `json:"running"`
	Phase    string   `json:"phase,omitempty"`
	Log      []string `json:"log"`
	Done     bool     `json:"done"`
	OK       bool     `json:"ok"`
	Err      string   `json:"error,omitempty"`
	Started  string   `json:"started_at,omitempty"`
	Finished string   `json:"finished_at,omitempty"`
}

func (j *installJob) snapshot() installJobSnapshot {
	j.mu.Lock()
	defer j.mu.Unlock()
	s := installJobSnapshot{
		Running: j.running, Phase: j.phase, Done: j.done, OK: j.ok, Err: j.err,
		Started: j.started.Format(time.RFC3339),
	}
	if !j.finished.IsZero() {
		s.Finished = j.finished.Format(time.RFC3339)
	}
	n := len(j.log)
	from := 0
	if n > 60 {
		from = n - 60
	}
	s.Log = append([]string{}, j.log[from:]...)
	return s
}

func (j *installJob) setPhase(p string) {
	j.mu.Lock()
	j.phase = p
	j.mu.Unlock()
	j.appendLog("── %s", p)
}

func (j *installJob) appendLog(format string, args ...any) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.log = append(j.log, fmt.Sprintf(format, args...))
	if len(j.log) > 500 {
		j.log = j.log[len(j.log)-500:]
	}
}

func (j *installJob) fail(msg string) {
	j.mu.Lock()
	j.running, j.done, j.ok, j.err = false, true, false, msg
	j.finished = time.Now()
	j.mu.Unlock()
	j.appendLog("✗ %s", msg)
}

func (j *installJob) succeed() {
	j.mu.Lock()
	j.running, j.done, j.ok = false, true, true
	j.finished = time.Now()
	j.mu.Unlock()
	j.appendLog("✓ selesai")
}

// runStreamed menjalankan perintah dan menyalurkan output ke log job.
func (j *installJob) runStreamed(ctx context.Context, name string, args ...string) error {
	j.appendLog("$ %s %s", name, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		j.appendLog("gagal memulai: %s", err.Error())
		return err
	}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) != "" {
			j.appendLog("%s", line)
		}
	}
	return cmd.Wait()
}

// runOutput menjalankan perintah dan mengembalikan output (untuk parsing JSON).
func (j *installJob) runOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	j.appendLog("$ %s %s", name, strings.Join(args, " "))
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if trimmed != "" {
		for _, line := range strings.Split(trimmed, "\n") {
			j.appendLog("%s", line)
		}
	}
	return out, err
}

// localStatusExtra metadata tambahan untuk halaman Status LlamaStash.
func localStatusExtra() map[string]any {
	bin := findLlamastashBin()
	extra := map[string]any{
		"installed":    bin != "",
		"in_container": inContainer(),
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

// handleLocalInstall POST /api/admin/local/install — memulai job (202).
func (a *App) handleLocalInstall(w http.ResponseWriter, r *http.Request) {
	if inContainer() {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "aplikasi berjalan di container — LlamaStash dipasang di HOST. Jalankan via SSH: " +
				"curl -fsSL " + llamastashInstallURL + " | sh && llamastash init --recommended --json",
		})
		return
	}
	installJobState.mu.Lock()
	if installJobState.job != nil && installJobState.job.running {
		installJobState.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "instalasi sedang berjalan — pantau lognya"})
		return
	}
	job := &installJob{running: true, phase: "menyiapkan", started: time.Now()}
	installJobState.job = job
	installJobState.mu.Unlock()

	ai := authFrom(r)
	actorID := int64(0)
	if ai != nil {
		actorID = ai.user.ID
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()

		// 1) binary — unduh bila belum ada
		job.setPhase("mengunduh installer")
		bin := findLlamastashBin()
		if bin == "" {
			if err := job.runStreamed(ctx, "curl", "-fsSL", llamastashInstallURL, "-o", "/tmp/llamastash-install.sh"); err != nil {
				job.fail("unduh installer gagal: " + err.Error())
				a.st.Audit(actorID, "local.install", "llamastash", nil, map[string]any{"ok": false, "error": "download"})
				return
			}
			job.setPhase("menjalankan installer")
			if err := job.runStreamed(ctx, "sh", "/tmp/llamastash-install.sh"); err != nil {
				job.fail("installer gagal: " + err.Error())
				a.st.Audit(actorID, "local.install", "llamastash", nil, map[string]any{"ok": false, "error": "installer"})
				return
			}
			_ = os.Remove("/tmp/llamastash-install.sh")
			bin = findLlamastashBin()
			if bin == "" {
				job.fail("installer selesai tetapi binary llamastash tidak ditemukan")
				a.st.Audit(actorID, "local.install", "llamastash", nil, map[string]any{"ok": false, "error": "binary not found"})
				return
			}
		} else {
			job.appendLog("llamastash sudah terpasang: %s", bin)
		}
		if v := llamastashVersion(bin); v != "" {
			job.appendLog("versi: %s", v)
		}

		// 2) init --recommended --json (bisa lama: unduh model recommended)
		if _, err := exec.Command(bin, "list", "--json").CombinedOutput(); err != nil {
			job.setPhase("init — mengunduh model recommended (bisa sangat lama, tergantung koneksi)")
			out, err := job.runOutput(ctx, bin, llamastashInitCmd, "--recommended", "--json")
			if err != nil {
				job.fail("init gagal: " + err.Error())
				a.st.Audit(actorID, "local.install", "llamastash", nil, map[string]any{"ok": false, "error": "init"})
				return
			}
			if key := extractBearerLike(out); key != "" {
				if p, err := a.st.GetProviderByPrefix("local"); err == nil {
					if creds, _ := a.st.ListCredentials(p.ID); len(creds) == 0 {
						if _, err := a.st.AddCredential(p.ID, "auto", key, 1); err == nil {
							job.appendLog("bearer key terdeteksi & disimpan sebagai credential ✓")
						}
					}
				}
			}
		} else {
			job.appendLog("daemon sudah ter-inisialisasi ✓")
		}

		// 3) daftarkan provider lokal (idempoten)
		job.setPhase("mendaftarkan provider lokal")
		p, err := a.st.GetProviderByPrefix("local")
		if err != nil {
			tpl := store.FindTemplate("local")
			if tpl == nil {
				job.fail("template lokal tidak ditemukan")
				return
			}
			p, err = a.st.SeedProvidersFromTemplate(tpl)
			if err != nil {
				job.fail("daftarkan provider gagal: " + err.Error())
				return
			}
		} else if !p.Enabled {
			enabled := true
			_ = a.st.UpdateProviderFields(p.ID, nil, nil, nil, &enabled)
		}

		// 4) sinkronisasi model
		job.setPhase("sinkronisasi model")
		names, source, err := a.fetchUpstreamModels(p, "")
		if err == nil {
			for _, n := range names {
				_ = a.st.UpsertModelSinkron(p.ID, n, 0)
			}
			job.appendLog("model tersinkron: %d (via %s)", len(names), source)
		} else {
			job.appendLog("sinkronisasi model dilewati: %s", err.Error())
		}

		a.st.Audit(actorID, "local.install", "llamastash", nil, map[string]any{
			"ok": true, "bin": bin, "models": len(names),
		})
		job.succeed()
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{"started": true, "status_url": "/api/admin/local/install/status"})
}

// handleLocalInstallStatus GET /api/admin/local/install/status
func (a *App) handleLocalInstallStatus(w http.ResponseWriter, r *http.Request) {
	installJobState.mu.Lock()
	job := installJobState.job
	installJobState.mu.Unlock()
	if job == nil {
		writeJSON(w, http.StatusOK, installJobSnapshot{Running: false, Done: false, Log: []string{}})
		return
	}
	writeJSON(w, http.StatusOK, job.snapshot())
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
