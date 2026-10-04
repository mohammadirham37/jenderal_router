// Package store menyediakan akses data ke SQLite untuk seluruh entitas
// JenderalRouter. Semua metode aman dipakai lintas goroutine (sql.DB).
package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/jenderal/jenderalrouter/internal/db"
)

// Store membungkus *sql.DB dengan query yang sudah disiapkan per entitas.
type Store struct {
	DB *sql.DB
}

// New membuka database di path dan mengembalikan Store siap pakai.
func New(pathOrURL string) (*Store, error) {
	d, err := db.Open(pathOrURL)
	if err != nil {
		return nil, err
	}
	return &Store{DB: d}, nil
}

// NewInMemory untuk pengujian.
func NewInMemory() (*Store, error) {
	d, err := db.OpenInMemory()
	if err != nil {
		return nil, err
	}
	return &Store{DB: d}, nil
}

// Close menutup koneksi database.
func (s *Store) Close() error { return s.DB.Close() }

// ---- Peran (FR-4.1) ----

const (
	RoleSuperAdmin = "super_admin"
	RoleAdmin      = "admin"
	RoleMember     = "member"
	RoleViewer     = "viewer"
)

var roleRank = map[string]int{
	RoleViewer:     1,
	RoleMember:     2,
	RoleAdmin:      3,
	RoleSuperAdmin: 4,
}

// RoleAtLeast melaporkan apakah role memenuhi tingkat minimum.
func RoleAtLeast(role, min string) bool {
	return roleRank[role] >= roleRank[min]
}

// ---- Waktu ----

// nowUTC mengembalikan waktu UTC sebagai RFC3339 (format penyimpanan).
func nowUTC() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func nullTime(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func fromNull(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// toJSON mem-serialisasi v aman (fallback "{}").
func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func parseJSONList(s string) []string {
	if s == "" || s == "*" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
