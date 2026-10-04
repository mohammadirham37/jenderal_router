package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jenderal/jenderalrouter/internal/store"
)

// Fitur "Perbarui dari dashboard": sinkronisasi mirror repo (service-owned),
// build binary baru, stage, lalu (bila helper sudo terpasang) pasang dan
// restart service. Di mode container, hanya petunjuk yang diberikan.
//
// Desain keamanan:
//   - Mirror repo milik user service (/var/lib/jenderalrouter/src) — bukan
//     clone milik admin — sehingga git pull tidak menyentuh milik orang lain
//     dan aman di-reset hard.
//   - Pemasangan binary + restart hanya lewat helper root yang sempit
//     (/usr/local/lib/jenderalrouter/apply-update.sh) dengan aturan sudoers
//     NOPASSWD terbatas (opsional, dipasang oleh install-ubuntu.sh).

const containerMarker = "/.dockerenv"

var containerMarkerPath = containerMarker // bisa ditimpa di test

func inContainer() bool {
	_, err := os.Stat(containerMarkerPath)
	return err == nil
}

var (
	updateMu   sync.Mutex
	updateBusy bool
)

// applyUpdateScript lokasi helper root (dipasang install-ubuntu.sh).
const applyUpdateScript = "/usr/local/lib/jenderalrouter/apply-update.sh"

// stageDir tempat binary hasil build distage (milik user service).
func stageDir() string {
	if v := os.Getenv("JR_DATA_DIR"); v != "" {
		return filepath.Join(v, "updates")
	}
	return "/var/lib/jenderalrouter/updates"
}

// mirrorDir lokasi mirror repo (milik user service).
func mirrorDir() string {
	if v := os.Getenv("JR_REPO_DIR"); v != "" {
		return v
	}
	base := stageDir() // <data>/updates
	return filepath.Dir(base) + "/src"
}

const repoURL = "https://github.com/mohammadirham37/jenderal_router.git"

func isRepo(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && (fi.IsDir() || fi.Mode().IsRegular())
}

func gitCmd(ctx context.Context, repo string, args ...string) *exec.Cmd {
	full := append([]string{"-c", "safe.directory=" + repo, "-C", repo}, args...)
	return exec.CommandContext(ctx, "git", full...)
}

func runWithLog(ctx context.Context, log *strings.Builder, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	log.WriteString("$ " + name + " " + strings.Join(args, " ") + "\n")
	log.WriteString(strings.TrimSpace(string(out)))
	log.WriteString("\n")
	return err
}

