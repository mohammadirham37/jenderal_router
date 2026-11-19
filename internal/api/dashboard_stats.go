package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jenderal/jenderalrouter/internal/usage"
)

// Data halaman Ringkasan dalam satu panggilan: total periode berjalan vs
// periode sebelumnya, deret waktu, model teratas, kesehatan provider,
// error terbaru, dan peringatan yang perlu ditindaklanjuti admin.

var dashboardRanges = map[string]struct {
	span   time.Duration
	hourly bool
}{
	"24h": {24 * time.Hour, true},
	"7d":  {7 * 24 * time.Hour, false},
	"30d": {30 * 24 * time.Hour, false},
	"90d": {90 * 24 * time.Hour, false},
}

type providerHealth struct {
	ID       int64                 `json:"id"`
	Name     string                `json:"name"`
	Type     string                `json:"type"`
	Enabled  bool                  `json:"enabled"`
	Status   string                `json:"status"` // ok|degraded|down|idle|disabled
	Period   *usage.ProviderWindow `json:"period"`
	LastHour *usage.ProviderWindow `json:"last_hour"`
}

type dashboardAlert struct {
	Level string `json:"level"` // err|warn
	Kind  string `json:"kind"`  // provider|quota|errors
	Text  string `json:"text"`
	Link  string `json:"link,omitempty"`
}

