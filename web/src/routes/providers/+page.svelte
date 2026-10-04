<script lang="ts">
	import { Cable, Plus, Trash2, Gauge, RefreshCw, Play, Pause, X, KeyRound, Zap } from '@lucide/svelte';
	import { api, truncate } from '$lib/api';
	import { t, toast } from '$lib/stores.svelte';
	import Modal from '$lib/components/Modal.svelte';

	type Provider = {
		id: number; type: string; name: string; prefix: string; base_url: string;
		enabled: boolean; credentials: any[]; model_count: number;
	};

	let providers = $state<Provider[]>([]);
	let loading = $state(true);
	let showAdd = $state(false);
	let templates: any[] = $state([]);

	async function reload() {
		loading = true;
		try {
			const res = await api.get('/api/admin/providers');
			providers = res.providers || [];
		} catch (e: any) {
			toast(e.message, 'err');
		} finally {
			loading = false;
		}
	}
	$effect(() => { reload(); });

	async function toggle(p: Provider) {
		try { await api.patch(`/api/admin/providers/${p.id}`, { enabled: !p.enabled }); reload(); }
		catch (e: any) { toast(e.message, 'err'); }
	}
	async function del(p: Provider) {
		if (!confirm(t('confirm_delete'))) return;
		try { await api.del(`/api/admin/providers/${p.id}`); reload(); }
		catch (e: any) { toast(e.message, 'err'); }
	}
	async function testConn(p: Provider) {
		toast('Menguji ' + p.name + '…');
		try {
			const r = await api.post(`/api/admin/providers/${p.id}/test`);
			toast(r.ok ? `${p.name}: OK ${r.latency_ms} ms` : `${p.name}: ${r.error}`, r.ok ? 'ok' : 'err');
		} catch (e: any) { toast(e.message, 'err'); }
	}
	async function sync(p: Provider) {
		try {
			const r = await api.post(`/api/admin/providers/${p.id}/sync-models`);
			toast(r.ok ? `${r.count} model (${r.source})` : r.error, r.ok ? 'ok' : 'err');
		} catch (e: any) { toast(e.message, 'err'); }
	}

	// ---- modal tambah provider ----
	let useTemplate = $state(true);
	let tplPrefix = $state('openai');
	let tplKey = $state('');
	let cName = $state(''); let cPrefix = $state(''); let cURL = $state(''); let cType = $state('openai-compatible');
	let adding = $state(false);
	async function addProvider() {
		if (useTemplate && !tplPrefix) { toast('pilih template provider dulu', 'err'); return; }
		if (!useTemplate && (!cName || !cPrefix || !cURL)) { toast('nama, prefix, dan base URL wajib diisi', 'err'); return; }
		adding = true;
		try {
			if (useTemplate) {
				await api.post('/api/admin/providers/seed', { prefix: tplPrefix, api_key: tplKey });
			} else {
				await api.post('/api/admin/providers', {
					type: cType, name: cName, prefix: cPrefix, base_url: cURL,
					settings: { strategy: 'round_robin' }
				});
			}
			showAdd = false;
			tplKey = '';
			toast(t('save') + ' ✓', 'ok');
			reload();
		} catch (e: any) { toast(e.message, 'err'); }
		finally { adding = false; }
	}
	$effect(() => {
		if (showAdd && templates.length === 0) {
			api.get('/api/admin/templates')
				.then((r) => {
					templates = r.templates || [];
					// pastikan prefix default valid (prefix asli: oa, an, gm, …)
					if (!templates.find((x) => x.prefix === tplPrefix)) {
						tplPrefix = templates[0]?.prefix || '';
					}
				})
				.catch(() => {});
		}
	});

	// ---- modal tambah kredensial ----
	let credFor = $state<Provider | null>(null);
	let credLabel = $state(''); let credKey = $state('');
	async function addCred() {
		if (!credFor) return;
		try {
			await api.post(`/api/admin/providers/${credFor.id}/credentials`, { label: credLabel, api_key: credKey, weight: 1 });
			credFor = null; credLabel = ''; credKey = '';
			toast(t('save') + ' ✓', 'ok');
			reload();
		} catch (e: any) { toast(e.message, 'err'); }
	}
</script>

<svelte:head><title>{t('providers')} — JenderalRouter</title></svelte:head>

<div class="page-head">
	<h1 class="page-title"><Cable size={20} /> {t('providers')}</h1>
	<button class="btn" onclick={() => (showAdd = true)}><Plus size={16} /> {t('add_provider')}</button>
</div>