// ensureMirror memastikan mirror repo siap: clone bila belum ada.
func ensureMirror(ctx context.Context, log *strings.Builder) (string, error) {
	dir := mirrorDir()
	if isRepo(dir) {
		return dir, nil
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	log.WriteString("$ git clone " + repoURL + " " + dir + "\n")
	cmd := exec.CommandContext(ctx, "git", "clone", repoURL, dir)
	out, err := cmd.CombinedOutput()
	log.WriteString(strings.TrimSpace(string(out)) + "\n")
	if err != nil {
		return "", fmt.Errorf("clone repo gagal: %v", err)
	}
	return dir, nil
}

func gitFetch(ctx context.Context, repo string) error {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	return gitCmd(ctx, repo, "fetch", "origin", "--prune").Run()
}

func gitBranch(repo string) string {
	out, err := gitCmd(context.Background(), repo, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "main"
	}
	return strings.TrimSpace(string(out))
}

func gitHead(repo string) string {
	out, err := gitCmd(context.Background(), repo, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitRemoteHead hash commit terakhir di origin/<branch>.
func gitRemoteHead(ctx context.Context, repo, branch string) string {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := gitCmd(ctx, repo, "rev-parse", "--short", "origin/"+branch).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitRemoteCommitInfo hash + subjek commit terbaru origin/<branch>.
func gitRemoteCommitInfo(ctx context.Context, repo, branch string) (hash, subject string) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := gitCmd(ctx, repo, "log", "-1", "--format=%h%x1f%s", "origin/"+branch).Output()
	if err != nil {
		return "", ""
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "\x1f", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

var (
	lastFetchMu   sync.Mutex
	lastFetchTime time.Time
)

func markFetched() {
	lastFetchMu.Lock()
	lastFetchTime = time.Now()
	lastFetchMu.Unlock()
}

func fetchStale() bool {
	lastFetchMu.Lock()
	defer lastFetchMu.Unlock()
	return time.Since(lastFetchTime) > 5*time.Minute
}

// gitBehind menghitung commit di belakang origin/<branch> (setelah fetch).
func gitBehind(ctx context.Context, repo string) (int, error) {
	branch := gitBranch(repo)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := gitCmd(ctx, repo, "rev-list", "--count", "HEAD..origin/"+branch).Output()
	if err != nil {
		return 0, err
	}
	var n int
	_, err = fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &n)
	return n, err
}

// buildStaged membangun binary dari mirror ke path stage.
func buildStaged(ctx context.Context, repo, outPath string) error {
	goBin, err := exec.LookPath("go")
	if err != nil {
		if _, e := os.Stat("/usr/local/go/bin/go"); e == nil {
			goBin = "/usr/local/go/bin/go"
		} else {
			return fmt.Errorf("Go tidak ditemukan (butuh go atau /usr/local/go/bin/go)")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, goBin, "build", "-trimpath", "-ldflags", "-s -w", "-o", outPath, "./cmd/jenderalrouter")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS="+runtime.GOOS,
		"GOARCH="+runtime.GOARCH,
		"PATH=/usr/local/go/bin:"+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build gagal: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// sudoApplyAvailable memeriksa helper root terpasang & diizinkan sudoers.
func sudoApplyAvailable() bool {
	if _, err := os.Stat(applyUpdateScript); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "sudo", "-n", applyUpdateScript, "check").Run() == nil
}

// lastUpdateMarker membaca catatan pembaruan terakhir (ditulis sebelum apply).
type lastUpdateInfo struct {
	Time    string `json:"time"`
	From    string `json:"from_version"`
	To      string `json:"to_version"`
	Commits int    `json:"commits"`
}

func readLastUpdate() lastUpdateInfo {
	var info lastUpdateInfo
	b, err := os.ReadFile(filepath.Join(stageDir(), "last-update.json"))
	if err == nil {
		_ = json.Unmarshal(b, &info)
	}
	return info
}

func writeLastUpdate(info lastUpdateInfo) error {
	_ = os.MkdirAll(stageDir(), 0o700)
	b, _ := json.MarshalIndent(info, "", "  ")
	return os.WriteFile(filepath.Join(stageDir(), "last-update.json"), b, 0o600)
}

// ---- handler status ----

func (a *App) handleSystemUpdateStatus(w http.ResponseWriter, r *http.Request) {
	st := updateStatusView(a, r)
	writeJSON(w, http.StatusOK, st)
}

func updateStatusView(a *App, r *http.Request) map[string]any {
	st := map[string]any{
		"version":          Version,
		"in_container":     inContainer(),
		"mode":             "binary",
		"busy":             updateBusy,
		"update_available": false,
	}
	if inContainer() {
		st["mode"] = "docker"
		st["hint"] = "Aplikasi berjalan di container: perbarui dari host dengan 'git pull && docker compose up -d --build' pada folder deploy/ repo."
		return st
	}
	_, gitOK := exec.LookPath("git")
	_, goOK := exec.LookPath("go")
	st["git_available"] = gitOK == nil
	st["go_available"] = goOK == nil || fileExists("/usr/local/go/bin/go")
	st["sudo_apply_available"] = sudoApplyAvailable()
	st["staged_binary"] = fileExists(filepath.Join(stageDir(), "jenderalrouter.new"))
	lu := readLastUpdate()
	if lu.Time != "" {
		st["last_update"] = lu
	}

	dir := mirrorDir()
	fetchParam := r.URL.Query().Get("fetch")
	needFetch := fetchParam == "1" || (fetchParam == "auto" && fetchStale())

	// mirror belum ada + diminta fetch → clone sekali agar perbandingan jalan
	if !isRepo(dir) && (needFetch || fetchParam == "1") {
		var log strings.Builder
		ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
		defer cancel()
		if _, err := ensureMirror(ctx, &log); err != nil {
			st["last_error"] = "clone mirror gagal: " + err.Error()
		}
	}

	if isRepo(dir) {
		st["repo_dir"] = dir
		st["repo_found"] = true
		st["branch"] = gitBranch(dir)
		localHead := gitHead(dir)
		st["current_head"] = localHead

		if needFetch {
			ctx, cancel := context.WithTimeout(r.Context(), 150*time.Second)
			defer cancel()
			if err := gitFetch(ctx, dir); err != nil {
				st["last_error"] = "fetch gagal: " + err.Error()
			} else {
				markFetched()
			}
		}

		branch := st["branch"].(string)
		remoteHead := gitRemoteHead(r.Context(), dir, branch)
		st["remote_head"] = remoteHead
		st["remote_commit"], st["remote_subject"] = gitRemoteCommitInfo(r.Context(), dir, branch)

		if n, err := gitBehind(r.Context(), dir); err == nil {
			st["behind_commits"] = n
			// ada pembaruan bila tertinggal commit ATAU head lokal ≠ remote
			st["update_available"] = n > 0 || (remoteHead != "" && remoteHead != localHead)
		} else if remoteHead != "" {
			st["update_available"] = remoteHead != localHead
		}
	} else {
		st["repo_dir"] = dir
		st["repo_found"] = false
	}
	return st
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// ---- handler POST /api/admin/system/update ----

func (a *App) handleSystemUpdate(w http.ResponseWriter, r *http.Request) {
	if inContainer() {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "mode container: perbarui dari host — 'git pull && docker compose up -d --build' pada folder deploy/ repo",
		})
		return
	}
	updateMu.Lock()
	if updateBusy {
		updateMu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "pembaruan sedang berjalan"})
		return
	}
	updateBusy = true
	updateMu.Unlock()
	defer func() {
		updateMu.Lock()
		updateBusy = false
		updateMu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Minute)
	defer cancel()
	var log strings.Builder
	fromVersion := Version
	result := map[string]any{"ok": false, "log": ""}

	fail := func(status int, msg string) {
		log.WriteString("\n✗ " + msg + "\n")
		result["log"] = log.String()
		result["error"] = msg
		a.audit(r, "system.update", "system", nil, map[string]any{"ok": false, "error": msg})
		writeJSON(w, status, result)
	}

	// 1) mirror siap
	dir, err := ensureMirror(ctx, &log)
	if err != nil {
		fail(http.StatusInternalServerError, err.Error())
		return
	}
	log.WriteString("mirror: " + dir + "\n")

	// 2) fetch + hitung ketinggalan
	if err := gitFetch(ctx, dir); err != nil {
		fail(http.StatusBadGateway, "fetch gagal: "+err.Error())
		return
	}
	behind, err := gitBehind(ctx, dir)
	if err != nil {
		fail(http.StatusInternalServerError, "hitung ketinggalan gagal: "+err.Error())
		return
	}
	log.WriteString(fmt.Sprintf("ketinggalan %d commit\n", behind))
	oldHead := gitHead(dir)

	// 3) reset mirror ke origin (mirror milik service; aman hard-reset)
	branch := gitBranch(dir)
	if err := runWithLog(ctx, &log, "git", "-c", "safe.directory="+dir, "-C", dir,
		"reset", "--hard", "origin/"+branch); err != nil {
		fail(http.StatusInternalServerError, "reset ke origin gagal: "+err.Error())
		return
	}
	if behind == 0 && oldHead == gitHead(dir) {
		result["ok"] = true
		result["already_latest"] = true
		result["log"] = log.String()
		result["message"] = "sudah versi terbaru"
		writeJSON(w, http.StatusOK, result)
		return
	}

	// 4) build binary baru ke stage
	stage := filepath.Join(stageDir(), "jenderalrouter.new")
	if err := os.MkdirAll(stageDir(), 0o700); err != nil {
		fail(http.StatusInternalServerError, err.Error())
		return
	}
	log.WriteString("build binary baru…\n")
	if err := buildStaged(ctx, dir, stage); err != nil {
		fail(http.StatusInternalServerError, err.Error())
		return
	}

	// 5) smoke test binary stage: jalankan -version
	if out, err := exec.CommandContext(ctx, stage, "-version").CombinedOutput(); err != nil {
		fail(http.StatusInternalServerError, "binary stage tidak bisa dijalankan: "+string(out))
		return
	} else {
		log.WriteString("smoke: " + strings.TrimSpace(string(out)) + "\n")
	}

	// 6) pasang + restart bila helper sudo tersedia; else minta langkah manual
	if sudoApplyAvailable() {
		_ = writeLastUpdate(lastUpdateInfo{
			Time: time.Now().UTC().Format(time.RFC3339), From: fromVersion,
			To: "origin/" + branch, Commits: behind,
		})
		log.WriteString("memasang & me-restart service (helper sudo)…\n")
		applyCtx, applyCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer applyCancel()
		if err := exec.CommandContext(applyCtx, "sudo", "-n", applyUpdateScript, "install").Run(); err != nil {
			log.WriteString("apply gagal: " + err.Error() + "\n")
			result["ok"] = false
			result["log"] = log.String()
			result["error"] = "apply gagal: " + err.Error()
			writeJSON(w, http.StatusInternalServerError, result)
			return
		}
		markFetched()
		result["ok"] = true
		result["restarted"] = true
		result["commits"] = behind
		result["log"] = log.String()
		a.audit(r, "system.update", "system", map[string]string{"version": fromVersion},
			map[string]any{"commits": behind, "restarted": true})
		writeJSON(w, http.StatusOK, result)
		return
	}

	// tanpa helper: beri perintah manual yang presisi
	result["ok"] = true
	result["staged"] = true
	result["commits"] = behind
	result["log"] = log.String()
	result["message"] = "binary baru sudah distage. Jalankan via SSH untuk memasang:\n" +
		"  sudo systemctl stop jenderalrouter\n" +
		"  sudo install -m 0755 " + stage + " /usr/local/bin/jenderalrouter\n" +
		"  sudo systemctl start jenderalrouter\n" +
		"(atau jalankan ulang 'sudo ./deploy/install-ubuntu.sh binary' dan pilih Y untuk mengaktifkan pembaruan otomatis dari dashboard)"
	a.audit(r, "system.update", "system", map[string]string{"version": fromVersion},
		map[string]any{"commits": behind, "staged": true})
	writeJSON(w, http.StatusOK, result)
}

var _ = store.RoleAdmin
