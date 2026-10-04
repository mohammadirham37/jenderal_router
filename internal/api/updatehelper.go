package api

// Helper root untuk "Perbarui sekarang" dari dashboard: memasang binary
// hasil build service + me-restart service tanpa perlu SSH manual.
//
// Helper dipasang dua cara — isinya identik:
//  1. otomatis oleh deploy/install-ubuntu.sh saat instalasi (opt-in);
//  2. kapan saja setelahnya lewat subcommand CLI
//     `sudo jenderalrouter install-update-helper` (sekali saja).
//
// Setelah helper aktif, dashboard menampilkan tombol "Terapkan & restart"
// ketika ada binary hasil stage (POST /api/admin/system/update/apply).

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	helperDir     = "/usr/local/lib/jenderalrouter"
	helperSudoers = "/etc/sudoers.d/jenderalrouter-update"
	helperUser    = "jenderalrouter"
)

// applyUpdateScriptContent harus identik dengan heredoc APPLY di
// deploy/install-ubuntu.sh — jangan mengubah salah satu tanpa yang lain.
const applyUpdateScriptContent = `#!/bin/bash
# Helper pembaruan JenderalRouter — dipanggil service via sudo NOPASSWD.
# Sengaja sempit: hanya memasang binary hasil build service (yang sudah
# lolos smoke test) dan me-restart service ini.
# CATATAN RISIKO: binary distage oleh user service lalu dieksekusi root.
# Bila tidak menerimanya, hapus /etc/sudoers.d/jenderalrouter-update.
set -euo pipefail
STAGE=/var/lib/jenderalrouter/updates/jenderalrouter.new
case "${1:-}" in
  check)
    exit 0
    ;;
  install)
    [ -f "$STAGE" ]
    /usr/bin/systemctl stop jenderalrouter || true
    /usr/bin/install -m 0755 "$STAGE" /usr/local/bin/jenderalrouter
    rm -f "$STAGE"
    /usr/bin/systemctl start jenderalrouter
    ;;
  *)
    exit 2
    ;;
esac
`

func sudoersContent() []byte {
	return []byte(
		helperUser + " ALL=(root) NOPASSWD: " + applyUpdateScript + " check\n" +
			helperUser + " ALL=(root) NOPASSWD: " + applyUpdateScript + " install\n")
}

// selfBin path binary yang sedang berjalan — untuk instruksi pemasangan.
func selfBin() string {
	if p, err := os.Executable(); err == nil && p != "" {
		return p
	}
	return "/usr/local/bin/jenderalrouter"
}

// InstallUpdateHelper subcommand CLI: pasang helper + sudoers (root saja).
// Idempoten — aman dijalankan berulang untuk memperbarui isi helper.
func InstallUpdateHelper() int {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, ">> harus dijalankan sebagai root: sudo "+selfBin()+" install-update-helper")
		return 1
	}
	if err := os.MkdirAll(helperDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, ">> gagal membuat "+helperDir+":", err)
		return 1
	}
	if err := writeFileRoot(applyUpdateScript, []byte(applyUpdateScriptContent), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, ">> gagal menulis helper:", err)
		return 1
	}
	fmt.Println(">> helper ditulis :", applyUpdateScript)

	tmp := helperSudoers + ".tmp"
	if err := writeFileRoot(tmp, sudoersContent(), 0o440); err != nil {
		fmt.Fprintln(os.Stderr, ">> gagal menulis sudoers:", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "visudo", "-cf", tmp).CombinedOutput()
	if err != nil {
		_ = os.Remove(tmp)
		fmt.Fprintln(os.Stderr, ">> sudoers tidak valid — dibatalkan:", strings.TrimSpace(string(out)))
		return 1
	}
	if err := os.Rename(tmp, helperSudoers); err != nil {
		_ = os.Remove(tmp)
		fmt.Fprintln(os.Stderr, ">> gagal memasang sudoers:", err)
		return 1
	}
	_ = os.Chown(helperSudoers, 0, 0)
	_ = os.Chmod(helperSudoers, 0o440)
	fmt.Println(">> sudoers terpasang:", helperSudoers, "(NOPASSWD terbatas, user " + helperUser + ")")
	fmt.Println(">> helper pembaruan dashboard AKTIF — 'Perbarui sekarang' kini memasang binary + restart otomatis.")
	return 0
}

func writeFileRoot(path string, content []byte, mode os.FileMode) error {
	if err := os.WriteFile(path, content, mode); err != nil {
		return err
	}
	_ = os.Chown(path, 0, 0)
	return os.Chmod(path, mode)
}

// handleSystemUpdateApply POST /api/admin/system/update/apply — pasang
// binary hasil stage + restart service via helper root. Helper me-restart
// service ini, jadi pekerjaan dijalankan di latar setelah respons terkirim.
func (a *App) handleSystemUpdateApply(w http.ResponseWriter, r *http.Request) {
	if inContainer() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "mode container: perbarui dari host — 'git pull && docker compose up -d --build' pada folder deploy/ repo"})
		return
	}
	stage := filepath.Join(stageDir(), "jenderalrouter.new")
	if !fileExists(stage) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tidak ada binary hasil stage"})
		return
	}
	if !sudoApplyAvailable() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "helper pembaruan belum terpasang — jalankan sekali di host: sudo " + selfBin() + " install-update-helper"})
		return
	}
	a.audit(r, "system.update.apply", "system", nil, nil)
	go func() {
		time.Sleep(500 * time.Millisecond) // beri waktu respons terkirim dulu
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "sudo", "-n", applyUpdateScript, "install").Run()
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"started": true})
}
