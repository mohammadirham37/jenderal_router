// Package captcha captcha gambar internal (tanpa layanan pihak ketiga) untuk
// login password. Kode dirender ke PNG dengan font bitmap sendiri, diputar,
// dibengkokkan gelombang sinus, dan ditimpa noise — teksnya tidak ada di
// respons dalam bentuk yang bisa dibaca mesin. Jawaban sekali pakai.
package captcha

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/big"
	mrand "math/rand/v2"
	"strings"
	"sync"
	"time"
)

// Alphabet tanpa karakter ambigu (0/O, 1/I/L, B/8, S/5, Z/2, G, U/V).
const Alphabet = "ACDEFHJKLMNPRTUWXY34679"

const (
	Length = 5
	TTL    = 2 * time.Minute
	width  = 210
	height = 72
)

type entry struct {
	answer  [32]byte // sha256 jawaban (huruf besar)
	expires time.Time
}

// Store penyimpan tantangan aktif (memori proses).
type Store struct {
	mu      sync.Mutex
	items   map[string]entry
	max     int
	now     func() time.Time
	answers map[string]string // hanya terisi bila keepAnswers (pengujian)
	keep    bool
}

// NewStore membuat store dengan kapasitas maksimum tantangan aktif.
func NewStore(max int) *Store {
	return &Store{items: map[string]entry{}, max: max, now: time.Now}
}

// KeepAnswersForTest menyimpan jawaban asli agar test bisa menyelesaikan
// captcha lewat Answer. Tidak pernah dipanggil di jalur produksi.
func (s *Store) KeepAnswersForTest() {
	s.mu.Lock()
	s.keep, s.answers = true, map[string]string{}
	s.mu.Unlock()
}

// Answer jawaban tantangan (hanya bila KeepAnswersForTest aktif).
func (s *Store) Answer(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.answers[id]
	return a, ok
}

func randCode(n int) string {
	b := make([]byte, n)
	for i := range b {
		k, _ := rand.Int(rand.Reader, big.NewInt(int64(len(Alphabet))))
		b[i] = Alphabet[k.Int64()]
	}
	return string(b)
}

func randID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// New membuat tantangan baru: id + PNG.
func (s *Store) New() (string, []byte, error) {
	code := randCode(Length)
	img, err := render(code)
	if err != nil {
		return "", nil, err
	}
	id := randID()
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if len(s.items) >= s.max {
		for k, e := range s.items {
			if now.After(e.expires) {
				delete(s.items, k)
			}
		}
		if len(s.items) >= s.max { // masih penuh: buang sembarang (paling lama tak terpakai)
			for k := range s.items {
				delete(s.items, k)
				break
			}
		}
	}
	s.items[id] = entry{answer: sha256.Sum256([]byte(code)), expires: now.Add(TTL)}
	if s.keep {
		s.answers[id] = code
	}
	return id, img, nil
}

// Verify memeriksa jawaban; tantangan SELALU dihapus (sekali pakai), benar
// maupun salah — mencegah tebakan berulang pada gambar yang sama.
func (s *Store) Verify(id, answer string) bool {
	s.mu.Lock()
	e, ok := s.items[id]
	delete(s.items, id)
	now := s.now()
	s.mu.Unlock()
	if !ok || now.After(e.expires) {
		return false
	}
	got := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(answer))))
	return subtle.ConstantTimeCompare(got[:], e.answer[:]) == 1
}

// ---------- rendering ----------

