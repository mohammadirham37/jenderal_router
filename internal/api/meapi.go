package api

// Endpoint /api/me/* untuk halaman "Penggunaan API" milik member:
// daftar key sendiri, salin key sendiri, dan log request pribadi.
// Semuanya terikat ke user sesi — tidak bisa melihat milik orang lain.

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jenderal/jenderalrouter/internal/store"
)

// handleMeKeys daftar API key milik user yang sedang login.
func (a *App) handleMeKeys(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	keys, err := a.st.ListKeysByUser(ai.user.ID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if keys == nil {
		keys = []*store.APIKey{}
	}
	writeJSON(w, 200, map[string]any{"keys": keys})
}

// handleMeCreateKey member membuat API key untuk DIRINYA SENDIRI.
// Aturan: 1 user = 1 key aktif (key yang dicabut tidak dihitung).
func (a *App) handleMeCreateKey(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	keys, err := a.st.ListKeysByUser(ai.user.ID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	for _, k := range keys {
		if k.RevokedAt == "" {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "Anda sudah punya API key aktif — hapus/cabut yang lama dulu bila ingin mengganti"})
			return
		}
	}
	plain, hash, err := newAPIKey()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	enc, encErr := a.st.EncryptSecret(plain)
	if encErr != nil {
		enc = ""
	}
	k, err := a.st.CreateAPIKey(ai.user.ID, "kunci", plain, hash, enc, "*", "", 0, 0, "")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.gate.InvalidateKey(hash)
	a.audit(r, "key.create.self", "keys/"+fmtInt(k.ID), nil, map[string]string{"prefix": k.Prefix})
	writeJSON(w, 201, map[string]any{"key": k, "plaintext": plain})
}

// handleMeRevealKey membuka plaintext key MILIK SENDIRI (untuk tombol salin).
func (a *App) handleMeRevealKey(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	k, err := a.st.GetAPIKey(id)
	if err != nil || k.UserID != ai.user.ID {
		writeJSON(w, 404, map[string]string{"error": "key tidak ditemukan"})
		return
	}
	if k.RevokedAt != "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "key sudah dicabut"})
		return
	}
	if k.SecretEnc == "" {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "salinan key tidak tersedia — minta admin membuat ulang key"})
		return
	}
	plain, err := a.st.DecryptSecret(k.SecretEnc)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a.audit(r, "key.reveal.self", "keys/"+fmtInt(id), nil, nil)
	writeJSON(w, 200, map[string]string{"plaintext": plain})
}

// handleMeLogs log request milik user yang sedang login.
func (a *App) handleMeLogs(w http.ResponseWriter, r *http.Request) {
	ai := authFrom(r)
	q := r.URL.Query()
	from := q.Get("from")
	if from == "" {
		from = time.Now().AddDate(0, 0, -7).UTC().Format(time.RFC3339)
	}
	to := q.Get("to")
	if to == "" {
		to = time.Now().UTC().Format(time.RFC3339)
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	logs, err := a.queryLogs(from, to, fmtInt(ai.user.ID), "", "", limit)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"logs": logs})
}
