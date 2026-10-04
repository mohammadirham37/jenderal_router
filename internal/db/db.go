// Package db membuka koneksi SQLite dan menerapkan migrasi embed.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"sort"
	"strings"

	_ "modernc.org/sqlite" // driver pure-Go tanpa CGO (NFR-15)
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open membuka (atau membuat) database sqlite di path, menyetel pragma
// yang aman untuk gateway, lalu menerapkan migrasi yang belum jalan.
func Open(pathOrURL string) (*sql.DB, error) {
	dsn := toDSN(pathOrURL)
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// batasi koneksi tulis paralel; SQLite single-writer
	d.SetMaxOpenConns(1)
	if err := configure(d); err != nil {
		d.Close()
		return nil, err
	}
	if err := migrate(d); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

// OpenInMemory membuat database sqlite di memori untuk pengujian.
func OpenInMemory() (*sql.DB, error) {
	return Open("file::memory:?cache=shared")
}

// toDSN menormalkan JR_DATABASE_URL (path atau file: URL) ke DSN driver
// dengan pragma yang dibutuhkan.
func toDSN(v string) string {
	if strings.Contains(v, "://") && !strings.HasPrefix(v, "file:") {
		// postgres://... diteruskan apa adanya (belum didukung MVP → error jelas di pemanggil)
		return v
	}
	if !strings.HasPrefix(v, "file:") {
		v = "file:" + v
	}
	u, err := url.Parse(v)
	if err != nil {
		return v + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	}
	q := u.Query()
	if !q.Has("_pragma") {
		q.Add("_pragma", "busy_timeout(5000)")
		q.Add("_pragma", "journal_mode(WAL)")
		q.Add("_pragma", "foreign_keys(1)")
		q.Add("_pragma", "synchronous(NORMAL)")
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func configure(d *sql.DB) error {
	for _, pragma := range []string{
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA synchronous = NORMAL",
	} {
		if _, err := d.Exec(pragma); err != nil {
			return fmt.Errorf("%s: %w", pragma, err)
		}
	}
	return nil
}

// migrate menerapkan file migrations/*.sql urut berdasar nama yang belum
// tercatat di schema_migrations.
func migrate(d *sql.DB) error {
	entries, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(entries)
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now')))`); err != nil {
		return err
	}
	for _, name := range entries {
		var version int
		if _, err := fmt.Sscanf(name, "migrations/%d", &version); err != nil {
			return fmt.Errorf("nama migrasi tidak sah %s: %w", name, err)
		}
		var exists int
		if err := d.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		body, err := migrationsFS.ReadFile(name)
		if err != nil {
			return err
		}
		tx, err := d.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrasi %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
