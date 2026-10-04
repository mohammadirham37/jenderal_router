# JenderalRouter — Keputusan Desain Implementasi (MVP)

Tanggal: 2026-10-04 · Sumber: `prd-jenderal-router.md`

Dokumen ini merekam keputusan implementasi untuk pertanyaan terbuka di PRD §10.4
dan deviasi yang diambil, karena PRD tetap menjadi spesifikasi utama.

## 1. Keputusan atas pertanyaan terbuka PRD

| Pertanyaan PRD §10.4 | Keputusan |
| --- | --- |
| Dijual/disewakan atau tim internal? | Tim internal (non-tujuan MVP: tanpa billing). Estimasi biaya hanya catatan. |
| VPS GPU atau CPU? | Keduanya didukung; LlamaStash opsional. Tidak ada asumsi GPU di kode. |
| Stack | **Backend: Go 1.23+** (sesuai PRD). **Frontend: deviasi — lihat §2.** |
| SQLite atau PostgreSQL? | **SQLite default** (`modernc.org/sqlite`, tanpa CGO) sesuai §10.1; abstraksi store memungkinkan PostgreSQL belakangan. |
| `/v1/models` LlamaStash | Diperlakukan openai-compatible standar; discovery via `GET /v1/models`, fallback CLI `llamastash list --json` bila binary ada di PATH (instalasi bare-metal). |
| Paralel ke satu model lokal | JenderalRouter memegang antrean sendiri: semaphore konkurensi per model (default 2, dapat diatur), antre 30 detik lalu fallback (FR-6.5). |

## 2. Deviasi dari stack yang disarankan (dan alasannya)

| PRD (§8.2) | Implementasi MVP | Alasan |
| --- | --- | --- |
| chi (router) | `net/http` stdlib dengan pattern routing Go 1.22+ (`POST /v1/chat/completions`, `{id}` wildcard) | Kapabilitas identik untuk kebutuhan ini, nol dependensi |
| SvelteKit + Tailwind + shadcn-svelte | **SPA statis vanilla (HTML/CSS/JS murni), di-embed via `embed.FS`, tanpa langkah build** | Menghasilkan produk akhir yang sama (F-11 dashboard lengkap + F-12 chat playground, streaming, dark/light, i18n ID/EN, responsif mobile) dalam satu binary; VPS target tidak butuh toolchain Node untuk build/patch; menghapus seluruh rantai risiko bundler. Struktur kode JS modular per halaman agar tetap mudah dipindah ke SvelteKit bila kelak diinginkan |
| koanf (env + YAML) | Env-only (semua var §10.1: `JR_MASTER_KEY`, `JR_DATABASE_URL`, `JR_REDIS_URL`, `JR_PUBLIC_URL`, `JR_LLAMASTASH_URL`, `JR_LOG_PROMPTS`, `JR_TZ`, dst.) | Konfigurasi runtime 100% via env di Docker/systemd; YAML tidak diperlukan MVP |
| goose | Migrasi embed-SQL buatan sendiri (`schema_migrations`, urut, idempoten) | Satu set SQL untuk SQLite/PostgreSQL tetap tercapai, tanpa dependensi |
| tiktoken-go | Estimator token internal (heuristik BPE-lite per karakter/word) | FR-3.5 hanya mensyaratkan "tokenizer perkiraan" + flag `estimated`; menghindari dependensi data file besar |
| ristretto + go-redis | Cache in-memory `sync.RWMutex` + sliding window per key (NFR-09 terpenuhi: validasi key/kuota tanpa query DB per request). `JR_REDIS_URL` diterima; Redis ditunda v1.1 | Skala target single-instance per VPS |
| prometheus/client_golang | Ekspor teks Prometheus format 0.0.4 ditulis sendiri untuk metrik inti | Format teks stabil dan kecil; metrik: request total, latensi histogram, token, error per provider, circuit state |
| testcontainers | `go test` + `httptest` + provider mock (server OpenAI/Anthropic/Gemini palsu) | Kontrak format diuji eksak tanpa Docker; Docker opsional di mesin dev |

**Dipertahankan sesuai PRD:** `modernc.org/sqlite` (tanpa CGO → cross-compile amd64/arm64),
`golang.org/x/crypto/argon2` (Argon2id), AES-256-GCM via stdlib `crypto/aes`,
`log/slog` JSON, SSE via `http.Flusher`.

