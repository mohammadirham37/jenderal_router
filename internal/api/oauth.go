package api

// Login OAuth Google — khusus email Unsoed (@unsoed.ac.id / @mhs.unsoed.ac.id).
//
// GET /google/auth     → redirect ke consent Google (state disimpan di cookie).
// GET /google/callback → tukar code, validasi domain, auto-create user member
//                        bila belum ada, lalu buat session dan redirect ke /chat.
//
// Password user OAuth diisi acak tak-terpakai (login tetap via Google).

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/jenderal/jenderalrouter/internal/crypto"
	"github.com/jenderal/jenderalrouter/internal/store"
)

const (
	oauthStateCookie = "jr_oauth_state"
	googleAuthURL    = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL   = "https://oauth2.googleapis.com/token"
	googleUserInfo   = "https://openidconnect.googleapis.com/v1/userinfo"
)

var allowedGoogleDomains = []string{"unsoed.ac.id", "mhs.unsoed.ac.id"}

func (a *App) googleCreds() (clientID, clientSecret string) {
	id, _ := a.st.GetSetting("google_oauth_client_id")
	sec, _ := a.st.GetSetting("google_oauth_client_secret")
	return strings.TrimSpace(id), strings.TrimSpace(sec)
}

func emailDomainAllowed(email string) bool {
	e := strings.ToLower(strings.TrimSpace(email))
	for _, d := range allowedGoogleDomains {
		if strings.HasSuffix(e, "@"+d) {
			return true
		}
	}
	return false
}

// oauthRedirectURI URI callback mengikuti host yang diakses — lewat tunnel
// Cloudflare otomatis https://chat.jenderalpanel.com/google/callback.
func oauthRedirectURI(r *http.Request) string {
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	return proto + "://" + r.Host + "/google/callback"
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func isSecureRequest(r *http.Request) bool {
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		return p == "https"
	}
	return r.TLS != nil
}

// renderOAuthError halaman error mandiri bergaya tema.
func renderOAuthError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	fmt.Fprintf(w, `<!doctype html><html lang="id"><head><meta charset="utf-8">
<title>Login Google gagal — JenderalRouter</title>
<style>body{font-family:Inter,system-ui,sans-serif;background:#070b14;color:#e8edf8;display:grid;place-items:center;min-height:100vh;margin:0}
.c{background:#0d1424;border:1px solid #1e2b47;border-radius:14px;padding:28px 32px;max-width:440px;text-align:center}
h2{margin:0 0 10px;font-size:18px}p{margin:0 0 16px;line-height:1.6;color:#8b98b8}a{color:#38bdf8}</style>
</head><body><div class="c"><h2>Login Google gagal</h2><p>%s</p><p><a href="/login">← Kembali ke halaman masuk</a></p></div></body></html>`,
		html.EscapeString(msg))
}

// handleGoogleAuth GET /google/auth — mulai alur OAuth.
func (a *App) handleGoogleAuth(w http.ResponseWriter, r *http.Request) {
	clientID, clientSecret := a.googleCreds()
	if clientID == "" || clientSecret == "" {
		renderOAuthError(w, "Login Google belum dikonfigurasi oleh admin.")
		return
	}
	state := randHex(16)
	http.SetCookie(w, &http.Cookie{
		Name: oauthStateCookie, Value: state, Path: "/",
		HttpOnly: true, Secure: isSecureRequest(r),
		MaxAge: 600, SameSite: http.SameSiteLaxMode,
	})
	q := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {oauthRedirectURI(r)},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"state":         {state},
		"prompt":        {"select_account"},
	}
	http.Redirect(w, r, googleAuthURL+"?"+q.Encode(), http.StatusFound)
}

// handleGoogleCallback GET /google/callback — tukar code jadi session.
func (a *App) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	// validasi state (CSRF OAuth)
	ck, err := r.Cookie(oauthStateCookie)
	if err != nil || ck.Value == "" || ck.Value != r.URL.Query().Get("state") {
		renderOAuthError(w, "State OAuth tidak sah — mulai ulang dari halaman masuk.")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: oauthStateCookie, Value: "", Path: "/", MaxAge: -1})

	code := r.URL.Query().Get("code")
	if code == "" {
		renderOAuthError(w, "Google tidak mengirimkan kode otorisasi.")
		return
	}
	clientID, clientSecret := a.googleCreds()
	if clientID == "" || clientSecret == "" {
		renderOAuthError(w, "Login Google belum dikonfigurasi oleh admin.")
		return
	}

	// tukar code → access token
	form := url.Values{
		"code":          {code},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"redirect_uri":  {oauthRedirectURI(r)},
		"grant_type":    {"authorization_code"},
	}
	tokResp, err := http.PostForm(googleTokenURL, form)
	if err != nil {
		renderOAuthError(w, "gagal menghubungi Google: "+err.Error())
		return
	}
	defer tokResp.Body.Close()
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(tokResp.Body).Decode(&tok); err != nil || tok.AccessToken == "" {
		renderOAuthError(w, "respons token Google tidak sah.")
		return
	}

	// profil email dari endpoint userinfo
	ureq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, googleUserInfo, nil)
	ureq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	uresp, err := http.DefaultClient.Do(ureq)
	if err != nil {
		renderOAuthError(w, "gagal mengambil profil Google.")
		return
	}
	defer uresp.Body.Close()
	var prof struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := json.NewDecoder(io.LimitReader(uresp.Body, 64<<10)).Decode(&prof); err != nil || prof.Email == "" {
		renderOAuthError(w, "profil Google tidak sah.")
		return
	}
	email := strings.ToLower(strings.TrimSpace(prof.Email))

	// whitelist domain Unsoed
	if !emailDomainAllowed(email) {
		renderOAuthError(w, "Hanya email @unsoed.ac.id atau @mhs.unsoed.ac.id yang diizinkan. Email terdeteksi: "+email)
		return
	}
	if !prof.EmailVerified {
		renderOAuthError(w, "Email Google Anda belum terverifikasi.")
		return
	}

	// ambil atau auto-create user member
	u, err := a.st.GetUserByEmail(email)
	if err != nil {
		rnd, _, keyErr := crypto.NewAPIKey() // password acak 40 karakter — tidak bisa dipakai login
		if keyErr != nil {
			renderOAuthError(w, "gagal menyiapkan akun.")
			return
		}
		ph, pErr := crypto.HashPassword(rnd)
		if pErr != nil {
			renderOAuthError(w, "gagal menyiapkan akun.")
			return
		}
		u, err = a.st.CreateUser(email, ph, store.RoleMember)
		if err != nil {
			renderOAuthError(w, "gagal membuat akun: "+err.Error())
			return
		}
		_ = a.st.Audit(u.ID, "auth.oauth.create", "users/"+fmtInt(u.ID), nil, map[string]string{"email": email})
	}
	if u.Status != "active" {
		renderOAuthError(w, "Akun Anda dinonaktifkan — hubungi admin.")
		return
	}

	token, _, err := a.st.CreateSession(u.ID, a.cfg.SessionTTL)
	if err != nil {
		renderOAuthError(w, "gagal membuat sesi.")
		return
	}
	a.setSessionCookie(w, token, a.cfg.SessionTTL)
	_ = a.st.Audit(u.ID, "auth.login.google", "users/"+fmtInt(u.ID), nil, map[string]string{"email": email})
	http.Redirect(w, r, "/chat", http.StatusFound)
}
