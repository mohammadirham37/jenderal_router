// Package apigate mengautentikasi API key inferensi (jr-…), menegakkan
// IP allowlist, model yang diizinkan, rate limit RPM/TPM, dan kuota —
// dengan cache memori agar tidak query DB per request (NFR-09).
package apigate

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jenderal/jenderalrouter/internal/crypto"
	"github.com/jenderal/jenderalrouter/internal/store"
)

// APIError error ber-HTTP untuk endpoint inferensi (kode sesuai PRD §9).
type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	// RetryAfter detik untuk 429 (header Retry-After)
	RetryAfter int `json:"-"`
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

func errInvalidKey(msg string) *APIError {
	return &APIError{Status: 401, Code: "invalid_api_key", Message: msg}
}

// ErrModelNotAllowed 403 — model tidak diizinkan untuk key ini (FR-4.3).
func ErrModelNotAllowed(model string) *APIError {
	return errModelNotAllowed(model)
}

// ErrModelNotFound 404 — model/combo tidak ada (§9).
func ErrModelNotFound(model string) *APIError {
	return errModelNotFound(model)
}

func errModelNotAllowed(model string) *APIError {
	return &APIError{Status: 403, Code: "model_not_allowed", Message: "model/combo \"" + model + "\" tidak diizinkan untuk key ini"}
}

func errModelNotFound(model string) *APIError {
	return &APIError{Status: 404, Code: "model_not_found", Message: "model/combo \"" + model + "\" tidak ditemukan"}
}

func errRateLimited(retryAfter int, msg string) *APIError {
	return &APIError{Status: 429, Code: "rate_limited", Message: msg, RetryAfter: retryAfter}
}

func errQuotaExceeded(msg string, retryAfter int) *APIError {
	return &APIError{Status: 429, Code: "quota_exceeded", Message: msg, RetryAfter: retryAfter}
}

// AuthContext hasil autentikasi sukses.
type AuthContext struct {
	User *store.User
	Key  *store.APIKey
}

// ModelAllowed memeriksa model/combo terhadap allowed_models key (FR-4.3).
// "*" berarti semua.
func (ac *AuthContext) ModelAllowed(name string) bool {
	if ac.Key == nil {
		return false
	}
	if ac.Key.AllowedModels == "" || ac.Key.AllowedModels == "*" {
		return true
	}
	for _, m := range ac.Key.AllowedList {
		if m == name {
			return true
		}
	}
	return false
}

// Gate menyediakan Authenticate + rate limit.
type Gate struct {
	Store *store.Store
	// KeyCache cache key berdasar hash; TTL pendek, dibersihkan saat revoke.
	mu    sync.RWMutex
	cache map[string]cacheEntry
	ttl   time.Duration
	// limitters sliding window RPM per key
	rlMu sync.Mutex
	rl   map[int64][]time.Time
	// TPM aproximasi: token terpakai dalam 60 detik
	tpmMu sync.Mutex
	tpm   map[int64]tpmWindow
}

type cacheEntry struct {
	ac      *AuthContext
	fetched time.Time
}

type tpmWindow struct {
	start  time.Time
	tokens int
}

// NewGate membuat Gate.
func NewGate(st *store.Store) *Gate {
	return &Gate{Store: st, cache: map[string]cacheEntry{}, ttl: 30 * time.Second,
		rl: map[int64][]time.Time{}, tpm: map[int64]tpmWindow{}}
}

// InvalidateKey menghapus key dari cache (dipanggil saat revoke/update).
func (g *Gate) InvalidateKey(hash string) {
	g.mu.Lock()
	delete(g.cache, hash)
	g.mu.Unlock()
}

// Authenticate memvalidasi request /v1/*: Bearer jr-… (atau x-api-key untuk
// endpoint Anthropic). Mengembalikan AuthContext atau *APIError.
func (g *Gate) Authenticate(r *http.Request) (*AuthContext, *APIError) {
	key := extractAPIKey(r)
	if key == "" {
		return nil, errInvalidKey("header Authorization Bearer atau x-api-key wajib diisi")
	}
	if !strings.HasPrefix(key, "jr-") {
		return nil, errInvalidKey("format API key tidak dikenal")
	}
	hash := crypto.HashToken(key)

	// cache
	g.mu.RLock()
	if ce, ok := g.cache[hash]; ok && time.Since(ce.fetched) < g.ttl {
		g.mu.RUnlock()
		return g.validate(ce.ac, r)
	}
	g.mu.RUnlock()

	dbKey, err := g.Store.GetAPIKeyByHash(hash)
	if err != nil {
		return nil, errInvalidKey("API key tidak dikenal atau sudah dicabut")
	}
	user, err := g.Store.GetUser(dbKey.UserID)
	if err != nil {
		return nil, errInvalidKey("pemilik key tidak ditemukan")
	}
	ac := &AuthContext{User: user, Key: dbKey}
	g.mu.Lock()
	g.cache[hash] = cacheEntry{ac: ac, fetched: time.Now()}
	g.mu.Unlock()
	return g.validate(ac, r)
}

func (g *Gate) validate(ac *AuthContext, r *http.Request) (*AuthContext, *APIError) {
	now := time.Now()
	if ac.User.Status != "active" {
		return nil, errInvalidKey("akun dinonaktifkan")
	}
	if !ac.Key.APIKeyValid(now) {
		g.InvalidateKey(ac.Key.KeyHash)
		return nil, errInvalidKey("API key dicabut atau kedaluwarsa")
	}
	ip := ClientIP(r)
	if !ac.Key.IPAllowed(ip) {
		return nil, &APIError{Status: 403, Code: "ip_not_allowed", Message: "IP " + ip + " tidak ada di allowlist key ini"}
	}
	return ac, nil
}

// CheckRateLimit menegakkan RPM key (FR-4.3/4.4). 0 = tanpa batas.
func (g *Gate) CheckRateLimit(keyID int64, rpm int) *APIError {
	if rpm <= 0 {
		return nil
	}
	g.rlMu.Lock()
	defer g.rlMu.Unlock()
	now := time.Now()
	window := g.rl[keyID]
	kept := window[:0]
	for _, t := range window {
		if now.Sub(t) < time.Minute {
			kept = append(kept, t)
		}
	}
	window = kept
	if len(window) >= rpm {
		retry := 60 - int(now.Sub(window[0]).Seconds())
		if retry < 1 {
			retry = 1
		}
		g.rl[keyID] = window
		return errRateLimited(retry, "batas "+itoa(rpm)+" request/menit terlampaui")
	}
	g.rl[keyID] = append(window, now)
	return nil
}

// RecordTPM mencatat token dan menegakkan TPM (0 = tanpa batas).
func (g *Gate) RecordTPM(keyID int64, tpm int, tokens int) *APIError {
	if tpm <= 0 {
		return nil
	}
	g.tpmMu.Lock()
	defer g.tpmMu.Unlock()
	now := time.Now()
	w := g.tpm[keyID]
	if now.Sub(w.start) >= time.Minute {
		w = tpmWindow{start: now, tokens: 0}
	}
	w.tokens += tokens
	g.tpm[keyID] = w
	if w.tokens > tpm {
		return errRateLimited(60, "batas "+itoa(tpm)+" token/menit terlampaui")
	}
	return nil
}

// extractAPIKey mengambil key dari Authorization Bearer atau x-api-key.
func extractAPIKey(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	if k := r.Header.Get("x-api-key"); k != "" {
		return strings.TrimSpace(k)
	}
	return ""
}

// ClientIP mengambil IP klien (mendukung X-Forwarded-For dari Caddy).
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return strings.TrimSpace(xr)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