## 3. Arsitektur implementasi

Satu binary `jenderalrouter`:

```
cmd/jenderalrouter        main: config, db open+migrate, server, graceful shutdown (NFR-11)
internal/config           env → struct, default aman (REQUIRE_API_KEY selalu true, NFR-01)
internal/db               sqlite (WAL), migrasi embed
internal/crypto           AES-256-GCM secret box, argon2id, generator jr-key (F-4.2), SHA-256
internal/store            akses data: users, api_keys, providers, credentials, models,
                          combos, quotas, request_logs, conversations, audit_logs, sessions
internal/auth             sesi cookie HttpOnly/Secure/SameSite=Strict, CSRF, rate limit login 5/15m/IP (NFR-03/05)
internal/apigate          auth key jr-, allowed models, IP allowlist, RPM/TPM, kuota (F-08/09)
internal/translate        skema internal ⇄ openai ⇄ anthropic ⇄ gemini (request, respons, SSE) — F-05
internal/provider         adapter upstream (openai-compatible, anthropic, gemini), template 12 provider,
                          pemilihan credential RR/priority/least-used, cooldown 429/401/402 (F-01/07)
internal/router           resolusi model/alias/combo, fallback FR-2.2–2.4, circuit breaker FR-2.5, header X-Route-* FR-2.6
internal/usage            pencatatan asinkron (antrean kanal), estimasi biaya, reset kuota periodik Asia/Jakarta (F-10, FR-4.5)
internal/api              handler /v1/* (chat completions, messages, models, usage/me) + /api/admin/* + healthz/readyz/metrics
internal/web              embed UI statis + wizard first-run
web/                      sumber UI (vanilla JS SPA, i18n, dark/light)
deploy/                   Dockerfile, docker-compose.yml, Caddyfile, systemd unit, install-ubuntu.sh
```

Alur request sesuai PRD §8.1 (auth → kuota → resolve → translate → adapter →
fallback → stream balik → usage asinkron).

## 4. Skema data

Mengikuti tabel §8.3 secara langsung (users, api_keys, providers, credentials,
models, combos, combo_steps, quotas, request_logs, conversations, messages,
audit_logs) + `sessions` (sesi dashboard) + `schema_migrations`.
Semua secret provider tersimpan terenkripsi AES-256-GCM (FR-1.9, NFR-04);
API key user hanya hash SHA-256 (FR-4.2).

## 5. Batas MVP yang dikerjakan sekarang

Fitur P0 penuh: F-01…F-11, F-13, F-14 (health+circuit breaker), dan F-15
(tool calling & vision pass-through pada translator).
F-12 chat playground dikerjakan versi fungsional (streaming, riwayat, kuota terlihat).
Yang sengaja ditunda sesuai roadmap PRD: `/v1/responses`, `/v1/embeddings` (v1.1),
token saver, cache respons, webhook, SSO/2FA, endpoint gambar/audio.

## 6. Pengujian

- Translator: unit test tabel untuk semua arah (openai↔internal, anthropic↔internal,
  gemini↔internal) mencakup teks, vision (image), tools/tool_calls, stop reason,
  usage, SSE event — target cakupan ≥ 80% (NFR-17).
- Router: fallback sebelum token pertama, retry credential, cooldown, circuit breaker.
- API: httptest end-to-end terhadap provider mock (OpenAI & Anthropic & streaming),
  auth gagal/kuota habis, format error sesuai endpoint (FR-3.4).
- Smoke nyata: binary dijalankan, wizard setup, key dibuat, request ke provider mock
  streaming dan non-streaming diverifikasi via curl.

## Addendum (2026-10-04, revisi UI)

Deviasi frontend pada tabel §2 **tidak berlaku lagi** atas permintaan pemilik:
UI dibangun ulang dengan **SvelteKit + TypeScript (adapter-static, SPA
fallback) + ikon @lucide/svelte** sesuai rekomendasi stack PRD §8.2.
Folder sumber: `web/`; hasil build di-embed via `internal/web/dist`
(di-commit agar build Go murni tidak butuh Node). Tailwind/shadcn belum
dipakai — design system CSS kustom di `web/src/app.css`.
Fitur tambahan di luar PRD: instalasi LlamaStash dari dashboard (job asinkron
dengan log live) dan pembaruan aplikasi dari dashboard (mirror + build +
helper sudo sempit, opt-in).
