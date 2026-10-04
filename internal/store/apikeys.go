package store

import (
	"database/sql"
	"errors"
	"net"
	"strings"
	"time"
)

// APIKey adalah kunci inferensi milik user (FR-4.2/4.3).
type APIKey struct {
	ID            int64    `json:"id"`
	UserID        int64    `json:"user_id"`
	Prefix        string   `json:"prefix"`
	KeyHash       string   `json:"-"`
	Name          string   `json:"name"`
	AllowedModels string   `json:"allowed_models"`
	IPAllowlist   string   `json:"ip_allowlist"`
	RPM           int      `json:"rpm"`
	TPM           int      `json:"tpm"`
	ExpiresAt     string   `json:"expires_at"`
	RevokedAt     string   `json:"revoked_at"`
	CreatedAt     string   `json:"created_at"`
	AllowedList   []string `json:"allowed_list,omitempty"` // hasil parse AllowedModels
}

// CreateAPIKey membuat key baru; plaintext dikembalikan hanya sekali.
func (s *Store) CreateAPIKey(userID int64, name string, plain, hash string, allowedModels, ipAllowlist string, rpm, tpm int, expiresAt string) (*APIKey, error) {
	if len(plain) < 8 {
		return nil, errors.New("key terlalu pendek")
	}
	prefix := plain[:8]
	res, err := s.DB.Exec(`INSERT INTO api_keys
		(user_id, prefix, key_hash, name, allowed_models, ip_allowlist, rpm, tpm, expires_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		userID, prefix, hash, name, allowedModels, ipAllowlist, rpm, tpm, expiresAt)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetAPIKey(id)
}

// GetAPIKey mengambil key berdasarkan id.
func (s *Store) GetAPIKey(id int64) (*APIKey, error) {
	k := &APIKey{}
	err := s.DB.QueryRow(`SELECT id, user_id, prefix, key_hash, name, allowed_models, ip_allowlist,
		rpm, tpm, expires_at, revoked_at, created_at FROM api_keys WHERE id = ?`, id).
		Scan(&k.ID, &k.UserID, &k.Prefix, &k.KeyHash, &k.Name, &k.AllowedModels, &k.IPAllowlist,
			&k.RPM, &k.TPM, &k.ExpiresAt, &k.RevokedAt, &k.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	k.AllowedList = parseJSONList(k.AllowedModels)
	return k, nil
}

// GetAPIKeyByHash mencari key aktif dari hash (jalur auth per request).
func (s *Store) GetAPIKeyByHash(hash string) (*APIKey, error) {
	k := &APIKey{}
	err := s.DB.QueryRow(`SELECT id, user_id, prefix, key_hash, name, allowed_models, ip_allowlist,
		rpm, tpm, expires_at, revoked_at, created_at FROM api_keys WHERE key_hash = ?`, hash).
		Scan(&k.ID, &k.UserID, &k.Prefix, &k.KeyHash, &k.Name, &k.AllowedModels, &k.IPAllowlist,
			&k.RPM, &k.TPM, &k.ExpiresAt, &k.RevokedAt, &k.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	k.AllowedList = parseJSONList(k.AllowedModels)
	return k, nil
}

// ListKeysByUser mengembalikan seluruh key milik user.
func (s *Store) ListKeysByUser(userID int64) ([]*APIKey, error) {
	rows, err := s.DB.Query(`SELECT id, user_id, prefix, key_hash, name, allowed_models, ip_allowlist,
		rpm, tpm, expires_at, revoked_at, created_at FROM api_keys WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*APIKey
	for rows.Next() {
		k := &APIKey{}
		if err := rows.Scan(&k.ID, &k.UserID, &k.Prefix, &k.KeyHash, &k.Name, &k.AllowedModels, &k.IPAllowlist,
			&k.RPM, &k.TPM, &k.ExpiresAt, &k.RevokedAt, &k.CreatedAt); err != nil {
			return nil, err
		}
		k.AllowedList = parseJSONList(k.AllowedModels)
		out = append(out, k)
	}
	return out, rows.Err()
}

// RevokeAPIKey mencabut key (tidak menghapus log).
func (s *Store) RevokeAPIKey(id int64) error {
	res, err := s.DB.Exec(`UPDATE api_keys SET revoked_at = ? WHERE id = ? AND revoked_at = ''`, nowUTC(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAPIKey menghapus key permanen.
func (s *Store) DeleteAPIKey(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM api_keys WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateAPIKeyFields memperbarui batasan key; nil = biarkan.
func (s *Store) UpdateAPIKeyFields(id int64, name, allowedModels, ipAllowlist *string, rpm, tpm *int, expiresAt *string) error {
	k, err := s.GetAPIKey(id)
	if err != nil {
		return err
	}
	if name != nil {
		k.Name = *name
	}
	if allowedModels != nil {
		k.AllowedModels = *allowedModels
	}
	if ipAllowlist != nil {
		k.IPAllowlist = *ipAllowlist
	}
	if rpm != nil {
		k.RPM = *rpm
	}
	if tpm != nil {
		k.TPM = *tpm
	}
	if expiresAt != nil {
		k.ExpiresAt = *expiresAt
	}
	_, err = s.DB.Exec(`UPDATE api_keys SET name=?, allowed_models=?, ip_allowlist=?, rpm=?, tpm=?, expires_at=? WHERE id=?`,
		k.Name, k.AllowedModels, k.IPAllowlist, k.RPM, k.TPM, k.ExpiresAt, id)
	return err
}

// APIKeyValid memeriksa status key: belum dicabut dan belum kedaluwarsa.
func (k *APIKey) APIKeyValid(now time.Time) bool {
	if k.RevokedAt != "" {
		return false
	}
	if k.ExpiresAt != "" {
		exp, err := time.Parse(time.RFC3339, k.ExpiresAt)
		if err != nil || now.After(exp) {
			return false
		}
	}
	return true
}

// IPAllowed memeriksa IP klien terhadap allowlist CIDR (kosong = semua).
func (k *APIKey) IPAllowed(ip string) bool {
	list := strings.TrimSpace(k.IPAllowlist)
	if list == "" {
		return true
	}
	for _, cidr := range strings.Split(list, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		if cidrContains(cidr, ip) {
			return true
		}
	}
	return false
}

// cidrContains memeriksa ip dalam cidr.
func cidrContains(cidr, ip string) bool {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	addr := net.ParseIP(ip)
	return addr != nil && network.Contains(addr)
}
