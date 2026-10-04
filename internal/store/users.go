package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jenderal/jenderalrouter/internal/crypto"
)

// ErrNotFound kembalikan bila baris tidak ditemukan.
var ErrNotFound = errors.New("tidak ditemukan")

// User adalah akun dashboard/playground.
type User struct {
	ID             int64  `json:"id"`
	Email          string `json:"email"`
	PasswordHash   string `json:"-"`
	Role           string `json:"role"`
	Status         string `json:"status"`
	DefaultComboID *int64 `json:"default_combo_id"`
	CreatedAt      string `json:"created_at"`
}

// CreateUser membuat user baru dengan hash password Argon2id.
func (s *Store) CreateUser(email, password, role string) (*User, error) {
	hash, err := crypto.HashPassword(password)
	if err != nil {
		return nil, err
	}
	res, err := s.DB.Exec(`INSERT INTO users (email, password_hash, role) VALUES (?,?,?)`,
		email, hash, role)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetUser(id)
}

// GetUser mengambil user berdasarkan id.
func (s *Store) GetUser(id int64) (*User, error) {
	u := &User{}
	var combo sql.NullInt64
	err := s.DB.QueryRow(`SELECT id, email, password_hash, role, status, default_combo_id, created_at
		FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &combo, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if combo.Valid {
		v := combo.Int64
		u.DefaultComboID = &v
	}
	return u, nil
}

// GetUserByEmail mengambil user berdasarkan email.
func (s *Store) GetUserByEmail(email string) (*User, error) {
	u := &User{}
	var combo sql.NullInt64
	err := s.DB.QueryRow(`SELECT id, email, password_hash, role, status, default_combo_id, created_at
		FROM users WHERE email = ?`, email).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &combo, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if combo.Valid {
		v := combo.Int64
		u.DefaultComboID = &v
	}
	return u, nil
}

// ListUsers mengembalikan semua user urut email.
func (s *Store) ListUsers() ([]*User, error) {
	rows, err := s.DB.Query(`SELECT id, email, password_hash, role, status, default_combo_id, created_at
		FROM users ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		u := &User{}
		var combo sql.NullInt64
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &combo, &u.CreatedAt); err != nil {
			return nil, err
		}
		if combo.Valid {
			v := combo.Int64
			u.DefaultComboID = &v
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UpdateUserFields memperbarui kolom opsional; parameter kosong dibiarkan.
func (s *Store) UpdateUserFields(id int64, email, role, status *string, password *string, defaultCombo *int64) error {
	u, err := s.GetUser(id)
	if err != nil {
		return err
	}
	if email != nil {
		u.Email = *email
	}
	if role != nil {
		u.Role = *role
	}
	if status != nil {
		u.Status = *status
	}
	if defaultCombo != nil {
		u.DefaultComboID = defaultCombo
	}
	hash := u.PasswordHash
	if password != nil {
		h, err := crypto.HashPassword(*password)
		if err != nil {
			return err
		}
		hash = h
	}
	_, err = s.DB.Exec(`UPDATE users SET email=?, password_hash=?, role=?, status=?, default_combo_id=? WHERE id=?`,
		u.Email, hash, u.Role, u.Status, u.DefaultComboID, id)
	return err
}

// DeleteUser menghapus user (cascade ke key & sesi).
func (s *Store) DeleteUser(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountSuperAdmins menghitung jumlah super_admin aktif (wizard setup).
func (s *Store) CountSuperAdmins() (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE role = ?`, RoleSuperAdmin).Scan(&n)
	return n, err
}

// ---- Sesi dashboard (NFR-03) ----

// Session adalah sesi login dashboard.
type Session struct {
	TokenHash string
	UserID    int64
	CSRFToken string
	ExpiresAt time.Time
}

// CreateSession membuat sesi baru; token plaintext dikembalikan sekali.
func (s *Store) CreateSession(userID int64, ttl time.Duration) (token, csrf string, err error) {
	token, err = crypto.NewToken()
	if err != nil {
		return "", "", err
	}
	csrf, err = crypto.NewToken()
	if err != nil {
		return "", "", err
	}
	_, err = s.DB.Exec(`INSERT INTO sessions (token_hash, user_id, csrf_token, expires_at) VALUES (?,?,?,?)`,
		crypto.HashToken(token), userID, csrf, time.Now().Add(ttl).UTC().Format(time.RFC3339Nano))
	return token, csrf, err
}

// GetSession mengambil sesi aktif berdasarkan token.
func (s *Store) GetSession(token string) (*Session, error) {
	var sess Session
	var exp string
	err := s.DB.QueryRow(`SELECT token_hash, user_id, csrf_token, expires_at FROM sessions WHERE token_hash = ?`,
		crypto.HashToken(token)).Scan(&sess.TokenHash, &sess.UserID, &sess.CSRFToken, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	sess.ExpiresAt, err = time.Parse(time.RFC3339Nano, exp)
	if err != nil {
		return nil, ErrNotFound
	}
	if time.Now().After(sess.ExpiresAt) {
		s.DeleteSession(token)
		return nil, ErrNotFound
	}
	return &sess, nil
}

// DeleteSession menghapus sesi (logout).
func (s *Store) DeleteSession(token string) error {
	_, err := s.DB.Exec(`DELETE FROM sessions WHERE token_hash = ?`, crypto.HashToken(token))
	return err
}

// PurgeExpiredSessions membersihkan sesi kedaluwarsa (dipanggil worker).
func (s *Store) PurgeExpiredSessions() (int64, error) {
	res, err := s.DB.Exec(`DELETE FROM sessions WHERE expires_at < ?`, nowUTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ---- Audit log (NFR-05) ----

// Audit mencatat aksi admin.
func (s *Store) Audit(actorID int64, action, target string, before, after any) error {
	_, err := s.DB.Exec(`INSERT INTO audit_logs (actor_id, action, target, before, after) VALUES (?,?,?,?,?)`,
		actorID, action, target, toJSON(before), toJSON(after))
	return err
}

// AuditEntry baris log audit untuk dashboard.
type AuditEntry struct {
	ID      int64           `json:"id"`
	TS      string          `json:"ts"`
	ActorID *int64          `json:"actor_id"`
	Action  string          `json:"action"`
	Target  string          `json:"target"`
	Before  json.RawMessage `json:"before"`
	After   json.RawMessage `json:"after"`
}

// ListAudit mengembalikan audit log terbaru.
func (s *Store) ListAudit(limit int) ([]*AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.DB.Query(`SELECT id, ts, actor_id, action, target, before, after
		FROM audit_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AuditEntry
	for rows.Next() {
		e := &AuditEntry{}
		var actor sql.NullInt64
		var before, after string
		if err := rows.Scan(&e.ID, &e.TS, &actor, &e.Action, &e.Target, &before, &after); err != nil {
			return nil, err
		}
		e.Before = json.RawMessage(before)
		e.After = json.RawMessage(after)
		if actor.Valid {
			v := actor.Int64
			e.ActorID = &v
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---- Settings key-value ----

// GetSetting membaca pengaturan; "" bila tidak ada.
func (s *Store) GetSetting(key string) (string, error) {
	var v string
	err := s.DB.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// SetSetting menyimpan pengaturan (upsert).
func (s *Store) SetSetting(key, value string) error {
	_, err := s.DB.Exec(`INSERT INTO settings (key, value) VALUES (?,?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// PublicUser tampilan user tanpa hash password.
type PublicUser struct {
	ID             int64  `json:"id"`
	Email          string `json:"email"`
	Role           string `json:"role"`
	Status         string `json:"status"`
	DefaultComboID *int64 `json:"default_combo_id"`
	CreatedAt      string `json:"created_at"`
}

// Public menghapus hash password untuk respons API.
func (u *User) Public() *PublicUser {
	return &PublicUser{ID: u.ID, Email: u.Email, Role: u.Role, Status: u.Status,
		DefaultComboID: u.DefaultComboID, CreatedAt: u.CreatedAt}
}
