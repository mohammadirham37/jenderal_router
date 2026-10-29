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
	"encoding/json"
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
	applyRootDir       = "/var/lib/jenderalrouter-apply" // milik root: backup + hasil
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

// applyUpdateScriptContent satu-satunya sumber isi helper root — dipasang
// oleh `jenderalrouter install-update-helper` (install-ubuntu.sh juga
// memanggil subcommand itu). Status update membandingkan isi file terpasang
// dengan konstanta ini untuk mendeteksi helper versi lama.
const applyUpdateScriptContent = `#!/bin/bash
# Helper pembaruan JenderalRouter — dipanggil unit apply (root) atau via
# sudo NOPASSWD. Sengaja sempit: hanya memasang binary hasil build service
# (yang sudah lolos smoke test), me-restart service ini, dan mengembalikan
# versi sebelumnya bila versi baru gagal sehat.
#
# Keamanan: folder updates/ milik user service (tidak tepercaya). Semua yang
# ditulis root (backup, salinan kerja, hasil) ada di $ROOT milik root; file
# dari updates/ DIPINDAH (rename, bukan diikuti) ke $ROOT lalu diperiksa
# bukan symlink — mencegah root menimpa/menyalin file lain lewat symlink.
# CATATAN RISIKO: binary distage oleh user service lalu dipasang root.
set -euo pipefail
umask 022
ROOT=/var/lib/jenderalrouter-apply
BIN=/usr/local/bin/jenderalrouter
SVC=jenderalrouter

conf() { # baca VAR dari /etc/default tanpa mengeksekusinya
  { grep -m1 "^$1=" /etc/default/jenderalrouter 2>/dev/null || true; } | cut -d= -f2- | tr -d "\"'"
}
DATA="$(conf JR_DATA_DIR)"; DATA="${DATA:-/var/lib/jenderalrouter}"
UPD="$DATA/updates"
STAGE=$UPD/jenderalrouter.new
REQ=$UPD/apply.request
NEW=$ROOT/jenderalrouter.new
BAK=$ROOT/jenderalrouter.bak
CUR=$ROOT/jenderalrouter.cur
RESULT=$ROOT/apply-result.json

/usr/bin/install -d -o root -g root -m 0755 "$ROOT"

# ambil file permintaan: pindahkan ke $ROOT dulu, tolak symlink
RID=""
REQ_ACTION=""
if /usr/bin/mv -fT -- "$REQ" "$ROOT/request" 2>/dev/null; then
  if [ -f "$ROOT/request" ] && [ ! -L "$ROOT/request" ]; then
    line="$(head -c 200 "$ROOT/request" | head -n1 || true)"
    REQ_ACTION="${line%% *}"
    if [ "$line" != "${line#* }" ]; then
      RID="$(printf '%s' "${line#* }" | tr -cd 'A-Za-z0-9._-' | head -c 64)"
    fi
  fi
  rm -f "$ROOT/request"
fi

result() { # ok(true|false) aksi pesan
  printf '{"time":"%s","ok":%s,"action":"%s","request":"%s","message":"%s"}\n' \
    "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$1" "$2" "$RID" "$3" > "$RESULT.tmp"
  chmod 0644 "$RESULT.tmp"
  /usr/bin/mv -fT "$RESULT.tmp" "$RESULT"
}

# sehat = unit aktif DAN /healthz menjawab (bila curl tersedia)
healthy() {
  sleep 3
  /usr/bin/systemctl is-active --quiet "$SVC" || return 1
  command -v curl >/dev/null 2>&1 || return 0
  local addr
  addr="$(conf JR_ADDR)"; addr="${addr:-127.0.0.1:20130}"
  case "$addr" in :*) addr="127.0.0.1$addr" ;; 0.0.0.0:*) addr="127.0.0.1:${addr#0.0.0.0:}" ;; esac
  for _ in $(seq 1 20); do
    curl -fsS --max-time 2 "http://$addr/healthz" >/dev/null 2>&1 && \
      /usr/bin/systemctl is-active --quiet "$SVC" && return 0
    sleep 1
  done
  return 1
}

swap_in() { # pasang binary $1 (di $ROOT) lalu restart
  /usr/bin/systemctl stop "$SVC" || true
  /usr/bin/install -T -m 0755 -- "$1" "$BIN"
  /usr/bin/systemctl start "$SVC" || true
}

ACTION="${1:-}"
# unit path memanggil "install"; isi file permintaan bisa meminta rollback
if [ "$ACTION" = "install" ] && [ "$REQ_ACTION" = "rollback" ]; then
  ACTION=rollback
fi

case "$ACTION" in
  check)
    exit 0
    ;;
  install)
    rm -f "$NEW"
    if ! /usr/bin/mv -fT -- "$STAGE" "$NEW" 2>/dev/null; then
      result false install "tidak ada binary hasil stage"; exit 1
    fi
    if [ -L "$NEW" ] || [ ! -f "$NEW" ]; then
      rm -f "$NEW"; result false install "binary stage bukan file biasa - ditolak"; exit 1
    fi
    /usr/bin/install -T -m 0755 -- "$BIN" "$BAK"
    swap_in "$NEW"
    rm -f "$NEW"
    if healthy; then result true install "versi baru aktif"; exit 0; fi
    swap_in "$BAK"
    if healthy; then
      result false install "versi baru gagal sehat - otomatis dikembalikan ke versi sebelumnya"
    else
      result false install "versi baru gagal dan rollback juga gagal - periksa journalctl -u jenderalrouter"
    fi
    exit 1
    ;;
  rollback)
    if [ ! -f "$BAK" ]; then result false rollback "tidak ada backup versi sebelumnya"; exit 1; fi
    /usr/bin/install -T -m 0755 -- "$BIN" "$CUR"
    swap_in "$BAK"
    if healthy; then
      /usr/bin/mv -fT -- "$CUR" "$BAK" # tukar: versi tadi jadi backup
      result true rollback "versi sebelumnya aktif kembali"
      exit 0
    fi
    swap_in "$CUR"
    rm -f "$CUR"
    result false rollback "versi sebelumnya gagal sehat - versi semula dipulihkan"
    exit 1
    ;;
  *)
    exit 2
    ;;
esac
`

