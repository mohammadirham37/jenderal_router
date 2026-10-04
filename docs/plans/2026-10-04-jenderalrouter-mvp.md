# JenderalRouter MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Membangun JenderalRouter MVP — gateway LLM self-hosted multi-user (OpenAI + Anthropic compatible API, fallback/combo, kuota, dashboard, LlamaStash) sesuai PRD.

**Architecture:** Satu binary Go (net/http pattern routing, modernc.org/sqlite) yang meng-embed UI statis vanilla; request melewati auth/kuota → router (combo/fallback/circuit breaker) → translator (openai/anthropic/gemini) → adapter upstream (cloud atau LlamaStash loopback) → stream balik + pencatatan usage asinkron.

**Tech Stack:** Go 1.23+ (stdlib-first), modernc.org/sqlite, golang.org/x/crypto (argon2id), AES-256-GCM stdlib, log/slog, embed.FS; UI: HTML/CSS/JS murni.

**Spec:** `docs/superpowers/specs/2026-10-04-jenderalrouter-design.md` + `prd-jenderal-router.md`

## Global Constraints

- Go module `github.com/jenderal/jenderalrouter`; build TANPA CGO (`CGO_ENABLED=0`) untuk amd64+arm64.
- Semua `/v1/*` wajib API key (NFR-01) — tidak ada flag untuk mematikan auth.
- Key user format `jr-` + 40 chars acak, disimpan SHA-256 (FR-4.2). Secret provider AES-256-GCM (FR-1.9).
- Password Argon2id; cookie sesi HttpOnly/Secure/SameSite=Strict + CSRF (NFR-03).
- Fallback streaming hanya sebelum token pertama (FR-2.3); 400/penolakan konten tidak memicu fallback (FR-2.2).
- Error response memakai format endpoint asal (FR-3.4); header `X-Route-Provider/Model/Attempts` (FR-2.6).
- Prompt tidak disimpan default (FR-5.2, NFR-14); log usage asinkron (NFR-10).
- Rencana dieksekusi inline di sesi ini; test dijalankan tiap tugas; `go vet` bersih.

---

### Task 1: Scaffold proyek + config + server + health
- Create: `go.mod`, `cmd/jenderalrouter/main.go`, `internal/config/config.go`, `internal/config/config_test.go`, `internal/server/server.go`
- Config env: `JR_ADDR` (default `127.0.0.1:20130`), `JR_MASTER_KEY` (wajib; jika kosong → derive & simpan ke file `data/master.key` 0600), `JR_DATABASE_URL` (default `data/jenderalrouter.db`), `JR_PUBLIC_URL`, `JR_LLAMASTASH_URL` (default `http://127.0.0.1:11435/v1`), `JR_LOG_PROMPTS` (default false), `JR_TZ` (default Asia/Jakarta), `JR_DATA_DIR`, `JR_SESSION_TTL`, `JR_COOKIE_SECURE`.
- Produces: `config.Load() (*Config, error)`; `server.New(deps)` dengan `/healthz`, `/readyz`.
- Test: config parsing & default; server 200 `/healthz`.

### Task 2: crypto (AES-GCM, argon2id, key gen)
- Create: `internal/crypto/crypto.go`, `internal/crypto/crypto_test.go`
- API: `Encrypt(master []byte, plaintext []byte) (string, error)` / `Decrypt`; `HashPassword(pw) (string, error)` / `VerifyPassword(hash, pw) (bool, error)` (argon2id, format `$argon2id$v=19$m=65536,t=2,p=1$...`); `NewAPIKey() (plain string, hash string)` (`jr-`+40, SHA-256 hex); `NewToken(n)`, `HashToken(s)`.
- Test: round-trip GCM (dan tamat → error), password hash/verify + salah password, panjang/format key, deterministik hash token.

### Task 3: db + migrasi embed
- Create: `internal/db/db.go`, `internal/db/migrations/0001_init.sql` (embed), `internal/db/db_test.go`
- Skema §8.3 + `sessions`, `settings`, `schema_migrations(version PK, applied_at)`. WAL mode, foreign_keys ON. SQL portabel SQLite (tipe teks/integer).
- API: `db.Open(path) (*sql.DB, error)` (migrasi otomatis), `db.OpenInMemory()` untuk test.
- Test: migrasi idempoten; tabel utama ada; foreign key aktif.

