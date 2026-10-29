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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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

// applyUpdateScript lokasi helper root (dipasang install-ubuntu.sh atau
// `install-update-helper`); lokasinya juga didefinisikan sebagai
// helperScriptPath di updatehelper.go — satu sumber: pakai var itu.
var applyUpdateScript = helperScriptPath

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

// runningCommit hash commit binary yang sedang berjalan, diambil dari
// Version hasil `git describe --tags --always [--dirty]` (mis. "0ed322d",
// "v1.2-3-g0ed322d"). Kosong bila Version bukan cap git (build rilis/dev).
func runningCommit() string {
	v := strings.TrimSuffix(Version, "-dirty")
	if i := strings.LastIndex(v, "-g"); i >= 0 {
		v = v[i+2:]
	}
	if len(v) < 7 || len(v) > 40 {
		return ""
	}
	for _, c := range v {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return ""
		}
	}
	return v
}

// gitHasCommit true bila commit ada di repo (sudah ter-fetch).
func gitHasCommit(ctx context.Context, repo, rev string) bool {
	return rev != "" && gitCmd(ctx, repo, "cat-file", "-e", rev+"^{commit}").Run() == nil
}

func gitDescribe(ctx context.Context, repo string) string {
	out, err := gitCmd(ctx, repo, "describe", "--tags", "--always").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// commitInfo satu baris changelog.
type commitInfo struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
	Author  string `json:"author"`
	Date    string `json:"date"`
}

