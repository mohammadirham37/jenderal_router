<script lang="ts">
	import { Gauge, KeyRound, Copy, Activity, RefreshCw, Bot } from '@lucide/svelte';
	import { browser } from '$app/environment';
	import { api, fmtTs } from '$lib/api';
	import { copyText } from '$lib/clipboard';
	import { t, toast } from '$lib/stores.svelte';
	import Stat from '$lib/components/Stat.svelte';

	const origin = browser ? window.location.origin : '';
	const baseURL = origin + '/v1';
	const contohModel = 'local/Qwen3-30B-A3B-Q6_K';

	type Key = {
		id: number; prefix: string; name: string; revoked_at: string;
		rpm: number; tpm: number; has_secret: boolean; created_at: string;
	};

	let keys = $state<Key[]>([]);
	let logs = $state<any[]>([]);
	let usage = $state<any>(null);
	let loading = $state(true);
	let revealing = $state(0);
	let revealed = $state<Record<number, string>>({});
	let guideKey = $state<number | 'manual'>('manual');
	let manualKey = $state('');
	let guideTab = $state<'opencode' | 'kilocode' | 'curl'>('opencode');
	// key baru hasil generate — plaintext hanya tampil sekali
	let newKeyPlain = $state('');
	let newKeyId = $state<number | null>(null);
	let generating = $state(false);

	$effect(() => {
		(async () => {
			try {
				const [k, u, l] = await Promise.all([
					api.get('/api/me/keys'),
					api.get('/api/me/usage'),
					api.get('/api/me/logs?limit=20')
				]);
				keys = k.keys || [];
				usage = u;
				logs = l.logs || [];
			} catch (e: any) {
				toast(e.message, 'err');
			} finally {
				loading = false;
			}
		})();
	});

	const today = $derived(usage?.today ?? {});
	const activeKeys = $derived(keys.filter((k) => !k.revoked_at));

	// generate API key untuk diri sendiri (1 user = 1 key aktif)
	async function generateKey() {
		generating = true;
		try {
			const r = await api.post('/api/me/keys', {});
			newKeyPlain = r.plaintext;
			newKeyId = r.key.id;
			revealed[r.key.id] = r.plaintext;
			selectedKeyId = r.key.id;
			const res = await api.get('/api/me/keys');
			keys = res.keys || [];
			toast('API key berhasil dibuat ✓', 'ok');
		} catch (e: any) {
			toast(e.message, 'err');
		} finally {
			generating = false;
		}
	}
	async function copyNewKey() {
		(await copyText(newKeyPlain)) ? toast('key disalin ✓', 'ok') : toast('gagal menyalin — salin manual', 'err');
	}

	async function copyKey(k: Key) {
		try {
			if (revealed[k.id]) {
				(await copyText(revealed[k.id])) ? toast('key disalin ✓', 'ok') : toast('gagal menyalin', 'err');
				return;
			}
			revealing = k.id;
			const r = await api.get(`/api/me/keys/${k.id}/reveal`);
			revealed[k.id] = r.plaintext;
			revealing = 0;
			(await copyText(r.plaintext)) ? toast('key disalin ✓', 'ok') : toast('gagal menyalin — salin manual', 'err');
		} catch (e: any) {
			revealing = 0;
			toast(e.message, 'err');
		}
	}

	// key aktif untuk panduan koneksi
	const guideKeys = $derived(activeKeys.filter((k) => k.has_secret));
	let selectedKeyId = $state<number | null>(null);
	$effect(() => {
		if (guideKeys.length && selectedKeyId === null) selectedKeyId = guideKeys[0].id;
	});

	async function guideKeyText(): Promise<string> {
		if (selectedKeyId) {
			if (!revealed[selectedKeyId]) {
				const r = await api.get(`/api/me/keys/${selectedKeyId}/reveal`);
				revealed[selectedKeyId] = r.plaintext;
			}
			return revealed[selectedKeyId];
		}
		return manualKey || 'sk-… (tempel key Anda di atas)';
	}

	function shortModel(m: string): string {
		return m.split('/').pop()?.replace(/\.gguf$/i, '') || m;
	}

	let guideSnippet = $state('');
	let guideBusy = $state(false);
	async function buildGuide() {
		guideBusy = true;
		const key = await guideKeyText();
		const model = contohModel;
		if (guideTab === 'opencode') {
			guideSnippet = JSON.stringify(
				{
					$schema: 'https://opencode.ai/config.json',
					provider: {
						jenderal: {
							npm: '@ai-sdk/openai-compatible',
							name: 'JenderalRouter (lokal)',
							options: { baseURL, apiKey: key },
							models: { [model]: { name: 'Qwen3 30B (lokal)' } }
						}
					}
				},
				null,
				2
			);
		} else if (guideTab === 'kilocode') {
			guideSnippet = [
				'Provider  : OpenAI Compatible',
				'Base URL  : ' + baseURL,
				'API Key   : ' + key,
				'Model ID  : ' + model
			].join('\n');
		} else {
			guideSnippet = `curl ${baseURL}/chat/completions \\\n  -H "Authorization: Bearer ${key}" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${model}","messages":[{"role":"user","content":"halo"}]}'`;
		}
		guideBusy = false;
	}
	async function copyGuide() {
		if (!guideSnippet) await buildGuide();
		(await copyText(guideSnippet)) ? toast('konfigurasi disalin ✓', 'ok') : toast('gagal menyalin', 'err');
	}