// applyPathUnit / applySvcUnit unit systemd pemicu apply; dataDir dibaca
// dari /etc/default/jenderalrouter saat pemasangan helper.
func applyPathUnit(dataDir string) string {
	return `[Unit]
Description=JenderalRouter — pemicu pemasangan update dari dashboard

[Path]
PathModified=` + dataDir + `/updates/apply.request

[Install]
WantedBy=multi-user.target
`
}

func applySvcUnit(dataDir string) string {
	return `[Unit]
Description=JenderalRouter — pasang binary hasil update dashboard

[Service]
Type=oneshot
ExecStart=` + helperScriptPath + ` install
# root, tapi hanya boleh menulis yang memang perlu
ProtectSystem=strict
ReadWritePaths=/usr/local/bin ` + applyRootDir + ` -` + dataDir + `/updates
ProtectHome=yes
PrivateTmp=yes
NoNewPrivileges=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
RestrictSUIDSGID=yes
LockPersonality=yes
`
}

// helperDataDir JR_DATA_DIR dari /etc/default/jenderalrouter (tanpa
// mengeksekusi file), default /var/lib/jenderalrouter.
func helperDataDir() string {
	b, err := os.ReadFile("/etc/default/jenderalrouter")
	if err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(line), "JR_DATA_DIR="); ok {
				if v = strings.Trim(v, `"'`); v != "" {
					return v
				}
			}
		}
	}
	return "/var/lib/jenderalrouter"
}

func sudoersContent() []byte {
	return []byte(
		helperUser + " ALL=(root) NOPASSWD: " + helperScriptPath + " check\n" +
			helperUser + " ALL=(root) NOPASSWD: " + helperScriptPath + " install\n" +
			helperUser + " ALL=(root) NOPASSWD: " + helperScriptPath + " rollback\n")
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
	if err := os.MkdirAll(applyRootDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, ">> gagal membuat "+applyRootDir+":", err)
		return 1
	}
	_ = os.Chown(applyRootDir, 0, 0)
	_ = os.Chmod(applyRootDir, 0o755)
	dataDir := helperDataDir()

	// unit systemd path+service — pemicu apply TANPA sudo (unit utama
	// memakai NoNewPrivileges=true; sudo tidak mungkin dari dalam service)
	systemdOK := false
	if _, err := os.Stat(systemdRuntimeDir); err == nil {
		if err := writeFileRoot(applyPathUnitFile, []byte(applyPathUnit(dataDir)), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, ">> gagal menulis "+applyPathUnitFile+":", err)
		} else if err := writeFileRoot(applySvcUnitFile, []byte(applySvcUnit(dataDir)), 0o644); err != nil {
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
	if systemdOK {
		// sudoers tidak diperlukan (service memakai NoNewPrivileges) —
		// buang sisa instalasi lama agar permukaan serangan minimal
		if err := os.Remove(helperSudoers); err == nil {
			fmt.Println(">> sudoers lama dihapus:", helperSudoers)
		}
		fmt.Println(">> helper pembaruan dashboard AKTIF — 'Perbarui sekarang' kini memasang binary + restart otomatis.")
		return 0
	}
	fmt.Fprintln(os.Stderr, ">> systemd tidak terdeteksi — memasang fallback sudo")

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
	fmt.Println(">> sudoers terpasang:", helperSudoers, "(fallback, user "+helperUser+")")
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
	a.triggerHelper(w, "install")
}

