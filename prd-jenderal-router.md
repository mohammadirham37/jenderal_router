# PRD — JenderalRouter, LLM Router Gateway Self-Hosted

Oct 4, 2026 · @Someone

## 1. Ringkasan produk

JenderalRouter adalah gateway LLM self-hosted yang dipasang di VPS: satu endpoint API kompatibel OpenAI dan Anthropic, satu dashboard GUI, dan banyak provider di belakangnya, termasuk model lokal di server yang sama melalui LlamaStash.

**Referensi utama.** [9Router](https://github.com/decolua/9router) adalah proxy open-source (MIT) yang duduk di antara tool AI (Claude Code, Codex, Cursor, Cline, dsb.) dan 40+ provider. Fitur intinya: fallback bertingkat (langganan → murah → gratis), multi-akun round-robin dengan pelacakan kuota, translasi format OpenAI ↔ Claude ↔ Gemini, kompresi token, analitik penggunaan, dan dashboard di port 20128. Stack-nya Node.js 20+, Next.js, SQLite, SSE.

**Masalah yang diselesaikan.**

- Setiap provider punya API key, format, limit, dan harga sendiri; aplikasi klien harus mengurus semuanya.
- Rate limit atau kuota habis menghentikan pekerjaan di tengah jalan.
- Tidak ada satu tempat untuk membagi akses ke tim/pelanggan dengan kuota, budget, dan log terpusat.
- Resource server (CPU/GPU/RAM VPS) tidak dimanfaatkan untuk inferensi lokal yang gratis dan privat.

**Pembeda dari 9Router.** 9Router dirancang untuk satu orang di laptop sendiri. JenderalRouter dirancang **multi-user** sejak awal (akun, peran, API key per user, kuota per user), dengan GUI chat untuk pengguna non-teknis, dan LlamaStash sebagai provider kelas satu.

## 2. Tujuan, non-tujuan, dan metrik

**Tujuan**

1. Satu base URL (`https://<domain>/v1`) yang bisa dipakai klien OpenAI SDK, Anthropic SDK, dan tool coding tanpa modifikasi.
2. Menghubungkan minimal 8 provider cloud + 1 provider lokal (LlamaStash) di MVP.
3. Fallback otomatis antar provider sehingga satu provider gagal tidak menghentikan request pengguna.
4. Multi-user: admin membuat user, API key, kuota, dan batas biaya per user.
5. GUI: dashboard admin dan chat playground untuk user akhir.
6. Instalasi di VPS dalam ≤ 10 menit via Docker Compose.

**Non-tujuan (MVP)**

- Tidak menagih uang ke pengguna (billing/payment gateway) — hanya mencatat estimasi biaya.
- Tidak memakai ulang akun langganan konsumen (Claude Pro/Max, ChatGPT Plus, Copilot) via OAuth untuk melayani banyak user. Ini berisiko melanggar ketentuan layanan provider; lihat Risiko.
- Tidak melatih atau fine-tune model.
- Tidak menjadi load balancer multi-node (satu instance per VPS di MVP).

**Metrik keberhasilan**

| Metrik | Target |
| --- | --- |
| Overhead latensi gateway (p95, di luar waktu provider) | < 50 ms |
| Time-to-first-token tambahan pada streaming | < 100 ms |
| Request sukses saat 1 provider down (dengan combo fallback) | ≥ 99% |
| Waktu instalasi dari VPS kosong ke request pertama | ≤ 10 menit |
| Akurasi pencatatan token vs laporan provider | ≥ 98% |
| Throughput per instance (VPS 2 vCPU / 4 GB, provider cloud) | ≥ 100 request/detik bersamaan (streaming) |

## 3. Persona dan use case

| Persona | Siapa | Kebutuhan utama | Antarmuka |
| --- | --- | --- | --- |
| Super Admin | Pemilik VPS | Pasang, hubungkan provider, atur keamanan, pantau biaya | Dashboard admin, CLI |
| Admin Tim | Lead tim / pengelola layanan | Buat user, API key, kuota, combo model | Dashboard admin |
| Developer | Pengguna API | Endpoint stabil, satu API key, banyak model, streaming | REST API, SDK OpenAI/Anthropic |
| Pengguna Akhir | Non-teknis | Chat dengan model pilihan via browser | Chat playground |
| Tool Coding | Claude Code, Codex CLI, Cursor, Cline, OpenCode | Base URL + key kompatibel | REST API |

**User story utama**

- Sebagai Super Admin, saya menambahkan API key OpenAI, Anthropic, Gemini, OpenRouter, dan LlamaStash lokal, lalu melihat semua model mereka di satu daftar.
- Sebagai Admin Tim, saya membuat combo `coding-hemat` = Claude Sonnet → GLM → model lokal, dan memberikan combo itu ke tim.
- Sebagai Developer, saya mengganti `base_url` di OpenAI SDK ke JenderalRouter dan kode saya berjalan tanpa perubahan lain.
- Sebagai Developer, saat Anthropic mengembalikan 429, request saya otomatis dilayani provider berikutnya tanpa error di sisi saya.
- Sebagai Pengguna Akhir, saya chat di browser, memilih model, dan riwayat chat saya tersimpan.
- Sebagai Admin Tim, saya membatasi user magang ke model lokal saja dengan kuota 200 ribu token/hari.
- Sebagai Super Admin, saya melihat token dan estimasi biaya per user, per provider, per hari.

## 4. Ruang lingkup fitur

MVP fokus pada gateway yang andal, multi-user, dan dua provider jenis (cloud API key + LlamaStash lokal); fitur penghemat token dan multimodal masuk fase berikutnya.

| Kode | Fitur | Fase | Prioritas |
| --- | --- | --- | --- |
| F-01 | Manajemen provider (API key, base URL kustom, test koneksi) | MVP | P0 |
| F-02 | Katalog model terpadu + alias `provider/model` | MVP | P0 |
| F-03 | Endpoint OpenAI-compatible `/v1/chat/completions`, `/v1/models` (streaming SSE) | MVP | P0 |
| F-04 | Endpoint Anthropic-compatible `/v1/messages` | MVP | P0 |
| F-05 | Translasi format OpenAI ↔ Anthropic ↔ Gemini | MVP | P0 |
| F-06 | Combo model + fallback otomatis + retry | MVP | P0 |
| F-07 | Multi-key per provider (round-robin / prioritas) + cooldown saat 429 | MVP | P0 |
| F-08 | User, peran (RBAC), API key per user | MVP | P0 |
| F-09 | Kuota token/request & batas biaya per user/key | MVP | P0 |
| F-10 | Log request + analitik penggunaan & estimasi biaya | MVP | P0 |
| F-11 | Dashboard admin (web) | MVP | P0 |
| F-12 | Chat playground untuk user akhir | MVP | P1 |
| F-13 | Provider LlamaStash (model lokal di server) | MVP | P0 |
| F-14 | Health check & circuit breaker per provider | MVP | P1 |
| F-15 | Tool/function calling & vision pass-through | MVP | P1 |
| F-16 | Endpoint `/v1/embeddings` | v1.1 | P1 |
| F-17 | Token saver (kompresi tool\_result, mirip RTK di 9Router) | v1.1 | P2 |
| F-18 | Cache respons (exact match, lalu semantik) | v1.1 | P2 |
| F-19 | Routing berbasis aturan (by biaya, latensi, panjang konteks) | v1.2 | P2 |
| F-20 | Endpoint gambar/audio (`/v1/images`, `/v1/audio`) | v1.2 | P3 |
| F-21 | Webhook & notifikasi (kuota habis, provider down) via Telegram/Email | v1.1 | P2 |
| F-22 | SSO (OIDC) dan 2FA admin | v1.2 | P2 |

## 5. Kebutuhan fungsional

### 5.1 Manajemen provider (F-01, F-02, F-07)

- FR-1.1 Admin dapat menambah provider dari template bawaan: OpenAI, Anthropic, Google Gemini, OpenRouter, Groq, DeepSeek, Mistral, Together, GLM (Zhipu), MiniMax, Kimi, LlamaStash.
- FR-1.2 Admin dapat menambah provider **kustom** dengan tipe `openai-compatible` atau `anthropic-compatible` (base URL + key + header tambahan).
- FR-1.3 Satu provider dapat punya banyak *credential* (API key). Strategi: `round-robin`, `priority`, atau `least-used`.
- FR-1.4 Credential yang menerima 429/401/402 masuk *cooldown* (default 60 detik, eksponensial hingga 15 menit); 401 berulang menandai key `invalid` dan memberi notifikasi.
- FR-1.5 Tombol "Test koneksi" mengirim request kecil dan menampilkan latensi + status.
- FR-1.6 Sinkronisasi daftar model otomatis dari `GET /models` provider (jika tersedia), dengan opsi daftar manual.
- FR-1.7 Setiap model diberi ID `prefix/nama-model` (contoh `oa/gpt-5.4`, `an/claude-sonnet-4-6`, `local/qwen3-8b`), plus alias bebas.
- FR-1.8 Tabel harga per model (input/output per 1 juta token) dapat diedit untuk estimasi biaya.
- FR-1.9 Semua API key provider dienkripsi saat disimpan (AES-256-GCM, master key dari env).

### 5.2 Routing, combo, dan fallback (F-06, F-14)

- FR-2.1 Combo = daftar berurutan model; dipanggil dengan nama combo di field `model`.
- FR-2.2 Fallback dipicu oleh: timeout koneksi, 5xx, 429, 402/kuota habis, error konten kosong. Tidak dipicu oleh 400 (request salah) atau penolakan konten.
- FR-2.3 Fallback pada streaming hanya boleh terjadi **sebelum** token pertama dikirim ke klien; setelah itu error diteruskan.
- FR-2.4 Retry per langkah: maksimal 1 retry ke credential lain di provider yang sama, lalu lanjut ke langkah combo berikutnya.
- FR-2.5 Circuit breaker per provider: 5 kegagalan dalam 60 detik → provider dilewati 30 detik, lalu *half-open*.
- FR-2.6 Header respons menyertakan model/provider aktual: `X-Route-Provider`, `X-Route-Model`, `X-Route-Attempts`.
- FR-2.7 Admin dapat menetapkan combo default per user dan daftar model yang diizinkan per user/peran.

### 5.3 API gateway (F-03, F-04, F-05, F-15)

- FR-3.1 Kompatibel penuh dengan skema OpenAI Chat Completions, termasuk `stream`, `tools`, `tool_choice`, `response_format`, `stream_options.include_usage`, input gambar.
- FR-3.2 Kompatibel dengan Anthropic Messages API (`/v1/messages`, header `x-api-key` dan `anthropic-version`) agar Claude Code bisa memakai `ANTHROPIC_BASE_URL`.
- FR-3.3 Translator format dua arah: request masuk dinormalisasi ke skema internal, lalu diterjemahkan ke format native provider; respons dan event SSE diterjemahkan balik.
- FR-3.4 Error dikembalikan dalam format yang sama dengan format endpoint yang dipanggil (OpenAI-style atau Anthropic-style).
- FR-3.5 Usage (prompt/completion tokens) selalu dicatat; jika provider tidak mengirim usage, dihitung dengan tokenizer perkiraan dan ditandai `estimated`.

### 5.4 User, API key, dan kuota (F-08, F-09)

- FR-4.1 Peran: `super_admin`, `admin`, `member`, `viewer`.
- FR-4.2 User dapat membuat beberapa API key (`format jr-` + 40 karakter acak), disimpan sebagai hash SHA-256; key hanya ditampilkan sekali.
- FR-4.3 Setiap key dapat diberi: masa berlaku, model yang diizinkan, IP allowlist, rate limit (RPM/TPM).
- FR-4.4 Kuota per user/key: token per hari/bulan, request per menit, batas biaya USD per bulan. Saat terlampaui → HTTP 429 dengan pesan jelas.
- FR-4.5 Kuota direset otomatis sesuai periode (zona waktu dapat diatur, default Asia/Jakarta).

### 5.5 Observabilitas & analitik (F-10)

- FR-5.1 Log setiap request: waktu, user, key, model diminta, provider/model aktual, status, latensi, TTFT, token in/out, estimasi biaya, jumlah percobaan.
- FR-5.2 Isi prompt/respons **tidak** disimpan secara default; dapat diaktifkan per user untuk debugging dengan retensi terbatas (default 7 hari).
- FR-5.3 Dashboard grafik: request/hari, token per provider, biaya per user, error rate per provider, latensi p50/p95.
- FR-5.4 Ekspor log ke CSV dan endpoint `/metrics` format Prometheus.

### 5.6 Dashboard admin (F-11)

Halaman: Ringkasan · Provider · Model & Harga · Combo · User & Peran · API Key · Kuota · Log · Analitik · Pengaturan (keamanan, backup, notifikasi) · Status LlamaStash.

### 5.7 Chat playground (F-12)

- FR-7.1 Login user, pilih model/combo yang diizinkan, chat dengan streaming dan render Markdown + kode.
- FR-7.2 Riwayat percakapan tersimpan per user; dapat dihapus dan diekspor.
- FR-7.3 Pengaturan per percakapan: system prompt, temperature, max tokens.
- FR-7.4 Unggah gambar untuk model vision; unggah dokumen teks (v1.1).
- FR-7.5 Menampilkan sisa kuota user dan model/provider yang menjawab.

## 6. Integrasi LlamaStash (provider lokal)

LlamaStash diperlakukan sebagai provider `openai-compatible` bernama `local`, yang diakses JenderalRouter lewat loopback di server yang sama, sehingga inferensi memakai CPU/GPU/RAM VPS tanpa biaya per token.

**Fakta LlamaStash yang relevan** (v0.6.1, MIT, [llamastash.dev](https://llamastash.dev/)):

- Satu binary Rust: TUI, CLI, dan daemon; daemon berjalan sendiri dan model tetap hidup walau UI ditutup.
- Backend default llama.cpp (GGUF); backend eksperimental vLLM, SGLang (safetensors), Lemonade (NPU), dan generic untuk server OpenAI-compatible lain.
- Proxy lokal di `127.0.0.1:11435` yang berbicara OpenAI API dan Anthropic Messages API, merutekan berdasarkan nama model dan **menyalakan model otomatis** saat diminta. Mode tiruan Ollama di port 11434.
- Proxy bisa dibuka ke LAN (`--proxy-host`) hanya dengan bearer key yang dibuat otomatis; control plane daemon selalu loopback dan memakai bearer token.
- CLI dengan output `--json` stabil dan exit code terdokumentasi; `llamastash init --recommended --json` untuk setup tanpa interaksi.
- Platform: Linux x86\_64/aarch64, macOS, Windows 11.

**Kebutuhan integrasi**

- FR-6.1 Template provider "LlamaStash (lokal)" dengan base URL default `http://127.0.0.1:11435/v1` dan field bearer key opsional.
- FR-6.2 Penemuan model: panggil `GET /v1/models` proxy; jika tidak lengkap, fallback ke CLI `llamastash list --json` (perlu diverifikasi nama perintahnya).
- FR-6.3 Halaman "Status LlamaStash" di dashboard: daemon hidup/mati, model yang sedang dimuat, pemakaian RAM/VRAM/CPU, tombol start/stop model (via CLI `--json` atau control plane).
- FR-6.4 Timeout khusus provider lokal: koneksi 5 detik, *cold start* model hingga 120 detik (karena auto-start memuat GGUF ke memori). Klien streaming menerima komentar SSE keep-alive selama menunggu.
- FR-6.5 Batas konkurensi lokal (default 2 request bersamaan per model, dapat diatur); request berlebih masuk antrean dengan batas 30 detik, lalu fallback ke langkah combo berikutnya.
- FR-6.6 Biaya model lokal = 0 USD, tetapi token tetap dicatat untuk kuota.
- FR-6.7 Label privasi: model lokal ditandai "data tidak keluar server"; admin dapat membuat kebijakan "user X hanya boleh model lokal".
- FR-6.8 Skrip instalasi opsional: memasang LlamaStash, menjalankan `llamastash init --recommended --json`, lalu mendaftarkan provider secara otomatis.

**Panduan kapasitas VPS (perkiraan, perlu diuji)**

| Spesifikasi VPS | Model lokal yang realistis | Catatan |
| --- | --- | --- |
| 4 vCPU / 8 GB, tanpa GPU | 1–4B parameter, kuantisasi Q4 | Lambat; cocok untuk tugas ringan/fallback |
| 8 vCPU / 16 GB, tanpa GPU | 7–8B Q4 | Beberapa token/detik per request |
| GPU 24 GB VRAM | 14–32B Q4 | Layak untuk penggunaan tim kecil |

Pemilik VPS tanpa GPU sebaiknya menempatkan model lokal sebagai langkah terakhir combo atau untuk user yang mewajibkan privasi.

## 7. Kebutuhan non-fungsional

| Kode | Area | Kebutuhan |
| --- | --- | --- |
| NFR-01 | Keamanan | Semua `/v1/*` wajib API key; tidak ada mode tanpa auth saat bind ke `0.0.0.0` (pelajaran dari 9Router yang default `REQUIRE_API_KEY=false` dan password awal `123456`) |
| NFR-02 | Keamanan | Wizard instalasi memaksa membuat password admin kuat; tidak ada password default |
| NFR-03 | Keamanan | Password di-hash Argon2id; sesi dashboard via cookie `HttpOnly`, `Secure`, `SameSite=Strict`; CSRF token |
| NFR-04 | Keamanan | Secret provider dienkripsi AES-256-GCM; master key hanya dari env/file dengan izin 600 |
| NFR-05 | Keamanan | Rate limit login (5 percobaan/15 menit/IP), audit log semua aksi admin |
| NFR-06 | Keamanan | LlamaStash tetap di loopback; port 11435 tidak pernah dibuka ke publik |
| NFR-07 | Keamanan | Proteksi SSRF untuk base URL provider kustom (blok IP metadata cloud `169.254.169.254`, kecuali diizinkan admin) |
| NFR-08 | Performa | Overhead gateway p95 < 50 ms; streaming diteruskan per chunk tanpa buffering |
| NFR-09 | Performa | Validasi API key dan kuota dari cache memori/Redis, bukan query DB per request |
| NFR-10 | Keandalan | Penulisan log & usage asinkron (antrean); kegagalan log tidak menggagalkan request |
| NFR-11 | Keandalan | Graceful shutdown: request streaming aktif diselesaikan hingga 30 detik |
| NFR-12 | Keandalan | Backup otomatis database harian, retensi 7 salinan |
| NFR-13 | Observabilitas | Log terstruktur JSON, `/healthz`, `/readyz`, `/metrics` (Prometheus) |
| NFR-14 | Privasi | Konten prompt tidak disimpan default; masking otomatis pola API key/kartu di log debug |
| NFR-15 | Portabilitas | Image Docker `linux/amd64` dan `linux/arm64`; berjalan di VPS 1 vCPU / 1 GB (tanpa model lokal) |
| NFR-16 | UX | Dashboard responsif (mobile), dark/light mode, bahasa Indonesia dan Inggris |
| NFR-17 | Kualitas | Cakupan unit test translator format ≥ 80%; test kontrak terhadap SDK resmi OpenAI dan Anthropic |

## 8. Arsitektur sistem

JenderalRouter berjalan sebagai satu proses di balik Caddy; setiap request melewati auth & kuota, router, translator, dan adapter sebelum menuju provider cloud atau LlamaStash di host yang sama.

&#91;embedded content: arsitektur JenderalRouter · klien, gateway, penyimpanan, provider\]

Hanya Caddy yang menghadap internet; port aplikasi dan port LlamaStash 11435 terikat ke loopback.

### 8.1 Alur satu request

1. Klien mengirim `POST /v1/chat/completions` dengan `Authorization: Bearer jr-…`.
2. Auth & kuota: hash key dicari di cache, lalu dicek status, model yang diizinkan, IP, RPM/TPM, kuota token, dan budget.
3. Router: `model` di-resolve ke model tunggal, alias, atau combo; provider dengan circuit terbuka dilewati.
4. Translator: request dinormalisasi ke skema internal, lalu diubah ke format native provider langkah ini.
5. Adapter: memilih credential (round-robin/prioritas), mengirim request, menerima stream.
6. Gagal sebelum token pertama → retry credential lain atau langkah combo berikutnya (FR-2.2–2.4).
7. Respons diterjemahkan balik ke format endpoint asal dan di-stream ke klien dengan header `X-Route-*`.
8. Pencatat usage menulis log dan mengurangi kuota secara asinkron.

### 8.2 Tech stack yang disarankan

| Lapisan | Pilihan | Alasan |
| --- | --- | --- |
| Bahasa backend | Go 1.23+ | Satu binary statis, goroutine cocok untuk ribuan koneksi streaming, pemakaian memori kecil di VPS |
| Server HTTP | `net/http` + chi | SSE via `http.Flusher`, middleware auth dan rate limit |
| Klien provider | `net/http` langsung + adapter per provider; opsional `openai-go` dan `anthropic-sdk-go` resmi | Kontrol penuh atas streaming dan translasi format |
| Frontend | SvelteKit (adapter-static) + TypeScript + Tailwind + shadcn-svelte | Bundle kecil dan cepat; hasil build di-embed ke binary Go via `embed`, satu container |
| Akses DB | sqlc + pgx (PostgreSQL); `modernc.org/sqlite` (SQLite tanpa CGO) | Query type-safe, build tanpa CGO untuk amd64/arm64 |
| Migrasi | goose | Satu set migrasi untuk SQLite dan PostgreSQL |
| Cache & rate limit | Memori (ristretto) default, Redis via go-redis opsional | Validasi key/kuota tanpa query DB per request |
| Validasi | go-playground/validator | Validasi skema request OpenAI/Anthropic |
| Tokenizer | tiktoken-go | Estimasi token jika provider tidak mengirim usage |
| Keamanan | `x/crypto/argon2`, `crypto/aes` (GCM), cookie HttpOnly | Hash password, enkripsi secret provider |
| Observabilitas | `log/slog` (JSON), prometheus/client\_golang | Log terstruktur dan `/metrics` |
| Konfigurasi | Env + file YAML (koanf) | Mudah di Docker maupun systemd |
| Deploy | Docker Compose + Caddy; atau binary + systemd | HTTPS otomatis, satu perintah |
| Test | `go test` + testcontainers; Vitest + Playwright untuk Svelte; test kontrak SDK resmi | Menjaga kompatibilitas format dan UI |

### 8.3 Model data

| Entitas | Field utama |
| --- | --- |
| `users` | id, email, password\_hash, role, status, default\_combo\_id, created\_at |
| `api_keys` | id, user\_id, prefix, key\_hash, name, allowed\_models, ip\_allowlist, rpm, tpm, expires\_at, revoked\_at |
| `providers` | id, type (openai, anthropic, gemini, openai-compatible, llamastash), name, prefix, base\_url, settings, enabled |
| `credentials` | id, provider\_id, secret\_enc, weight, status, cooldown\_until, last\_error |
| `models` | id, provider\_id, upstream\_name, public\_id, alias, price\_in\_per\_1m, price\_out\_per\_1m, context\_window, capabilities, enabled |
| `combos` | id, name, description |
| `combo_steps` | combo\_id, position, model\_id, timeout\_ms |
| `quotas` | id, scope (user/key), scope\_id, period (day/month), token\_limit, request\_limit, cost\_limit\_usd, used\_tokens, used\_cost, reset\_at |
| `request_logs` | id, ts, user\_id, key\_id, requested\_model, provider\_id, model\_id, status, latency\_ms, ttft\_ms, tokens\_in, tokens\_out, cost\_usd, attempts, error\_code |
| `conversations`, `messages` | Riwayat chat playground: id, user\_id, title, model, role, content, created\_at |
| `audit_logs` | id, actor\_id, action, target, before, after, ts |

## 9. Spesifikasi API

API terbagi dua: **API inferensi** (untuk klien, auth Bearer API key) dan **API manajemen** (untuk dashboard/otomasi, auth sesi admin atau admin token).

**API inferensi**

| Method | Path | Fungsi | Fase |
| --- | --- | --- | --- |
| POST | `/v1/chat/completions` | Chat OpenAI-compatible, mendukung `stream` | MVP |
| POST | `/v1/messages` | Anthropic Messages API (untuk Claude Code) | MVP |
| GET | `/v1/models` | Daftar model + combo yang diizinkan untuk key ini | MVP |
| POST | `/v1/responses` | OpenAI Responses API (untuk Codex CLI) | v1.1 |
| POST | `/v1/embeddings` | Embedding | v1.1 |
| GET | `/v1/usage/me` | Sisa kuota dan pemakaian key/user saat ini | MVP |

**API manajemen** (prefix `/api/admin`)

| Resource | Endpoint |
| --- | --- |
| Provider | `GET/POST /providers`, `PATCH/DELETE /providers/{id}`, `POST /providers/{id}/test`, `POST /providers/{id}/sync-models` |
| Credential | `POST /providers/{id}/credentials`, `DELETE /credentials/{id}` |
| Model | `GET /models`, `PATCH /models/{id}` (alias, harga, aktif) |
| Combo | `GET/POST /combos`, `PATCH/DELETE /combos/{id}` |
| User | `GET/POST /users`, `PATCH/DELETE /users/{id}` |
| API key | `GET/POST /users/{id}/keys`, `DELETE /keys/{id}` |
| Kuota | `PUT /users/{id}/quota`, `PUT /keys/{id}/quota` |
| Log & analitik | `GET /logs?from&to&user&provider&status`, `GET /analytics/summary`, `GET /logs/export.csv` |
| LlamaStash | `GET /local/status`, `POST /local/models/{name}/start`, `POST /local/models/{name}/stop` |
| Sistem | `GET /healthz`, `GET /readyz`, `GET /metrics`, `POST /system/backup` |

**Contoh request**

```bash
curl https://ai.domainanda.com/v1/chat/completions \
  -H "Authorization: Bearer jr-xxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{"model": "coding-hemat", "stream": true,
       "messages": [{"role": "user", "content": "Halo"}]}'
```

**Kode error**

| HTTP | Kode | Arti |
| --- | --- | --- |
| 401 | `invalid_api_key` | Key salah, dicabut, atau kedaluwarsa |
| 403 | `model_not_allowed` | Model tidak diizinkan untuk key/user ini |
| 404 | `model_not_found` | Model atau combo tidak ada |
| 429 | `quota_exceeded` / `rate_limited` | Kuota atau RPM/TPM habis; header `Retry-After` |
| 502 | `upstream_error` | Semua langkah combo gagal; detail tiap percobaan di `error.attempts` |
| 504 | `upstream_timeout` | Provider tidak merespons dalam batas waktu |

## 10. Deployment, roadmap, risiko

### 10.1 Deployment di VPS

- Distribusi utama: `docker compose up -d` dengan layanan `jenderalrouter` (app), `postgres` (opsional, default SQLite), `redis` (opsional), `caddy` (reverse proxy + HTTPS otomatis Let's Encrypt).
- LlamaStash dipasang **di host** (bukan container) agar akses GPU/driver mudah; container JenderalRouter mengaksesnya via `host.docker.internal:11435` atau `network_mode: host`.
- Alternatif tanpa Docker: unduh satu binary Go (UI Svelte sudah ter-embed) + service systemd.
- Wizard first-run: set domain, password admin, master key, lalu tambah provider pertama.
- Port publik hanya 80/443. Port app (default 20130) dan 11435 hanya loopback.
- Variabel env utama: `JR_MASTER_KEY`, `JR_DATABASE_URL`, `JR_REDIS_URL`, `JR_PUBLIC_URL`, `JR_LLAMASTASH_URL`, `JR_LOG_PROMPTS=false`, `JR_TZ=Asia/Jakarta`.
- Spesifikasi minimum: 1 vCPU / 1 GB (hanya provider cloud); disarankan 2 vCPU / 4 GB; tambah RAM/GPU sesuai model lokal.

### 10.2 Roadmap (estimasi, 1–2 developer)

1. **Fase 0 — Fondasi (minggu 1–2):** skema DB, auth admin, CRUD provider/credential, enkripsi secret.
2. **Fase 1 — Gateway inti (minggu 3–5):** `/v1/chat/completions` + streaming, translator OpenAI ↔ Anthropic ↔ Gemini, `/v1/messages`, `/v1/models`.
3. **Fase 2 — Routing (minggu 6–7):** combo, fallback, multi-key, cooldown, circuit breaker.
4. **Fase 3 — Multi-user (minggu 8–9):** user, peran, API key, kuota, rate limit, log usage.
5. **Fase 4 — LlamaStash & GUI (minggu 10–12):** provider lokal + halaman status, dashboard analitik, chat playground.
6. **Fase 5 — Rilis MVP (minggu 13–14):** Docker image, Caddy, wizard, dokumentasi, uji beban, audit keamanan.
7. **v1.1 / v1.2:** embeddings, Responses API, token saver, cache, notifikasi, SSO/2FA, routing berbasis aturan.

### 10.3 Risiko

| Risiko | Dampak | Mitigasi |
| --- | --- | --- |
| Memakai akun langganan konsumen (OAuth Claude Code, Codex, Copilot, Cursor) untuk melayani banyak user | Akun diblokir, pelanggaran ketentuan layanan | MVP hanya mendukung API key resmi; jika kelak ditambah, batasi ke penggunaan pribadi pemilik akun dan tampilkan peringatan |
| Perbedaan format antar provider (tool calling, thinking, vision) | Respons rusak/tidak lengkap | Test kontrak per provider, fitur yang tidak didukung ditolak dengan error jelas |
| Gateway terekspos tanpa auth | Penyalahgunaan kuota/biaya | Auth wajib, tanpa password default, rate limit, alert biaya |
| Model lokal lambat di VPS tanpa GPU | Timeout, UX buruk | Batas konkurensi, antrean, fallback, panduan kapasitas |
| API LlamaStash berubah (masih v0.x) | Integrasi rusak saat update | Adapter terisolasi, pin versi, health check |
| Kebocoran API key provider | Kerugian finansial | Enkripsi, masking log, audit log, rotasi key |
| Perubahan harga/model provider | Estimasi biaya salah | Tabel harga dapat diedit + sinkron model berkala |

### 10.4 Pertanyaan terbuka

- [ ] Apakah aplikasi akan dijual/disewakan ke pihak luar (perlu billing & ToS) atau hanya untuk tim internal?
- [ ] Target spesifikasi VPS: ada GPU atau CPU saja? Ini menentukan model lokal default.
- [ ] Stack sudah diputuskan: Go untuk backend, Svelte (SvelteKit) untuk frontend.
- [ ] SQLite cukup, atau langsung PostgreSQL untuk multi-user?
- [ ] Apakah endpoint `GET /v1/models` LlamaStash mengembalikan semua model terpindai, termasuk yang belum dimuat? Perlu diverifikasi.
- [ ] Apakah LlamaStash menangani request paralel ke satu model, atau perlu antrean penuh di sisi JenderalRouter?
- [ ] Nama produk final dan domain.

### Sumber

- [9Router — README (fork XM-Chen dari decolua/9router)](https://github.com/XM-Chen/9router)
- [9Router — repositori upstream](https://github.com/decolua/9router)
- [9Router Cloudron package](https://github.com/vRobM/9router-cloudron)
- [LlamaStash — situs resmi](https://llamastash.dev/)
