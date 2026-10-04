package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jenderal/jenderalrouter/internal/crypto"
	"github.com/jenderal/jenderalrouter/internal/store"
)

const sessionCookie = "jr_session"

// loginLimiter rate limit login 5 percobaan/15 menit/IP (NFR-05).
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	limit    int
	window   time.Duration
}

var logins = &loginLimiter{attempts: map[string][]time.Time{}, limit: 5, window: 15 * time.Minute}

// reset membersihkan riwayat (untuk pengujian).
func (l *loginLimiter) reset() {
	l.mu.Lock()
	l.attempts = map[string][]time.Time{}
	l.mu.Unlock()
}

// allow melaporkan apakah IP boleh mencoba login lagi.
func (l *loginLimiter) allow(ip string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	kept := l.attempts[ip][:0]
	for _, t := range l.attempts[ip] {
		if now.Sub(t) < l.window {
			kept = append(kept, t)
		}
	}
	l.attempts[ip] = kept
	if len(kept) >= l.limit {
		return false, int(l.window.Seconds() - now.Sub(kept[0]).Seconds())
	}
	l.attempts[ip] = append(kept, now)
	return true, 0
}

// routesDashboard mendaftarkan endpoint dashboard & API manajemen.
func (a *App) routesDashboard() {
	// wizard first-run (NFR-02: tanpa password default)
	a.mux.HandleFunc("GET /api/setup/status", a.handleSetupStatus)
	a.mux.HandleFunc("POST /api/setup", a.handleSetup)
	// sesi
	a.mux.HandleFunc("POST /api/login", a.handleLogin)
	a.mux.HandleFunc("POST /api/logout", a.requireSession(a.handleLogout, store.RoleViewer, true))
	a.mux.HandleFunc("GET /api/me", a.requireSession(a.handleMe, store.RoleViewer, false))
	// admin API
	a.mux.HandleFunc("GET /api/admin/templates", a.requireSession(a.handleListTemplates, store.RoleViewer, false))
	a.mux.HandleFunc("GET /api/admin/providers", a.requireSession(a.handleListProviders, store.RoleViewer, false))
	a.mux.HandleFunc("POST /api/admin/providers", a.requireSession(a.handleCreateProvider, store.RoleAdmin, true))
	a.mux.HandleFunc("POST /api/admin/providers/seed", a.requireSession(a.handleSeedProvider, store.RoleAdmin, true))
	a.mux.HandleFunc("PATCH /api/admin/providers/{id}", a.requireSession(a.handleUpdateProvider, store.RoleAdmin, true))
	a.mux.HandleFunc("DELETE /api/admin/providers/{id}", a.requireSession(a.handleDeleteProvider, store.RoleSuperAdmin, true))
	a.mux.HandleFunc("POST /api/admin/providers/{id}/test", a.requireSession(a.handleTestProvider, store.RoleAdmin, true))
	a.mux.HandleFunc("POST /api/admin/providers/{id}/sync-models", a.requireSession(a.handleSyncModels, store.RoleAdmin, true))
	a.mux.HandleFunc("POST /api/admin/providers/{id}/credentials", a.requireSession(a.handleAddCredential, store.RoleAdmin, true))
	a.mux.HandleFunc("DELETE /api/admin/credentials/{id}", a.requireSession(a.handleDeleteCredential, store.RoleAdmin, true))

	a.mux.HandleFunc("GET /api/admin/models", a.requireSession(a.handleListModels, store.RoleViewer, false))
	a.mux.HandleFunc("PATCH /api/admin/models/{id}", a.requireSession(a.handleUpdateModel, store.RoleAdmin, true))

	a.mux.HandleFunc("GET /api/admin/combos", a.requireSession(a.handleListCombos, store.RoleViewer, false))
	a.mux.HandleFunc("POST /api/admin/combos", a.requireSession(a.handleCreateCombo, store.RoleAdmin, true))
	a.mux.HandleFunc("PATCH /api/admin/combos/{id}", a.requireSession(a.handleUpdateCombo, store.RoleAdmin, true))
	a.mux.HandleFunc("DELETE /api/admin/combos/{id}", a.requireSession(a.handleDeleteCombo, store.RoleAdmin, true))

	a.mux.HandleFunc("GET /api/admin/users", a.requireSession(a.handleListUsers, store.RoleAdmin, false))
	a.mux.HandleFunc("POST /api/admin/users", a.requireSession(a.handleCreateUser, store.RoleAdmin, true))
	a.mux.HandleFunc("PATCH /api/admin/users/{id}", a.requireSession(a.handleUpdateUser, store.RoleAdmin, true))
	a.mux.HandleFunc("DELETE /api/admin/users/{id}", a.requireSession(a.handleDeleteUser, store.RoleSuperAdmin, true))
	a.mux.HandleFunc("GET /api/admin/users/{id}/keys", a.requireSession(a.handleListKeys, store.RoleAdmin, false))
	a.mux.HandleFunc("POST /api/admin/users/{id}/keys", a.requireSession(a.handleCreateKey, store.RoleAdmin, true))
	a.mux.HandleFunc("DELETE /api/admin/keys/{id}", a.requireSession(a.handleDeleteKey, store.RoleAdmin, true))
	a.mux.HandleFunc("PUT /api/admin/users/{id}/quota", a.requireSession(a.handlePutUserQuota, store.RoleAdmin, true))
	a.mux.HandleFunc("PUT /api/admin/keys/{id}/quota", a.requireSession(a.handlePutKeyQuota, store.RoleAdmin, true))

	a.mux.HandleFunc("GET /api/admin/logs", a.requireSession(a.handleListLogs, store.RoleViewer, false))
	a.mux.HandleFunc("GET /api/admin/logs/export.csv", a.requireSession(a.handleExportLogs, store.RoleViewer, false))
	a.mux.HandleFunc("GET /api/admin/analytics/summary", a.requireSession(a.handleAnalyticsSummary, store.RoleViewer, false))
	a.mux.HandleFunc("GET /api/admin/audit", a.requireSession(a.handleListAudit, store.RoleAdmin, false))
	a.mux.HandleFunc("GET /api/admin/settings", a.requireSession(a.handleGetSettings, store.RoleAdmin, false))
	a.mux.HandleFunc("PUT /api/admin/settings", a.requireSession(a.handlePutSettings, store.RoleSuperAdmin, true))
	a.mux.HandleFunc("POST /api/admin/system/backup", a.requireSession(a.handleSystemBackup, store.RoleSuperAdmin, true))

	// LlamaStash (FR-6.3 + FR-6.8)
	a.mux.HandleFunc("GET /api/admin/local/status", a.requireSession(a.handleLocalStatus, store.RoleViewer, false))
	a.mux.HandleFunc("POST /api/admin/local/install", a.requireSession(a.handleLocalInstall, store.RoleAdmin, true))
	a.mux.HandleFunc("GET /api/admin/local/install/status", a.requireSession(a.handleLocalInstallStatus, store.RoleViewer, false))

	// pembaruan aplikasi dari dashboard
	a.mux.HandleFunc("GET /api/admin/system/update/status", a.requireSession(a.handleSystemUpdateStatus, store.RoleAdmin, false))
	a.mux.HandleFunc("POST /api/admin/system/update", a.requireSession(a.handleSystemUpdate, store.RoleSuperAdmin, true))
	a.mux.HandleFunc("POST /api/admin/system/update/apply", a.requireSession(a.handleSystemUpdateApply, store.RoleSuperAdmin, true))
	a.mux.HandleFunc("POST /api/admin/local/models/{name}/start", a.requireSession(a.handleLocalModelStart, store.RoleAdmin, true))
	a.mux.HandleFunc("POST /api/admin/local/models/{name}/stop", a.requireSession(a.handleLocalModelStop, store.RoleAdmin, true))

	// playground: riwayat percakapan (FR-7.2)
	a.mux.HandleFunc("GET /api/me/conversations", a.requireSession(a.handleListConversations, store.RoleMember, false))
	a.mux.HandleFunc("POST /api/me/conversations", a.requireSession(a.handleCreateConversation, store.RoleMember, true))
	a.mux.HandleFunc("GET /api/me/conversations/{id}", a.requireSession(a.handleGetConversation, store.RoleMember, false))
	a.mux.HandleFunc("DELETE /api/me/conversations/{id}", a.requireSession(a.handleDeleteConversation, store.RoleMember, true))
	a.mux.HandleFunc("PATCH /api/me/conversations/{id}", a.requireSession(a.handleUpdateConversation, store.RoleMember, true))
	a.mux.HandleFunc("GET /api/me/conversations/{id}/export", a.requireSession(a.handleExportConversation, store.RoleMember, false))
	a.mux.HandleFunc("POST /api/me/conversations/{id}/messages", a.requireSession(a.handleSaveMessage, store.RoleMember, true))

	// playground: model, chat streaming, kuota (F-12)
	a.mux.HandleFunc("GET /api/me/models", a.requireSession(a.handleMeModels, store.RoleMember, false))
	a.mux.HandleFunc("POST /api/me/chat", a.requireSession(a.handleMeChat, store.RoleMember, true))
	a.mux.HandleFunc("GET /api/me/usage", a.requireSession(a.handleMeUsage, store.RoleMember, false))
}

