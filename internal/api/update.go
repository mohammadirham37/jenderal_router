// Fitur "Perbarui dari dashboard" — BERJALAN ASINKRON (job + log live):
// sinkronisasi mirror repo milik service, build binary baru, stage, lalu
// (bila helper sudo terpasang) pasang dan restart service.
//
// Catatan penting: pembaruan memakai MIRROR INTERNAL (<data>/src, milik
// user service) — folder clone lain (mis. ~/jenderal_router milik admin)
// TIDAK disentuh. Yang diperbarui adalah binary service.
//
// Desain keamanan: pemasangan binary + restart hanya lewat helper root
// yang sempit (/usr/local/lib/jenderalrouter/apply-update.sh) dengan
// sudoers NOPASSWD terbatas (opsional, dipasang install-ubuntu.sh).

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
	"runtime"
	"strings"
	"sync"
	"time"
)

const containerMarker = "/.dockerenv"

var containerMarkerPath = containerMarker // bisa ditimpa di test

func inContainer() bool {
	_, err := os.Stat(containerMarkerPath)
	return err == nil
}

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
	return filepath.Dir(stageDir()) + "/src"
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

// ---- job pembaruan (asinkron, log live) ----

type updateJob struct {
	mu             sync.Mutex
	running        bool
	phase          string
	log            []string
	started        time.Time
	finished       time.Time
	done           bool
	ok             bool
	err            string
	restarted      bool
	staged         bool
	commits        int
	manualCommands []string
}

var (
	updJobMu      sync.Mutex
	updJob        *updateJob
	lastFetchTime time.Time
	lastFetchMu   sync.Mutex
)

func updateJobSnapshot() map[string]any {
	updJobMu.Lock()
	j := updJob
	updJobMu.Unlock()
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	n := len(j.log)
	from := 0
	if n > 80 {
		from = n - 80
	}
	return map[string]any{
		"running":         j.running,
		"phase":           j.phase,
		"log":             append([]string{}, j.log[from:]...),
		"done":            j.done,
		"ok":              j.ok,
		"error":           j.err,
		"restarted":       j.restarted,
		"staged":          j.staged,
		"commits":         j.commits,
		"manual_commands": j.manualCommands,
		"started_at":      j.started.Format(time.RFC3339),
	}
}

func (j *updateJob) setPhase(p string) {
	j.mu.Lock()
	j.phase = p
	j.mu.Unlock()
	j.appendLog("── %s", p)
}

func (j *updateJob) appendLog(format string, args ...any) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.log = append(j.log, fmt.Sprintf(format, args...))
	if len(j.log) > 600 {
		j.log = j.log[len(j.log)-600:]
	}
}

func (j *updateJob) runStreamed(ctx context.Context, name string, args ...string) error {
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

// ensureGo memastikan toolchain Go tersedia: PATH → /usr/local/go →
// unduh toolchain ke dir user service (tanpa root).
func (j *updateJob) ensureGo(ctx context.Context) (string, error) {
	if p, err := exec.LookPath("go"); err == nil {
		return p, nil
	}
	if _, err := os.Stat("/usr/local/go/bin/go"); err == nil {
		return "/usr/local/go/bin/go", nil
	}
	toolDir := filepath.Join(stageDir(), "go-toolchain")
	goBin := filepath.Join(toolDir, "go", "bin", "go")
	if _, err := os.Stat(goBin); err == nil {
		return goBin, nil
	}
	j.setPhase("memasang Go (sekali saja, ~75 MB)")
	_ = os.MkdirAll(stageDir(), 0o700) // pastikan direktori ada sebelum unduh
	arch := runtime.GOARCH
	url := "https://go.dev/dl/go1.27.1.linux-" + arch + ".tar.gz"
	tgz := filepath.Join(stageDir(), "go.tgz")
	if err := j.runStreamed(ctx, "curl", "-fsSL", url, "-o", tgz); err != nil {
		return "", fmt.Errorf("unduh Go gagal: %w", err)
	}
	_ = os.RemoveAll(toolDir)
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		return "", err
	}
	if err := j.runStreamed(ctx, "tar", "-C", toolDir, "-xzf", tgz); err != nil {
		return "", fmt.Errorf("ekstrak Go gagal: %w", err)
	}
	_ = os.Remove(tgz)
	if _, err := os.Stat(goBin); err != nil {
		return "", fmt.Errorf("go binary tidak ditemukan setelah ekstrak")
	}
	return goBin, nil
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

// last-update.json — catatan pembaruan terakhir (ditulis sebelum apply).
type lastUpdateInfo struct {
	Time    string `json:"time"`
	From    string `json:"from_version"`
	To      string `json:"to_commit"`
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

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// ---- handler status ----

func (a *App) handleSystemUpdateStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.updateStatusView(r))
}

