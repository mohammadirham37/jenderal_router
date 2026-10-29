package usage

import (
	"testing"
	"time"
)

func TestGetTopUsersChatVsAPIAndModelFilter(t *testing.T) {
	s := usageStore(t)
	for _, e := range []struct{ email string }{{"a@x"}, {"b@x"}} {
		if _, err := s.CreateUser(e.email, "Sandi-Kuat-123", "member"); err != nil {
			t.Fatal(err)
		}
	}
	r := NewRecorder(s, 64)
	now := time.Now()
	// user 1: 2 chat (tanpa key) + 1 API, dua model
	r.Record(Entry{TS: now, UserID: 1, RequestedModel: "local/qwen", Status: 200, TokensIn: 10, TokensOut: 5})
	r.Record(Entry{TS: now, UserID: 1, RequestedModel: "local/qwen", Status: 200, TokensIn: 10, TokensOut: 5})
	r.Record(Entry{TS: now, UserID: 1, KeyID: 7, RequestedModel: "oa/gpt", Status: 200, CostUSD: 0.5})
	// user 2: 1 API memakai oa/gpt
	r.Record(Entry{TS: now, UserID: 2, KeyID: 8, RequestedModel: "oa/gpt", Status: 200})
	// tanpa user (ditolak sebelum autentikasi) tidak dihitung
	r.Record(Entry{TS: now, RequestedModel: "oa/gpt", Status: 401})
	// di luar rentang
	r.Record(Entry{TS: now.AddDate(0, 0, -30), UserID: 2, KeyID: 8, RequestedModel: "local/qwen", Status: 200})
	r.Close()

	from, to := now.AddDate(0, 0, -7), now.Add(time.Minute)
	res, err := GetTopUsers(s, from, to, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Models) != 2 || res.Models[0].Model != "local/qwen" || res.Models[0].Requests != 2 ||
		res.Models[1].Model != "oa/gpt" || res.Models[1].Users != 2 {
		t.Fatalf("models = %+v", res.Models)
	}
	if len(res.Users) != 2 {
		t.Fatalf("users = %+v", res.Users)
	}
	u := res.Users[0]
	if u.Email != "a@x" || u.Requests != 3 || u.ChatRequests != 2 || u.APIRequests != 1 ||
		u.TokensIn != 20 || u.CostUSD != 0.5 || u.TopModel != "local/qwen" || u.LastUsed == "" {
		t.Fatalf("user teratas = %+v", u)
	}

	// filter model: hanya request oa/gpt yang dihitung
	res, err = GetTopUsers(s, from, to, "oa/gpt", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Models) != 2 {
		t.Fatalf("daftar model untuk filter harus tetap lengkap: %+v", res.Models)
	}
	if len(res.Users) != 2 || res.Users[0].Requests != 1 || res.Users[0].ChatRequests != 0 || res.Users[0].TopModel != "oa/gpt" {
		t.Fatalf("users terfilter = %+v", res.Users)
	}

	// limit
	res, _ = GetTopUsers(s, from, to, "", 1)
	if len(res.Users) != 1 {
		t.Fatalf("limit 1 → %d user", len(res.Users))
	}
}
