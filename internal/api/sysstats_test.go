package api

import "testing"

func TestParseProcStat(t *testing.T) {
	total, idle, ok := parseProcStat("cpu  100 0 100 700 100 0 0 0 0 0")
	if !ok {
		t.Fatal("harus valid")
	}
	// total = 1000, idle = 700+100 = 800 → usage 20%
	if total != 1000 || idle != 800 {
		t.Fatalf("total=%d idle=%d", total, idle)
	}
	if _, _, ok := parseProcStat("cpu0 1 2 3 4"); ok {
		t.Fatal("baris cpu0 tidak boleh dianggap agregat")
	}
}

func TestParseMemInfo(t *testing.T) {
	sample := `MemTotal:       65432100 kB
MemFree:         1234567 kB
MemAvailable:   60123456 kB
Buffers:          999999 kB
Cached:         20000000 kB
`
	total, used, ok := parseMemInfo(sample)
	if !ok {
		t.Fatal("harus valid")
	}
	if total != 65432100*1024 {
		t.Fatalf("total=%d", total)
	}
	if used != (65432100-60123456)*1024 {
		t.Fatalf("used=%d", used)
	}
	if _, _, ok := parseMemInfo("MemFree: 1 kB\n"); ok {
		t.Fatal("tanpa MemTotal harus gagal")
	}
}

func TestPickGGUFPinnedShortest(t *testing.T) {
	files := []string{"m-UD-Q6_K_XL.gguf", "m-Q6_K.gguf", "mmproj-m-Q6_K.gguf"}
	// mmproj sudah dibuang hfListFiles; di sini uji pemilihan terpendek
	if got := pickGGUFPinned(files[:2], "Q6_K"); got != "m-Q6_K.gguf" {
		t.Fatalf("file=%s", got)
	}
}