func (a *App) updateStatusView(r *http.Request) map[string]any {
	st := map[string]any{
		"version":          Version,
		"in_container":     inContainer(),
		"mode":             "binary",
		"busy":             false,
		"update_available": false,
		"repo_found":       false,
	}
	if inContainer() {
		st["mode"] = "docker"
		st["hint"] = "Aplikasi berjalan di container: perbarui dari host dengan 'git pull && docker compose up -d --build' pada folder deploy/ repo."
		return st
	}
	_, gitOKbool := exec.LookPath("git")
	_, goOKbool := exec.LookPath("go")
	gitOK := gitOKbool == nil
	goOK := goOKbool == nil
	st["git_available"] = gitOK
	st["go_available"] = goOK || fileExists("/usr/local/go/bin/go")
	st["sudo_apply_available"] = sudoApplyAvailable()
	st["staged_binary"] = fileExists(filepath.Join(stageDir(), "jenderalrouter.new"))
	if lu := readLastUpdate(); lu.Time != "" {
		st["last_update"] = lu
	}
	if job := updateJobSnapshot(); job != nil {
		st["job"] = job
		if b, _ := job["running"].(bool); b {
			st["busy"] = true
		}
	}

	dir := mirrorDir()
	st["repo_dir"] = dir
	fetchParam := r.URL.Query().Get("fetch")
	needFetch := fetchParam == "1" || (fetchParam == "auto" && fetchStale())

	// mirror belum ada + diminta fetch → clone sekali agar badge akurat
	if !isRepo(dir) && needFetch && gitOK {
		ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
		defer cancel()
		j := &updateJob{}
		if _, err := ensureMirror(ctx, j); err != nil {
			st["last_error"] = "clone mirror gagal: " + err.Error()
		} else {
			markFetched()
		}
	}

	if isRepo(dir) {
		st["repo_found"] = true
		st["branch"] = gitBranch(dir)
		localHead := gitHead(dir)
		st["current_head"] = localHead

		if needFetch && gitOK {
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
			st["update_available"] = n > 0 || (remoteHead != "" && remoteHead != localHead)
		} else if remoteHead != "" {
			st["update_available"] = remoteHead != localHead
		}
	}
	return st
}