{#if loading}
	<div class="card"><span class="spinner dark"></span> {t('loading')}…</div>
{:else}
	{#each providers as p (p.id)}
		<div class="card" style="margin-bottom:14px">
			<div class="page-head" style="margin-bottom:10px">
				<div>
					<div class="row">
						<strong style="font-size:15px">{p.name}</strong>
						<span class="badge info">{p.prefix}</span>
						{#if p.enabled}<span class="badge ok"><span class="dot"></span>ON</span>
						{:else}<span class="badge err"><span class="dot"></span>OFF</span>{/if}
						<span class="badge">{p.model_count} model</span>
					</div>
					<div class="muted small mono" style="margin-top:4px">{p.base_url}</div>
				</div>
				<div class="row">
					<button class="btn ghost sm" onclick={() => testConn(p)}><Zap size={13} /> {t('test')}</button>
					<button class="btn ghost sm" onclick={() => sync(p)}><RefreshCw size={13} /> {t('sync')}</button>
					<button class="btn ghost sm" onclick={() => toggle(p)} title={t('enabled')}>
						{#if p.enabled}<Pause size={13} />{:else}<Play size={13} />{/if}
					</button>
					<button class="btn danger sm" onclick={() => del(p)}><Trash2 size={13} /></button>
				</div>
			</div>
			<div>
				<div class="muted small" style="font-weight:700;margin-bottom:6px">{t('credentials')} ({(p.credentials || []).length})</div>
				{#each p.credentials as c (c.id)}
					<div class="kv" style="margin-bottom:5px">
						<span class="badge info">#{c.id}</span>
						<span class="small">{c.label || '—'}</span>
						{#if c.cooldown}<span class="badge warn"><span class="dot pulse"></span>cooldown</span>
						{:else if c.status === 'active'}<span class="badge ok"><span class="dot"></span>active</span>
						{:else}<span class="badge err">{c.status}</span>{/if}
						<span class="muted small">{c.use_count}x · {c.fail_count} err</span>
						<button class="btn ghost sm" onclick={async () => { await api.del('/api/admin/credentials/' + c.id); reload(); }}>
							<X size={12} />
						</button>
					</div>
				{:else}
					<span class="muted small">—</span>
				{/each}
				<button class="btn sm" style="margin-top:8px" onclick={() => { credFor = p; credLabel = ''; credKey = ''; }}>
					<KeyRound size={13} /> {t('add_key')}
				</button>
			</div>
		</div>
	{:else}
		<div class="card muted">{t('no_data')}</div>
	{/each}
{/if}

{#if showAdd}
	<Modal title={t('add_provider')} onclose={() => (showAdd = false)}>
		<label style="display:flex;gap:8px;align-items:center;margin-top:0">
			<input type="checkbox" bind:checked={useTemplate} style="width:auto" />
			{t('providers')} dari template bawaan
		</label>
		{#if useTemplate}
			<label>{t('providers')}</label>
			<select bind:value={tplPrefix}>
				{#each templates as tp (tp.prefix)}
					<option value={tp.prefix}>{tp.name} ({tp.prefix}/…)</option>
				{/each}
			</select>
			<label>{t('api_key')}</label>
			<input bind:value={tplKey} placeholder="sk-…" />
		{:else}
			<label>{t('name')}</label>
			<input bind:value={cName} placeholder="Provider internal" />
			<label>{t('prefix')}</label>
			<input bind:value={cPrefix} placeholder="internal" />
			<label>Tipe</label>
			<select bind:value={cType}>
				<option value="openai-compatible">openai-compatible</option>
				<option value="anthropic">anthropic</option>
				<option value="gemini">gemini</option>
				<option value="llamastash">llamastash</option>
			</select>
			<label>{t('base_url')}</label>
			<input bind:value={cURL} placeholder="http://…" />
		{/if}
		<div class="modal-actions">
			<button class="btn ghost" onclick={() => (showAdd = false)}>{t('cancel')}</button>
			<button class="btn" onclick={addProvider} disabled={adding}>
				{#if adding}<span class="spinner"></span>{:else}<Gauge size={15} />{/if}
				{t('save')}
			</button>
		</div>
	</Modal>
{/if}

{#if credFor}
	<Modal title={`${t('add_key')} — ${credFor.name}`} onclose={() => (credFor = null)}>
		<label>{t('credentials')}: label</label>
		<input bind:value={credLabel} placeholder="kunci utama" />
		<label>{t('api_key')}</label>
		<input bind:value={credKey} placeholder="sk-…" />
		<div class="modal-actions">
			<button class="btn ghost" onclick={() => (credFor = null)}>{t('cancel')}</button>
			<button class="btn" onclick={addCred}><KeyRound size={15} /> {t('save')}</button>
		</div>
	</Modal>
{/if}
