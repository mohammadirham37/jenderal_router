package api

// Pengaman login password (Google OAuth tidak lewat sini):
//   - captcha gambar internal wajib, diverifikasi SEBELUM hashing Argon2
//     (bot tidak bisa membakar CPU dengan tebakan password);
//   - batas kegagalan per IP dan per akun dalam jendela waktu — login yang
//     berhasil tidak dihitung; penguncian akun berlaku untuk string email
//     apa pun (tidak membocorkan email mana yang terdaftar);
//   - jeda acak pada setiap kegagalan; penguncian dicatat di audit log.

import (
	"encoding/base64"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// failLimiter menghitung kejadian per kunci dalam jendela waktu bergeser.
type failLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
}

func newFailLimiter(limit int, window time.Duration) *failLimiter {
	return &failLimiter{hits: map[string][]time.Time{}, limit: limit, window: window}
}

// recent membuang catatan kedaluwarsa milik key (dipanggil dengan mu terkunci).
func (l *failLimiter) recent(key string, now time.Time) []time.Time {
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < l.window {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.hits, key)
		return nil
	}
	l.hits[key] = kept
	return kept
}

// blocked melaporkan apakah key sudah mencapai batas, plus detik sisa.
func (l *failLimiter) blocked(key string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	h := l.recent(key, now)
	if len(h) < l.limit {
		return false, 0
	}
	retry := int(l.window.Seconds() - now.Sub(h[len(h)-l.limit]).Seconds())
	if retry < 1 {
		retry = 1
	}
	return true, retry
}

// hit mencatat satu kejadian; true bila key kini mencapai batas.
func (l *failLimiter) hit(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.hits) > 5000 {
		for k := range l.hits {
			l.recent(k, now) // pangkas agar map tidak tumbuh tanpa batas
		}
	}
	l.hits[key] = append(l.recent(key, now), now)
	return len(l.hits[key]) >= l.limit
}

func (l *failLimiter) reset(key string) {
	l.mu.Lock()
	delete(l.hits, key)
	l.mu.Unlock()
}

func (l *failLimiter) clear() {
	l.mu.Lock()
	l.hits = map[string][]time.Time{}
	l.mu.Unlock()
}

var (
	loginIPFails   = newFailLimiter(10, 15*time.Minute) // per IP (NAT kantor/kampus berbagi)
	loginAcctFails = newFailLimiter(5, 15*time.Minute)  // per email yang dicoba
	captchaRate    = newFailLimiter(30, time.Minute)    // pembuatan captcha per IP
)

// resetAuthLimiters membersihkan semua limiter (pengujian).
func resetAuthLimiters() {
	loginIPFails.clear()
	loginAcctFails.clear()
	captchaRate.clear()
}

// failDelay jeda 300–600 ms pada kegagalan — memperlambat brute force tanpa
// memboroskan CPU (goroutine hanya tidur).
var failDelay = func() { time.Sleep(time.Duration(300+rand.IntN(300)) * time.Millisecond) }

func tooMany(w http.ResponseWriter, retry int, msg string) {
	w.Header().Set("Retry-After", strconv.Itoa(retry))
	writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": msg, "retry_after": retry})
}

// handleCaptcha GET /api/captcha — tantangan baru (PNG data URL, sekali pakai).
func (a *App) handleCaptcha(w http.ResponseWriter, r *http.Request) {
	ip := apigateClientIP(r)
	if ok, retry := captchaRate.blocked(ip); ok {
		tooMany(w, retry, "terlalu sering meminta captcha; coba lagi sebentar")
		return
	}
	captchaRate.hit(ip)
	id, img, err := a.captcha.New()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "gagal membuat captcha"})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"id":         id,
		"image":      "data:image/png;base64," + base64.StdEncoding.EncodeToString(img),
		"expires_in": 120,
	})
}

// checkCaptcha memverifikasi captcha; bila salah menulis 400 dan mencatat
// kegagalan IP. Mengembalikan false bila request harus berhenti.
func (a *App) checkCaptcha(w http.ResponseWriter, ip, id, answer string) bool {
	if a.captcha.Verify(id, answer) {
		return true
	}
	loginIPFails.hit(ip)
	failDelay()
	writeJSON(w, http.StatusBadRequest, map[string]any{"error": "kode captcha salah atau kedaluwarsa", "captcha": true})
	return false
}

// loginFailed mencatat kegagalan password untuk IP & akun; bila akun baru
// saja terkunci, catat ke log & audit.
func (a *App) loginFailed(ip, email string, userID int64) {
	loginIPFails.hit(ip)
	if loginAcctFails.hit("acct:" + email) {
		slog.Warn("akun dikunci sementara: terlalu banyak login gagal", "email", email, "ip", ip)
		_ = a.st.Audit(userID, "auth.login.locked", "users/"+fmtInt(userID), nil,
			map[string]string{"email": email, "ip": ip})
	}
	failDelay()
}
