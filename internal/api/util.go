package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jenderal/jenderalrouter/internal/crypto"
	"github.com/jenderal/jenderalrouter/internal/store"
)

// jsonUnmarshalBytes alias kecil agar admin.go ringkas.
func jsonUnmarshalBytes(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// contextWithTimeout util timeout.
func contextWithTimeout(base context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if base == nil {
		base = context.Background()
	}
	return context.WithTimeout(base, d)
}

// contextWithTimeoutCLI untuk perintah CLI.
func contextWithTimeoutCLI(d time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	_ = cancel // proses CLI diakhiri CommandContext; cancel diklaim di sini
	return ctx
}

// newAPIKey delegasi generator key (jr- + hash).
func newAPIKey() (plain, hash string, err error) {
	return crypto.NewAPIKey()
}

// cryptoStrength delegasi pemeriksaan kekuatan password.
func cryptoStrength(pw string) error {
	return crypto.PasswordStrength(pw)
}

// hashToken delegasi.
func hashToken(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

var _ = store.RoleAdmin
var _ = http.MethodGet
