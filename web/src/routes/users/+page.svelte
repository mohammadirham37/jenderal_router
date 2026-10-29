<script lang="ts">
	import {
		Users,
		Plus,
		Trash2,
		KeyRound,
		Gauge,
		Copy,
		X,
		Activity,
		Search,
		ChartLine
	} from '@lucide/svelte';

	import { browser } from '$app/env';
	import { api, fmtNum, fmtCost, fmtTs } from '#lib/api.js';
	import { copyText } from '#lib/clipboard.js';
	import { t, toast } from '#lib/stores.svelte.js';
	import Modal from '#lib/components/Modal.svelte';
	import Stat from '#lib/components/Stat.svelte';
	import ApiGuide from '#lib/components/ApiGuide.svelte';

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
	const baseURL = (browser ? window.location.origin : '') + '/v1';

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
		await copyText(plaintext)
			? toast('disalin ✓', 'ok')
			: toast('gagal menyalin — blok teksnya dan salin manual', 'err');
	}

	// salin ulang key dari salinan terenkripsi (key lama tanpa salinan → 409)
	let revealingKey = $state(0);
	async function revealAndCopy(k: any) {
		try {
			revealingKey = k.id;
			const r = await api.get(`/api/admin/keys/${k.id}/reveal`);

			await copyText(r.plaintext)
				? toast('key disalin ✓', 'ok')
				: toast('gagal menyalin', 'err');
		} catch(e: any) {
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
						<a class="uc-email" href="/users/{u.id}" title="lihat detail penggunaan"><strong>{u.email}</strong></a>
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
						<a class="btn ghost sm" href="/users/{u.id}"><ChartLine size={13} /> Detail</a>
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
	<Modal
		title={t('create_user')}
		onclose={() => showUser = false}
	>
		<label for="f-u-email">{t('email')}</label>
		<input id="f-u-email" bind:value={uEmail} placeholder="budi@unsoed.ac.id" />
		<label for="f-u-pass">{t('password')}</label>
		<input id="f-u-pass" type="password" bind:value={uPass} placeholder="≥12 karakter" />
		<label for="f-u-role">{t('role')}</label>
		<select id="f-u-role" bind:value={uRole}>
			<option value="member">member</option>
			<option value="viewer">viewer</option>
			<option value="admin">admin</option>
		</select>
		<div class="modal-actions">
			<button class="btn ghost" onclick={() => showUser = false}>{t('cancel')}</button>
			<button class="btn" onclick={createUser}>{t('create_user')}</button>
		</div>
	</Modal>
{/if}

{#if keyFor}
	<Modal
		wide={!!plaintext}
		title={`${t('api_key')} — ${keyFor.email}`}
		onclose={() => keyFor = null}
	>
		{#if plaintext}
			<div style="padding:10px 12px;border-radius:10px;background:color-mix(in srgb, var(--warn) 12%, transparent);border:1px solid color-mix(in srgb, var(--warn) 40%, transparent);margin-bottom:10px">
				<span class="small" style="color:var(--warn);font-weight:700">⚠ API key hanya tampil sekali — salin sekarang sebelum menutup!</span>
			</div>
			<div class="kv">
				<code class="mono" style="background:var(--bg);padding:10px;border-radius:8px;word-break:break-all;flex:1">{plaintext}</code>
				<button class="btn sm" onclick={copyKey}><Copy size={13} /> Salin key</button>
			</div>

			<div style="border-top:1px solid var(--border);margin:16px 0 12px"></div>
			<div class="small" style="font-weight:700;margin-bottom:10px">Cara menghubungkan aplikasi — bagikan ke {keyFor.email}</div>
			<ApiGuide keyPrefix={plaintext.slice(0, 8)} getKey={() => Promise.resolve(plaintext)} />
		{:else}
			<label for="f-k-name">{t('name')}</label>
			<input id="f-k-name" bind:value={kName} placeholder="kunci {keyFor.email.split('@')[0]}" />
			<label for="f-k-allowed">Model diizinkan</label>
			<input id="f-k-allowed" bind:value={kAllowed} placeholder="* (semua) atau local/Qwen3-30B-A3B-Q6_K, local/GLM-4.7-Flash-Q6_K" />
			<p class="muted small" style="margin:4px 0 0">format <span class="mono">prefix/model</span>, pisah dengan koma — <span class="mono">*</span> = semua model</p>
			<div class="row">
				<div class="grow">
					<label for="f-k-rpm">RPM (request/menit)</label>
					<input id="f-k-rpm" bind:value={kRpm} placeholder="60" />
				</div>
				<div class="grow">
					<label for="f-k-tpm">TPM (token/menit, 0 = tanpa batas)</label>
					<input id="f-k-tpm" bind:value={kTpm} placeholder="0" />
				</div>
			</div>
			<div style="padding:10px 12px;border-radius:10px;background:var(--surface-2);border:1px solid var(--border);margin-top:12px">
				<div class="small" style="font-weight:700;margin-bottom:2px">API kompatibel OpenAI</div>
				<div class="muted small">Base URL: <code class="mono">{baseURL}</code> — panduan koneksi langkah demi langkah tampil setelah key dibuat.</div>
			</div>
		{/if}
		<div class="modal-actions">
			{#if plaintext}
				<button class="btn" onclick={() => keyFor = null}>Selesai</button>
			{:else}
				<button class="btn ghost" onclick={() => keyFor = null}>{t('cancel')}</button>
				<button class="btn" onclick={createKey}><KeyRound size={14} />Buat API key</button>
			{/if}
		</div>
	</Modal>
{/if}

{#if qFor}
	<Modal
		title={`${t('quota')} — ${qFor.email}`}
		onclose={() => qFor = null}
	>
		<label for="f-q-day-tok">Kuota harian — token</label>
		<input id="f-q-day-tok" bind:value={qDayTok} placeholder="0 = tanpa batas" />
		<label for="f-q-day-req">Kuota harian — request</label>
		<input id="f-q-day-req" bind:value={qDayReq} placeholder="0 = tanpa batas" />
		<label for="f-q-day-cost">Kuota harian — biaya (USD)</label>
		<input id="f-q-day-cost" bind:value={qDayCost} placeholder="0 = tanpa batas" />
		<label for="f-q-mon-tok">Kuota bulanan — token</label>
		<input id="f-q-mon-tok" bind:value={qMonTok} placeholder="0 = tanpa batas" />
		<label for="f-q-mon-cost">Kuota bulanan — biaya (USD)</label>
		<input id="f-q-mon-cost" bind:value={qMonCost} placeholder="0 = tanpa batas" />
		<div class="modal-actions">
			<button class="btn ghost" onclick={() => qFor = null}>{t('cancel')}</button>
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
	.uc-email { color: inherit; text-decoration: none; }
	.uc-email:hover { color: var(--accent-2); text-decoration: underline; }
	.uc-usage { flex: 1; min-width: 260px; }
	.uc-keys { margin-top: 6px; display: flex; flex-direction: column; }
	.uc-keys .kv { gap: 8px; }
	.uc-actions { display: flex; flex-direction: column; gap: 6px; margin-left: auto; }
	@media (max-width: 900px) {
		.uc-main { flex-direction: column; }
		.uc-actions { flex-direction: row; margin-left: 0; }
	}
</style>
