package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/jenderal/jenderalrouter/internal/apigate"
	"github.com/jenderal/jenderalrouter/internal/store"
)

// Version versi aplikasi; diisi saat build via ldflags.
var Version = "0.1.0-dev"

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fmtInt(n int64) string { return strconv.FormatInt(n, 10) }

// ---- konteks auth dashboard (diset requireSession) ----

type authInfoKeyType int

const authInfoKey authInfoKeyType = 1

type authInfo struct {
	user *store.User
	sess *store.Session
}

func withAuthValue(r *http.Request, ai *authInfo) context.Context {
	return context.WithValue(r.Context(), authInfoKey, ai)
}

// authFrom mengambil info auth dari request dashboard.
func authFrom(r *http.Request) *authInfo {
	ai, _ := r.Context().Value(authInfoKey).(*authInfo)
	return ai
}

// apigateClientIP alias ke utilitas apigate.
func apigateClientIP(r *http.Request) string {
	return apigate.ClientIP(r)
}

// decodeBody mem-parsing body JSON dengan batas ukuran.
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) error {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(dst)
}
