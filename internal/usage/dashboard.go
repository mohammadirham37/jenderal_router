package usage

import (
	"sort"
	"time"

	"github.com/jenderal/jenderalrouter/internal/store"
)

// Agregasi untuk halaman Ringkasan (dashboard): total periode + pembanding,
// deret waktu terisi nol, model & provider teratas, error terbaru, dan
// user yang kuotanya hampir habis.

// PeriodTotals total satu rentang waktu.
type PeriodTotals struct {
	Requests     int64   `json:"requests"`
	Errors       int64   `json:"errors"`
	TokensIn     int64   `json:"tokens_in"`
	TokensOut    int64   `json:"tokens_out"`
	CostUSD      float64 `json:"cost_usd"`
	ActiveUsers  int64   `json:"active_users"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	P95LatencyMs int64   `json:"p95_latency_ms"`
}

func rfc(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// GetPeriodTotals total request/error/token/biaya/user aktif pada [from, to).
func GetPeriodTotals(st *store.Store, from, to time.Time) (PeriodTotals, error) {
	var p PeriodTotals
	err := st.DB.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status >= 400 OR status = 0 THEN 1 ELSE 0 END),0),
		COALESCE(SUM(tokens_in),0), COALESCE(SUM(tokens_out),0), COALESCE(SUM(cost_usd),0),
		COUNT(DISTINCT user_id), COALESCE(AVG(latency_ms),0)
		FROM request_logs WHERE ts >= ? AND ts < ?`, rfc(from), rfc(to)).
		Scan(&p.Requests, &p.Errors, &p.TokensIn, &p.TokensOut, &p.CostUSD, &p.ActiveUsers, &p.AvgLatencyMs)
	if err != nil {
		return p, err
	}
	lat, err := latencies(st, from, to)
	if err == nil && len(lat) > 0 {
		p.P95LatencyMs = percentile(lat, 0.95)
	}
	return p, nil
}

// Bucket satu titik deret waktu.
type Bucket struct {
	T        string  `json:"t"` // "2026-11-01" (harian) atau "2026-11-01 14:00" (per jam)
	Requests int64   `json:"requests"`
	Errors   int64   `json:"errors"`
	Tokens   int64   `json:"tokens"`
	CostUSD  float64 `json:"cost_usd"`
	Chat     int64   `json:"chat"` // request tanpa API key (chat dashboard)
	API      int64   `json:"api"`
}

