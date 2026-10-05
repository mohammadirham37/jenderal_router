<script lang="ts">
	import { Users, Plus, Trash2, KeyRound, Gauge, Copy, X, Activity, Search } from '@lucide/svelte';
	import { browser } from '$app/environment';
	import { api, fmtNum, fmtCost, fmtTs } from '$lib/api';
	import { copyText } from '$lib/clipboard';
	import { t, toast } from '$lib/stores.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Stat from '$lib/components/Stat.svelte';

	let users = $state<any[]>([]);
	let keysByUser = $state<Record<number, any[]>>({});
	let usageByUser = $state<Record<number, any>>({});
	let loading = $state(true);
	let search = $state('');

	async function reload() {
		loading = true;
		try {
			const [res, u] = await Promise.all([
				api.get('/api/admin/users'),
				api.get('/api/admin/users/usage?days=7')
			]);
			users = (res.users || []).map((x: any) => x.user);
			const map: Record<number, any> = {};
			for (const pu of u.per_user || []) map[pu.user_id] = pu;
			usageByUser = map;
			await Promise.all(users.map((usr) => loadKeys(usr)));
		} catch (e: any) {
			toast(e.message, 'err');
		} finally {
			loading = false;
		}
	}
	$effect(() => { reload(); });

	async function loadKeys(u: any) {
		try {
			const res = await api.get(`/api/admin/users/${u.id}/keys`);
			keysByUser[u.id] = res.keys || [];
		} catch { keysByUser[u.id] = []; }
	}

	const filtered = $derived(
		users.filter((u) => !search || u.email.toLowerCase().includes(search.toLowerCase()))
	);
	const totals = $derived.by(() => {
		let req = 0, tok = 0, cost = 0, aktif = 0;
		for (const pu of Object.values(usageByUser) as any[]) {
			req += pu.requests || 0;
			tok += (pu.tokens_in || 0) + (pu.tokens_out || 0);
			cost += pu.cost_usd || 0;
			if ((pu.requests || 0) > 0) aktif++;
		}
		return { req, tok, cost, aktif };
	});

	// ---- buat user ----
	let showUser = $state(false);
	let uEmail = $state(''); let uPass = $state(''); let uRole = $state('member');
	async function createUser() {
		try {
			await api.post('/api/admin/users', { email: uEmail, password: uPass, role: uRole });
			showUser = false; uEmail = ''; uPass = '';
			toast(t('save') + ' ✓', 'ok');
			reload();
		} catch (e: any) { toast(e.message, 'err'); }
	}

	// ---- buat key ----
	let keyFor = $state<any>(null);
	let kName = $state(''); let kAllowed = $state('*'); let kRpm = $state('60'); let kTpm = $state('0');
	let plaintext = $state('');
	let guideTab = $state<'opencode' | 'kilocode' | 'curl'>('opencode');
	const baseURL = (browser ? window.location.origin : '') + '/v1';
	const contohModel = 'local/Qwen3-30B-A3B-Q6_K';

	async function createKey() {
		if (!keyFor) return;
		try {
			const r = await api.post(`/api/admin/users/${keyFor.id}/keys`, {
				name: kName, allowed_models: kAllowed, rpm: parseInt(kRpm) || 0, tpm: parseInt(kTpm) || 0
			});
			plaintext = r.plaintext;
			loadKeys(keyFor);
		} catch (e: any) { toast(e.message, 'err'); }
	}
	async function copyKey() {
		(await copyText(plaintext)) ? toast('disalin ✓', 'ok') : toast('gagal menyalin — blok teksnya dan salin manual', 'err');
	}

	// ---- panduan koneksi klien pihak ketiga ----
	function guideText(): string {
		const key = plaintext || 'sk-…';
		if (guideTab === 'opencode') {
			return JSON.stringify(
				{
					$schema: 'https://opencode.ai/config.json',
					provider: {
						jenderal: {
							npm: '@ai-sdk/openai-compatible',
							name: 'JenderalRouter (lokal)',
							options: { baseURL, apiKey: key },
							models: { [contohModel]: { name: 'Qwen3 30B (lokal)' } }
						}
					}
				},
				null,
				2
			);
		}
		if (guideTab === 'kilocode') {
			return [
				'Provider  : OpenAI Compatible',
				'Base URL  : ' + baseURL,
				'API Key   : ' + key,
				'Model ID  : ' + contohModel,
				'',
				'(buka pengaturan Kilocode → pilih provider "OpenAI Compatible"',
				' → isi kolom di atas → simpan → pilih model tersebut di chat)'
			].join('\n');
		}
		return `curl ${baseURL}/chat/completions \\
  -H "Authorization: Bearer ${key}" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"${contohModel}","messages":[{"role":"user","content":"halo"}]}'`;
	}
	async function copyGuide() {
		(await copyText(guideText())) ? toast('konfigurasi disalin ✓', 'ok') : toast('gagal menyalin — salin manual dari kotak konfigurasi', 'err');
	}

	// salin ulang key dari salinan terenkripsi (key lama tanpa salinan → 409)
	let revealingKey = $state(0);
	async function revealAndCopy(k: any) {
		try {
			revealingKey = k.id;
			const r = await api.get(`/api/admin/keys/${k.id}/reveal`);
			(await copyText(r.plaintext)) ? toast('key disalin ✓', 'ok') : toast('gagal menyalin', 'err');
		} catch (e: any) {
			toast(e.message || 'salinan tidak tersedia — buat ulang key', 'err');
		} finally {
			revealingKey = 0;
		}
	}

	// ---- kuota ----
	let qFor = $state<any>(null);
	let qDayTok = $state('0'); let qDayReq = $state('0'); let qDayCost = $state('0');
	let qMonTok = $state('0'); let qMonCost = $state('0');
	async function saveQuota() {
		if (!qFor) return;
		try {
			await api.put(`/api/admin/users/${qFor.id}/quota`, { period: 'day', token_limit: +qDayTok || 0, request_limit: +qDayReq || 0, cost_limit_usd: +qDayCost || 0 });
			await api.put(`/api/admin/users/${qFor.id}/quota`, { period: 'month', token_limit: +qMonTok || 0, cost_limit_usd: +qMonCost || 0 });
			qFor = null;
			toast(t('save') + ' ✓', 'ok');
		} catch (e: any) { toast(e.message, 'err'); }
	}

	async function delUser(u: any) {
		if (!confirm(t('confirm_delete'))) return;
		try { await api.del('/api/admin/users/' + u.id); reload(); }
		catch (e: any) { toast(e.message, 'err'); }
	}
	async function delKey(k: any) {
		try { await api.del('/api/admin/keys/' + k.id); if (keyFor) loadKeys(keyFor); }
		catch (e: any) { toast(e.message, 'err'); }
	}