// glyph 5x7 untuk tiap karakter Alphabet ('#' = piksel).
var glyphs = map[byte][7]string{
	'A': {".###.", "#...#", "#...#", "#####", "#...#", "#...#", "#...#"},
	'C': {".####", "#....", "#....", "#....", "#....", "#....", ".####"},
	'D': {"####.", "#...#", "#...#", "#...#", "#...#", "#...#", "####."},
	'E': {"#####", "#....", "#....", "####.", "#....", "#....", "#####"},
	'F': {"#####", "#....", "#....", "####.", "#....", "#....", "#...."},
	'H': {"#...#", "#...#", "#...#", "#####", "#...#", "#...#", "#...#"},
	'J': {"..###", "...#.", "...#.", "...#.", "...#.", "#..#.", ".##.."},
	'K': {"#...#", "#..#.", "#.#..", "##...", "#.#..", "#..#.", "#...#"},
	'L': {"#....", "#....", "#....", "#....", "#....", "#....", "#####"},
	'M': {"#...#", "##.##", "#.#.#", "#.#.#", "#...#", "#...#", "#...#"},
	'N': {"#...#", "##..#", "#.#.#", "#..##", "#...#", "#...#", "#...#"},
	'P': {"####.", "#...#", "#...#", "####.", "#....", "#....", "#...."},
	'R': {"####.", "#...#", "#...#", "####.", "#.#..", "#..#.", "#...#"},
	'T': {"#####", "..#..", "..#..", "..#..", "..#..", "..#..", "..#.."},
	'U': {"#...#", "#...#", "#...#", "#...#", "#...#", "#...#", ".###."},
	'W': {"#...#", "#...#", "#...#", "#.#.#", "#.#.#", "##.##", "#...#"},
	'X': {"#...#", "#...#", ".#.#.", "..#..", ".#.#.", "#...#", "#...#"},
	'Y': {"#...#", "#...#", ".#.#.", "..#..", "..#..", "..#..", "..#.."},
	'3': {"####.", "....#", "....#", ".###.", "....#", "....#", "####."},
	'4': {"...#.", "..##.", ".#.#.", "#..#.", "#####", "...#.", "...#."},
	'6': {".###.", "#....", "#....", "####.", "#...#", "#...#", ".###."},
	'7': {"#####", "....#", "...#.", "..#..", ".#...", ".#...", ".#..."},
	'9': {".###.", "#...#", "#...#", ".####", "....#", "....#", ".###."},
}

func on(g [7]string, x, y int) bool {
	return x >= 0 && x < 5 && y >= 0 && y < 7 && g[y][x] == '#'
}

func render(code string) ([]byte, error) {
	r := mrand.New(mrand.NewPCG(uint64(time.Now().UnixNano()), 0x9e3779b97f4a7c15))
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	// latar terang bergradasi + bintik
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			v := uint8(232 + (x*12)/width + r.IntN(10))
			img.Set(x, y, color.NRGBA{v, v + 2 - uint8(r.IntN(3)), 248, 255})
		}
	}
	waveA, waveP, waveF := 3+r.Float64()*3, r.Float64()*math.Pi*2, 18+r.Float64()*14
	for i := 0; i < len(code); i++ {
		g := glyphs[code[i]]
		scale := 5.2 + r.Float64()*1.3
		theta := (r.Float64() - 0.5) * 0.7
		cx := 22 + float64(i)*38 + (r.Float64()-0.5)*8
		cy := 36 + (r.Float64()-0.5)*10
		col := color.NRGBA{uint8(20 + r.IntN(70)), uint8(30 + r.IntN(70)), uint8(70 + r.IntN(110)), 255}
		sin, cos := math.Sin(-theta), math.Cos(-theta)
		for py := int(cy) - 30; py < int(cy)+30; py++ {
			for px := int(cx) - 26; px < int(cx)+26; px++ {
				// gelombang horizontal lalu rotasi balik ke koordinat glyph
				wy := float64(py) + waveA*math.Sin(float64(px)/waveF+waveP)
				dx, dy := float64(px)-cx, wy-cy
				gx := (dx*cos-dy*sin)/scale + 2.5
				gy := (dx*sin+dy*cos)/scale + 3.5
				if on(g, int(math.Floor(gx)), int(math.Floor(gy))) && px >= 0 && py >= 0 && px < width && py < height {
					img.Set(px, py, col)
				}
			}
		}
	}
	// garis pengganggu melintasi teks
	for k := 0; k < 6; k++ {
		x0, y0 := r.IntN(width), r.IntN(height)
		x1, y1 := r.IntN(width), r.IntN(height)
		col := color.NRGBA{uint8(60 + r.IntN(120)), uint8(60 + r.IntN(120)), uint8(90 + r.IntN(120)), 255}
		line(img, x0, y0, x1, y1, col, 1+r.IntN(2))
	}
	for k := 0; k < 420; k++ {
		img.Set(r.IntN(width), r.IntN(height), color.NRGBA{uint8(r.IntN(200)), uint8(r.IntN(200)), uint8(r.IntN(255)), 255})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func line(img *image.NRGBA, x0, y0, x1, y1 int, c color.NRGBA, thick int) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := sign(x1-x0), sign(y1-y0)
	e := dx + dy
	for {
		for t := 0; t < thick; t++ {
			img.Set(x0, y0+t, c)
		}
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * e
		if e2 >= dy {
			e += dy
			x0 += sx
		}
		if e2 <= dx {
			e += dx
			y0 += sy
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}
