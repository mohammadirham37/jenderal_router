package usage

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jenderal/jenderalrouter/internal/store"
)

// QuotaStatus hasil pemeriksaan kuota (FR-4.4).
type QuotaStatus struct {
	OK         bool
	Reason     string // pesan bila OK=false
	RetryAfter int
}

// QuotaCheck memeriksa semua kuota milik user dan key (scope user & key,
// periode day & month). Kuota tanpa baris = tanpa batas.
func QuotaCheck(st *store.Store, userID, keyID int64) QuotaStatus {
	now := time.Now()
	for _, q := range collectQuotas(st, userID, keyID) {
		// bila reset_at lewat, anggap bersih (worker akan mereset segera)
		if q.ResetAt != "" {
			if t, err := time.Parse(time.RFC3339, q.ResetAt); err == nil && now.After(t) {
				continue
			}
		}
		if q.TokenLimit > 0 && q.UsedTokens >= q.TokenLimit {
			return QuotaStatus{OK: false, RetryAfter: secondsUntilReset(q, now),
				Reason: "kuota token " + q.Period + " habis (" + humanInt(q.UsedTokens) + "/" + humanInt(q.TokenLimit) + ")"}
		}
		if q.RequestLimit > 0 && q.UsedRequests >= q.RequestLimit {
			return QuotaStatus{OK: false, RetryAfter: secondsUntilReset(q, now),
				Reason: "kuota request " + q.Period + " habis (" + humanInt(q.UsedRequests) + "/" + humanInt(q.RequestLimit) + ")"}
		}
		if q.CostLimitUSD > 0 && q.UsedCost >= q.CostLimitUSD {
			return QuotaStatus{OK: false, RetryAfter: secondsUntilReset(q, now),
				Reason: "batas biaya " + q.Period + " terlampaui ($" + humanFloat(q.UsedCost) + "/$" + humanFloat(q.CostLimitUSD) + ")"}
		}
	}
	return QuotaStatus{OK: true}
}

func collectQuotas(st *store.Store, userID, keyID int64) []*store.Quota {
	var out []*store.Quota
	if qs, err := st.ListQuotasByScope("user", userID); err == nil {
		out = append(out, qs...)
	}
	if keyID > 0 {
		if qs, err := st.ListQuotasByScope("key", keyID); err == nil {
			out = append(out, qs...)
		}
	}
	return out
}

func secondsUntilReset(q *store.Quota, now time.Time) int {
	if q.ResetAt == "" {
		return 60
	}
	if t, err := time.Parse(time.RFC3339, q.ResetAt); err == nil {
		if d := int(t.Sub(now).Seconds()); d > 0 {
			return d
		}
	}
	return 60
}

// NextResetAt menghitung waktu reset berikutnya sesuai periode dan zona
// waktu (FR-4.5; default Asia/Jakarta).
func NextResetAt(period, tz string, now time.Time) string {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	local := now.In(loc)
	var next time.Time
	if period == "day" {
		next = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	} else {
		next = time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, 1, 0)
	}
	return next.UTC().Format(time.RFC3339)
}

// ResetWorker memeriksa kuota jatuh tempo tiap menit dan meresetnya.
func ResetWorker(st *store.Store, tz string, stop <-chan struct{}) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			now := time.Now()
			due, err := st.QuotasDueReset(now.UTC().Format(time.RFC3339))
			if err != nil {
				slog.Error("queri kuota jatuh tempo gagal", "err", err)
				continue
			}
			for _, q := range due {
				next := NextResetAt(q.Period, tz, now)
				if err := st.ResetQuota(q.ID, next); err != nil {
					slog.Error("reset kuota gagal", "quota", q.ID, "err", err)
				} else {
					slog.Info("kuota direset", "scope", q.Scope, "scope_id", q.ScopeID, "period", q.Period)
				}
			}
		}
	}
}

