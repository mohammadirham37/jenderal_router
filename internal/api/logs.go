package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// queryLogs mengambil log request dengan filter (FR-5.1) sebagai peta baris.
// Isi prompt tidak pernah disertakan (NFR-14).
func (a *App) queryLogs(from, to, user, providerName, status string, limit int) ([]map[string]string, error) {
	where := []string{"ts >= ?", "ts < ?"}
	args := []any{from, to}
	if user != "" {
		where = append(where, "user_id = ?")
		args = append(args, user)
	}
	if providerName != "" {
		where = append(where, "provider_name = ?")
		args = append(args, providerName)
	}
	if status != "" {
		if strings.HasSuffix(status, "xx") && len(status) == 3 {
			// pola "4xx"/"5xx"
			prefix := status[:1]
			where = append(where, "substr(CAST(status AS TEXT),1,1) = ?")
			args = append(args, prefix)
		} else {
			where = append(where, "CAST(status AS TEXT) = ?")
			args = append(args, status)
		}
	}
	query := `SELECT id, ts, COALESCE(CAST(user_id AS TEXT),''), COALESCE(CAST(key_id AS TEXT),''),
		requested_model, provider_name, model_name, CAST(status AS TEXT), error_code,
		CAST(latency_ms AS TEXT), CAST(ttft_ms AS TEXT), CAST(tokens_in AS TEXT), CAST(tokens_out AS TEXT),
		CAST(cost_usd AS TEXT), CAST(attempts AS TEXT)
		FROM request_logs WHERE ` + strings.Join(where, " AND ") + ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := a.st.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := []string{"id", "ts", "user_id", "key_id", "requested_model", "provider_name", "model_name",
		"status", "error_code", "latency_ms", "ttft_ms", "tokens_in", "tokens_out", "cost_usd", "attempts"}
	out := []map[string]string{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := map[string]string{}
		for i, c := range cols {
			if v, ok := vals[i].([]byte); ok {
				m[c] = string(v)
			} else if vals[i] != nil {
				m[c] = stringOf(vals[i])
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func stringOf(v any) string {
	type stringer interface{ String() string }
	if s, ok := v.(stringer); ok {
		return s.String()
	}
	b, err := marshalJSON(v)
	if err != nil {
		return ""
	}
	return trimQuotes(string(b))
}

func marshalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

func trimQuotes(s string) string {
	s = strings.TrimPrefix(s, `"`)
	return strings.TrimSuffix(s, `"`)
}

var _ = time.Now
var _ = http.MethodGet