</script>

<svelte:head><title>{t('usage_api')} — JenderalRouter</title></svelte:head>

<div class="page-head">
	<div>
		<h1 class="page-title"><Gauge size={20} /> {t('usage_api')}</h1>
		<div class="subtitle">{t('my_usage')} — API kompatibel OpenAI</div>
	</div>
	<button class="btn ghost sm" onclick={() => location.reload()}><RefreshCw size={14} /></button>
</div>

{#if loading}
	<div class="card"><span class="spinner dark"></span> {t('loading')}…</div>
{:else}
	<div class="grid cols-3">
		<Stat icon={Activity} label={t('requests_today')} value={String(today.requests ?? 0)} />
		<Stat icon={Bot} label={t('tokens_today')} value={String((today.tokens_in ?? 0) + (today.tokens_out ?? 0))} />
		<Stat icon={KeyRound} label={t('keys')} value={String(activeKeys.length)} sub={keys.length - activeKeys.length > 0 ? `${keys.length - activeKeys.length} dicabut` : undefined} />
	</div>

	<div class="card" style="margin-top:14px">
		<h2><KeyRound size={15} /> API Key saya</h2>
		{#if newKeyPlain}
			<div style="padding:12px;border-radius:10px;background:color-mix(in srgb, var(--warn) 12%, transparent);border:1px solid color-mix(in srgb, var(--warn) 40%, transparent);margin-bottom:12px">
				<div class="small" style="color:var(--warn);font-weight:700;margin-bottom:6px">⚠ API key hanya tampil sekali — salin sekarang!</div>
				<div class="kv">
					<code class="mono" style="background:var(--bg);padding:10px;border-radius:8px;word-break:break-all;flex:1">{newKeyPlain}</code>
					<button class="btn sm" onclick={copyNewKey}><Copy size={13} /> Salin</button>
				</div>
			</div>
		{/if}
		{#each keys as k (k.id)}
			<div class="kv" style="border-bottom:1px solid var(--border);padding:7px 0">
				<span class="mono small">{k.prefix}…</span>
				<span class="small" style="font-weight:700">{k.name}</span>
				{#if k.revoked_at}<span class="badge err">revoked</span>
				{:else}<span class="badge ok"><span class="dot"></span>active</span>{/if}
				<span class="muted small">rpm={k.rpm} tpm={k.tpm}</span>
				<span class="grow"></span>
				{#if !k.revoked_at && k.has_secret}
					<button class="btn ghost sm" onclick={() => copyKey(k)}>
						{#if revealing === k.id}<span class="spinner"></span>{:else}<Copy size={12} />{/if}
						Salin key
					</button>
				{/if}
			</div>
		{:else}
			<div class="muted small" style="padding:8px 0">Belum punya API key — buat sendiri sekarang:</div>
		{/each}
		{#if activeKeys.length === 0}
			<div style="margin-top:10px">
				<button class="btn" onclick={generateKey} disabled={generating}>
					{#if generating}<span class="spinner"></span>{:else}<KeyRound size={14} />{/if}
					Generate API Key
				</button>
				<div class="muted small" style="margin-top:6px">1 user = 1 API key aktif. Key lama yang dicabut tidak dihitung.</div>
			</div>
		{/if}
		<div class="kv small" style="margin-top:8px">
			<span class="muted">Base URL:</span><span class="mono">{baseURL}</span>
		</div>
	</div>

	<div class="card" style="margin-top:14px">
		<h2><Activity size={15} /> Panduan koneksi</h2>
		<div class="row" style="margin-bottom:8px">
			<button class="btn sm" class:ghost={guideTab !== 'opencode'} onclick={() => (guideTab = 'opencode')}>OpenCode</button>
			<button class="btn sm" class:ghost={guideTab !== 'kilocode'} onclick={() => (guideTab = 'kilocode')}>Kilocode</button>
			<button class="btn sm" class:ghost={guideTab !== 'curl'} onclick={() => (guideTab = 'curl')}>curl</button>
			<span class="grow"></span>
			<button class="btn sm" onclick={buildGuide} disabled={guideBusy}>
				{#if guideBusy}<span class="spinner"></span>{:else}<KeyRound size={13} />{/if}
				Isi dengan key saya
			</button>
			<button class="btn ghost sm" onclick={copyGuide} disabled={!guideSnippet}><Copy size={13} /> Salin</button>
		</div>
		{#if guideKeys.length === 0}
			<div class="kv small" style="margin-bottom:8px">
				<span class="muted">Belum ada key yang bisa dipakai — isi manual:</span>
				<input style="max-width:380px" bind:value={manualKey} placeholder="tempel API key Anda (sk-…)" />
			</div>
		{/if}
		{#if guideSnippet}
			<pre class="logbox" style="max-height:300px;overflow:auto">{guideSnippet}</pre>
		{:else}
			<p class="muted small" style="margin:0">
				Klik "Isi dengan key saya" untuk membuat konfigurasi siap-tempel
				(endpoint: <span class="mono">{baseURL}</span>).
			</p>
		{/if}
	</div>

	<div class="card" style="margin-top:14px">
		<h2><Activity size={15} /> Aktivitas terbaru (7 hari)</h2>
		<div class="table-wrap">
			<table>
				<thead><tr><th>Waktu</th><th>Model</th><th>Status</th><th>Latensi</th><th>Token</th></tr></thead>
				<tbody>
					{#each logs as l (l.id)}
						<tr>
							<td class="mono">{fmtTs(l.ts)}</td>
							<td class="mono">{l.requested_model}</td>
							<td><span class="badge {l.status >= 200 && l.status < 400 ? 'ok' : 'err'}">{l.status}</span></td>
							<td>{l.latency_ms} ms</td>
							<td>{l.tokens_in}/{l.tokens_out}</td>
						</tr>
					{:else}
						<tr><td colspan="5" class="muted">Belum ada aktivitas</td></tr>
					{/each}
				</tbody>
			</table>
		</div>
		{#if usage?.quotas?.length}
			<div class="kv small" style="margin-top:10px">
				{#each usage.quotas as q (q.period)}
					<span class="badge info">{q.period}: {q.used_tokens}/{q.token_limit || '∞'} token</span>
				{/each}
			</div>
		{/if}
	</div>
{/if}
