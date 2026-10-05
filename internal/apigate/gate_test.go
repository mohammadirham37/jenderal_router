package apigate

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jenderal/jenderalrouter/internal/crypto"
	"github.com/jenderal/jenderalrouter/internal/store"
)

func gateTestEnv(t *testing.T) (*store.Store, *Gate, string, *store.User) {
	t.Helper()
	store.SetMasterKey(bytes.Repeat([]byte{7}, 32))
	s, err := store.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	user, err := s.CreateUser("dev@local", "Sandi-Kuat-123", store.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	plain, hash, _ := crypto.NewAPIKey()
	_, err = s.CreateAPIKey(user.ID, "kunci", plain, hash, "", "*", "", 0, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	return s, NewGate(s), plain, user
}

func reqWithKey(key string) *http.Request {
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Bearer "+key)
	r.RemoteAddr = "203.0.113.10:5555"
	return r
}

func TestAuthenticateValid(t *testing.T) {
	s, g, key, user := gateTestEnv(t)
	ac, apiErr := g.Authenticate(reqWithKey(key))
	if apiErr != nil {
		t.Fatalf("auth: %v", apiErr)
	}
	if ac.User.ID != user.ID {
		t.Errorf("user id = %d", ac.User.ID)
	}
	_ = s
}

func TestAuthenticateInvalidKey(t *testing.T) {
	_, g, _, _ := gateTestEnv(t)
	cases := []struct {
		name   string
		header map[string]string
	}{
		{"tanpa key", nil},
		{"bukan jr-", map[string]string{"Authorization": "Bearer sk-openai"}},
		{"key palsu", map[string]string{"Authorization": "Bearer jr-zzz"}},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		for k, v := range c.header {
			r.Header.Set(k, v)
		}
		if _, apiErr := g.Authenticate(r); apiErr == nil || apiErr.Code != "invalid_api_key" {
			t.Errorf("%s: harus invalid_api_key, dapat %v", c.name, apiErr)
		}
	}
}

func TestAuthenticateXAPIKeyHeader(t *testing.T) {
	_, g, key, _ := gateTestEnv(t)
	r := httptest.NewRequest("POST", "/v1/messages", nil)
	r.Header.Set("x-api-key", key)
	ac, apiErr := g.Authenticate(r)
	if apiErr != nil || ac == nil {
		t.Fatalf("x-api-key harus didukung: %v", apiErr)
	}
}

func TestAuthenticateRevokedAndExpired(t *testing.T) {
	s, g, _, user := gateTestEnv(t)
	plain, hash, _ := crypto.NewAPIKey()
	k, _ := s.CreateAPIKey(user.ID, "sementara", plain, hash, "", "*", "", 0, 0, "")
	if err := s.RevokeAPIKey(k.ID); err != nil {
		t.Fatal(err)
	}
	if _, apiErr := g.Authenticate(reqWithKey(plain)); apiErr == nil || apiErr.Code != "invalid_api_key" {
		t.Fatalf("revoked harus ditolak: %v", apiErr)
	}

	plain2, hash2, _ := crypto.NewAPIKey()
	exp := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	s.CreateAPIKey(user.ID, "kedaluwarsa", plain2, hash2, "", "*", "", 0, 0, exp)
	if _, apiErr := g.Authenticate(reqWithKey(plain2)); apiErr == nil {
		t.Fatal("kedaluwarsa harus ditolak")
	}
}

func TestAuthenticateDisabledUser(t *testing.T) {
	s, g, key, user := gateTestEnv(t)
	status := "disabled"
	s.UpdateUserFields(user.ID, nil, nil, &status, nil, nil)
	if _, apiErr := g.Authenticate(reqWithKey(key)); apiErr == nil {
		t.Fatal("user disabled harus ditolak")
	}
}

func TestIPAllowlist(t *testing.T) {
	s, g, _, user := gateTestEnv(t)
	plain, hash, _ := crypto.NewAPIKey()
	s.CreateAPIKey(user.ID, "dibatasi", plain, hash, "", "*", "10.0.0.0/8,192.168.1.1", 0, 0, "")
	r := reqWithKey(plain)
	r.RemoteAddr = "203.0.113.99:1234"
	if _, apiErr := g.Authenticate(r); apiErr == nil || apiErr.Code != "ip_not_allowed" {
		t.Fatalf("IP luar allowlist harus ditolak: %v", apiErr)
	}
	r2 := reqWithKey(plain)
	r2.RemoteAddr = "10.1.2.3:9"
	if _, apiErr := g.Authenticate(r2); apiErr != nil {
		t.Fatalf("IP dalam allowlist harus lolos: %v", apiErr)
	}
}

func TestModelAllowed(t *testing.T) {
	s, g, _, user := gateTestEnv(t)
	plain, hash, _ := crypto.NewAPIKey()
	s.CreateAPIKey(user.ID, "terbatas", plain, hash, "", "oa/gpt-5.4,coding-hemat", "", 0, 0, "")
	ac, apiErr := g.Authenticate(reqWithKey(plain))
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	if !ac.ModelAllowed("oa/gpt-5.4") || !ac.ModelAllowed("coding-hemat") {
		t.Error("model dalam daftar harus diizinkan")
	}
	if ac.ModelAllowed("local/qwen3-8b") {
		t.Error("model luar daftar harus ditolak")
	}
}

func TestRateLimitRPM(t *testing.T) {
	_, g, _, user := gateTestEnv(t)
	plain, hash, _ := crypto.NewAPIKey()
	if _, err := g.Store.CreateAPIKey(user.ID, "rpm", plain, hash, "", "*", "", 3, 0, ""); err != nil {
		t.Fatal(err)
	}
	var lastErr *APIError
	for i := 0; i < 5; i++ {
		if apiErr := g.CheckRateLimit(1, 3); apiErr != nil {
			lastErr = apiErr
			break
		}
	}
	if lastErr == nil || lastErr.Code != "rate_limited" || lastErr.RetryAfter <= 0 {
		t.Fatalf("harus rate_limited: %v", lastErr)
	}
}

func TestRecordTPM(t *testing.T) {
	_, g, _, _ := gateTestEnv(t)
	if apiErr := g.RecordTPM(2, 1000, 800); apiErr != nil {
		t.Fatalf("800 token harus lolos: %v", apiErr)
	}
	if apiErr := g.RecordTPM(2, 1000, 300); apiErr == nil {
		t.Fatal("1100 token harus melebihi TPM")
	}
}

func TestCacheInvalidation(t *testing.T) {
	s, g, key, _ := gateTestEnv(t)
	if _, apiErr := g.Authenticate(reqWithKey(key)); apiErr != nil {
		t.Fatal(apiErr)
	}
	// revocation harus segera berlaku tanpa menunggu TTL cache
	keys, _ := s.ListKeysByUser(1)
	for _, k := range keys {
		if k.KeyHash == crypto.HashToken(key) {
			if err := s.RevokeAPIKey(k.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	hash := crypto.HashToken(key)
	g.InvalidateKey(hash)
	if _, apiErr := g.Authenticate(reqWithKey(key)); apiErr == nil {
		t.Fatal("setelah invalidasi + revoke harus ditolak")
	}
}

func TestClientIPForwarded(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.2")
	if got := ClientIP(r); got != "198.51.100.7" {
		t.Errorf("ClientIP = %s", got)
	}
}

func TestModelAllowedWildcard(t *testing.T) {
	ac := &AuthContext{Key: &store.APIKey{AllowedModels: "*"}}
	if !ac.ModelAllowed("apa/saja") {
		t.Error("wildcard harus mengizinkan semua")
	}
}