// gitLog daftar commit pada rentang rev (mis. "abc..origin/main"), terbaru dulu.
func gitLog(ctx context.Context, repo, rev string, limit int) []commitInfo {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := gitCmd(ctx, repo, "log", fmt.Sprintf("-%d", limit),
		"--format=%h%x1f%s%x1f%an%x1f%cI", rev).Output()
	if err != nil {
		return nil
	}
	list := []commitInfo{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		p := strings.Split(line, "\x1f")
		if len(p) == 4 {
			list = append(list, commitInfo{Hash: p[0], Subject: p[1], Author: p[2], Date: p[3]})
		}
	}
	return list
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
	want, ok := goToolchainSHA256[arch]
	if !ok {
		return "", fmt.Errorf("arsitektur %s tidak didukung untuk unduh Go otomatis", arch)
	}
	url := "https://go.dev/dl/" + goToolchainVersion + ".linux-" + arch + ".tar.gz"
	tgz := filepath.Join(stageDir(), "go.tgz")
	if err := j.runStreamed(ctx, "curl", "-fsSL", url, "-o", tgz); err != nil {
		return "", fmt.Errorf("unduh Go gagal: %w", err)
	}
	// toolchain ini membangun binary yang lalu dipasang root — wajib cocok
	// dengan checksum resmi yang dipatok, bukan hanya mengandalkan TLS
	if got, err := fileSHA256(tgz); err != nil || got != want {
		_ = os.Remove(tgz)
		return "", fmt.Errorf("checksum toolchain Go tidak cocok (dapat %s)", got)
	}
	j.appendLog("checksum toolchain Go OK (sha256 %s…)", want[:12])
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

// goToolchainVersion + checksum resmi (go.dev/dl/?mode=json) arsip Linux.
const goToolchainVersion = "go1.27.1"

var goToolchainSHA256 = map[string]string{
	"amd64": "63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445",
	"arm64": "3450b45a3f9ee8568792736a5c5e70a1f2e9b36c35a8f74958c03e51d7d92bec",
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ensureOriginURL memaksa remote origin mirror kembali ke repoURL resmi —
// .git/config mirror bisa diubah user service; build selalu dari repo resmi.
func ensureOriginURL(ctx context.Context, repo string, j *updateJob) error {
	out, _ := gitCmd(ctx, repo, "config", "--get", "remote.origin.url").Output()
	if cur := strings.TrimSpace(string(out)); cur != repoURL {
		j.appendLog("⚠ remote origin mirror (%q) dikembalikan ke %s", cur, repoURL)
		return gitCmd(ctx, repo, "remote", "set-url", "origin", repoURL).Run()
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
	st["sudo_apply_available"] = applyAvailable()
	switch {
	case applyPathUnitInstalled():
		st["apply_mode"] = "path_unit"
	case sudoApplyAvailable():
		st["apply_mode"] = "sudo"
	default:
		st["apply_mode"] = ""
	}
	st["staged_binary"] = fileExists(filepath.Join(stageDir(), "jenderalrouter.new"))
	st["backup_available"] = fileExists(backupBinPath())
	st["helper_outdated"] = helperOutdated()
	if res := readApplyResult(); res != nil {
		st["apply_result"] = res
	}
	st["apply_requested"] = lastApplyRequest()
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

		// basis pembanding = commit binary yang BERJALAN (bila tercap & ada di
		// mirror); else HEAD mirror. Dengan begitu build yang distage tapi
		// belum terpasang tetap terhitung belum terbarui.
		base := localHead
		if rc := runningCommit(); gitHasCommit(r.Context(), dir, rc) {
			base = rc
			st["running_commit"] = rc
		}
		pending := gitLog(r.Context(), dir, base+"..origin/"+branch, 50)
		if pending == nil {
			pending = []commitInfo{}
		}
		st["pending_commits"] = pending
		st["behind_commits"] = len(pending)
		st["update_available"] = len(pending) > 0
		st["recent_commits"] = gitLog(r.Context(), dir, base, 10)
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
		job.appendLog("⏳ job pembaruan dimulai (versi berjalan %s)", Version)
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
		if err := ensureOriginURL(ctx, dir, job); err != nil {
			fail("memulihkan remote origin gagal: " + err.Error())
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
		rc := runningCommit()
		runningIsHead := rc == "" || strings.HasPrefix(newHead, rc) || strings.HasPrefix(rc, newHead)
		if behind == 0 && newHead == oldHead && runningIsHead {
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
		buildVer := gitDescribe(ctx, dir)
		if buildVer == "" {
			buildVer = newHead
		}
		job.appendLog("versi build: %s", buildVer)
		cmd := exec.CommandContext(buildCtx, goBin, "build", "-trimpath",
			"-ldflags", "-s -w -X github.com/jenderal/jenderalrouter/internal/api.Version="+buildVer,
			"-o", stage, "./cmd/jenderalrouter")
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

		// 5) pasang + restart bila mekanisme apply tersedia; else instruksikan manual
		markApply := func() {
			markFetched()
			job.mu.Lock()
			job.running, job.done, job.ok = false, true, true
			job.restarted, job.commits = true, behind
			job.finished = time.Now()
			job.mu.Unlock()
			a.st.Audit(actorID, "system.update", "system", map[string]string{"version": fromVersion},
				map[string]any{"commits": behind, "restarted": true})
		}
		if applyPathUnitInstalled() {
			_ = writeLastUpdate(lastUpdateInfo{
				Time: time.Now().UTC().Format(time.RFC3339), From: fromVersion, To: newHead, Commits: behind,
			})
			// tanpa sudo: tulis file permintaan — unit jenderalrouter-apply.path
			// (root) yang menjalankan pemasangan; service lalu di-restart
			job.setPhase("memasang & me-restart service (unit path systemd)")
			if _, err := writeApplyRequest("install"); err != nil {
				fail("gagal menulis permintaan apply: " + err.Error())
				return
			}
			markApply()
			return
		}
		if sudoApplyAvailable() {
			_ = writeLastUpdate(lastUpdateInfo{
				Time: time.Now().UTC().Format(time.RFC3339), From: fromVersion, To: newHead, Commits: behind,
			})
			job.setPhase("memasang & me-restart service (helper sudo)")
			if _, err := writeApplyRequest("install"); err != nil {
				fail("gagal menulis permintaan apply: " + err.Error())
				return
			}
			applyCtx, applyCancel := context.WithTimeout(context.Background(), 150*time.Second)
			defer applyCancel()
			if err := exec.CommandContext(applyCtx, "sudo", "-n", applyUpdateScript, "install").Run(); err != nil {
				fail("apply gagal: " + err.Error())
				return
			}
			markApply()
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