// Summary baris ringkasan analitik (FR-5.3).
type Summary struct {
	Day          string  `json:"day"`
	Requests     int64   `json:"requests"`
	Errors       int64   `json:"errors"`
	TokensIn     int64   `json:"tokens_in"`
	TokensOut    int64   `json:"tokens_out"`
	CostUSD      float64 `json:"cost_usd"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	P50LatencyMs int64   `json:"p50_latency_ms"`
	P95LatencyMs int64   `json:"p95_latency_ms"`
}

// AnalyticsSummary ringkasan rentang tanggal.
type AnalyticsSummary struct {
	From        string          `json:"from"`
	To          string          `json:"to"`
	Totals      Summary         `json:"totals"`
	PerDay      []Summary       `json:"per_day"`
	PerProvider []ProviderUsage `json:"per_provider"`
	PerUser     []UserUsage     `json:"per_user"`
	ErrorRate   float64         `json:"error_rate"`
}

// ProviderUsage pemakaian per provider.
type ProviderUsage struct {
	ProviderID   int64   `json:"provider_id"`
	ProviderName string  `json:"provider_name"`
	Requests     int64   `json:"requests"`
	Errors       int64   `json:"errors"`
	TokensIn     int64   `json:"tokens_in"`
	TokensOut    int64   `json:"tokens_out"`
	CostUSD      float64 `json:"cost_usd"`
}

// UserUsage pemakaian per user.
type UserUsage struct {
	UserID    int64   `json:"user_id"`
	Email     string  `json:"email"`
	Requests  int64   `json:"requests"`
	TokensIn  int64   `json:"tokens_in"`
	TokensOut int64   `json:"tokens_out"`
	CostUSD   float64 `json:"cost_usd"`
}

// GetAnalytics mengagregasi request_logs pada rentang [from, to) (FR-5.3).
func GetAnalytics(st *store.Store, from, to time.Time) (*AnalyticsSummary, error) {
	res := &AnalyticsSummary{From: from.UTC().Format("2006-01-02"), To: to.UTC().Format("2006-01-02")}

	// per hari
	rows, err := st.DB.Query(`SELECT substr(ts,1,10) d,
		COUNT(*), SUM(CASE WHEN status >= 400 OR status = 0 THEN 1 ELSE 0 END),
		COALESCE(SUM(tokens_in),0), COALESCE(SUM(tokens_out),0), COALESCE(SUM(cost_usd),0),
		COALESCE(AVG(latency_ms),0)
		FROM request_logs WHERE ts >= ? AND ts < ? GROUP BY d ORDER BY d`,
		from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s Summary
		if err := rows.Scan(&s.Day, &s.Requests, &s.Errors, &s.TokensIn, &s.TokensOut, &s.CostUSD, &s.AvgLatencyMs); err != nil {
			return nil, err
		}
		res.PerDay = append(res.PerDay, s)
		res.Totals.Requests += s.Requests
		res.Totals.Errors += s.Errors
		res.Totals.TokensIn += s.TokensIn
		res.Totals.TokensOut += s.TokensOut
		res.Totals.CostUSD += s.CostUSD
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// per provider
	prows, err := st.DB.Query(`SELECT COALESCE(provider_id,0), COALESCE(provider_name,'-'), COUNT(*),
		SUM(CASE WHEN status >= 400 OR status = 0 THEN 1 ELSE 0 END),
		COALESCE(SUM(tokens_in),0), COALESCE(SUM(tokens_out),0), COALESCE(SUM(cost_usd),0)
		FROM request_logs WHERE ts >= ? AND ts < ? GROUP BY provider_id, provider_name ORDER BY COUNT(*) DESC`,
		from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer prows.Close()
	for prows.Next() {
		var p ProviderUsage
		if err := prows.Scan(&p.ProviderID, &p.ProviderName, &p.Requests, &p.Errors, &p.TokensIn, &p.TokensOut, &p.CostUSD); err != nil {
			return nil, err
		}
		res.PerProvider = append(res.PerProvider, p)
	}

	// per user
	urows, err := st.DB.Query(`SELECT COALESCE(l.user_id,0), COALESCE(u.email,'-'), COUNT(*),
		COALESCE(SUM(l.tokens_in),0), COALESCE(SUM(l.tokens_out),0), COALESCE(SUM(l.cost_usd),0)
		FROM request_logs l LEFT JOIN users u ON u.id = l.user_id
		WHERE l.ts >= ? AND l.ts < ? GROUP BY l.user_id ORDER BY COUNT(*) DESC`,
		from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer urows.Close()
	for urows.Next() {
		var u UserUsage
		if err := urows.Scan(&u.UserID, &u.Email, &u.Requests, &u.TokensIn, &u.TokensOut, &u.CostUSD); err != nil {
			return nil, err
		}
		res.PerUser = append(res.PerUser, u)
	}

	// persentil latensi
	lat, err := latencies(st, from, to)
	if err != nil {
		return nil, err
	}
	res.Totals.P50LatencyMs = percentile(lat, 0.50)
	res.Totals.P95LatencyMs = percentile(lat, 0.95)
	if res.Totals.Requests > 0 {
		res.ErrorRate = float64(res.Totals.Errors) / float64(res.Totals.Requests)
	}
	return res, nil
}

func latencies(st *store.Store, from, to time.Time) ([]int64, error) {
	rows, err := st.DB.Query(`SELECT latency_ms FROM request_logs
		WHERE ts >= ? AND ts < ? ORDER BY latency_ms`,
		from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p * float64(len(sorted)-1))
	return sorted[idx]
}

func humanInt(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func humanFloat(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}
