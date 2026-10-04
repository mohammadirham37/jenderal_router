package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jenderal/jenderalrouter/internal/crypto"
)

// Tipe provider.
const (
	ProviderOpenAI       = "openai"
	ProviderAnthropic    = "anthropic"
	ProviderGemini       = "gemini"
	ProviderOpenAICompat = "openai-compatible"
	ProviderLlamaStash   = "llamastash"
)

// Strategi pemilihan credential (FR-1.3).
const (
	CredStrategyRoundRobin = "round_robin"
	CredStrategyPriority   = "priority"
	CredStrategyLeastUsed  = "least_used"
)

// Provider adalah layanan LLM cloud/lokal.
type Provider struct {
	ID        int64           `json:"id"`
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	Prefix    string          `json:"prefix"`
	BaseURL   string          `json:"base_url"`
	Settings  json.RawMessage `json:"settings"`
	Enabled   bool            `json:"enabled"`
	CreatedAt string          `json:"created_at"`
}

// CredentialSettings konfigurasi tambahan provider (kolom settings JSON).
type CredentialSettings struct {
	Strategy           string            `json:"strategy,omitempty"` // round_robin|priority|least_used
	ExtraHeaders       map[string]string `json:"extra_headers,omitempty"`
	SSRFAllowPrivate   bool              `json:"ssrf_allow_private,omitempty"`     // NFR-07
	LocalConcurrency   int               `json:"local_concurrency,omitempty"`      // FR-6.5, default 2
	LocalQueueTimeout  int               `json:"local_queue_timeout_ms,omitempty"` // default 30000
	ConnectTimeoutMs   int               `json:"connect_timeout_ms,omitempty"`     // default 10000; lokal 5000
	ColdStartTimeoutMs int               `json:"cold_start_timeout_ms,omitempty"`  // lokal 120000
}

// Credential adalah API key provider (tersimpan terenkripsi).
type Credential struct {
	ID            int64     `json:"id"`
	ProviderID    int64     `json:"provider_id"`
	Label         string    `json:"label"`
	SecretEnc     string    `json:"-"`
	Weight        int       `json:"weight"`
	Status        string    `json:"status"` // active|invalid|disabled
	CooldownUntil time.Time `json:"-"`
	UseCount      int64     `json:"use_count"`
	FailCount     int64     `json:"fail_count"`
	LastError     string    `json:"last_error,omitempty"`
	CreatedAt     string    `json:"created_at"`
}

