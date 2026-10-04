<script lang="ts">
	import { BrainCircuit, Save } from '@lucide/svelte';
	import { api, fmtNum, truncate } from '$lib/api';
	import { t, toast } from '$lib/stores.svelte';

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
		try { await api.patch('/api/admin/models/' + m.id, patch); toast(t('save') + ' ✓', 'ok'); }
		catch (e: any) { toast(e.message, 'err'); }
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
	<div class="card table-wrap">
		<table>
			<thead>
				<tr>
					<th>{t('model')}</th><th>{t('alias')}</th><th>{t('price_in')}</th><th>{t('price_out')}</th>
					<th>{t('context')}</th><th>Tools / Vision</th><th>{t('enabled')}</th>
				</tr>
			</thead>
			<tbody>
				{#each models as m (m.id)}
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
						<td><input type="checkbox" checked={m.enabled} onchange={(ev) => save(m, { enabled: ev.currentTarget.checked })} /></td>
					</tr>
				{:else}
					<tr><td colspan="7" class="muted">{t('no_data')}</td></tr>
				{/each}
			</tbody>
		</table>
	</div>
	<div class="muted small" style="margin-top:10px;display:flex;gap:6px;align-items:center">
		<Save size={13} /> perubahan tersimpan saat keluar dari kolom (onchange)
	</div>
{/if}
