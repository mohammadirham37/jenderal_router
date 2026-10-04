<script lang="ts">
	import { Users, Plus, Trash2, KeyRound, Gauge, Copy, X } from '@lucide/svelte';
	import { api } from '$lib/api';
	import { t, toast } from '$lib/stores.svelte';
	import Modal from '$lib/components/Modal.svelte';

	let users = $state<any[]>([]);
	let keysByUser = $state<Record<number, any[]>>({});
	let loading = $state(true);

	async function reload() {
		loading = true;
		try {
			const res = await api.get('/api/admin/users');
			users = (res.users || []).map((x: any) => x.user);
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
		try { await navigator.clipboard.writeText(plaintext); toast('disalin ✓', 'ok'); } catch { /* */ }
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

{#if loading}
	<div class="card"><span class="spinner dark"></span> {t('loading')}…</div>
{:else}
	{#each users as u (u.id)}
		{@const keys = keysByUser[u.id]}
		<div class="card" style="margin-bottom:12px">
			<div class="page-head" style="margin-bottom:8px">
				<div class="row">
					<strong>{u.email}</strong>
					<span class="badge {u.role === 'super_admin' ? 'err' : 'info'}">{u.role}</span>
					{#if u.status === 'active'}<span class="badge ok"><span class="dot"></span>active</span>
					{:else}<span class="badge warn">{u.status}</span>{/if}
				</div>
				<div class="row">
					<button class="btn ghost sm" onclick={() => { qFor = u; loadKeys(u); }}><Gauge size={13} /> {t('quota')}</button>
					<button class="btn ghost sm" onclick={() => { keyFor = u; plaintext = ''; kName = ''; loadKeys(u); }}><KeyRound size={13} /> {t('api_key')}</button>
					<button class="btn danger sm" onclick={() => delUser(u)}><Trash2 size={13} /></button>
				</div>
			</div>
			<div class="muted small" style="font-weight:700;margin-bottom:4px">{t('keys')}</div>
			{#if keys}
				{#each keys as k (k.id)}
					<div class="kv" style="margin-bottom:4px">
						<span class="mono small">{k.prefix}…</span>
						<span class="small">{k.name}</span>
						{#if k.revoked_at}<span class="badge err">revoked</span>{:else}<span class="badge ok"><span class="dot"></span>active</span>{/if}
						<span class="muted small">rpm={k.rpm} tpm={k.tpm}</span>
						<button class="btn ghost sm" onclick={() => delKey(k)}><X size={12} /></button>
					</div>
				{:else}
					<span class="muted small">—</span>
				{/each}
			{:else}
				<span class="muted small">…</span>
			{/if}
		</div>
	{/each}
{/if}

{#if showUser}
	<Modal title={t('create_user')} onclose={() => (showUser = false)}>
		<label>{t('email')}</label>
		<input bind:value={uEmail} placeholder="budi@tim.com" />
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
			<p class="small" style="color:var(--ok);font-weight:700">⚠ {t('api_key')} hanya tampil sekali — salin sekarang!</p>
			<div class="kv">
				<code class="mono" style="background:var(--bg);padding:10px;border-radius:8px;word-break:break-all;flex:1">{plaintext}</code>
				<button class="btn sm" onclick={copyKey}><Copy size={13} /></button>
			</div>
		{:else}
			<label>{t('name')}</label>
			<input bind:value={kName} placeholder="kunci budi" />
			<label>Model diizinkan (* = semua)</label>
			<input bind:value={kAllowed} />
			<label>RPM</label>
			<input bind:value={kRpm} />
			<label>TPM</label>
			<input bind:value={kTpm} />
		{/if}
		<div class="modal-actions">
			<button class="btn ghost" onclick={() => (keyFor = null)}>{t('cancel')}</button>
			{#if plaintext}
				<button class="btn" onclick={() => (keyFor = null)}>Selesai</button>
			{:else}
				<button class="btn" onclick={createKey}><KeyRound size={15} /> {t('save')}</button>
			{/if}
		</div>
	</Modal>
{/if}

{#if qFor}
	<Modal title={`${t('quota')} — ${qFor.email}`} onclose={() => (qFor = null)}>
		<h2 style="margin-top:0">{t('day')}</h2>
		<label>Batas token</label><input bind:value={qDayTok} />
		<label>Batas request</label><input bind:value={qDayReq} />
		<label>Batas biaya (USD)</label><input bind:value={qDayCost} />
		<h2 style="margin-top:16px">{t('month')}</h2>
		<label>Batas token</label><input bind:value={qMonTok} />
		<label>Batas biaya (USD)</label><input bind:value={qMonCost} />
		<div class="modal-actions">
			<button class="btn ghost" onclick={() => (qFor = null)}>{t('cancel')}</button>
			<button class="btn" onclick={saveQuota}><Gauge size={15} /> {t('save')}</button>
		</div>
	</Modal>
{/if}