// PublicCredential tanpa secret, untuk API dashboard.
type PublicCredential struct {
	ID         int64  `json:"id"`
	Label      string `json:"label"`
	Weight     int    `json:"weight"`
	Status     string `json:"status"`
	Cooldown   bool   `json:"cooldown"`
	UseCount   int64  `json:"use_count"`
	FailCount  int64  `json:"fail_count"`
	LastError  string `json:"last_error,omitempty"`
	PrefixHint string `json:"prefix_hint,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// ErrNoCredential bila tidak ada credential siap pakai.
var ErrNoCredential = errors.New("tidak ada credential aktif untuk provider ini")

// CreateProvider mendaftarkan provider baru.
func (s *Store) CreateProvider(pType, name, prefix, baseURL string, settings CredentialSettings, enabled bool) (*Provider, error) {
	sett := toJSON(settings)
	res, err := s.DB.Exec(`INSERT INTO providers (type, name, prefix, base_url, settings, enabled) VALUES (?,?,?,?,?,?)`,
		pType, name, prefix, baseURL, sett, boolInt(enabled))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetProvider(id)
}

// GetProvider mengambil provider by id.
func (s *Store) GetProvider(id int64) (*Provider, error) {
	p := &Provider{}
	var enabled int
	var settings string
	err := s.DB.QueryRow(`SELECT id, type, name, prefix, base_url, settings, enabled, created_at
		FROM providers WHERE id = ?`, id).
		Scan(&p.ID, &p.Type, &p.Name, &p.Prefix, &p.BaseURL, &settings, &enabled, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Enabled = enabled == 1
	p.Settings = json.RawMessage(settings)
	return p, nil
}

// ListProviders mengembalikan semua provider.
func (s *Store) ListProviders() ([]*Provider, error) {
	rows, err := s.DB.Query(`SELECT id, type, name, prefix, base_url, settings, enabled, created_at
		FROM providers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Provider
	for rows.Next() {
		p := &Provider{}
		var enabled int
		var settings string
		if err := rows.Scan(&p.ID, &p.Type, &p.Name, &p.Prefix, &p.BaseURL, &settings, &enabled, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.Enabled = enabled == 1
		p.Settings = json.RawMessage(settings)
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdateProviderFields memperbarui kolom opsional provider; nil = biarkan.
func (s *Store) UpdateProviderFields(id int64, name, baseURL *string, settings *CredentialSettings, enabled *bool) error {
	p, err := s.GetProvider(id)
	if err != nil {
		return err
	}
	if name != nil {
		p.Name = *name
	}
	if baseURL != nil {
		p.BaseURL = *baseURL
	}
	if enabled != nil {
		p.Enabled = *enabled
	}
	sett := string(p.Settings)
	if settings != nil {
		sett = toJSON(*settings)
	}
	_, err = s.DB.Exec(`UPDATE providers SET name=?, base_url=?, settings=?, enabled=? WHERE id=?`,
		p.Name, p.BaseURL, sett, boolInt(p.Enabled), id)
	return err
}

// DeleteProvider menghapus provider (cascade credential & model).
func (s *Store) DeleteProvider(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM providers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- Credential ----

// AddCredential menambah credential dengan secret dienkripsi (FR-1.9).
func (s *Store) AddCredential(providerID int64, label, secret string, weight int) (*Credential, error) {
	enc, err := s.EncryptSecret(secret)
	if err != nil {
		return nil, err
	}
	if weight <= 0 {
		weight = 1
	}
	res, err := s.DB.Exec(`INSERT INTO credentials (provider_id, label, secret_enc, weight) VALUES (?,?,?,?)`,
		providerID, label, enc, weight)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetCredential(id)
}

// GetCredential mengambil credential by id.
func (s *Store) GetCredential(id int64) (*Credential, error) {
	c := &Credential{}
	var cooldown string
	err := s.DB.QueryRow(`SELECT id, provider_id, label, secret_enc, weight, status, cooldown_until,
		use_count, fail_count, last_error, created_at FROM credentials WHERE id = ?`, id).
		Scan(&c.ID, &c.ProviderID, &c.Label, &c.SecretEnc, &c.Weight, &c.Status, &cooldown,
			&c.UseCount, &c.FailCount, &c.LastError, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if cooldown != "" {
		c.CooldownUntil, _ = time.Parse(time.RFC3339, cooldown)
	}
	return c, nil
}

// ListCredentials provider tertentu.
func (s *Store) ListCredentials(providerID int64) ([]*Credential, error) {
	rows, err := s.DB.Query(`SELECT id, provider_id, label, secret_enc, weight, status, cooldown_until,
		use_count, fail_count, last_error, created_at FROM credentials WHERE provider_id = ? ORDER BY weight, id`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Credential
	for rows.Next() {
		c := &Credential{}
		var cooldown string
		if err := rows.Scan(&c.ID, &c.ProviderID, &c.Label, &c.SecretEnc, &c.Weight, &c.Status, &cooldown,
			&c.UseCount, &c.FailCount, &c.LastError, &c.CreatedAt); err != nil {
			return nil, err
		}
		if cooldown != "" {
			c.CooldownUntil, _ = time.Parse(time.RFC3339, cooldown)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteCredential menghapus credential.
func (s *Store) DeleteCredential(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM credentials WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// PublicCredential menghapus secret untuk tampilan.
func (c *Credential) Public() *PublicCredential {
	return &PublicCredential{
		ID: c.ID, Label: c.Label, Weight: c.Weight, Status: c.Status,
		Cooldown: time.Now().Before(c.CooldownUntil),
		UseCount: c.UseCount, FailCount: c.FailCount, LastError: c.LastError,
		CreatedAt: c.CreatedAt,
	}
}

// rrCounter counters round-robin per provider (in-memory, aman goroutine).
var rrCounter sync.Map // map[int64]*atomicCounter

type atomicCounter struct {
	mu sync.Mutex
	n  uint64
}

func nextRR(providerID int64) uint64 {
	v, _ := rrCounter.LoadOrStore(providerID, &atomicCounter{})
	c := v.(*atomicCounter)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	return c.n
}

// PickCredential memilih credential siap pakai sesuai strategi (FR-1.3):
// skip yang cooldown/invalid/disabled; kandidat bisa disaring via skipIDs.
func (s *Store) PickCredential(providerID int64, strategy string, skipIDs map[int64]bool) (*Credential, error) {
	creds, err := s.ListCredentials(providerID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var ready []*Credential
	for _, c := range creds {
		if c.Status != "active" {
			continue
		}
		if now.Before(c.CooldownUntil) {
			continue
		}
		if skipIDs != nil && skipIDs[c.ID] {
			continue
		}
		ready = append(ready, c)
	}
	if len(ready) == 0 {
		return nil, ErrNoCredential
	}
	switch strategy {
	case CredStrategyPriority:
		// urut weight desc lalu id (sudah terurut weight asc → balik)
		return ready[len(ready)-1], nil
	case CredStrategyLeastUsed:
		best := ready[0]
		for _, c := range ready[1:] {
			if c.UseCount < best.UseCount {
				best = c
			}
		}
		return best, nil
	default: // round_robin
		idx := nextRR(providerID) % uint64(len(ready))
		return ready[idx], nil
	}
}

// MarkCredentialSuccess mencatat pemakaian sukses.
func (s *Store) MarkCredentialSuccess(id int64) error {
	_, err := s.DB.Exec(`UPDATE credentials SET use_count = use_count + 1, fail_count = 0,
		cooldown_until = '', last_error = '' WHERE id = ?`, id)
	return err
}

// MarkCredentialFailure menangani kegagalan credential (FR-1.4):
// 429/402 → cooldown 60s eksponensial maks 15 menit; 401 berulang → invalid.
func (s *Store) MarkCredentialFailure(id int64, statusCode int, message string) error {
	c, err := s.GetCredential(id)
	if err != nil {
		return err
	}
	if statusCode == 401 || statusCode == 403 {
		// 401 berulang menandai key invalid
		if c.FailCount >= 1 || statusCode == 403 {
			_, err = s.DB.Exec(`UPDATE credentials SET fail_count = fail_count + 1, status = 'invalid',
				last_error = ? WHERE id = ?`, message, id)
			return err
		}
		_, err = s.DB.Exec(`UPDATE credentials SET fail_count = fail_count + 1, last_error = ? WHERE id = ?`, message, id)
		return err
	}
	if statusCode == 429 || statusCode == 402 {
		// cooldown eksponensial: 60s, 120s, 240s ... maks 15 menit
		backoff := 60 * time.Second
		for i := 0; i < int(c.FailCount) && backoff < 15*time.Minute; i++ {
			backoff *= 2
		}
		if backoff > 15*time.Minute {
			backoff = 15 * time.Minute
		}
		until := time.Now().Add(backoff).UTC().Format(time.RFC3339)
		_, err = s.DB.Exec(`UPDATE credentials SET fail_count = fail_count + 1, cooldown_until = ?,
			last_error = ? WHERE id = ?`, until, message, id)
		return err
	}
	// error lain: catat saja
	_, err = s.DB.Exec(`UPDATE credentials SET fail_count = fail_count + 1, last_error = ? WHERE id = ?`, message, id)
	return err
}

// ClearCredentialCooldown memaksa credential siap dipakai lagi.
func (s *Store) ClearCredentialCooldown(id int64) error {
	_, err := s.DB.Exec(`UPDATE credentials SET cooldown_until = '', status = 'active' WHERE id = ?`, id)
	return err
}

// ---- Enkripsi secret (dipegang store; master key diset saat init) ----

var masterKey []byte

// SetMasterKey menyetel kunci enkripsi secret provider.
func SetMasterKey(key []byte) {
	if len(key) == 32 {
		masterKey = key
	}
}

// EncryptSecret mengenkripsi API key provider (FR-1.9).
func (s *Store) EncryptSecret(plain string) (string, error) {
	if len(masterKey) != 32 {
		return "", errors.New("master key belum diset")
	}
	return crypto.Encrypt(masterKey, []byte(plain))
}

// DecryptSecret membuka API key provider.
func (s *Store) DecryptSecret(enc string) (string, error) {
	if len(masterKey) != 32 {
		return "", errors.New("master key belum diset")
	}
	b, err := crypto.Decrypt(masterKey, enc)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// RandomID pengenal acak 8 byte untuk berbagai keperluan.
func RandomID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return strings.ToLower(fmt.Sprintf("%016x", binary.BigEndian.Uint64(b[:])))
}