// ---- sesi & middleware ----

// requireSession membungkus handler dashboard: wajib sesi cookie valid,
// role minimal, dan CSRF token untuk metode mutasi (NFR-03).
func (a *App) requireSession(next func(http.ResponseWriter, *http.Request), minRole string, mutating bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "belum login"})
			return
		}
		sess, err := a.st.GetSession(c.Value)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sesi tidak valid"})
			return
		}
		user, err := a.st.GetUser(sess.UserID)
		if err != nil || user.Status != "active" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "akun tidak aktif"})
			return
		}
		if !store.RoleAtLeast(user.Role, minRole) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "peran tidak memadai"})
			return
		}
		if mutating {
			token := r.Header.Get("X-CSRF-Token")
			if token == "" || token != sess.CSRFToken {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "CSRF token tidak sah"})
				return
			}
		}
		next(w, r.WithContext(withAuthValue(r, &authInfo{user: user, sess: sess})))
	}
}

// setSessionCookie menulis cookie sesi (NFR-03).
func (a *App) setSessionCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

func (a *App) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true,
		Secure: a.cfg.CookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: -1,
	})
}

// ---- setup wizard ----

func (a *App) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	n, err := a.st.CountSuperAdmins()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"needs_setup": n == 0, "version": Version})
}

func (a *App) handleSetup(w http.ResponseWriter, r *http.Request) {
	n, err := a.st.CountSuperAdmins()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if n > 0 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "setup sudah dilakukan"})
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body tidak sah"})
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !strings.Contains(req.Email, "@") {
		writeJSON(w, 400, map[string]string{"error": "email tidak sah"})
		return
	}
	if err := crypto.PasswordStrength(req.Password); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	u, err := a.st.CreateUser(req.Email, req.Password, store.RoleSuperAdmin)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	token, csrf, err := a.st.CreateSession(u.ID, a.cfg.SessionTTL)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.setSessionCookie(w, token, a.cfg.SessionTTL)
	_ = a.st.Audit(u.ID, "setup.completed", "users/"+fmtInt(u.ID), nil, map[string]string{"email": u.Email})
	writeJSON(w, http.StatusOK, map[string]any{"user": u.Public(), "csrf": csrf})
}