### Task 4: store — users, api_keys, sessions, settings, audit
- Create: `internal/store/store.go` (struct + konstruksi), `internal/store/users.go`, `internal/store/apikeys.go`, `internal/store/sessions.go`, `internal/store/audit.go` + `*_test.go`
- API: `CreateUser(email, passwordHash, role)`, `GetUserByEmail`, `GetUser`, `UpdateUser`, `DeleteUser`, `ListUsers`; `CreateAPIKey(userID, name, hash, prefix, opts) (*APIKey)`, `GetAPIKeyByHash`, `ListKeysByUser`, `RevokeKey`, `DeleteKey`; `CreateSession`, `GetSession`, `DeleteSession`, `PurgeExpiredSessions`; `Audit(actor, action, target, before, after json.RawMessage)`; `GetSetting/SetSetting`.
- Role: `super_admin, admin, member, viewer` (konstanta + hierarki `roleAtLeast`).
- Test: CRUD dasar in-memory, revocation, purge.

### Task 5: translate — skema internal + OpenAI ⇄ internal
- Create: `internal/translate/types.go`, `internal/translate/openai.go`, `internal/translate/openai_test.go`
- Internal: `ChatRequest{Model, Messages []Message, Tools []Tool, ToolChoice any, Temperature *float64, TopP *float64, MaxTokens *int, Stop any, Stream bool, ResponseFormat *ResponseFormat, User string}`, `Message{Role, Content []Part atau string, ToolCalls []ToolCall, ToolCallID, Name}`, `Part{Type: text|image_url|input_image|refusal, Text, ImageURL}`, `ToolCall{ID, Name, Arguments}`.
- Fungsi: `ParseOpenAIRequest([]byte) (*ChatRequest, error)`, `RenderOpenAIRequest(*ChatRequest) ([]byte, error)`, `ParseOpenAIResponse`, `RenderOpenAIResponse`, `RenderOpenAIError(code, msg, typ, param) []byte`, `ParseOpenAISSEChunk`/`RenderOpenAISSEChunk` (termasuk `delta.tool_calls` incremental, `finish_reason`, `usage` + `stream_options.include_usage`), `EstimateTokens(text) int` + `Estimated bool`.
- Test (tabel): teks sederhana, multi-part vision (data URL), tools + tool_choice, response_format, stop array/string, delta tool_call id/index/arguments accross chunks, usage include, error render.
- Test DULU (TDD): tulis fixture JSON dari spesifikasi OpenAI, jalankan gagal, implementasi, lolos.

### Task 6: translate — Anthropic ⇄ internal
- Create: `internal/translate/anthropic.go`, `internal/translate/anthropic_test.go`
- `ParseAnthropicRequest` (system top-level, content blocks text/image/tool_use/tool_result, `max_tokens` wajib, `anthropic_version` header), `RenderAnthropicRequest`, `Parse/RenderAnthropicResponse` (content blocks, `stop_reason` end_turn/tool_use/max_tokens, usage input/output), `RenderAnthropicError(type, message)` (format `{"type":"error","error":{...}}`), SSE: `message_start`, `content_block_start/delta/stop`, `message_delta` (stop_reason+usage), `message_stop`, `ping`.
- Test: konversi dua arah openai internal ⇄ anthropic (system, tool loop: assistant tool_use → user tool_result ⇄ OpenAI tool_calls → role tool), SSE event sequence lengkap dua arah, error format.

### Task 7: translate — Gemini ⇄ internal
- Create: `internal/translate/gemini.go`, `internal/translate/gemini_test.go`
- `RenderGeminiRequest` (contents[].role user/model, parts[].text/inline_data/function_call/function_response, systemInstruction, generationConfig maxOutputTokens/temperature/stopSequences), `ParseGeminiResponse` (candidates[].content.parts, finishReason, usageMetadata), `ParseGeminiSSEResponse` (array chunk per event `data:`), URL builder `GeminiURL(base, model, stream)`.
- Test: mapping role, function calling dua arah, usage, sistem prompt.

### Task 8: store — providers, credentials, models, combos + template
- Create: `internal/store/providers.go`, `internal/store/models.go`, `internal/store/combos.go`, `internal/store/quota.go` + tests, `internal/provider/templates.go` (+ `templates_test.go`)
- API: CRUD provider (type: openai|anthropic|gemini|openai-compatible|llamastash; prefix unik), credential (secret_enc; `PickCredential(strategy, skipSet)` round-robin/priority/least-used + `MarkResult(credID, status, errCode)` men-set `cooldown_until` 60s eksponensial maks 15m, 401 berulang → status invalid), model (public_id `prefix/name`, alias unik, harga in/out per 1M, context window, capabilities JSON, enabled), combo + steps (position), quota scope user/key period day/month + `CheckQuota`/`RecordUsage`/`ResetDue`.
- Template 12 provider (FR-1.1): openai, anthropic, gemini, openrouter, groq, deepseek, mistral, together, glm, minimax, kimi, llamastash — masing-masing base URL, prefix default, daftar model awal (nama, context window, harga, capabilities) yang dapat di-edit & sync ulang.
- Test: cooldown eksponensial & invalid 401, least-used, combo urutan, quota day/month + reset_due, template lengkap & prefix unik.

