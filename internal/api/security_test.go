package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jenderal/jenderalrouter/internal/crypto"
	"github.com/jenderal/jenderalrouter/internal/store"
)

// Regresi audit keamanan: admin biasa tidak boleh menyentuh key/kuota milik
// admin atau super_admin (H1/H2).
func TestAdminCannotManageHigherRoleKeysOrQuota(t *testing.T) {
	app, st, _ := appTestEnv(t)
	super, _ := st.GetUserByEmail("admin@test")
	plain, hash, _ := crypto.NewAPIKey()
	enc, _ := st.EncryptSecret(plain)
	k, err := st.CreateAPIKey(super.ID, "punya super", plain, hash, enc, "*", "", 0, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	adm, _ := st.CreateUser("admin2@test", "Kuat-2026-Pass", store.RoleAdmin)
	cookie, csrf := loginAs(app, t, "admin2@test", "Kuat-2026-Pass")
	sid, kid, aid := fmtInt(super.ID), fmtInt(k.ID), fmtInt(adm.ID)

	cases := []struct{ method, path string }{
		{"GET", "/api/admin/keys/" + kid + "/reveal"},
		{"DELETE", "/api/admin/keys/" + kid},
		{"GET", "/api/admin/users/" + sid + "/keys"},
		{"POST", "/api/admin/users/" + sid + "/keys"},
		{"PUT", "/api/admin/users/" + sid + "/quota"},
		{"PUT", "/api/admin/keys/" + kid + "/quota"},
		{"PUT", "/api/admin/users/" + aid + "/quota"}, // kuota diri sendiri
		{"GET", "/api/admin/users/" + sid + "/usage"},
	}
	for _, c := range cases {
		w := dashReq(app, c.method, c.path, map[string]any{"period": "day", "token_limit": 0, "name": "x"}, cookie, csrf)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s = %d, mau 403 (%s)", c.method, c.path, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), plain) {
			t.Fatalf("%s %s membocorkan plaintext key", c.method, c.path)
		}
	}
	// admin tetap boleh mengelola member
	dev, _ := st.GetUserByEmail("dev@test")
	if w := dashReq(app, "GET", "/api/admin/users/"+fmtInt(dev.ID)+"/keys", nil, cookie, ""); w.Code != 200 {
		t.Fatalf("admin kelola key member = %d", w.Code)
	}
}

// H3: super_admin terakhir tidak bisa diturunkan/dinonaktifkan, dan setup
// tidak terbuka lagi walau super_admin hilang.
func TestLastSuperAdminAndSetupOneTime(t *testing.T) {
	app, st, _ := appTestEnv(t)
	cookie, csrf := loginAs(app, t, "admin@test", "Sandi-Kuat-123")
	super, _ := st.GetUserByEmail("admin@test")
	path := "/api/admin/users/" + fmtInt(super.ID)

	for _, body := range []map[string]any{{"role": "member"}, {"status": "disabled"}} {
		if w := dashReq(app, "PATCH", path, body, cookie, csrf); w.Code != http.StatusConflict {
			t.Fatalf("PATCH %v = %d, mau 409", body, w.Code)
		}
	}
	// dengan super_admin kedua, penurunan boleh
	_, _ = st.CreateUser("super2@test", "Kuat-2026-Pass", store.RoleSuperAdmin)
	if w := dashReq(app, "PATCH", path, map[string]any{"role": "admin"}, cookie, csrf); w.Code != 200 {
		t.Fatalf("turunkan dengan super kedua = %d %s", w.Code, w.Body.String())
	}
	// setup tetap tertutup walau super_admin dihapus langsung dari DB
	if _, err := st.DB.Exec(`DELETE FROM users WHERE role = ?`, store.RoleSuperAdmin); err != nil {
		t.Fatal(err)
	}
	w := dashReq(app, "POST", "/api/setup", map[string]string{"email": "evil@x.id", "password": "Kuat-2026-Pass"}, "", "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("setup ulang = %d, mau 403", w.Code)
	}
}

// M2: ganti password mencabut sesi lain milik user tersebut.
func TestPasswordChangeRevokesSessions(t *testing.T) {
	app, st, _ := appTestEnv(t)
	devCookie, _ := loginAs(app, t, "dev@test", "Sandi-Kuat-123")
	adminCookie, csrf := loginAs(app, t, "admin@test", "Sandi-Kuat-123")
	dev, _ := st.GetUserByEmail("dev@test")
	w := dashReq(app, "PATCH", "/api/admin/users/"+fmtInt(dev.ID), map[string]any{"password": "Baru-Kuat-2026"}, adminCookie, csrf)
	if w.Code != 200 {
		t.Fatalf("ganti password = %d %s", w.Code, w.Body.String())
	}
	if w := dashReq(app, "GET", "/api/me", nil, devCookie, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("sesi lama masih hidup: %d", w.Code)
	}
	// aktor yang mengganti password orang lain tetap login
	if w := dashReq(app, "GET", "/api/me", nil, adminCookie, ""); w.Code != 200 {
		t.Fatalf("sesi admin ikut tercabut: %d", w.Code)
	}
}

// M1: setting rahasia tidak dikembalikan GET, tidak bocor ke audit, dan key
// tak dikenal ditolak.
func TestSettingsSecretsRedacted(t *testing.T) {
	app, _, _ := appTestEnv(t)
	cookie, csrf := loginAs(app, t, "admin@test", "Sandi-Kuat-123")
	secret := "rahasia-oauth-123"
	w := dashReq(app, "PUT", "/api/admin/settings", map[string]string{"google_oauth_client_secret": secret, "notification_chat_id": "42"}, cookie, csrf)
	if w.Code != 200 {
		t.Fatalf("put settings = %d %s", w.Code, w.Body.String())
	}
	for _, p := range []string{"/api/admin/settings", "/api/admin/audit"} {
		if w := dashReq(app, "GET", p, nil, cookie, ""); strings.Contains(w.Body.String(), secret) {
			t.Fatalf("%s membocorkan secret: %s", p, w.Body.String())
		}
	}
	if w := dashReq(app, "GET", "/api/admin/settings", nil, cookie, ""); !strings.Contains(w.Body.String(), `"google_oauth_client_secret_set":"true"`) {
		t.Fatalf("penanda _set tidak ada: %s", w.Body.String())
	}
	// kosong = pertahankan nilai lama
	dashReq(app, "PUT", "/api/admin/settings", map[string]string{"google_oauth_client_secret": ""}, cookie, csrf)
	if v, _ := app.st.GetSetting("google_oauth_client_secret"); v != secret {
		t.Fatalf("secret tertimpa kosong: %q", v)
	}
	if w := dashReq(app, "PUT", "/api/admin/settings", map[string]string{"setup_completed": "0"}, cookie, csrf); w.Code != 400 {
		t.Fatalf("key tak dikenal = %d, mau 400", w.Code)
	}
}

// Rate limit login tidak bisa dilewati dengan memalsukan X-Forwarded-For.
func TestLoginRateLimitIgnoresSpoofedXFF(t *testing.T) {
	app, _, _ := appTestEnv(t)
	blocked := 0
	for i := 0; i < 8; i++ {
		r := dashReqWith(app, "POST", "/api/login", map[string]string{"email": "admin@test", "password": "salah"},
			map[string]string{"X-Forwarded-For": "203.0.113." + fmtInt(int64(i))})
		if r.Code == http.StatusTooManyRequests {
			blocked++
		}
	}
	if blocked == 0 {
		t.Fatal("XFF palsu melewati rate limit login")
	}
}
