//go:build linux

package api

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// helperSandbox menyiapkan salinan helper root yang path-nya dialihkan ke
// direktori sementara, dengan systemctl tiruan: service "sehat" hanya bila
// binary terpasang berisi kata "good". Hanya Linux (GNU mv/install -T).
type helperSandbox struct {
	dir, script, bin, root, upd string
}

func newHelperSandbox(t *testing.T) *helperSandbox {
	t.Helper()
	d := t.TempDir()
	sb := &helperSandbox{
		dir: d, bin: filepath.Join(d, "bin", "jenderalrouter"),
		root: filepath.Join(d, "apply-root"), upd: filepath.Join(d, "data", "updates"),
	}
	for _, p := range []string{filepath.Dir(sb.bin), sb.upd, filepath.Join(d, "etc")} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	stub := filepath.Join(d, "systemctl")
	_ = os.WriteFile(stub, []byte("#!/bin/bash\ncase \"$1\" in is-active) grep -q good "+sb.bin+" ;; *) exit 0 ;; esac\n"), 0o755)
	_ = os.WriteFile(filepath.Join(d, "etc", "default"), []byte("JR_DATA_DIR="+filepath.Join(d, "data")+"\n"), 0o600)

	s := applyUpdateScriptContent
	for old, repl := range map[string]string{
		"ROOT=/var/lib/jenderalrouter-apply":          "ROOT=" + sb.root,
		"BIN=/usr/local/bin/jenderalrouter":           "BIN=" + sb.bin,
		"/etc/default/jenderalrouter":                 filepath.Join(d, "etc", "default"),
		"/usr/bin/systemctl":                          stub,
		"-o root -g root ":                            "",
		"command -v curl >/dev/null 2>&1 || return 0": "return 0",
		"sleep 3": "sleep 0",
	} {
		if !strings.Contains(s, old) {
			t.Fatalf("pola %q tidak ada di helper", old)
		}
		s = strings.ReplaceAll(s, old, repl)
	}
	sb.script = filepath.Join(d, "apply-update.sh")
	_ = os.WriteFile(sb.script, []byte(s), 0o755)
	_ = os.WriteFile(sb.bin, []byte("old good"), 0o755)
	return sb
}

func (sb *helperSandbox) run(t *testing.T, action, request string) (applyResult, error) {
	t.Helper()
	if request != "" {
		_ = os.WriteFile(filepath.Join(sb.upd, "apply.request"), []byte(request+"\n"), 0o644)
	}
	out, err := exec.Command("bash", sb.script, action).CombinedOutput()
	var res applyResult
	if b, rerr := os.ReadFile(filepath.Join(sb.root, "apply-result.json")); rerr == nil {
		if jerr := json.Unmarshal(b, &res); jerr != nil {
			t.Fatalf("hasil bukan JSON: %s", b)
		}
	}
	if err != nil {
		t.Logf("helper: %v\n%s", err, out)
	}
	return res, err
}

func readFile(t *testing.T, p string) string {
	b, _ := os.ReadFile(p)
	return string(b)
}

func TestHelperInstallAndRollback(t *testing.T) {
	sb := newHelperSandbox(t)
	_ = os.WriteFile(filepath.Join(sb.upd, "jenderalrouter.new"), []byte("new good"), 0o755)
	res, err := sb.run(t, "install", "install 111")
	if err != nil || !res.OK || res.Request != "111" || readFile(t, sb.bin) != "new good" {
		t.Fatalf("install: err=%v res=%+v bin=%q", err, res, readFile(t, sb.bin))
	}
	if readFile(t, filepath.Join(sb.root, "jenderalrouter.bak")) != "old good" {
		t.Fatal("backup bukan binary lama")
	}
	// rollback lewat isi file permintaan (unit path memanggil "install")
	res, err = sb.run(t, "install", "rollback 222")
	if err != nil || !res.OK || res.Action != "rollback" || res.Request != "222" || readFile(t, sb.bin) != "old good" {
		t.Fatalf("rollback: err=%v res=%+v bin=%q", err, res, readFile(t, sb.bin))
	}
	if readFile(t, filepath.Join(sb.root, "jenderalrouter.bak")) != "new good" {
		t.Fatal("setelah rollback, versi tadi harus jadi backup")
	}
}

func TestHelperAutoRollbackWhenUnhealthy(t *testing.T) {
	sb := newHelperSandbox(t)
	_ = os.WriteFile(filepath.Join(sb.upd, "jenderalrouter.new"), []byte("new BROKEN"), 0o755)
	res, err := sb.run(t, "install", "install 333")
	if err == nil || res.OK || readFile(t, sb.bin) != "old good" {
		t.Fatalf("harus gagal + pulih: err=%v res=%+v bin=%q", err, res, readFile(t, sb.bin))
	}
}

// Serangan: user service menanam symlink di updates/ agar root menyalin
// file rahasia atau membocorkan isinya. Semua harus ditolak.
func TestHelperRejectsSymlinks(t *testing.T) {
	sb := newHelperSandbox(t)
	secret := filepath.Join(sb.dir, "shadow")
	_ = os.WriteFile(secret, []byte("root:RAHASIA"), 0o600)

	_ = os.Symlink(secret, filepath.Join(sb.upd, "jenderalrouter.new"))
	_ = os.Symlink(secret, filepath.Join(sb.upd, "apply.request"))
	res, err := sb.run(t, "install", "")
	if err == nil || res.OK {
		t.Fatalf("stage symlink harus ditolak: %+v", res)
	}
	if strings.Contains(readFile(t, sb.bin), "RAHASIA") {
		t.Fatal("isi file rahasia tersalin ke binary")
	}
	if strings.Contains(readFile(t, filepath.Join(sb.root, "apply-result.json")), "RAHASIA") || res.Request != "" {
		t.Fatalf("isi file rahasia bocor ke hasil: %+v", res)
	}
	if readFile(t, secret) != "root:RAHASIA" {
		t.Fatal("file rahasia berubah")
	}
	// hasil lama yang berupa symlink di folder root tidak bisa ditanam
	// (folder root milik root); cukup pastikan result file biasa
	fi, err := os.Lstat(filepath.Join(sb.root, "apply-result.json"))
	if err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("apply-result.json harus file biasa")
	}
}
