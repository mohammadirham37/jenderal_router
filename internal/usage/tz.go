package usage

import (
	"fmt"
	"time"

	"github.com/jenderal/jenderalrouter/internal/store"
)

// loc zona waktu pengelompokan harian analitik (JR_TZ); default UTC.
var loc = time.UTC

// SetLocation mengatur zona waktu "hari" analitik agar sejalan dengan reset
// kuota (JR_TZ) — tanpa ini request 00:00–07:00 WIB jatuh ke hari kemarin.
func SetLocation(l *time.Location) {
	if l != nil {
		loc = l
	}
}

// dayModifier modifier SQLite date(ts, ?) untuk menggeser UTC ke zona lokal.
func dayModifier() string {
	_, off := time.Now().In(loc).Zone()
	return fmt.Sprintf("%+d seconds", off)
}

// StartOfToday awal hari ini menurut zona analitik.
func StartOfToday(now time.Time) time.Time {
	n := now.In(loc)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
}

// UserTotals agregat ringan satu user pada [from, to) — untuk /usage/me,
// tanpa menghitung analitik seluruh user.
func UserTotals(st *store.Store, userID int64, from, to time.Time) (UserUsage, error) {
	u := UserUsage{UserID: userID}
	err := st.DB.QueryRow(`SELECT COUNT(*), COALESCE(SUM(tokens_in),0), COALESCE(SUM(tokens_out),0),
		COALESCE(SUM(cost_usd),0) FROM request_logs WHERE user_id = ? AND ts >= ? AND ts < ?`,
		userID, from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano)).
		Scan(&u.Requests, &u.TokensIn, &u.TokensOut, &u.CostUSD)
	return u, err
}

// PurgeLogs menghapus request_logs lebih tua dari batas (retensi).
func PurgeLogs(st *store.Store, before time.Time) (int64, error) {
	res, err := st.DB.Exec(`DELETE FROM request_logs WHERE ts < ?`, before.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
