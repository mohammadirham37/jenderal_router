<script lang="ts">
	import { Gauge, KeyRound, Copy, Activity, RefreshCw, Bot, Plug } from '@lucide/svelte';
	import { browser } from '$app/env';
	import { api, fmtTs } from '#lib/api.js';
	import { copyText } from '#lib/clipboard.js';
	import { t, toast } from '#lib/stores.svelte.js';
	import Stat from '#lib/components/Stat.svelte';
	import ApiGuide from '#lib/components/ApiGuide.svelte';

	const origin = browser ? window.location.origin : '';
	const baseURL = origin + '/v1';

	type Key = {
		id: number; prefix: string; name: string; revoked_at: string;
		rpm: number; tpm: number; has_secret: boolean; created_at: string;
	};

	let keys = $state<Key[]>([]);
	// transparansi: key yang isi request-nya sedang dicatat admin
	let contentLogged = $state<Record<string, boolean>>({});
	let retentionDays = $state(7);
	let logs = $state<any[]>([]);
	let usage = $state<any>(null);
	let loading = $state(true);
	let revealing = $state(0);
	let revealed = $state<Record<number, string>>({});
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
				contentLogged = k.content_logged || {};
				retentionDays = k.content_retention_days || 7;
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
		await copyText(newKeyPlain)
			? toast('key disalin ✓', 'ok')
			: toast('gagal menyalin — salin manual', 'err');
	}

	async function copyKey(k: Key) {
		try {
			if (revealed[k.id]) {
				await copyText(revealed[k.id])
					? toast('key disalin ✓', 'ok')
					: toast('gagal menyalin', 'err');

				return;
			}
			revealing = k.id;
			const r = await api.get(`/api/me/keys/${k.id}/reveal`);
			revealed[k.id] = r.plaintext;
			revealing = 0;

			await copyText(r.plaintext)
				? toast('key disalin ✓', 'ok')
				: toast('gagal menyalin — salin manual', 'err');
		} catch(e: any) {
			revealing = 0;
			toast(e.message, 'err');
		}
	}

	// key aktif untuk panduan koneksi (yang punya salinan terenkripsi)
	const guideKey = $derived(activeKeys.find((k) => k.has_secret) ?? null);
	async function guideGetKey(): Promise<string> {
		const k = guideKey;
		if (!k) throw new Error('belum ada API key aktif');
		if (!revealed[k.id]) {
			const r = await api.get(`/api/me/keys/${k.id}/reveal`);
			revealed[k.id] = r.plaintext;
		}
		return revealed[k.id];
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
				{#if contentLogged[k.id] && !k.revoked_at}
					<span class="badge warn" title="Admin menyalakan pencatatan isi request untuk akun/key ini. Isi disimpan {retentionDays} hari lalu dihapus otomatis.">isi request dicatat ({retentionDays} hari)</span>
				{/if}
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
		<h2><Plug size={15} /> Cara menghubungkan aplikasi</h2>
		<ApiGuide keyPrefix={guideKey?.prefix ?? ''} getKey={guideGetKey} onGenerate={generateKey} {generating} />
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