// GetSeries deret waktu [from, to) per jam atau per hari (zona analitik),
// dengan bucket kosong diisi nol. userID > 0 membatasi ke satu user.
func GetSeries(st *store.Store, from, to time.Time, hourly bool, userID int64) ([]Bucket, error) {
	format := "%Y-%m-%d"
	if hourly {
		format = "%Y-%m-%d %H:00"
	}
	q := `SELECT strftime('` + format + `', ts, ?) b, COUNT(*),
		SUM(CASE WHEN status >= 400 OR status = 0 THEN 1 ELSE 0 END),
		COALESCE(SUM(tokens_in + tokens_out),0), COALESCE(SUM(cost_usd),0),
		SUM(CASE WHEN key_id IS NULL THEN 1 ELSE 0 END)
		FROM request_logs WHERE ts >= ? AND ts < ?`
	args := []any{dayModifier(), rfc(from), rfc(to)}
	if userID > 0 {
		q += ` AND user_id = ?`
		args = append(args, userID)
	}
	rows, err := st.DB.Query(q+` GROUP BY b`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	got := map[string]Bucket{}
	for rows.Next() {
		var b Bucket
		if err := rows.Scan(&b.T, &b.Requests, &b.Errors, &b.Tokens, &b.CostUSD, &b.Chat); err != nil {
			return nil, err
		}
		b.API = b.Requests - b.Chat
		got[b.T] = b
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// isi bucket kosong agar grafik tidak bolong
	var out []Bucket
	step := 24 * time.Hour
	layout := "2006-01-02"
	cur := StartOfToday(from)
	if hourly {
		step, layout = time.Hour, "2006-01-02 15:00"
		f := from.In(loc)
		cur = time.Date(f.Year(), f.Month(), f.Day(), f.Hour(), 0, 0, 0, loc)
	}
	for ; cur.Before(to); cur = cur.Add(step) {
		k := cur.In(loc).Format(layout)
		b := got[k]
		b.T = k
		out = append(out, b)
	}
	return out, nil
}

// ModelStat pemakaian per model yang diminta.
type ModelStat struct {
	Model        string  `json:"model"`
	Requests     int64   `json:"requests"`
	Errors       int64   `json:"errors"`
	Tokens       int64   `json:"tokens"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
}

// GetTopModels model paling banyak diminta pada [from, to).
func GetTopModels(st *store.Store, from, to time.Time, limit int) ([]ModelStat, error) {
	rows, err := st.DB.Query(`SELECT requested_model, COUNT(*),
		SUM(CASE WHEN status >= 400 OR status = 0 THEN 1 ELSE 0 END),
		COALESCE(SUM(tokens_in + tokens_out),0), COALESCE(AVG(latency_ms),0)
		FROM request_logs WHERE ts >= ? AND ts < ? AND requested_model != ''
		GROUP BY requested_model ORDER BY COUNT(*) DESC LIMIT ?`, rfc(from), rfc(to), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ModelStat{}
	for rows.Next() {
		var m ModelStat
		if err := rows.Scan(&m.Model, &m.Requests, &m.Errors, &m.Tokens, &m.AvgLatencyMs); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ProviderWindow request & error satu provider pada sebuah rentang.
type ProviderWindow struct {
	Requests    int64   `json:"requests"`
	Errors      int64   `json:"errors"`
	AvgLatency  float64 `json:"avg_latency_ms"`
	LastError   string  `json:"last_error,omitempty"`
	LastErrorAt string  `json:"last_error_at,omitempty"`
}

// GetProviderWindows statistik per provider_id pada [from, to).
func GetProviderWindows(st *store.Store, from, to time.Time) (map[int64]*ProviderWindow, error) {
	rows, err := st.DB.Query(`SELECT provider_id, COUNT(*),
		SUM(CASE WHEN status >= 400 OR status = 0 THEN 1 ELSE 0 END), COALESCE(AVG(latency_ms),0)
		FROM request_logs WHERE ts >= ? AND ts < ? AND provider_id IS NOT NULL GROUP BY provider_id`, rfc(from), rfc(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]*ProviderWindow{}
	for rows.Next() {
		var id int64
		w := &ProviderWindow{}
		if err := rows.Scan(&id, &w.Requests, &w.Errors, &w.AvgLatency); err != nil {
			return nil, err
		}
		out[id] = w
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	erows, err := st.DB.Query(`SELECT provider_id, error_code, CAST(status AS TEXT), ts FROM request_logs
		WHERE id IN (SELECT MAX(id) FROM request_logs WHERE ts >= ? AND ts < ? AND provider_id IS NOT NULL
			AND (status >= 400 OR status = 0) GROUP BY provider_id)`, rfc(from), rfc(to))
	if err != nil {
		return out, nil
	}
	defer erows.Close()
	for erows.Next() {
		var id int64
		var code, status, ts string
		if erows.Scan(&id, &code, &status, &ts) == nil && out[id] != nil {
			if code == "" {
				code = "HTTP " + status
			}
			out[id].LastError, out[id].LastErrorAt = code, ts
		}
	}
	return out, erows.Err()
}

// ErrorRow satu request gagal.
type ErrorRow struct {
	ID        int64  `json:"id"`
	TS        string `json:"ts"`
	Email     string `json:"email"`
	Model     string `json:"model"`
	Provider  string `json:"provider"`
	Status    int    `json:"status"`
	ErrorCode string `json:"error_code"`
	LatencyMs int64  `json:"latency_ms"`
	Via       string `json:"via"` // chat|api
}

// GetRecentErrors request gagal terbaru pada [from, to).
func GetRecentErrors(st *store.Store, from, to time.Time, limit int) ([]ErrorRow, error) {
	rows, err := st.DB.Query(`SELECT l.id, l.ts, COALESCE(u.email,'-'), l.requested_model, l.provider_name,
		l.status, l.error_code, l.latency_ms, CASE WHEN l.key_id IS NULL THEN 'chat' ELSE 'api' END
		FROM request_logs l LEFT JOIN users u ON u.id = l.user_id
		WHERE l.ts >= ? AND l.ts < ? AND (l.status >= 400 OR l.status = 0)
		ORDER BY l.id DESC LIMIT ?`, rfc(from), rfc(to), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ErrorRow{}
	for rows.Next() {
		var e ErrorRow
		if err := rows.Scan(&e.ID, &e.TS, &e.Email, &e.Model, &e.Provider, &e.Status, &e.ErrorCode, &e.LatencyMs, &e.Via); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// QuotaAlert user yang pemakaian kuotanya mendekati/melewati batas.
type QuotaAlert struct {
	UserID int64   `json:"user_id"`
	Email  string  `json:"email"`
	Period string  `json:"period"`
	Metric string  `json:"metric"` // token|request|biaya
	Pct    float64 `json:"pct"`
}

// GetQuotaAlerts kuota user dengan pemakaian ≥ threshold (0..1), terberat dulu.
func GetQuotaAlerts(st *store.Store, threshold float64, limit int) ([]QuotaAlert, error) {
	rows, err := st.DB.Query(`SELECT q.scope_id, u.email, q.period, q.token_limit, q.used_tokens,
		q.request_limit, q.used_requests, q.cost_limit_usd, q.used_cost
		FROM quotas q JOIN users u ON u.id = q.scope_id WHERE q.scope = 'user'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []QuotaAlert{}
	for rows.Next() {
		var a QuotaAlert
		var tl, tu, rl, ru int64
		var cl, cu float64
		if err := rows.Scan(&a.UserID, &a.Email, &a.Period, &tl, &tu, &rl, &ru, &cl, &cu); err != nil {
			return nil, err
		}
		best := 0.0
		for _, m := range []struct {
			name      string
			used, lim float64
		}{{"token", float64(tu), float64(tl)}, {"request", float64(ru), float64(rl)}, {"biaya", cu, cl}} {
			if m.lim > 0 && m.used/m.lim > best {
				best, a.Metric = m.used/m.lim, m.name
			}
		}
		if best >= threshold {
			a.Pct = best * 100
			out = append(out, a)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pct > out[j].Pct })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
