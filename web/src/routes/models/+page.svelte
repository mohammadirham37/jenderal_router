<script lang="ts">
	import { BrainCircuit, Save, Search, MessageSquare, Plug } from '@lucide/svelte';
	import { api, fmtNum, truncate } from '#lib/api.js';
	import { t, toast } from '#lib/stores.svelte.js';

	let models = $state<any[]>([]);
	let loading = $state(true);
	// nilai edit per baris (map id → field)
	let edits = $state<Record<number, { alias: string; pin: string; pout: string }>>({});

	async function reload() {
		loading = true;
		try {
			const res = await api.get('/api/admin/models');
			models = res.models || [];
			edits = Object.fromEntries(
				models.map((m) => [m.id, { alias: m.alias || '', pin: String(m.price_in_per_1m), pout: String(m.price_out_per_1m) }])
			);
		} catch (e: any) {
			toast(e.message, 'err');
		} finally {
			loading = false;
		}
	}
	$effect(() => { reload(); });

	async function save(m: any, patch: any) {
		try {
			const r = await api.patch('/api/admin/models/' + m.id, patch);
			if (r?.model) Object.assign(m, r.model);
			toast(t('save') + ' ✓', 'ok');
		} catch (e: any) { toast(e.message, 'err'); }
	}

	// ---- ketersediaan per kanal: chat dashboard & API (/v1) ----
	let search = $state('');
	let provider = $state('');
	const providers = $derived([...new Set(models.map((m) => m.provider_name))].sort());
	const filtered = $derived(
		models.filter((m) =>
			(!provider || m.provider_name === provider) &&
			(!search || (m.public_id + ' ' + (m.alias || '')).toLowerCase().includes(search.toLowerCase())))
	);
	const count = $derived({
		chat: models.filter((m) => m.chat_enabled).length,
		api: models.filter((m) => m.api_enabled).length
	});

	async function toggle(m: any, field: 'chat_enabled' | 'api_enabled', on: boolean) {
		m[field] = on;
		await save(m, { [field]: on });
	}
	async function bulk(field: 'chat_enabled' | 'api_enabled', on: boolean) {
		const ids = filtered.map((m) => m.id);
		if (!ids.length) return;
		if (!confirm(`${on ? 'Aktifkan' : 'Nonaktifkan'} ${field === 'chat_enabled' ? 'Chat' : 'API'} untuk ${ids.length} model yang tampil?`)) return;
		try {
			await api.patch('/api/admin/models/bulk', { ids, [field]: on });
			for (const m of filtered) m[field] = on;
			toast(t('save') + ' ✓', 'ok');
		} catch (e: any) { toast(e.message, 'err'); }
	}
</script>

<svelte:head><title>{t('models')} — JenderalRouter</title></svelte:head>

<div class="page-head">
	<h1 class="page-title"><BrainCircuit size={20} /> {t('models')}</h1>
	<span class="muted small">{t('models')} & harga per 1 juta token (bisa diedit)</span>
</div>

{#if loading}
	<div class="card"><span class="spinner dark"></span> {t('loading')}…</div>
{:else}
	<div class="card" style="margin-bottom:12px">
		<div class="kv" style="flex-wrap:wrap;gap:8px">
			<div class="row grow" style="position:relative;min-width:220px">
				<Search size={15} style="position:absolute;left:10px;color:var(--muted)" />
				<input style="padding-left:32px" bind:value={search} placeholder="cari model / alias…" aria-label="cari model" />
			</div>
			<select bind:value={provider} style="width:auto" aria-label="filter provider">
				<option value="">Semua provider</option>
				{#each providers as p (p)}<option value={p}>{p}</option>{/each}
			</select>
			<span class="badge info"><MessageSquare size={11} /> Chat {count.chat}/{models.length}</span>
			<span class="badge"><Plug size={11} /> API {count.api}/{models.length}</span>
		</div>
		<div class="kv small" style="margin-top:10px;flex-wrap:wrap;gap:6px">
			<span class="muted">Untuk {filtered.length} model yang tampil:</span>
			<button class="btn ghost sm" onclick={() => bulk('chat_enabled', true)}>Centang Chat</button>
			<button class="btn ghost sm" onclick={() => bulk('chat_enabled', false)}>Kosongkan Chat</button>
			<button class="btn ghost sm" onclick={() => bulk('api_enabled', true)}>Centang API</button>
			<button class="btn ghost sm" onclick={() => bulk('api_enabled', false)}>Kosongkan API</button>
		</div>
		<p class="muted small" style="margin:8px 0 0">
			<b>Chat</b> = muncul &amp; bisa dipakai di menu Chat dashboard. <b>API</b> = muncul di <span class="mono">/v1/models</span>
			&amp; bisa dipanggil lewat API key. Combo otomatis melewati model yang kanalnya dimatikan.
		</p>
	</div>

	<div class="card table-wrap">
		<table>
			<thead>
				<tr>
					<th>{t('model')}</th><th>{t('alias')}</th><th>{t('price_in')}</th><th>{t('price_out')}</th>
					<th>{t('context')}</th><th>Tools / Vision</th><th title="tersedia di Chat dashboard">Chat</th><th title="tersedia lewat API /v1">API</th>
				</tr>
			</thead>
			<tbody>
				{#each filtered as m (m.id)}
					{@const e = edits[m.id]}
					<tr>
						<td class="mono" title={m.provider_name}>{truncate(m.public_id, 34)}</td>
						<td><input style="width:130px" bind:value={e.alias} placeholder="alias"
							onchange={() => save(m, { alias: e.alias })} /></td>
						<td><input style="width:95px" bind:value={e.pin}
							onchange={() => save(m, { price_in_per_1m: parseFloat(e.pin) || 0 })} /></td>
						<td><input style="width:95px" bind:value={e.pout}
							onchange={() => save(m, { price_out_per_1m: parseFloat(e.pout) || 0 })} /></td>
						<td>{fmtNum(m.context_window)}</td>
						<td>{m.capabilities?.tools ? '🔧' : '–'} / {m.capabilities?.vision ? '👁️' : '–'}</td>
						<td><input type="checkbox" aria-label="Chat {m.public_id}" checked={m.chat_enabled}
							onchange={(ev) => toggle(m, 'chat_enabled', ev.currentTarget.checked)} /></td>
						<td><input type="checkbox" aria-label="API {m.public_id}" checked={m.api_enabled}
							onchange={(ev) => toggle(m, 'api_enabled', ev.currentTarget.checked)} /></td>
					</tr>
				{:else}
					<tr><td colspan="8" class="muted">{t('no_data')}</td></tr>
				{/each}
			</tbody>
		</table>
	</div>
	<div class="muted small" style="margin-top:10px;display:flex;gap:6px;align-items:center">
		<Save size={13} /> perubahan tersimpan saat keluar dari kolom (onchange)
	</div>
{/if}
