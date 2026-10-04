package api

// Helper root untuk "Perbarui sekarang" dari dashboard: memasang binary
// hasil build service + me-restart service tanpa perlu SSH manual.
//
// Helper dipasang dua cara — isinya identik:
//  1. otomatis oleh deploy/install-ubuntu.sh saat instalasi (opt-in);
//  2. kapan saja setelahnya lewat subcommand CLI
//     `sudo jenderalrouter install-update-helper` (sekali saja).
//
// Mekanisme apply utama: systemd PATH UNIT tanpa sudo. Unit service utama
// memakai NoNewPrivileges=true (sudo mustahil dari dalam service), jadi
// service hanya MENULIS file permintaan di data-dir-nya sendiri; unit
// jenderalrouter-apply.path (root) memicu jenderalrouter-apply.service yang
// menjalankan apply-update.sh sebagai root. Sudo + sudoers dipertahankan
// sebagai fallback untuk host non-systemd.
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

// bisa ditimpa di test
var (
	applyPathUnitFile  = "/etc/systemd/system/jenderalrouter-apply.path"
	applySvcUnitFile   = "/etc/systemd/system/jenderalrouter-apply.service"
	systemdRuntimeDir  = "/run/systemd/system"
	helperScriptPath   = "/usr/local/lib/jenderalrouter/apply-update.sh"
	applyRequestMarker = "apply.request"
)

func applyRequestPath() string { return filepath.Join(stageDir(), applyRequestMarker) }

func applyPathUnitInstalled() bool {
	_, err := os.Stat(applyPathUnitFile)
	return err == nil
}

// applyAvailable — tombol/pemasangan otomatis bisa dipakai bila salah satu
// mekanisme terpasang: path unit systemd (utama) atau sudo helper (fallback).
func applyAvailable() bool {
	return applyPathUnitInstalled() || sudoApplyAvailable()
}

// applyUpdateScriptContent harus identik dengan heredoc APPLY di
// deploy/install-ubuntu.sh — jangan mengubah salah satu tanpa yang lain.
const applyUpdateScriptContent = `#!/bin/bash
# Helper pembaruan JenderalRouter — dipanggil unit apply (root) atau via
# sudo NOPASSWD. Sengaja sempit: hanya memasang binary hasil build service
# (yang sudah lolos smoke test) dan me-restart service ini.
# CATATAN RISIKO: binary distage oleh user service lalu dieksekusi root.
# Bila tidak menerimanya, hapus /etc/sudoers.d/jenderalrouter-update dan
# unit jenderalrouter-apply.{path,service}.
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
    rm -f /var/lib/jenderalrouter/updates/apply.request
    /usr/bin/systemctl start jenderalrouter
    ;;
  *)
    exit 2
    ;;
esac
`

const applyPathUnitContent = `[Unit]
Description=JenderalRouter — pemicu pemasangan update dari dashboard

[Path]
PathModified=/var/lib/jenderalrouter/updates/apply.request

[Install]
WantedBy=multi-user.target
`

const applySvcUnitContent = `[Unit]
Description=JenderalRouter — pasang binary hasil update dashboard

[Service]
Type=oneshot
ExecStart=/usr/local/lib/jenderalrouter/apply-update.sh install
`

func sudoersContent() []byte {
	return []byte(
		helperUser + " ALL=(root) NOPASSWD: " + helperScriptPath + " check\n" +
			helperUser + " ALL=(root) NOPASSWD: " + helperScriptPath + " install\n")
}

// selfBin path binary yang sedang berjalan — untuk instruksi pemasangan.
func selfBin() string {
	if p, err := os.Executable(); err == nil && p != "" {
		return p
	}
	return "/usr/local/bin/jenderalrouter"
}

// InstallUpdateHelper subcommand CLI: pasang helper + mekanisme apply
// (root saja). Idempoten — aman dijalankan berulang.
func InstallUpdateHelper() int {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, ">> harus dijalankan sebagai root: sudo "+selfBin()+" install-update-helper")
		return 1
	}
	if err := os.MkdirAll(helperDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, ">> gagal membuat "+helperDir+":", err)
		return 1
	}
	if err := writeFileRoot(helperScriptPath, []byte(applyUpdateScriptContent), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, ">> gagal menulis helper:", err)
		return 1
	}
	fmt.Println(">> helper ditulis :", helperScriptPath)

	// unit systemd path+service — pemicu apply TANPA sudo (unit utama
	// memakai NoNewPrivileges=true; sudo tidak mungkin dari dalam service)
	systemdOK := false
	if _, err := os.Stat(systemdRuntimeDir); err == nil {
		if err := writeFileRoot(applyPathUnitFile, []byte(applyPathUnitContent), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, ">> gagal menulis "+applyPathUnitFile+":", err)
		} else if err := writeFileRoot(applySvcUnitFile, []byte(applySvcUnitContent), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, ">> gagal menulis "+applySvcUnitFile+":", err)
		} else {
			for _, args := range [][]string{
				{"daemon-reload"},
				{"enable", "--now", "jenderalrouter-apply.path"},
			} {
				if out, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
					fmt.Fprintln(os.Stderr, ">> systemctl", strings.Join(args, " "), "gagal:", strings.TrimSpace(string(out)))
				}
			}
			systemdOK = true
			fmt.Println(">> unit apply terpasang:", applyPathUnitFile, "(pemicu tanpa sudo)")
		}
	}
	if !systemdOK {
		fmt.Fprintln(os.Stderr, ">> systemd tidak terdeteksi — hanya helper sudo yang dipasang")
	}

	// sudoers sebagai fallback untuk host non-systemd
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
	fmt.Println(">> sudoers terpasang:", helperSudoers, "(fallback, user " + helperUser + ")")
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
// binary hasil stage + restart service. Via path unit: service hanya
// menulis file permintaan; via sudo: helper dijalankan langsung. Keduanya
// me-restart service ini, jadi respons dikirim lebih dulu.
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
	a.audit(r, "system.update.apply", "system", nil, nil)

	if applyPathUnitInstalled() {
		if err := os.WriteFile(applyRequestPath(), []byte(time.Now().UTC().Format(time.RFC3339Nano)), 0o644); err != nil {
			writeJSON(w, 500, map[string]string{"error": "gagal menulis permintaan apply: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"started": true, "mode": "path_unit"})
		return
	}
	if sudoApplyAvailable() {
		go func() {
			time.Sleep(500 * time.Millisecond) // beri waktu respons terkirim dulu
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			_ = exec.CommandContext(ctx, "sudo", "-n", helperScriptPath, "install").Run()
		}()
		writeJSON(w, http.StatusAccepted, map[string]any{"started": true, "mode": "sudo"})
		return
	}
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error": "helper pembaruan belum terpasang — jalankan sekali di host: sudo " + selfBin() + " install-update-helper"})
}
