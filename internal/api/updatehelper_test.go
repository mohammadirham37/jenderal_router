package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// jalur apply: unit path systemd (utama) → file permintaan ditulis tanpa sudo.
func TestApplyViaPathUnit(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("JR_DATA_DIR", tmp)
	unitFile := filepath.Join(tmp, "jenderalrouter-apply.path")
	if err := os.WriteFile(unitFile, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldUnit := applyPathUnitFile
	applyPathUnitFile = unitFile
	t.Cleanup(func() { applyPathUnitFile = oldUnit })

	if err := os.MkdirAll(stageDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(stageDir(), "jenderalrouter.new")
	if err := os.WriteFile(stage, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	app, _, _ := appTestEnv(t)
	w := httptest.NewRecorder()
	app.handleSystemUpdateApply(w, httptest.NewRequest("POST", "/api/admin/system/update/apply", nil))
	if w.Code != http.StatusAccepted {
		t.Fatalf("kode %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "path_unit") {
		t.Fatalf("mode tidak sesuai: %s", w.Body.String())
	}
	if _, err := os.Stat(applyRequestPath()); err != nil {
		t.Fatalf("file permintaan tidak ditulis: %v", err)
	}
}

// tanpa mekanisme apa pun → 501 + instruksi pemasangan helper.
func TestApplyTanpaMekanisme(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("JR_DATA_DIR", tmp)
	oldUnit := applyPathUnitFile
	applyPathUnitFile = filepath.Join(tmp, "tidak-ada.path")
	t.Cleanup(func() { applyPathUnitFile = oldUnit })
	oldScript := applyUpdateScript
	applyUpdateScript = filepath.Join(tmp, "apply-update.sh")
	t.Cleanup(func() { applyUpdateScript = oldScript })

	if err := os.MkdirAll(stageDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stageDir(), "jenderalrouter.new"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	app, _, _ := appTestEnv(t)
	w := httptest.NewRecorder()
	app.handleSystemUpdateApply(w, httptest.NewRequest("POST", "/api/admin/system/update/apply", nil))
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("kode %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "install-update-helper") {
		t.Fatalf("pesan tidak memuat instruksi: %s", w.Body.String())
	}
}

func TestInstallUpdateHelperNonRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("berjalan sebagai root")
	}
	if code := InstallUpdateHelper(); code != 1 {
		t.Fatalf("kode exit %d, harapannya 1", code)
	}
}

// rollback memakai file permintaan berisi "rollback" (dibaca helper root).
func TestRollbackViaPathUnit(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("JR_DATA_DIR", tmp)
	unitFile := filepath.Join(tmp, "jenderalrouter-apply.path")
	if err := os.WriteFile(unitFile, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldUnit := applyPathUnitFile
	applyPathUnitFile = unitFile
	t.Cleanup(func() { applyPathUnitFile = oldUnit })
	if err := os.MkdirAll(stageDir(), 0o755); err != nil {
		t.Fatal(err)
	}

	oldRoot := applyRootDir
	applyRootDir = filepath.Join(tmp, "apply-root")
	t.Cleanup(func() { applyRootDir = oldRoot })
	if err := os.MkdirAll(applyRootDir, 0o755); err != nil {
		t.Fatal(err)
	}

	app, _, _ := appTestEnv(t)
	// tanpa backup → 400
	w := httptest.NewRecorder()
	app.handleSystemUpdateRollback(w, httptest.NewRequest("POST", "/api/admin/system/update/rollback", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("tanpa backup kode %d: %s", w.Code, w.Body.String())
	}

	if err := os.WriteFile(backupBinPath(), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	app.handleSystemUpdateRollback(w, httptest.NewRequest("POST", "/api/admin/system/update/rollback", nil))
	if w.Code != http.StatusAccepted {
		t.Fatalf("kode %d: %s", w.Code, w.Body.String())
	}
	b, err := os.ReadFile(applyRequestPath())
	rid := lastApplyRequest()
	if err != nil || rid == "" || strings.TrimSpace(string(b)) != "rollback "+rid {
		t.Fatalf("isi permintaan = %q (id %q), %v", b, rid, err)
	}
	if !strings.Contains(w.Body.String(), `"request":"`+rid+`"`) {
		t.Fatalf("respons tanpa id permintaan: %s", w.Body.String())
	}
}

func TestHelperOutdated(t *testing.T) {
	tmp := t.TempDir()
	old := applyUpdateScript
	applyUpdateScript = filepath.Join(tmp, "apply-update.sh")
	t.Cleanup(func() { applyUpdateScript = old })

	if helperOutdated() {
		t.Fatal("helper tidak terpasang tidak boleh dianggap usang")
	}
	_ = os.WriteFile(applyUpdateScript, []byte("#!/bin/bash\nversi lama"), 0o755)
	if !helperOutdated() {
		t.Fatal("helper beda isi harus usang")
	}
	_ = os.WriteFile(applyUpdateScript, []byte(applyUpdateScriptContent), 0o755)
	if helperOutdated() {
		t.Fatal("helper identik tidak boleh usang")
	}
}

// helper harus lolos parse bash (bash -n) — salah kutip di raw string Go
// baru ketahuan saat dijalankan root di server.
func TestHelperScriptSyntax(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash tidak tersedia")
	}
	p := filepath.Join(t.TempDir(), "apply-update.sh")
	if err := os.WriteFile(p, []byte(applyUpdateScriptContent), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("bash", "-n", p).CombinedOutput(); err != nil {
		t.Fatalf("bash -n: %v\n%s", err, out)
	}
}