### Task 9: provider adapter upstream
- Create: `internal/provider/client.go`, `internal/provider/openai_compat.go`, `internal/provider/anthropic.go`, `internal/provider/gemini.go`, `internal/provider/client_test.go`, `internal/provider/mock_test.go`
- Interface: `Send(ctx, req *upstream.Request) (*upstream.Stream, error)`; `upstream.Request{Format, BaseURL, APIKey, ExtraHeaders, Internal *translate.ChatRequest, Timeout, ConnectTimeout}`; `upstream.Stream` = iterasi event internal (`Event{Type: response|delta|done|error, ...}`) dari SSE upstream ATAU respons non-stream yang dinormalisasi.
- SSRF guard (NFR-07): blokir host/IP privat & `169.254.169.254` untuk provider kustom kecuali di-allowlist; loopback diizinkan hanya untuk type llamastash.
- Keep-alive SSE comment untuk cold start lokal (FR-6.4) ditangani di sini lewat callback `OnWait`.
- Test dengan `httptest`: openai-compatible non-stream + stream; anthropic; gemini; timeout; SSRF ditolak.

### Task 10: router — resolusi, fallback, retry, circuit breaker
- Create: `internal/router/router.go`, `internal/router/breaker.go`, `internal/router/router_test.go`
- `Resolve(nameOrAlias) ([]Step, error)` (combo urut posisi, alias/model tunggal → 1 langkah); `Step{ModelID, ProviderID, TimeoutMs}`.
- Fallback classifier (FR-2.2): retryable = timeout, 5xx, 429, 402, konten kosong; non-retry = 400, 401 invalid (setelah tandai), content-filter. Retry: 1× credential lain di provider sama (FR-2.4), lalu langkah berikutnya.
- Breaker per provider: ≥5 gagal/60 dtk → open 30 dtk → half-open (FR-2.5).
- Test: urutan fallback, retry credential, breaker state transitions, 400 tidak fallback.

### Task 11: apigate — auth key, rate limit, kuota
- Create: `internal/apigate/auth.go`, `internal/apigate/limiter.go`, `internal/apigate/*_test.go`
- `Authenticate(r) (*AuthContext, *APIError)`: Bearer hash → cache (memori, TTL 30s, invalidasi saat revoke) → status user/key, expires_at, IP allowlist, allowed_models (list/combo). RPM/TPM sliding window in-memory per key.
- Error JSON sesuai §9 (401 invalid_api_key, 403 model_not_allowed, 404 model_not_found, 429 quota_exceeded/rate_limited + `Retry-After`).
- Test: key valid/revoked/expired, IP allowlist, model tidak diizinkan, RPM limit, cache invalidasi.

### Task 12: usage — pencatatan asinkron, biaya, reset kuota
- Create: `internal/usage/recorder.go`, `internal/usage/cost.go`, `internal/usage/recorder_test.go`
- `Recorder.Record(LogEntry)` — kanal buffer + worker flush batch; kegagalan log tidak menggagalkan request (NFR-10). Kurangi kuota `used_tokens/used_cost` atomik; reset periodik zona `JR_TZ` (interval scheduler 1 menit, `reset_at`).
- Cost = tokens×harga/1M; model lokal 0 (FR-6.6). `/v1/usage/me` data dari sini.
- Test: flush batch, cost hitung, reset kuota lewat waktu (injeksi clock).

### Task 13: orkestrasi inferensi + endpoint /v1/*
- Create: `internal/api/inference.go`, `internal/api/inference_test.go`, `internal/api/helpers.go`
- `POST /v1/chat/completions`: parse OpenAI → resolve → per langkah translate+send → stream balik (Flusher per chunk, `X-Route-*`, error format OpenAI) → usage record.
- `POST /v1/messages`: idem format Anthropic (header `x-api-key`/`anthropic-version` juga diterima).
- `GET /v1/models` (model+combo yang diizinkan key ini), `GET /v1/usage/me`.
- TTFT diukur; token usage → estimated bila provider tak kirim (FR-3.5).
- Test e2e httptest dengan mock provider OpenAI (stream & non-stream, 429 lalu sukses → fallback, header X-Route-Attempts=2) dan mock Anthropic; error format sesuai endpoint.