// handleSystemUpdateRollback POST /api/admin/system/update/rollback —
// kembalikan binary sebelumnya (backup yang dibuat helper saat pemasangan
// terakhir) lalu restart service.
func (a *App) handleSystemUpdateRollback(w http.ResponseWriter, r *http.Request) {
	if inContainer() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "mode container: rollback dilakukan dari host (docker compose)"})
		return
	}
	if !fileExists(backupBinPath()) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "belum ada backup versi sebelumnya"})
		return
	}
	a.audit(r, "system.update.rollback", "system", map[string]string{"version": Version}, nil)
	a.triggerHelper(w, "rollback")
}

// triggerHelper menjalankan aksi helper root (install|rollback). Via path
// unit: service hanya menulis file permintaan (isinya "rollback" untuk
// rollback); via sudo: helper dijalankan langsung. Keduanya me-restart
// service ini, jadi respons dikirim lebih dulu.
func (a *App) triggerHelper(w http.ResponseWriter, action string) {
	if !applyAvailable() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "helper pembaruan belum terpasang — jalankan sekali di host: sudo " + selfBin() + " install-update-helper"})
		return
	}
	rid, err := writeApplyRequest(action)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "gagal menulis permintaan apply: " + err.Error()})
		return
	}
	if applyPathUnitInstalled() {
		writeJSON(w, http.StatusAccepted, map[string]any{"started": true, "mode": "path_unit", "action": action, "request": rid})
		return
	}
	go func() {
		time.Sleep(500 * time.Millisecond) // beri waktu respons terkirim dulu
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "sudo", "-n", helperScriptPath, action).Run()
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"started": true, "mode": "sudo", "action": action, "request": rid})
}

// writeApplyRequest menulis "<aksi> <id>" ke file permintaan (memicu unit
// path) dan mencatat id terakhir agar UI bisa mencocokkan hasil helper —
// file hasil milik root, jadi service tidak bisa menghapusnya.
func writeApplyRequest(action string) (string, error) {
	rid := fmt.Sprintf("%d", time.Now().UnixNano())
	if err := os.MkdirAll(stageDir(), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(applyRequestedPath(), []byte(rid), 0o600); err != nil {
		return "", err
	}
	return rid, os.WriteFile(applyRequestPath(), []byte(action+" "+rid+"\n"), 0o644)
}

func applyRequestedPath() string { return filepath.Join(stageDir(), "apply.requested") }

// lastApplyRequest id permintaan apply terakhir yang ditulis service.
func lastApplyRequest() string {
	b, _ := os.ReadFile(applyRequestedPath())
	return strings.TrimSpace(string(b))
}

func backupBinPath() string   { return filepath.Join(applyRootDir, "jenderalrouter.bak") }
func applyResultPath() string { return filepath.Join(applyRootDir, "apply-result.json") }

// applyResult hasil aksi helper terakhir (ditulis helper root).
type applyResult struct {
	Time    string `json:"time"`
	OK      bool   `json:"ok"`
	Action  string `json:"action"`
	Request string `json:"request"`
	Message string `json:"message"`
}

func readApplyResult() *applyResult {
	b, err := os.ReadFile(applyResultPath())
	if err != nil {
		return nil
	}
	var res applyResult
	if json.Unmarshal(b, &res) != nil {
		return nil
	}
	return &res
}

// helperOutdated true bila helper terpasang tapi isinya bukan versi binary
// ini (mis. helper lama tanpa rollback) — perlu install-update-helper ulang.
func helperOutdated() bool {
	b, err := os.ReadFile(applyUpdateScript)
	if err != nil {
		return false
	}
	return string(b) != applyUpdateScriptContent
}