// ensureMirror clone mirror bila belum ada (dipakai status & job).
func ensureMirror(ctx context.Context, j *updateJob) (string, error) {
	dir := mirrorDir()
	if isRepo(dir) {
		return dir, nil
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	j.appendLog("$ git clone %s %s", repoURL, dir)
	cmd := exec.CommandContext(ctx, "git", "clone", repoURL, dir)
	out, err := cmd.CombinedOutput()
	j.appendLog("%s", strings.TrimSpace(string(out)))
	if err != nil {
		return "", fmt.Errorf("clone repo gagal: %w", err)
	}
	return dir, nil
}

// ---- handler POST /api/admin/system/update (memulai job, 202) ----

func (a *App) handleSystemUpdate(w http.ResponseWriter, r *http.Request) {
	if inContainer() {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "mode container: perbarui dari host — 'git pull && docker compose up -d --build' pada folder deploy/ repo",
		})
		return
	}
	updJobMu.Lock()
	if updJob != nil && updJob.running {
		updJobMu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "pembaruan sedang berjalan — pantau lognya"})
		return
	}
	job := &updateJob{running: true, phase: "menyiapkan", started: time.Now()}
	updJob = job
	updJobMu.Unlock()

	ai := authFrom(r)
	actorID := int64(0)
	if ai != nil {
		actorID = ai.user.ID
	}
	fromVersion := Version

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		fail := func(msg string) {
			job.mu.Lock()
			job.running, job.done, job.ok, job.err = false, true, false, msg
			job.finished = time.Now()
			job.mu.Unlock()
			job.appendLog("✗ %s", msg)
			a.st.Audit(actorID, "system.update", "system", map[string]string{"version": fromVersion},
				map[string]any{"ok": false, "error": msg})
		}

		// 1) mirror siap (clone bila perlu)
		job.setPhase("sinkronisasi repo (mirror internal — folder clone lain tidak disentuh)")
		dir, err := ensureMirror(ctx, job)
		if err != nil {
			fail(err.Error())
			return
		}
		if err := gitFetch(ctx, dir); err != nil {
			fail("fetch gagal: " + err.Error())
			return
		}
		branch := gitBranch(dir)
		oldHead := gitHead(dir)
		behind, err := gitBehind(ctx, dir)
		if err != nil {
			fail("hitung ketinggalan gagal: " + err.Error())
			return
		}
		job.appendLog("mirror @ %s — ketinggalan %d commit", oldHead, behind)

		// 2) reset mirror ke origin (milik service; aman hard-reset)
		if err := job.runStreamed(ctx, "git", "-c", "safe.directory="+dir, "-C", dir,
			"reset", "--hard", "origin/"+branch); err != nil {
			fail("reset ke origin gagal: " + err.Error())
			return
		}
		newHead := gitHead(dir)
		if behind == 0 && newHead == oldHead {
			markFetched()
			job.mu.Lock()
			job.running, job.done, job.ok = false, true, true
			job.finished = time.Now()
			job.mu.Unlock()
			job.appendLog("sudah versi terbaru")
			return
		}

		// 3) build
		job.setPhase("build binary baru (2–5 menit)")
		goBin, err := job.ensureGo(ctx)
		if err != nil {
			fail(err.Error())
			return
		}
		stage := filepath.Join(stageDir(), "jenderalrouter.new")
		_ = os.MkdirAll(stageDir(), 0o700)
		buildCtx, buildCancel := context.WithTimeout(ctx, 15*time.Minute)
		defer buildCancel()
		cmd := exec.CommandContext(buildCtx, goBin, "build", "-trimpath", "-ldflags", "-s -w", "-o", stage, "./cmd/jenderalrouter")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH,
			"PATH=/usr/local/go/bin:"+filepath.Dir(goBin)+":"+os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			fail(fmt.Sprintf("build gagal: %s", strings.TrimSpace(string(out))))
			return
		}
		job.appendLog("build OK")

		// 4) smoke test
		if out, err := exec.CommandContext(ctx, stage, "-version").CombinedOutput(); err != nil {
			fail("binary stage tidak bisa dijalankan: " + string(out))
			return
		} else {
			job.appendLog("smoke: %s", strings.TrimSpace(string(out)))
		}

		// 5) pasang + restart bila helper sudo tersedia; else instruksikan manual
		if sudoApplyAvailable() {
			_ = writeLastUpdate(lastUpdateInfo{
				Time: time.Now().UTC().Format(time.RFC3339), From: fromVersion, To: newHead, Commits: behind,
			})
			job.setPhase("memasang & me-restart service (helper sudo)")
			applyCtx, applyCancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer applyCancel()
			if err := exec.CommandContext(applyCtx, "sudo", "-n", applyUpdateScript, "install").Run(); err != nil {
				fail("apply gagal: " + err.Error())
				return
			}
			markFetched()
			job.mu.Lock()
			job.running, job.done, job.ok = false, true, true
			job.restarted, job.commits = true, behind
			job.finished = time.Now()
			job.mu.Unlock()
			a.st.Audit(actorID, "system.update", "system", map[string]string{"version": fromVersion},
				map[string]any{"commits": behind, "restarted": true})
			return
		}

		_ = writeLastUpdate(lastUpdateInfo{
			Time: time.Now().UTC().Format(time.RFC3339), From: fromVersion, To: newHead, Commits: behind,
		})
		manual := []string{
			"sudo systemctl stop jenderalrouter",
			"sudo install -m 0755 " + stage + " /usr/local/bin/jenderalrouter",
			"sudo rm -f " + stage,
			"sudo systemctl start jenderalrouter",
		}
		job.mu.Lock()
		job.running, job.done, job.ok = false, true, true
		job.staged, job.commits = true, behind
		job.manualCommands = manual
		job.finished = time.Now()
		job.mu.Unlock()
		job.appendLog("⚠ binary distage di %s — belum AKTIF sampai perintah pemasangan dijalankan", stage)
		a.st.Audit(actorID, "system.update", "system", map[string]string{"version": fromVersion},
			map[string]any{"commits": behind, "staged": true})
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{"started": true, "status_url": "/api/admin/system/update/status"})
}
