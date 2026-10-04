package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("JR_MASTER_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	dir := t.TempDir()
	t.Setenv("JR_DATA_DIR", dir)
	t.Setenv("JR_DATABASE_URL", "")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Addr != "127.0.0.1:20130" {
		t.Errorf("Addr default = %q", c.Addr)
	}
	if c.DatabaseURL != filepath.Join(dir, "jenderalrouter.db") {
		t.Errorf("DatabaseURL default = %q", c.DatabaseURL)
	}
	if c.LLamastashURL != "http://127.0.0.1:11435/v1" {
		t.Errorf("LlamaStash default = %q", c.LLamastashURL)
	}
	if c.LogPrompts {
		t.Error("LogPrompts harus default false")
	}
	if c.TZ != "Asia/Jakarta" {
		t.Errorf("TZ default = %q", c.TZ)
	}
	if len(c.MasterKey) != 32 {
		t.Errorf("MasterKey len = %d", len(c.MasterKey))
	}
}

func TestLoadMasterKeyPassphrase(t *testing.T) {
	t.Setenv("JR_MASTER_KEY", "rahasia-super-panjang")
	t.Setenv("JR_DATA_DIR", t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.MasterKey) != 32 {
		t.Errorf("derived key len = %d", len(c.MasterKey))
	}
	c2, err := Load()
	if err != nil {
		t.Fatalf("Load 2: %v", err)
	}
	if string(c.MasterKey) != string(c2.MasterKey) {
		t.Error("derivasi passphrase harus deterministik")
	}
}

func TestLoadMasterKeyFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JR_MASTER_KEY", "")
	t.Setenv("JR_DATA_DIR", dir)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.MasterKey) != 32 {
		t.Fatalf("key len = %d", len(c.MasterKey))
	}
	// file master.key harus ada dengan izin 0600 (NFR-04)
	st, err := os.Stat(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatalf("master.key tidak dibuat: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("izin master.key = %v, mau 0600", st.Mode().Perm())
	}
	// load kedua harus memakai key yang sama
	c2, _ := Load()
	if string(c.MasterKey) != string(c2.MasterKey) {
		t.Error("master key dari file harus stabil")
	}
}

func TestCookieSecureFromPublicURL(t *testing.T) {
	t.Setenv("JR_MASTER_KEY", "k")
	t.Setenv("JR_DATA_DIR", t.TempDir())
	t.Setenv("JR_PUBLIC_URL", "https://ai.example.com")
	c, _ := Load()
	if !c.CookieSecure {
		t.Error("cookie harus Secure untuk public URL https")
	}
}