### Task 14: auth dashboard + setup wizard + audit + /api/admin/*
- Create: `internal/api/auth_handlers.go`, `internal/api/admin_handlers.go`, `internal/api/admin_test.go`
- `POST /api/setup` (hanya bila belum ada user super_admin; validasi password kuat ≥12 char), `POST /api/login` (rate limit 5/15m/IP NFR-05), `POST /api/logout`, `GET /api/me`, CSRF double-submit untuk POST/PATCH/PUT/DELETE (header `X-CSRF-Token`).
- Admin API sesuai §9: providers CRUD+test+sync-models; credentials POST/DELETE; models GET/PATCH; combos CRUD; users CRUD; keys POST/DELETE (plain ditampilkan sekali); quota PUT (user/key); logs GET filter from/to/user/provider/status + `logs/export.csv`; `analytics/summary` (hari ini/7 hari: request, token, biaya per user/provider, error rate, latensi p50/p95); `local/status|models/{name}/start|stop`; `system/backup` (copy sqlite file → data/backups, retensi 7 NFR-12); RBAC (admin hanya kelola user non-admin, super_admin penuh).
- Test: setup sekali, login rate limit, CSRF ditolak tanpa header, RBAC, alur buat provider→key→logs.

### Task 15: metrics + backup scheduler
- Create: `internal/metrics/metrics.go`, `internal/metrics/metrics_test.go`
- Counter/gauge/histogram minimal-format 0.0.4: `jr_requests_total{provider,status}`, `jr_request_duration_seconds` (bucket), `jr_tokens_total{direction}`, `jr_upstream_errors_total{provider,code}`, `jr_circuit_state{provider}`, `jr_quota_active`. `/metrics` publik loopback-only bind note + text render.
- Test: render mengandung HELP/TYPE dan sample benar.

### Task 16: UI statis (dashboard + playground)
- Create: `web/index.html`, `web/css/app.css`, `web/js/api.js`, `web/js/i18n.js`, `web/js/app.js`, `web/js/pages/*.js` (login, dashboard, providers, models, combos, users, keys, quotas, logs, analytics, settings, local, chat), `internal/web/web.go` (embed)
- SPA hash-router, fetch `X-CSRF-Token`, i18n id/en (NFR-16), dark/light toggle, responsif.
- Chat playground: SSE streaming via fetch reader, render markdown sederhana (heading/bold/italic/code/list/link), pilih model/combo diizinkan, tampilkan sisa kuota + provider penjawab (FR-7.5), riwayat tersimpan (`/api/me/conversations` CRUD — ditambahkan ke Task 14).
- Wizard first-run bila `GET /api/setup/status` → perlu setup.
- Verifikasi: buka via browser-use (screenshot) setelah binary jalan.

### Task 17: deployment Ubuntu
- Create: `deploy/Dockerfile`, `deploy/docker-compose.yml`, `deploy/Caddyfile`, `deploy/jenderalrouter.service`, `deploy/install-ubuntu.sh`, `scripts/build.sh`
- Dockerfile multi-stage (golang:1.27 → distroless/static), `CGO_ENABLED=0`, TARGETARCH. compose: app (network host opsional via profil `local-llm`), caddy, postgres/redis berkomentar-opsional. install.sh: deteksi 22.04/24.04/26.04, buat user sistem, install binary+systemd atau docker compose, UFW hanya 80/443, wizard env.
- build.sh: lint/vet/test → build linux amd64+arm64 → embed UI otomatis.
- Verifikasi: docker build lokal bila docker ada; else build binary + jalankan systemd-style smoke.

### Task 18: README + dokumentasi operasi
- Create: `README.md` (id), `docs/OPERATIONS.md`
- Isi: instalasi ≤10 menit (tujuan metrik), env vars, contoh SDK OpenAI/Anthropic + Claude Code (`ANTHROPIC_BASE_URL`), combo, LlamaStash setup + panduan kapasitas §6, backup/restore, keamanan.

### Task 19: verifikasi akhir
- `go vet ./...`, `go test ./... -cover` (translator ≥80%), build amd64+arm64, smoke e2e: jalankan binary → setup wizard via curl → tambah provider mock → buat key → chat completions stream/non-stream → /v1/messages → usage/me → logs muncul → dashboard screenshot.
- checklist PRD F-01..F-15 dicocokkan; hasil dilaporkan.