</script>

<svelte:head><title>{t('users')} — JenderalRouter</title></svelte:head>

<div class="page-head">
	<h1 class="page-title"><Users size={20} /> {t('users')}</h1>
	<button class="btn" onclick={() => { showUser = true; uEmail = ''; uPass = ''; uRole = 'member'; }}>
		<Plus size={16} /> {t('create_user')}
	</button>
</div>

<div class="grid cols-4">
	<Stat icon={Users} label="Total user" value={String(users.length)} />
	<Stat icon={Activity} label="User aktif (7 hari)" value={String(totals.aktif)} />
	<Stat icon={Activity} label="Request (7 hari)" value={fmtNum(totals.req)} />
	<Stat icon={Activity} label="Biaya (7 hari)" value={fmtCost(totals.cost)} />
</div>

<div class="card" style="margin-top:14px">
	<div class="kv" style="margin-bottom:4px">
		<div class="row grow" style="position:relative">
			<Search size={15} style="position:absolute;left:10px;color:var(--muted)" />
			<input style="padding-left:32px" bind:value={search} placeholder="cari email…" />
		</div>
		<span class="muted small">{filtered.length} dari {users.length} user</span>
		<span class="grow"></span>
		<button class="btn ghost sm" onclick={reload}><Activity size={13} /> Muat ulang</button>
	</div>

	{#if loading}
		<div style="padding:20px 0"><span class="spinner dark"></span> {t('loading')}…</div>
	{:else}
		{#each filtered as u (u.id)}
			{@const uu = usageByUser[u.id]}
			{@const uk = keysByUser[u.id]}
			<div class="ucard">
				<div class="uc-main">
					<div class="uc-id">
						<strong>{u.email}</strong>
						<div class="row" style="margin-top:3px">
							<span class="badge {u.role === 'super_admin' ? 'err' : 'info'}">{u.role}</span>
							{#if u.status === 'active'}<span class="badge ok"><span class="dot"></span>active</span>
							{:else}<span class="badge warn">{u.status}</span>{/if}
							<span class="muted small">dibuat {fmtTs(u.created_at)}</span>
						</div>
					</div>
					<div class="uc-usage">
						<div class="kv small">
							<span class="muted">Pemakaian 7 hari:</span>
							{#if uu && (uu.requests || 0) > 0}
								<span class="badge info">{uu.requests} request</span>
								<span class="muted small mono">{fmtNum((uu.tokens_in || 0) + (uu.tokens_out || 0))} token · {fmtCost(uu.cost_usd)}</span>
							{:else}
								<span class="muted small">belum ada aktivitas</span>
							{/if}
						</div>
						<div class="uc-keys">
							{#if uk}
								{#each uk as k (k.id)}
									<div class="kv" style="padding:3px 0">
										<span class="mono small">{k.prefix}…</span>
										<span class="small muted">{k.name}</span>
										{#if k.revoked_at}<span class="badge err">revoked</span>
										{:else}<span class="badge ok"><span class="dot"></span>active</span>{/if}
										<span class="grow"></span>
										{#if !k.revoked_at && k.has_secret}
											<button class="btn ghost sm" title="salin API key lengkap" onclick={() => revealAndCopy(k)}>
												{#if revealingKey === k.id}<span class="spinner"></span>{:else}<Copy size={12} />{/if}
												Salin
											</button>
										{/if}
										<button class="btn ghost sm" title="hapus key" onclick={() => delKey(k)}><X size={12} /></button>
									</div>
								{/each}
							{/if}
						</div>
					</div>
					<div class="uc-actions">
						<button class="btn ghost sm" onclick={() => { qFor = u; loadKeys(u); }}><Gauge size={13} /> {t('quota')}</button>
						<button class="btn ghost sm" onclick={() => { keyFor = u; plaintext = ''; kName = ''; loadKeys(u); }}><KeyRound size={13} /> {t('api_key')}</button>
						<button class="btn danger sm" onclick={() => delUser(u)}><Trash2 size={13} /></button>
					</div>
				</div>
			</div>
		{:else}
			<div class="muted" style="padding:12px 0">Tidak ada user yang cocok.</div>
		{/each}
	{/if}
</div>

{#if showUser}
	<Modal title={t('create_user')} onclose={() => (showUser = false)}>
		<label>{t('email')}</label>
		<input bind:value={uEmail} placeholder="budi@unsoed.ac.id" />
		<label>{t('password')}</label>
		<input type="password" bind:value={uPass} placeholder="≥12 karakter" />
		<label>{t('role')}</label>
		<select bind:value={uRole}>
			<option value="member">member</option>
			<option value="viewer">viewer</option>
			<option value="admin">admin</option>
		</select>
		<div class="modal-actions">
			<button class="btn ghost" onclick={() => (showUser = false)}>{t('cancel')}</button>
			<button class="btn" onclick={createUser}>{t('create_user')}</button>
		</div>
	</Modal>
{/if}

{#if keyFor}
	<Modal title={`${t('api_key')} — ${keyFor.email}`} onclose={() => (keyFor = null)}>
		{#if plaintext}
			<div style="padding:10px 12px;border-radius:10px;background:color-mix(in srgb, var(--warn) 12%, transparent);border:1px solid color-mix(in srgb, var(--warn) 40%, transparent);margin-bottom:10px">
				<span class="small" style="color:var(--warn);font-weight:700">⚠ API key hanya tampil sekali — salin sekarang sebelum menutup!</span>
			</div>
			<div class="kv">
				<code class="mono" style="background:var(--bg);padding:10px;border-radius:8px;word-break:break-all;flex:1">{plaintext}</code>
				<button class="btn sm" onclick={copyKey}><Copy size={13} /> Salin key</button>
			</div>

			<div style="border-top:1px solid var(--border);margin:16px 0 12px"></div>
			<div class="small" style="font-weight:700;margin-bottom:2px">Cara pakai di aplikasi lain</div>
			<p class="muted small" style="margin:0 0 8px">
				API kompatibel OpenAI — Base URL: <code class="mono">{baseURL}</code>
			</p>
			<div class="row" style="margin-bottom:8px">
				<button class="btn sm" class:ghost={guideTab !== 'opencode'} onclick={() => (guideTab = 'opencode')}>OpenCode</button>
				<button class="btn sm" class:ghost={guideTab !== 'kilocode'} onclick={() => (guideTab = 'kilocode')}>Kilocode</button>
				<button class="btn sm" class:ghost={guideTab !== 'curl'} onclick={() => (guideTab = 'curl')}>curl</button>
				<span class="grow"></span>
				<button class="btn ghost sm" onclick={copyGuide}><Copy size={13} /> Salin</button>
			</div>
			<pre class="logbox" style="max-height:280px;overflow:auto">{guideText()}</pre>
			<p class="muted small" style="margin:8px 0 0">
				Klien OpenAI-compatible lain (Cline, Roo Code, Cherry Studio, dll) tinggal diisi
				Base URL + API Key yang sama.
			</p>
		{:else}
			<label>{t('name')}</label>
			<input bind:value={kName} placeholder="kunci {keyFor.email.split('@')[0]}" />
			<label>Model diizinkan</label>
			<input bind:value={kAllowed} placeholder="* (semua) atau local/Qwen3-30B-A3B-Q6_K, local/GLM-4.7-Flash-Q6_K" />
			<p class="muted small" style="margin:4px 0 0">format <span class="mono">prefix/model</span>, pisah dengan koma — <span class="mono">*</span> = semua model</p>
			<div class="row">
				<div class="grow">
					<label>RPM (request/menit)</label>
					<input bind:value={kRpm} placeholder="60" />
				</div>
				<div class="grow">
					<label>TPM (token/menit, 0 = tanpa batas)</label>
					<input bind:value={kTpm} placeholder="0" />
				</div>
			</div>
			<div style="padding:10px 12px;border-radius:10px;background:var(--surface-2);border:1px solid var(--border);margin-top:12px">
				<div class="small" style="font-weight:700;margin-bottom:2px">API kompatibel OpenAI</div>
				<div class="muted small">Base URL: <code class="mono">{baseURL}</code> — panduan koneksi (OpenCode, Kilocode, curl) tampil setelah key dibuat.</div>
			</div>
		{/if}
		<div class="modal-actions">
			{#if plaintext}
				<button class="btn" onclick={() => (keyFor = null)}>Selesai</button>
			{:else}
				<button class="btn ghost" onclick={() => (keyFor = null)}>{t('cancel')}</button>
				<button class="btn" onclick={createKey}><KeyRound size={14} /> Buat API key</button>
			{/if}
		</div>
	</Modal>
{/if}

{#if qFor}
	<Modal title={`${t('quota')} — ${qFor.email}`} onclose={() => (qFor = null)}>
		<label>Kuota harian — token</label>
		<input bind:value={qDayTok} placeholder="0 = tanpa batas" />
		<label>Kuota harian — request</label>
		<input bind:value={qDayReq} placeholder="0 = tanpa batas" />
		<label>Kuota harian — biaya (USD)</label>
		<input bind:value={qDayCost} placeholder="0 = tanpa batas" />
		<label>Kuota bulanan — token</label>
		<input bind:value={qMonTok} placeholder="0 = tanpa batas" />
		<label>Kuota bulanan — biaya (USD)</label>
		<input bind:value={qMonCost} placeholder="0 = tanpa batas" />
		<div class="modal-actions">
			<button class="btn ghost" onclick={() => (qFor = null)}>{t('cancel')}</button>
			<button class="btn" onclick={saveQuota}>{t('save')}</button>
		</div>
	</Modal>
{/if}

<style>
	.ucard {
		border: 1px solid var(--border);
		border-radius: 12px;
		padding: 14px;
		margin-bottom: 10px;
		transition: border-color var(--speed);
	}
	.ucard:hover { border-color: var(--border-strong); }
	.uc-main { display: flex; gap: 16px; align-items: flex-start; flex-wrap: wrap; }
	.uc-id { min-width: 240px; }
	.uc-usage { flex: 1; min-width: 260px; }
	.uc-keys { margin-top: 6px; display: flex; flex-direction: column; }
	.uc-keys .kv { gap: 8px; }
	.uc-actions { display: flex; flex-direction: column; gap: 6px; margin-left: auto; }
	@media (max-width: 900px) {
		.uc-main { flex-direction: column; }
		.uc-actions { flex-direction: row; margin-left: 0; }
	}
</style>
