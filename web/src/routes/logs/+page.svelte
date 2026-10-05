<script lang="ts">
	import { ScrollText, RefreshCw, Download } from '@lucide/svelte';
	import { api, fmtCost, fmtTs } from '$lib/api';
	import { t } from '$lib/stores.svelte';

	let logs = $state<any[]>([]);
	let loading = $state(true);
	let fUser = $state('');
	let fProvider = $state('');
	let fStatus = $state('');

	async function load() {
		loading = true;
		const q = new URLSearchParams({ limit: '200' });
		if (fUser) q.set('user', fUser);
		if (fProvider) q.set('provider', fProvider);
		if (fStatus) q.set('status', fStatus);
		try {
			const res = await api.get('/api/admin/logs?' + q.toString());
			logs = res.logs || [];
		} catch { /* */ }
		loading = false;
	}
	$effect(() => { load(); });
</script>

<svelte:head><title>{t('logs')} — JenderalRouter</title></svelte:head>

<div class="page-head">
	<h1 class="page-title"><ScrollText size={20} /> {t('logs')}</h1>
	<div class="row">
		<a class="btn ghost" href="/api/admin/logs/export.csv" download><Download size={15} /> {t('export_csv')}</a>
		<button class="btn" onclick={load}><RefreshCw size={15} /></button>
	</div>
</div>

<div class="card" style="margin-bottom:14px">
	<div class="row">
		<input style="max-width:170px" bind:value={fUser} placeholder="{t('user')} (id)" />
		<input style="max-width:170px" bind:value={fProvider} placeholder={t('provider')} />
		<select style="max-width:120px" bind:value={fStatus}>
			<option value="">{t('status')}</option>
			<option value="2xx">2xx</option>
			<option value="4xx">4xx</option>
			<option value="5xx">5xx</option>
		</select>
		<button class="btn" onclick={load}>Terapkan</button>
	</div>
	<div class="muted small" style="margin-top:6px">Isi prompt tidak pernah disimpan — hanya metadata (privasi, NFR-14).</div>
</div>

<div class="card table-wrap">
	{#if loading}
		<span class="spinner dark"></span> {t('loading')}…
	{:else}
		<table>
			<thead>
				<tr><th>{t('time')}</th><th>{t('requested')}</th><th>{t('provider')}</th><th>{t('model')}</th><th>{t('status')}</th><th>{t('latency')}</th><th>TTFT</th><th>{t('tokens')}</th><th>{t('cost')}</th><th>{t('attempts')}</th></tr>
			</thead>
			<tbody>
				{#each logs as l (l.id)}
					<tr>
						<td class="mono">{fmtTs(l.ts)}</td>
						<td class="mono">{l.requested_model}</td>
						<td>{l.provider_name}</td>
						<td class="mono">{l.model_name}</td>
						<td><span class="badge {+l.status >= 200 && +l.status < 400 ? 'ok' : +l.status >= 500 || +l.status === 0 ? 'err' : 'warn'}">{l.status}{l.error_code ? ' · ' + l.error_code : ''}</span></td>
						<td>{l.latency_ms} ms</td>
						<td>{l.ttft_ms} ms</td>
						<td>{l.tokens_in}/{l.tokens_out}</td>
						<td>{fmtCost(l.cost_usd)}</td>
						<td>{l.attempts}</td>
					</tr>
				{:else}
					<tr><td colspan="10" class="muted">{t('no_data')}</td></tr>
				{/each}
			</tbody>
		</table>
	{/if}
</div>
