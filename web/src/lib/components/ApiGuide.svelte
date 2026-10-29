<script lang="ts">
	// Panduan koneksi API langkah demi langkah — dipakai halaman Penggunaan API
	// (member) dan modal API key (admin). Contoh konfigurasi selalu memakai
	// model yang benar-benar aktif untuk API; key di layar disamarkan, tombol
	// Salin memakai key asli.
	import { KeyRound, Copy, Check, Boxes, Plug, PlayCircle, CircleCheck, CircleAlert, ChevronRight } from '@lucide/svelte';
	import { browser } from '$app/env';
	import { api } from '#lib/api.js';
	import { copyText } from '#lib/clipboard.js';
	import { toast } from '#lib/stores.svelte.js';

	let {
		keyPrefix = '',
		getKey,
		onGenerate,
		generating = false
	}: {
		keyPrefix?: string; // kosong = belum punya key aktif
		getKey: () => Promise<string>;
		onGenerate?: () => void;
		generating?: boolean;
	} = $props();

	const origin = browser ? window.location.origin : 'https://ai.domainanda.com';
	const baseURL = origin + '/v1';
	const hasKey = $derived(!!keyPrefix);

	// ---- model yang aktif untuk API ----
	let models = $state<{ id: string; kind: string; owner: string; local: boolean }[]>([]);
	let model = $state('');
	let modelsLoaded = $state(false);
	$effect(() => {
		api.get('/api/me/api-models')
			.then((r) => {
				models = r.models || [];
				model = models.find((m) => m.kind === 'model')?.id ?? models[0]?.id ?? '';
			})
			.catch(() => {})
			.finally(() => (modelsLoaded = true));
	});
	const modelId = $derived(model || 'NAMA-MODEL');

	// ---- key: disamarkan di layar, asli saat disalin/dites ----
	let realKey = $state('');
	let showKey = $state(false);
	async function ensureKey(): Promise<string> {
		if (!realKey) realKey = await getKey();
		return realKey;
	}
	const shownKey = $derived(showKey && realKey ? realKey : hasKey ? keyPrefix + '••••••••' : 'API_KEY_ANDA');
	async function toggleShow() {
		if (!showKey) {
			try { await ensureKey(); } catch (e: any) { toast(e.message, 'err'); return; }
		}
		showKey = !showKey;
	}

	// ---- aplikasi tujuan ----
	type App = { id: string; label: string; hint: string; lang: string; steps: string[]; code: (k: string) => string };
	const APPS: App[] = [
		{
			id: 'python', label: 'Python', hint: 'skrip & notebook', lang: 'python',
			steps: ['Pasang library: <code>pip install openai</code>', 'Simpan kode di bawah sebagai <code>tes.py</code>', 'Jalankan: <code>python tes.py</code>'],
			code: (k) => `from openai import OpenAI\n\nclient = OpenAI(\n    base_url="${baseURL}",\n    api_key="${k}",\n)\n\njawaban = client.chat.completions.create(\n    model="${modelId}",\n    messages=[{"role": "user", "content": "Halo, perkenalkan dirimu"}],\n)\nprint(jawaban.choices[0].message.content)`
		},
		{
			id: 'js', label: 'JavaScript', hint: 'Node.js / web', lang: 'javascript',
			steps: ['Pasang library: <code>npm install openai</code>', 'Simpan kode di bawah sebagai <code>tes.mjs</code>', 'Jalankan: <code>node tes.mjs</code>'],
			code: (k) => `import OpenAI from 'openai';\n\nconst client = new OpenAI({\n  baseURL: '${baseURL}',\n  apiKey: '${k}',\n});\n\nconst jawaban = await client.chat.completions.create({\n  model: '${modelId}',\n  messages: [{ role: 'user', content: 'Halo, perkenalkan dirimu' }],\n});\nconsole.log(jawaban.choices[0].message.content);`
		},
		{
			id: 'curl', label: 'curl', hint: 'tes cepat di terminal', lang: 'bash',
			steps: ['Buka terminal (Linux/macOS, atau Git Bash di Windows)', 'Tempel perintah di bawah lalu tekan Enter', 'Jawaban model ada di bagian <code>"content"</code>'],
			code: (k) => `curl ${baseURL}/chat/completions \\\n  -H "Authorization: Bearer ${k}" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model": "${modelId}", "messages": [{"role": "user", "content": "Halo"}]}'`
		},
		{
			id: 'vscode', label: 'Kilo Code / Cline / Roo', hint: 'ekstensi VS Code', lang: 'text',
			steps: [
				'Buka ekstensi di VS Code → ikon ⚙️ <b>Settings</b>',
				'Pada <b>API Provider</b> pilih <b>OpenAI Compatible</b>',
				'Isi <b>Base URL</b>, <b>API Key</b>, dan <b>Model ID</b> persis seperti di bawah',
				'Klik <b>Save / Done</b>, lalu mulai chat di panel ekstensi'
			],
			code: (k) => `API Provider : OpenAI Compatible\nBase URL     : ${baseURL}\nAPI Key      : ${k}\nModel ID     : ${modelId}`
		},
		{
			id: 'opencode', label: 'OpenCode', hint: 'agen coding di terminal', lang: 'json',
			steps: [
				'Buat file <code>opencode.json</code> di folder proyek (atau <code>~/.config/opencode/opencode.json</code> agar berlaku global)',
				'Tempel isi di bawah lalu simpan',
				'Jalankan <code>opencode</code>, ketik <code>/models</code>, pilih model <b>JenderalRouter</b>'
			],
			code: (k) => JSON.stringify({
				$schema: 'https://opencode.ai/config.json',
				provider: {
					jenderal: {
						npm: '@ai-sdk/openai-compatible',
						name: 'JenderalRouter',
						options: { baseURL, apiKey: k },
						models: { [modelId]: { name: modelId } }
					}
				}
			}, null, 2)
		},
		{
			id: 'claude', label: 'Claude Code', hint: 'via endpoint Anthropic', lang: 'bash',
			steps: [
				'Base URL untuk Claude Code <b>tanpa</b> <code>/v1</code> (format Anthropic)',
				'Jalankan baris di bawah di terminal, lalu ketik <code>claude</code>',
				'Windows PowerShell: ganti <code>export NAMA=nilai</code> menjadi <code>$env:NAMA="nilai"</code>'
			],
			code: (k) => `export ANTHROPIC_BASE_URL=${origin}\nexport ANTHROPIC_API_KEY=${k}\nexport ANTHROPIC_MODEL=${modelId}\nclaude`
		},
		{
			id: 'chatapp', label: 'Aplikasi chat', hint: 'Chatbox, Cherry Studio, Open WebUI', lang: 'text',
			steps: [
				'Tambahkan provider/model baru bertipe <b>OpenAI</b> atau <b>OpenAI-compatible</b>',
				'Isi kolom sesuai tabel di bawah (beberapa aplikasi menyebut Base URL sebagai <i>API Host</i> / <i>Endpoint</i>)',
				'Simpan, pilih modelnya, lalu mulai chat'
			],
			code: (k) => `Tipe provider : OpenAI Compatible\nBase URL      : ${baseURL}\nAPI Key       : ${k}\nModel         : ${modelId}`
		}
	];
	let appId = $state('python');
	const app = $derived(APPS.find((a) => a.id === appId)!);
	const snippet = $derived(app.code(shownKey));

	let copied = $state('');
	async function copyValue(label: string, value: string) {
		if (await copyText(value)) {
			copied = label;
			setTimeout(() => (copied = copied === label ? '' : copied), 1500);
		} else toast('gagal menyalin — blok teks lalu salin manual', 'err');
	}
	async function copyWithKey(label: string, build: (k: string) => string) {
		try { await copyValue(label, build(hasKey ? await ensureKey() : 'API_KEY_ANDA')); }
		catch (e: any) { toast(e.message, 'err'); }
	}

	// ---- tes koneksi langsung dari browser ----
	let testing = $state(false);
	let result = $state<{ ok: boolean; title: string; detail: string; ms?: number } | null>(null);
	const ERRORS: Record<number, string> = {
		401: 'API key salah, sudah dicabut, atau kedaluwarsa. Salin ulang key dari langkah 1.',
		403: 'Key ini tidak diizinkan memakai model tersebut, atau IP Anda tidak ada di allowlist key.',
		404: 'Model tidak ditemukan atau tidak diaktifkan untuk API. Pilih model lain di langkah 2.',
		429: 'Batas request/token per menit atau kuota tercapai. Tunggu sebentar lalu coba lagi.',
		502: 'Provider AI di belakang gateway sedang bermasalah. Coba lagi atau pilih model lain.',
		503: 'Layanan sementara tidak tersedia. Coba lagi beberapa saat lagi.'
	};
	async function testConnection() {
		testing = true;
		result = null;
		const t0 = performance.now();
		try {
			const key = await ensureKey();
			const res = await fetch(baseURL + '/chat/completions', {
				method: 'POST',
				headers: { Authorization: 'Bearer ' + key, 'Content-Type': 'application/json' },
				body: JSON.stringify({ model: modelId, max_tokens: 60, messages: [{ role: 'user', content: 'Balas singkat: koneksi berhasil?' }] })
			});
			const ms = Math.round(performance.now() - t0);
			const j = await res.json().catch(() => ({}));
			if (res.ok) {
				const text = (j.choices?.[0]?.message?.content || '').trim();
				result = { ok: true, ms, title: 'Terhubung!', detail: `Model ${res.headers.get('X-Route-Model') || modelId} menjawab: “${text.slice(0, 160) || '(kosong)'}”` };
			} else {
				const msg = j.error?.message || j.error || `HTTP ${res.status}`;
				result = { ok: false, ms, title: `Gagal (HTTP ${res.status})`, detail: (ERRORS[res.status] ?? 'Terjadi kesalahan.') + ' Pesan server: ' + msg };
			}
		} catch (e: any) {
			result = { ok: false, title: 'Tidak bisa menghubungi server', detail: e.message };
		} finally {
			testing = false;
		}
	}
