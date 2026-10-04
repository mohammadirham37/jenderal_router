package api

import (
	"bufio"
	"bytes"
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

// ---- kontrak `status --json` (diverifikasi terhadap v0.6.1) ----

type llamastashStatusJSON struct {
	Models []map[string]any `json:"models"`
	Host   struct {
		CPUPct      float64 `json:"cpu_pct"`
		RAMUsed     int64   `json:"ram_used_bytes"`
		RAMTotal    int64   `json:"ram_total_bytes"`
		GPUMemUsed  *int64  `json:"gpu_mem_used_bytes"`
		GPUMemTotal *int64  `json:"gpu_mem_total_bytes"`
		GPUBackend  string  `json:"gpu_backend"`
	} `json:"host"`
	Daemon struct {
		PID   int    `json:"pid"`
		Build string `json:"build"`
		IPC   string `json:"ipc_url"`
	} `json:"daemon"`
	Proxy struct {
		Enabled bool   `json:"enabled"`
		Listen  string `json:"listen"`
		Status  string `json:"status"`
		Auth    string `json:"auth"`
		UIURL   string `json:"ui_url"`
	} `json:"proxy"`
}

// llamastashStatusData menjalankan `llamastash status --json`.
func llamastashStatusData(bin string) (*llamastashStatusJSON, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "status", "--json").Output()
	if err != nil {
		return nil, err
	}
	var st llamastashStatusJSON
	if err := json.Unmarshal(out, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// llamastashAPIKey mengambil bearer key proxy via CLI; "" bila keyless.
// Loopback keyless mencetak stub "llamastash" (nilai diabaikan proxy).
func llamastashAPIKey(bin string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "api-key").Output()
	if err != nil {
		return ""
	}
	key := strings.TrimSpace(string(out))
	if key == "" || key == "llamastash" {
		return ""
	}
	return key
}

// llamastashModelRow satu baris `llamastash list --json`. Objek `status`
// hanya ada pada baris yang launch-nya hidup (kontrak list --json v0.6.1);
// state-nya salah satu dari loading/ready/error/stopped/external.
type llamastashModelRow struct {
	Name         string `json:"name"`
	DisplayLabel string `json:"display_label"`
	Repo         string `json:"repo"`
	Status *struct {
		State string `json:"state"`
		Port  int    `json:"port"`
	} `json:"status"`
	Launches []struct {
		State string `json:"state"`
	} `json:"launches"`
}

// running melaporkan (loaded, state): status utama, lalu fallback ke
// launches[] — model bisa berjalan ganda; cukup satu launch hidup.
func (r llamastashModelRow) running() (bool, string) {
	st := ""
	if r.Status != nil {
		st = strings.ToLower(strings.TrimSpace(r.Status.State))
	}
	alive := func(s string) bool {
		return s != "" && s != "error" && s != "stopped"
	}
	if !alive(st) {
		for _, l := range r.Launches {
			if s := strings.ToLower(strings.TrimSpace(l.State)); alive(s) {
				st = s
				break
			}
		}
	}
	if !alive(st) {
		return false, ""
	}
	return true, st
}

func (r *llamastashModelRow) displayName() string {
	if r.Name != "" {
		return r.Name
	}
	return r.DisplayLabel
}

func llamastashCLIModelRows() ([]llamastashModelRow, error) {
	path := findLlamastashBin()
	if path == "" {
		return nil, fmt.Errorf("binary llamastash tidak ditemukan")
	}
	ctx := contextWithTimeoutCLI(10 * time.Second)
	out, err := exec.CommandContext(ctx, path, "list", "--json").Output()
	if err != nil {
		return nil, err
	}
	rows, err := parseLlamastashListJSON(out)
	if err != nil {
		return nil, fmt.Errorf("output list --json tidak dikenali: %w", err)
	}
	return rows, nil
}

// parseLlamastashListJSON menerima beberapa kemungkinan bentuk output:
// array polos [{name,status},…] atau objek berbungkus {models|items|rows:[…]}.
func parseLlamastashListJSON(out []byte) ([]llamastashModelRow, error) {
	raw := bytes.TrimSpace(out)
	if len(raw) == 0 {
		return nil, nil
	}
	var rows []llamastashModelRow
	if raw[0] == '[' {
		if err := json.Unmarshal(raw, &rows); err != nil {
			return nil, err
		}
	} else {
		var wrap struct {
			Models []llamastashModelRow `json:"models"`
			Items  []llamastashModelRow `json:"items"`
			Rows   []llamastashModelRow `json:"rows"`
		}
		if err := json.Unmarshal(raw, &wrap); err != nil {
			return nil, err
		}
		rows = wrap.Models
		if len(rows) == 0 {
			rows = wrap.Items
		}
		if len(rows) == 0 {
			rows = wrap.Rows
		}
	}
	for i := range rows {
		if rows[i].Name == "" {
			rows[i].Name = rows[i].DisplayLabel
		}
	}
	return rows, nil
}

// localStatusExtra metadata tambahan untuk halaman Status LlamaStash:
// binary terpasang + `status --json` (proxy.listen/auth/build, host stats,
// launch berjalan). proxyBase = Base URL provider lokal dari template.
func localStatusExtra(proxyBase string) map[string]any {
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
		if st, err := llamastashStatusData(bin); err == nil {
			extra["daemon_alive"] = st.Proxy.Status == "listening"
			extra["proxy_listen"] = st.Proxy.Listen
			extra["proxy_status"] = st.Proxy.Status
			extra["proxy_auth"] = st.Proxy.Auth
			extra["ui_url"] = st.Proxy.UIURL
			extra["daemon_pid"] = st.Daemon.PID
			extra["daemon_build"] = st.Daemon.Build
			extra["host"] = map[string]any{
				"cpu_pct":       round2(st.Host.CPUPct),
				"ram_used_gb":   round2(float64(st.Host.RAMUsed) / 1e9),
				"ram_total_gb":  round2(float64(st.Host.RAMTotal) / 1e9),
				"gpu_backend":   st.Host.GPUBackend,
				"gpu_mem_total": gbOrNil(st.Host.GPUMemTotal),
			}
			var running []string
			for _, m := range st.Models {
				for _, k := range []string{"name", "model", "id"} {
					if s2, ok := m[k].(string); ok && s2 != "" {
						running = append(running, s2)
						break
					}
				}
			}
			extra["running_models"] = running
			// porta proxy bisa bergeser (11435–11440) — beri tahu bila menyimpang
			if st.Proxy.Listen != "" && proxyBase != "" && !strings.Contains(proxyBase, strings.Split(st.Proxy.Listen, ":")[1]) {
				extra["listen_mismatch"] = true
				extra["hint"] = "Proxy mendengarkan di " + st.Proxy.Listen +
					" — sesuaikan Base URL provider lokal bila berbeda dari " + proxyBase
			}
		}
	}
	if inContainer() {
		extra["hint"] = "Aplikasi berjalan di container — LlamaStash harus dipasang di HOST. " +
			"SSH ke server lalu: curl -fsSL " + llamastashInstallURL + " | sh && llamastash init --recommended --json"
	}
	return extra
}

func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }

func gbOrNil(p *int64) any {
	if p == nil {
		return nil
	}
	return round2(float64(*p) / 1e9)
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
			key := llamastashAPIKey(bin) // kontrak resmi: `llamastash api-key`
			if key == "" {
				key = extractBearerLike(out) // fallback parsing output init
			}
			if key != "" {
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
