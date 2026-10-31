package api

import (
	"net/http"
	"time"

	"github.com/jenderal/jenderalrouter/internal/store"
	"github.com/jenderal/jenderalrouter/internal/usage"
)

// Kuota default: satu pengaturan yang langsung berlaku untuk semua member &
// viewer (lama maupun baru). Kuota khusus per user tetap diutamakan.

// handleGetDefaultQuota GET /api/admin/quota-default.
func (a *App) handleGetDefaultQuota(w http.ResponseWriter, r *http.Request) {
	var users int64
	_ = a.st.DB.QueryRow(`SELECT COUNT(DISTINCT scope_id) FROM quotas WHERE scope = 'user' AND source = 'default'`).Scan(&users)
	var custom int64
	_ = a.st.DB.QueryRow(`SELECT COUNT(DISTINCT q.scope_id) FROM quotas q JOIN users u ON u.id = q.scope_id
		WHERE q.scope = 'user' AND q.source = 'custom' AND u.role IN ('member','viewer')`).Scan(&custom)
	writeJSON(w, 200, map[string]any{"default": a.st.GetDefaultQuota(), "users_default": users, "users_custom": custom})
}

// handlePutDefaultQuota PUT /api/admin/quota-default — {day:{...}, month:{...}}.
func (a *App) handlePutDefaultQuota(w http.ResponseWriter, r *http.Request) {
	var req store.DefaultQuota
	if err := decodeBody(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body tidak sah"})
		return
	}
	for p, l := range req {
		if p != "day" && p != "month" {
			writeJSON(w, 400, map[string]string{"error": "periode harus day atau month"})
			return
		}
		if l.TokenLimit < 0 || l.RequestLimit < 0 || l.CostLimitUSD < 0 {
			writeJSON(w, 400, map[string]string{"error": "batas tidak boleh negatif"})
			return
		}
	}
	before := a.st.GetDefaultQuota()
	n, err := a.st.SetDefaultQuota(req)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	usage.ScheduleMissingResets(a.st, a.cfg.TZ, time.Now()) // baris baru langsung terjadwal
	a.audit(r, "quota.default", "quota-default", before, a.st.GetDefaultQuota())
	writeJSON(w, 200, map[string]any{"default": a.st.GetDefaultQuota(), "users_default": n})
}

// handleGetUserQuota GET /api/admin/users/{id}/quota — kuota user (dengan sumber).
func (a *App) handleGetUserQuota(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, ok := a.manageable(w, r, id); !ok {
		return
	}
	qs, err := a.st.ListQuotasByScope("user", id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if qs == nil {
		qs = []*store.Quota{}
	}
	writeJSON(w, 200, map[string]any{"quotas": qs, "default": a.st.GetDefaultQuota()})
}

// handleUseDefaultQuota DELETE /api/admin/users/{id}/quota?period=day|month —
// hapus kuota khusus periode itu; user kembali memakai kuota default.
func (a *App) handleUseDefaultQuota(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	u, ok := a.manageable(w, r, id)
	if !ok {
		return
	}
	period := r.URL.Query().Get("period")
	if period != "day" && period != "month" {
		writeJSON(w, 400, map[string]string{"error": "period harus day atau month"})
		return
	}
	if err := a.st.UseDefaultQuota(id, u.Role, period); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	usage.ScheduleMissingResets(a.st, a.cfg.TZ, time.Now())
	a.audit(r, "quota.use_default", "users/"+fmtInt(id), nil, map[string]string{"period": period})
	writeJSON(w, 200, map[string]string{"ok": "true"})
}