</script>

<div class="guide">
	<!-- ringkasan 3 nilai yang selalu dibutuhkan -->
	<div class="essentials">
		<div class="ess">
			<span class="lbl">Base URL</span>
			<code>{baseURL}</code>
			<button class="mini" onclick={() => copyValue('base', baseURL)} aria-label="Salin Base URL">
				{#if copied === 'base'}<Check size={13} />{:else}<Copy size={13} />{/if}
			</button>
		</div>
		<div class="ess">
			<span class="lbl">API Key</span>
			<code>{shownKey}</code>
			{#if hasKey}
				<button class="mini" onclick={() => copyWithKey('key', (k) => k)} aria-label="Salin API key">
					{#if copied === 'key'}<Check size={13} />{:else}<Copy size={13} />{/if}
				</button>
			{/if}
		</div>
		<div class="ess">
			<span class="lbl">Model</span>
			<code>{modelId}</code>
			<button class="mini" onclick={() => copyValue('model', modelId)} aria-label="Salin model">
				{#if copied === 'model'}<Check size={13} />{:else}<Copy size={13} />{/if}
			</button>
		</div>
		<p class="ess-note">Tiga nilai ini yang diminta hampir semua aplikasi. API kompatibel <b>OpenAI</b> (dan <b>Anthropic</b> untuk Claude Code).</p>
	</div>

	<ol class="steps">
		<!-- 1. key -->
		<li class:done={hasKey}>
			<div class="step-head"><span class="num">1</span><KeyRound size={15} /> Siapkan API key</div>
			<div class="step-body">
				{#if hasKey}
					<p>Key aktif Anda: <code>{keyPrefix}…</code>. Simpan seperti password — jangan dibagikan atau di-commit ke Git.</p>
					<div class="row-actions">
						<button class="btn ghost sm" onclick={() => copyWithKey('key2', (k) => k)}>
							{#if copied === 'key2'}<Check size={13} />{:else}<Copy size={13} />{/if} Salin key
						</button>
						<button class="btn ghost sm" onclick={toggleShow}>{showKey ? 'Sembunyikan' : 'Tampilkan'} key di contoh</button>
					</div>
				{:else if onGenerate}
					<p>Anda belum punya API key. Buat satu — key hanya ditampilkan penuh sekali, jadi langsung salin.</p>
					<button class="btn sm" onclick={onGenerate} disabled={generating}>
						{#if generating}<span class="spinner"></span>{:else}<KeyRound size={13} />{/if} Buat API key
					</button>
				{:else}
					<p>Belum ada API key aktif. Buat key terlebih dahulu.</p>
				{/if}
			</div>
		</li>

		<!-- 2. model -->
		<li class:done={!!model}>
			<div class="step-head"><span class="num">2</span><Boxes size={15} /> Pilih model</div>
			<div class="step-body">
				{#if modelsLoaded && models.length === 0}
					<p class="warn-text">Belum ada model yang diaktifkan untuk API. Minta admin mencentang kolom <b>API</b> di halaman Model.</p>
				{:else}
					<p>Nama model ditulis <b>persis</b> seperti ini di aplikasi Anda (huruf besar-kecil berpengaruh).</p>
					<select bind:value={model} aria-label="Pilih model">
						{#each models as m (m.id)}
							<option value={m.id}>{m.id} — {m.kind === 'combo' ? 'combo (fallback otomatis)' : m.owner}{m.local ? ' · lokal' : ''}</option>
						{/each}
					</select>
				{/if}
			</div>
		</li>

		<!-- 3. aplikasi -->
		<li>
			<div class="step-head"><span class="num">3</span><Plug size={15} /> Hubungkan aplikasi Anda</div>
			<div class="step-body">
				<div class="apps" role="tablist">
					{#each APPS as a (a.id)}
						<button role="tab" aria-selected={appId === a.id} class:on={appId === a.id} onclick={() => (appId = a.id)}>
							<b>{a.label}</b><span>{a.hint}</span>
						</button>
					{/each}
				</div>
				<ol class="howto">
					{#each app.steps as s}
						<!-- teks langkah statis milik komponen ini, bukan input pengguna -->
						<li>{@html s}</li>
					{/each}
				</ol>
				<div class="snippet">
					<div class="snip-head">
						<span>{app.lang}</span>
						<button class="mini wide" onclick={() => copyWithKey('snip', (k) => app.code(k))}>
							{#if copied === 'snip'}<Check size={13} /> Tersalin{:else}<Copy size={13} /> Salin (dengan key asli){/if}
						</button>
					</div>
					<pre><code>{snippet}</code></pre>
				</div>
			</div>
		</li>

		<!-- 4. tes -->
		<li class:done={result?.ok}>
			<div class="step-head"><span class="num">4</span><PlayCircle size={15} /> Tes koneksi</div>
			<div class="step-body">
				<p>Kirim pesan percobaan singkat dengan key &amp; model di atas — memastikan semuanya benar sebelum dipakai di aplikasi.</p>
				<button class="btn sm" onclick={testConnection} disabled={testing || !hasKey || !model}>
					{#if testing}<span class="spinner"></span> Menguji…{:else}<PlayCircle size={14} /> Tes sekarang{/if}
				</button>
				{#if result}
					<div class="result" class:ok={result.ok}>
						{#if result.ok}<CircleCheck size={16} />{:else}<CircleAlert size={16} />{/if}
						<div>
							<b>{result.title}</b>{#if result.ms} <span class="muted">· {result.ms} ms</span>{/if}
							<div>{result.detail}</div>
						</div>
					</div>
				{/if}
			</div>
		</li>
	</ol>

	<details class="trouble">
		<summary><ChevronRight size={14} /> Kalau gagal — arti kode error</summary>
		<table>
			<tbody>
				{#each Object.entries(ERRORS) as [code, text] (code)}
					<tr><td><code>{code}</code></td><td>{text}</td></tr>
				{/each}
				<tr><td><code>—</code></td><td>Pastikan Base URL diakhiri <code>/v1</code> (kecuali Claude Code), tanpa garis miring di akhir, dan perangkat Anda bisa membuka {origin}.</td></tr>
			</tbody>
		</table>
	</details>
</div>

<style>
	.guide { display: flex; flex-direction: column; gap: 14px; }
	.essentials {
		display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px;
		padding: 12px; border-radius: 14px; background: var(--grad-accent-soft);
		border: 1px solid color-mix(in srgb, var(--accent) 25%, var(--border));
	}
	.ess { display: flex; align-items: center; gap: 8px; min-width: 0; padding: 8px 10px; border-radius: 10px; background: var(--surface); border: 1px solid var(--border); }
	.ess .lbl { font-size: 10.5px; font-weight: 800; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); white-space: nowrap; }
	.ess code { flex: 1; min-width: 0; font-family: var(--mono); font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ess-note { grid-column: 1 / -1; margin: 2px 2px 0; font-size: 12px; color: var(--muted); }
	.mini {
		all: unset; cursor: pointer; display: inline-flex; align-items: center; gap: 5px; flex: 0 0 auto;
		padding: 4px 6px; border-radius: 7px; color: var(--muted); font-size: 11.5px;
		transition: color var(--speed), background var(--speed);
	}
	.mini:hover { color: var(--text); background: var(--surface-2); }
	.mini.wide { padding: 3px 8px; }

	.steps { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }
	.steps > li { position: relative; padding: 0 0 18px 40px; }
	.steps > li::before { /* garis penghubung antar langkah */
		content: ''; position: absolute; left: 13px; top: 30px; bottom: 2px; width: 2px; background: var(--border);
	}
	.steps > li:last-child::before { display: none; }
	.step-head { display: flex; align-items: center; gap: 8px; font-weight: 750; font-size: 14px; min-height: 28px; }
	.num {
		position: absolute; left: 0; top: 0; width: 28px; height: 28px; border-radius: 50%; display: grid; place-items: center;
		font-size: 13px; font-weight: 800; background: var(--surface-3); border: 1px solid var(--border-strong); color: var(--text);
	}
	.steps > li.done .num { background: var(--grad-accent); color: #fff; border-color: transparent; }
	.step-body { margin-top: 6px; font-size: 13px; }
	.step-body p { margin: 0 0 8px; color: var(--muted); }
	.step-body code, .howto :global(code) { font-family: var(--mono); font-size: 12px; padding: 1px 5px; border-radius: 5px; background: var(--surface-3); border: 1px solid var(--border); }
	.step-body select { max-width: 520px; }
	.row-actions { display: flex; gap: 8px; flex-wrap: wrap; }
	.warn-text { color: var(--warn) !important; }

	.apps { display: grid; grid-template-columns: repeat(auto-fill, minmax(128px, 1fr)); gap: 6px; margin-bottom: 12px; }
	.apps button {
		all: unset; cursor: pointer; padding: 8px 10px; border-radius: 10px; border: 1px solid var(--border);
		display: flex; flex-direction: column; gap: 1px; background: var(--surface);
		transition: border-color var(--speed), background var(--speed);
	}
	.apps button:hover { border-color: var(--border-strong); }
	.apps button.on { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 10%, var(--surface)); }
	.apps b { font-size: 12.5px; }
	.apps span { font-size: 11px; color: var(--muted); }
	.howto { margin: 0 0 10px; padding-left: 20px; display: flex; flex-direction: column; gap: 4px; }
	.howto li { line-height: 1.55; }
	.snippet { border: 1px solid var(--border); border-radius: 12px; overflow: hidden; background: var(--bg); }
	.snip-head {
		display: flex; justify-content: space-between; align-items: center; padding: 5px 8px 5px 12px;
		font-family: var(--mono); font-size: 11px; color: var(--muted); background: var(--surface-2); border-bottom: 1px solid var(--border);
	}
	.snippet pre { margin: 0; padding: 12px 14px; overflow-x: auto; font-size: 12.5px; line-height: 1.6; }
	.snippet code { font-family: var(--mono); background: none; border: 0; padding: 0; font-size: inherit; }

	.result {
		display: flex; gap: 10px; align-items: flex-start; margin-top: 10px; padding: 10px 12px; border-radius: 10px;
		background: color-mix(in srgb, var(--err) 10%, transparent); border: 1px solid color-mix(in srgb, var(--err) 35%, transparent);
		color: var(--text); font-size: 12.5px; animation: res-in 200ms ease;
	}
	.result.ok { background: color-mix(in srgb, var(--ok) 10%, transparent); border-color: color-mix(in srgb, var(--ok) 35%, transparent); }
	.result :global(svg) { flex: 0 0 auto; margin-top: 1px; color: var(--err); }
	.result.ok :global(svg) { color: var(--ok); }
	@keyframes res-in { from { opacity: 0; transform: translateY(4px); } to { opacity: 1; transform: none; } }

	.trouble summary { cursor: pointer; display: inline-flex; align-items: center; gap: 6px; font-size: 12.5px; font-weight: 700; color: var(--muted); list-style: none; }
	.trouble summary::-webkit-details-marker { display: none; }
	.trouble[open] summary :global(svg) { transform: rotate(90deg); }
	.trouble table { margin-top: 8px; width: 100%; font-size: 12.5px; border-collapse: collapse; }
	.trouble td { padding: 6px 8px; border-bottom: 1px solid var(--border); vertical-align: top; }
	.trouble td:first-child { width: 56px; }

	@media (max-width: 760px) {
		.essentials { grid-template-columns: 1fr; }
		.steps > li { padding-left: 36px; }
	}
</style>
