package captcha

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
	"time"
)

func TestCaptchaLifecycle(t *testing.T) {
	s := NewStore(100)
	s.KeepAnswersForTest()
	id, img, err := s.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(img)); err != nil {
		t.Fatalf("bukan PNG sah: %v", err)
	}
	ans, _ := s.Answer(id)
	if len(ans) != Length || strings.Trim(ans, Alphabet) != "" {
		t.Fatalf("jawaban = %q", ans)
	}
	if !s.Verify(id, " "+strings.ToLower(ans)+" ") {
		t.Fatal("jawaban benar (huruf kecil + spasi) harus diterima")
	}
	if s.Verify(id, ans) {
		t.Fatal("captcha harus sekali pakai")
	}

	// jawaban salah juga menghanguskan tantangan
	id2, _, _ := s.New()
	ans2, _ := s.Answer(id2)
	if s.Verify(id2, "XXXXX") || s.Verify(id2, ans2) {
		t.Fatal("setelah jawaban salah, tantangan harus hangus")
	}

	// kedaluwarsa
	id3, _, _ := s.New()
	ans3, _ := s.Answer(id3)
	s.now = func() time.Time { return time.Now().Add(TTL + time.Second) }
	if s.Verify(id3, ans3) {
		t.Fatal("captcha kedaluwarsa harus ditolak")
	}
	if s.Verify("tidak-ada", "ABCDE") {
		t.Fatal("id tak dikenal harus ditolak")
	}
}

func TestCaptchaStoreBounded(t *testing.T) {
	s := NewStore(10)
	for i := 0; i < 50; i++ {
		if _, _, err := s.New(); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(s.items); n > 10 {
		t.Fatalf("store tumbuh tanpa batas: %d", n)
	}
}

func TestAllGlyphsDefined(t *testing.T) {
	for i := 0; i < len(Alphabet); i++ {
		if _, ok := glyphs[Alphabet[i]]; !ok {
			t.Fatalf("glyph %q tidak ada", Alphabet[i])
		}
	}
}
