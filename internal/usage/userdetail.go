package usage

import (
	"time"

	"github.com/jenderal/jenderalrouter/internal/store"
)

// UserDetail rincian pemakaian satu user untuk halaman detail admin.
type UserDetail struct {
	From      string       `json:"from"`
	To        string       `json:"to"`
	Totals    Summary      `json:"totals"`
	PerDay    []Summary    `json:"per_day"`
	PerModel  []ModelUsage `json:"per_model"`
	PerKey    []KeyUsage   `json:"per_key"`
	ErrorRate float64      `json:"error_rate"`
}

// ModelUsage agregat per model yang diminta user.
type ModelUsage struct {
	Model        string  `json:"model"`
	ProviderName string  `json:"provider_name"`
	Requests     int64   `json:"requests"`
	Errors       int64   `json:"errors"`
	TokensIn     int64   `json:"tokens_in"`
	TokensOut    int64   `json:"tokens_out"`
	CostUSD      float64 `json:"cost_usd"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
}

// KeyUsage agregat per API key milik user.
type KeyUsage struct {
	KeyID     int64   `json:"key_id"`
	Name      string  `json:"name"`
	Prefix    string  `json:"prefix"`
	Requests  int64   `json:"requests"`
	Errors    int64   `json:"errors"`
	TokensIn  int64   `json:"tokens_in"`
	TokensOut int64   `json:"tokens_out"`
	CostUSD   float64 `json:"cost_usd"`
	LastUsed  string  `json:"last_used"`
}

// GetUserDetail mengagregasi request_logs milik satu user pada rentang [from, to).
func GetUserDetail(st *store.Store, userID int64, from, to time.Time) (*UserDetail, error) {
	f, t := from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano)
	res := &UserDetail{
		From: from.UTC().Format("2006-01-02"), To: to.UTC().Format("2006-01-02"),
		PerDay: []Summary{}, PerModel: []ModelUsage{}, PerKey: []KeyUsage{},
	}

	// per hari
	rows, err := st.DB.Query(`SELECT substr(ts,1,10) d,
		COUNT(*), SUM(CASE WHEN status >= 400 OR status = 0 THEN 1 ELSE 0 END),
		COALESCE(SUM(tokens_in),0), COALESCE(SUM(tokens_out),0), COALESCE(SUM(cost_usd),0),
		COALESCE(AVG(latency_ms),0)
		FROM request_logs WHERE user_id = ? AND ts >= ? AND ts < ? GROUP BY d ORDER BY d`,
		userID, f, t)
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

	// per model
	mrows, err := st.DB.Query(`SELECT requested_model, COALESCE(MAX(provider_name),''), COUNT(*),
		SUM(CASE WHEN status >= 400 OR status = 0 THEN 1 ELSE 0 END),
		COALESCE(SUM(tokens_in),0), COALESCE(SUM(tokens_out),0), COALESCE(SUM(cost_usd),0),
		COALESCE(AVG(latency_ms),0)
		FROM request_logs WHERE user_id = ? AND ts >= ? AND ts < ?
		GROUP BY requested_model ORDER BY COUNT(*) DESC`, userID, f, t)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var m ModelUsage
		if err := mrows.Scan(&m.Model, &m.ProviderName, &m.Requests, &m.Errors, &m.TokensIn, &m.TokensOut, &m.CostUSD, &m.AvgLatencyMs); err != nil {
			return nil, err
		}
		res.PerModel = append(res.PerModel, m)
	}
	if err := mrows.Err(); err != nil {
		return nil, err
	}

	// per API key (key terhapus tetap tampil dengan nama kosong)
	krows, err := st.DB.Query(`SELECT COALESCE(l.key_id,0), COALESCE(k.name,''), COALESCE(k.prefix,''), COUNT(*),
		SUM(CASE WHEN l.status >= 400 OR l.status = 0 THEN 1 ELSE 0 END),
		COALESCE(SUM(l.tokens_in),0), COALESCE(SUM(l.tokens_out),0), COALESCE(SUM(l.cost_usd),0),
		COALESCE(MAX(l.ts),'')
		FROM request_logs l LEFT JOIN api_keys k ON k.id = l.key_id
		WHERE l.user_id = ? AND l.ts >= ? AND l.ts < ?
		GROUP BY l.key_id ORDER BY COUNT(*) DESC`, userID, f, t)
	if err != nil {
		return nil, err
	}
	defer krows.Close()
	for krows.Next() {
		var k KeyUsage
		if err := krows.Scan(&k.KeyID, &k.Name, &k.Prefix, &k.Requests, &k.Errors, &k.TokensIn, &k.TokensOut, &k.CostUSD, &k.LastUsed); err != nil {
			return nil, err
		}
		res.PerKey = append(res.PerKey, k)
	}
	if err := krows.Err(); err != nil {
		return nil, err
	}

	// persentil latensi
	lrows, err := st.DB.Query(`SELECT latency_ms FROM request_logs
		WHERE user_id = ? AND ts >= ? AND ts < ? ORDER BY latency_ms`, userID, f, t)
	if err != nil {
		return nil, err
	}
	defer lrows.Close()
	var lat []int64
	for lrows.Next() {
		var v int64
		if err := lrows.Scan(&v); err != nil {
			return nil, err
		}
		lat = append(lat, v)
	}
	if err := lrows.Err(); err != nil {
		return nil, err
	}
	res.Totals.P50LatencyMs = percentile(lat, 0.50)
	res.Totals.P95LatencyMs = percentile(lat, 0.95)
	if res.Totals.Requests > 0 {
		res.ErrorRate = float64(res.Totals.Errors) / float64(res.Totals.Requests)
	}
	return res, nil
}