// ---- login/logout/me ----

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := apigateClientIP(r)
	if ok, retry := logins.allow(ip); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(retry))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "terlalu banyak percobaan login; coba lagi nanti"})
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body tidak sah"})
		return
	}
	u, err := a.st.GetUserByEmail(strings.TrimSpace(strings.ToLower(req.Email)))
	if err != nil || !crypto.VerifyPassword(u.PasswordHash, req.Password) {
		// respons seragam agar tidak membocorkan keberadaan email
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "email atau password salah"})
		return
	}
	if u.Status != "active" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "akun dinonaktifkan"})
		return
	}
	token, csrf, err := a.st.CreateSession(u.ID, a.cfg.SessionTTL)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.setSessionCookie(w, token, a.cfg.SessionTTL)
	_ = a.st.Audit(u.ID, "auth.login", "users/"+fmtInt(u.ID), nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"user": u.Public(), "csrf": csrf})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = a.st.DeleteSession(c.Value)
	}
	if ai != nil {
		_ = a.st.Audit(ai.user.ID, "auth.logout", "users/"+fmtInt(ai.user.ID), nil, nil)
	}
	a.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	writeJSON(w, http.StatusOK, map[string]any{"user": ai.user.Public(), "csrf": ai.sess.CSRFToken})
}
