package api

import (
	"strings"
	"testing"
)

func TestParseListJSONArray(t *testing.T) {
	rows, err := parseLlamastashListJSON([]byte(`[
		{"name":"Qwen3-30B-A3B-Q6_K","repo":"Qwen/Qwen3-30B-A3B","status":{"state":"ready","port":41100,"launch_id":"L1"}},
		{"name":"gpt-oss-20b-Q6_K","repo":"openai/gpt-oss-20b"}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("jumlah baris %d", len(rows))
	}
	if loaded, state := rows[0].running(); !loaded || state != "ready" {
		t.Fatalf("baris 1: loaded=%v state=%q", loaded, state)
	}
	if loaded, _ := rows[1].running(); loaded {
		t.Fatal("baris 2 tidak boleh dianggap jalan")
	}
}

func TestParseListJSONWrapped(t *testing.T) {
	rows, err := parseLlamastashListJSON([]byte(`{"models":[
		{"display_label":"Qwen3-30B-A3B-Q6_K","status":{"state":"loading"}}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("jumlah baris %d", len(rows))
	}
	if rows[0].displayName() != "Qwen3-30B-A3B-Q6_K" {
		t.Fatalf("nama: %q", rows[0].displayName())
	}
	if loaded, state := rows[0].running(); !loaded || state != "loading" {
		t.Fatalf("loaded=%v state=%q", loaded, state)
	}
}

func TestRunningViaLaunchesFallback(t *testing.T) {
	rows, err := parseLlamastashListJSON([]byte(`[
		{"name":"q","launches":[{"state":"stopped"},{"state":"ready"}]}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if loaded, state := rows[0].running(); !loaded || state != "ready" {
		t.Fatalf("loaded=%v state=%q — launch hidup harus terdeteksi", loaded, state)
	}
}

func TestRunningErrorAndStoppedIgnored(t *testing.T) {
	rows, err := parseLlamastashListJSON([]byte(`[
		{"name":"a","status":{"state":"error"}},
		{"name":"b","status":{"state":"stopped"}},
		{"name":"c","status":{"state":"external"}}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if loaded, _ := rows[0].running(); loaded {
		t.Fatal("error tidak boleh dianggap jalan")
	}
	if loaded, _ := rows[1].running(); loaded {
		t.Fatal("stopped tidak boleh dianggap jalan")
	}
	if loaded, state := rows[2].running(); !loaded || state != "external" {
		t.Fatalf("external = melayani: loaded=%v state=%q", loaded, state)
	}
}

func TestParseListJSONGarbage(t *testing.T) {
	if _, err := parseLlamastashListJSON([]byte("bukan json")); err == nil || !strings.Contains(err.Error(), "invalid") {
		if err == nil {
			t.Fatal("harusnya error")
		}
	}
}
