package db

import (
	"database/sql"
	"testing"
)

func TestMigrateAppliesAndIsIdempotent(t *testing.T) {
	d, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer d.Close()

	// jalankan migrasi kedua kali — harus idempoten
	if err := migrate(d); err != nil {
		t.Fatalf("migrate ulang: %v", err)
	}

	for _, table := range []string{
		"users", "providers", "credentials", "models", "combos", "combo_steps",
		"api_keys", "quotas", "request_logs", "conversations", "messages",
		"audit_logs", "sessions", "settings", "schema_migrations",
	} {
		var name string
		if err := d.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name); err != nil {
			t.Errorf("tabel %s tidak ada: %v", table, err)
		}
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	d, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()

	var fk int
	if err := d.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, mau 1", fk)
	}
	// user tanpa parent untuk api_keys harus ditolak
	if _, err := d.Exec(`INSERT INTO api_keys (user_id, prefix, key_hash) VALUES (999, 'jr', 'h')`); err == nil {
		t.Fatal("FK harus menolak api_keys tanpa user")
	}
}

func TestWALMode(t *testing.T) {
	d, err := OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var mode string
	if err := d.QueryRow("PRAGMA journal_mode").Scan(&mode); err == nil && mode != "" {
		t.Logf("journal_mode = %s", mode) // memory untuk :memory:, WAL untuk file
	}
	_ = sql.ErrNoRows
}
