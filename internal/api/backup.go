package api

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// backupDatabase menyalin file SQLite ke data/backups (NFR-12) dan
// memangkas salinan lama hingga 7 terakhir. Aman dipanggil saat runtime:
// sqlite file copy dengan WAL perlu checkpoint terlebih dahulu.
func (a *App) backupDatabase() error {
	backupDir := filepath.Join(a.cfg.DataDir, "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return err
	}
	// checkpoint WAL agar file utama mutakhir
	if _, err := a.st.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("checkpoint: %w", err)
	}
	src := a.cfg.DatabaseURL
	if src == "" || src == ":memory:" {
		return fmt.Errorf("database in-memory tidak dapat dibackup")
	}
	name := fmt.Sprintf("jr-%s.db", time.Now().UTC().Format("20060102-150405"))
	dst := filepath.Join(backupDir, name)
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return err
	}
	a.pruneBackups(backupDir, 7)
	return nil
}

// pruneBackups menyimpan maksimal keep salinan terbaru.
func (a *App) pruneBackups(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var files []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".db" {
			files = append(files, e)
		}
	}
	if len(files) <= keep {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name() > files[j].Name() })
	for _, f := range files[keep:] {
		_ = os.Remove(filepath.Join(dir, f.Name()))
	}
}