// handleAdminDashboard GET /api/admin/dashboard?range=24h|7d|30d|90d.
func (a *App) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	rk := r.URL.Query().Get("range")
	rg, ok := dashboardRanges[rk]
	if !ok {
		rk, rg = "7d", dashboardRanges["7d"]
	}
	now := time.Now()
	to := now.Add(time.Minute)
	from := now.Add(-rg.span)
	fail := func(err error) { writeJSON(w, 500, map[string]string{"error": err.Error()}) }

	cur, err := usage.GetPeriodTotals(a.st, from, to)
	if err != nil {
		fail(err)
		return
	}
	prev, err := usage.GetPeriodTotals(a.st, from.Add(-rg.span), from)
	if err != nil {
		fail(err)
		return
	}
	series, err := usage.GetSeries(a.st, from, to, rg.hourly, 0)
	if err != nil {
		fail(err)
		return
	}
	models, err := usage.GetTopModels(a.st, from, to, 6)
	if err != nil {
		fail(err)
		return
	}
	errs, err := usage.GetRecentErrors(a.st, from, to, 8)
	if err != nil {
		fail(err)
		return
	}

	// kesehatan provider: circuit breaker + error rate 1 jam terakhir
	hourAgo := now.Add(-time.Hour)
	period, _ := usage.GetProviderWindows(a.st, from, to)
	lastHour, _ := usage.GetProviderWindows(a.st, hourAgo, to)
	providers, err := a.st.ListProviders()
	if err != nil {
		fail(err)
		return
	}
	alerts := []dashboardAlert{}
	health := []providerHealth{}
	for _, p := range providers {
		h := providerHealth{ID: p.ID, Name: p.Name, Type: p.Type, Enabled: p.Enabled,
			Period: orEmptyWindow(period[p.ID]), LastHour: orEmptyWindow(lastHour[p.ID])}
		switch {
		case !p.Enabled:
			h.Status = "disabled"
		case a.breakers.For(p.ID).IsOpen():
			h.Status = "down"
			alerts = append(alerts, dashboardAlert{Level: "err", Kind: "provider", Link: "/providers",
				Text: "Provider " + p.Name + " sedang diputus (circuit breaker terbuka) karena gagal beruntun"})
		case h.LastHour.Requests >= 3 && float64(h.LastHour.Errors)/float64(h.LastHour.Requests) >= 0.5:
			h.Status = "degraded"
			alerts = append(alerts, dashboardAlert{Level: "warn", Kind: "provider", Link: "/logs?provider=" + p.Name,
				Text: "Provider " + p.Name + " gagal " + strconv.FormatInt(h.LastHour.Errors, 10) + " dari " +
					strconv.FormatInt(h.LastHour.Requests, 10) + " request dalam 1 jam terakhir"})
		case h.Period.Requests == 0:
			h.Status = "idle"
		default:
			h.Status = "ok"
		}
		health = append(health, h)
	}

	// lonjakan error 1 jam terakhir (seluruh gateway)
	if hour, err := usage.GetPeriodTotals(a.st, hourAgo, to); err == nil && hour.Requests >= 5 &&
		float64(hour.Errors)/float64(hour.Requests) >= 0.25 {
		alerts = append(alerts, dashboardAlert{Level: "warn", Kind: "errors", Link: "/logs?status=5xx",
			Text: strconv.FormatInt(hour.Errors, 10) + " dari " + strconv.FormatInt(hour.Requests, 10) +
				" request gagal dalam 1 jam terakhir"})
	}
	quotaAlerts, _ := usage.GetQuotaAlerts(a.st, 0.9, 6)
	for _, q := range quotaAlerts {
		lvl, verb := "warn", "hampir habis"
		if q.Pct >= 100 {
			lvl, verb = "err", "habis"
		}
		per := "harian"
		if q.Period == "month" {
			per = "bulanan"
		}
		alerts = append(alerts, dashboardAlert{Level: lvl, Kind: "quota", Link: "/users/" + fmtInt(q.UserID),
			Text: "Kuota " + q.Metric + " " + per + " " + q.Email + " " + verb + " (" + strconv.Itoa(int(q.Pct)) + "%)"})
	}

	var usersTotal, keysActive int64
	_ = a.st.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&usersTotal)
	_ = a.st.DB.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE revoked_at = ''`).Scan(&keysActive)

	writeJSON(w, 200, map[string]any{
		"range": rk, "hourly": rg.hourly, "current": cur, "previous": prev, "series": series,
		"top_models": models, "providers": health, "recent_errors": errs, "alerts": alerts,
		"users_total": usersTotal, "keys_active": keysActive, "generated_at": now.UTC().Format(time.RFC3339),
	})
}

func orEmptyWindow(w *usage.ProviderWindow) *usage.ProviderWindow {
	if w == nil {
		return &usage.ProviderWindow{}
	}
	return w
}

// handleMeActivity GET /api/me/activity?days=7 — deret harian pemakaian
// user sendiri (chat vs API) untuk dashboard member.
func (a *App) handleMeActivity(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 90 {
		days = 7
	}
	now := time.Now()
	from := usage.StartOfToday(now).AddDate(0, 0, -(days - 1))
	series, err := usage.GetSeries(a.st, from, now.Add(time.Minute), false, authFrom(r).user.ID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	var total usage.Bucket
	for _, b := range series {
		total.Requests += b.Requests
		total.Errors += b.Errors
		total.Tokens += b.Tokens
		total.Chat += b.Chat
		total.API += b.API
	}
	writeJSON(w, 200, map[string]any{"days": days, "series": series, "total": total})
}

// handleNavStatus GET /api/admin/nav-status — angka ringan untuk lencana
// sidebar (dipoll tiap menit): provider bermasalah, error 1 jam terakhir,
// dan user yang kuotanya ≥90%.
func (a *App) handleNavStatus(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	hourAgo := now.Add(-time.Hour)
	lastHour, _ := usage.GetProviderWindows(a.st, hourAgo, now.Add(time.Minute))
	providers, err := a.st.ListProviders()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	down, degraded := 0, 0
	for _, p := range providers {
		if !p.Enabled {
			continue
		}
		if a.breakers.For(p.ID).IsOpen() {
			down++
			continue
		}
		if h := lastHour[p.ID]; h != nil && h.Requests >= 3 && float64(h.Errors)/float64(h.Requests) >= 0.5 {
			degraded++
		}
	}
	hour, _ := usage.GetPeriodTotals(a.st, hourAgo, now.Add(time.Minute))
	quota, _ := usage.GetQuotaAlerts(a.st, 0.9, 1000)
	writeJSON(w, 200, map[string]any{
		"providers_down": down, "providers_degraded": degraded,
		"errors_hour": hour.Errors, "quota_alerts": len(quota),
	})
}
