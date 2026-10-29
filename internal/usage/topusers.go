package usage

import (
	"time"

	"github.com/jenderal/jenderalrouter/internal/store"
)

// TopUsers peringkat user paling aktif + daftar model untuk filter.
type TopUsers struct {
	Models []ModelCount `json:"models"`
	Users  []TopUser    `json:"users"`
}

// ModelCount pemakaian satu model pada rentang (tanpa filter).
type ModelCount struct {
	Model    string `json:"model"`
	Requests int64  `json:"requests"`
	Users    int64  `json:"users"`
}

// TopUser agregat satu user. Chat = request dari dashboard (tanpa API key),
// API = request lewat API key.
type TopUser struct {
	UserID       int64   `json:"user_id"`
	Email        string  `json:"email"`
	Requests     int64   `json:"requests"`
	ChatRequests int64   `json:"chat_requests"`
	APIRequests  int64   `json:"api_requests"`
	TokensIn     int64   `json:"tokens_in"`
	TokensOut    int64   `json:"tokens_out"`
	CostUSD      float64 `json:"cost_usd"`
	TopModel     string  `json:"top_model"`
	LastUsed     string  `json:"last_used"`
}

// GetTopUsers mengurutkan user menurut jumlah request pada [from, to).
// model kosong = semua model; daftar Models selalu tanpa filter.
func GetTopUsers(st *store.Store, from, to time.Time, model string, limit int) (*TopUsers, error) {
	f, t := from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano)
	res := &TopUsers{Models: []ModelCount{}, Users: []TopUser{}}

	mrows, err := st.DB.Query(`SELECT requested_model, COUNT(*), COUNT(DISTINCT user_id)
		FROM request_logs WHERE user_id IS NOT NULL AND ts >= ? AND ts < ?
		GROUP BY requested_model ORDER BY COUNT(*) DESC, requested_model`, f, t)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var m ModelCount
		if err := mrows.Scan(&m.Model, &m.Requests, &m.Users); err != nil {
			return nil, err
		}
		res.Models = append(res.Models, m)
	}
	if err := mrows.Err(); err != nil {
		return nil, err
	}

	// filter model berlaku di agregat utama dan subquery model favorit
	modelCond, args := "", []any{f, t}
	if model != "" {
		modelCond = " AND requested_model = ?"
		args = append(args, model)
	}
	sub := append([]any{}, args...)
	urows, err := st.DB.Query(`SELECT l.user_id, COALESCE(u.email,'-'), COUNT(*),
		SUM(CASE WHEN l.key_id IS NULL THEN 1 ELSE 0 END),
		SUM(CASE WHEN l.key_id IS NOT NULL THEN 1 ELSE 0 END),
		COALESCE(SUM(l.tokens_in),0), COALESCE(SUM(l.tokens_out),0), COALESCE(SUM(l.cost_usd),0),
		COALESCE((SELECT requested_model FROM request_logs m
			WHERE m.user_id = l.user_id AND m.ts >= ? AND m.ts < ?`+modelCond+`
			GROUP BY requested_model ORDER BY COUNT(*) DESC LIMIT 1),''),
		MAX(l.ts)
		FROM request_logs l LEFT JOIN users u ON u.id = l.user_id
		WHERE l.user_id IS NOT NULL AND l.ts >= ? AND l.ts < ?`+modelCond+`
		GROUP BY l.user_id ORDER BY COUNT(*) DESC, MAX(l.ts) DESC LIMIT ?`,
		append(append(sub, args...), limit)...)
	if err != nil {
		return nil, err
	}
	defer urows.Close()
	for urows.Next() {
		var u TopUser
		if err := urows.Scan(&u.UserID, &u.Email, &u.Requests, &u.ChatRequests, &u.APIRequests,
			&u.TokensIn, &u.TokensOut, &u.CostUSD, &u.TopModel, &u.LastUsed); err != nil {
			return nil, err
		}
		res.Users = append(res.Users, u)
	}
	return res, urows.Err()
}
