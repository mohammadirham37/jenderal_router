<script lang="ts">
	import { ChartLine, Activity, Coins, AlertTriangle, Hash } from '@lucide/svelte';
	import { api, fmtNum, fmtCost, truncate } from '$lib/api';
	import { t, toast } from '$lib/stores.svelte';
	import Stat from '$lib/components/Stat.svelte';

	type Summary = {
		totals: any;
		per_day: any[];
		error_rate: number;
	};

	let res = $state<Summary | null>(null);
	let logs = $state<any[]>([]);
	let loading = $state(true);

	$effect(() => {
		(async () => {
			try {
				res = await api.get('/api/admin/analytics/summary?days=7');
				const l = await api.get('/api/admin/logs?limit=8');
				logs = l.logs || [];
			} catch (e: any) {
				toast(e.message, 'err');
			} finally {
				loading = false;
			}
		})();
	});

	const today = $derived(res?.per_day?.[res.per_day.length - 1] ?? {});
	const maxReq = $derived(Math.max(1, ...(res?.per_day ?? []).map((d: any) => d.requests)));
</script>

<svelte:head><title>Ringkasan — JenderalRouter</title></svelte:head>

<div class="page-head">
	<div>
		<h1 class="page-title"><ChartLine size={20} /> {t('dashboard')}</h1>
		<div class="subtitle">7 hari terakhir</div>
	</div>
</div>

{#if loading}
	<div class="card"><span class="spinner dark"></span> {t('loading')}…</div>
{:else if res}
	<div class="grid cols-4">
		<Stat icon={Activity} label={t('requests_today')} value={fmtNum(today.requests)} />
		<Stat icon={Hash} label={t('tokens_today')} value={fmtNum((today.tokens_in || 0) + (today.tokens_out || 0))} />
		<Stat icon={Coins} label={t('cost_7d')} value={fmtCost(res.totals?.cost_usd)} />
		<Stat icon={AlertTriangle} label={t('error_rate')} value={((res.error_rate || 0) * 100).toFixed(1) + '%'} />
	</div>

	<div class="card" style="margin-top:14px">
		<h2>{t('per_day')}</h2>
		<div class="bar-chart">
			{#each res.per_day as d (d.day)}
				<div class="bar" style="height:{Math.max(4, (d.requests / maxReq) * 100)}%" data-tip="{d.day}: {d.requests}"></div>
			{:else}
				<span class="muted small">{t('no_data')}</span>
			{/each}
		</div>
	</div>

	<div class="card" style="margin-top:14px">
		<h2>{t('recent_logs')}</h2>
		<div class="table-wrap">
			<table>
				<thead>
					<tr><th>{t('time')}</th><th>{t('requested')}</th><th>{t('provider')}</th><th>{t('status')}</th><th>{t('latency')}</th><th>{t('tokens')}</th><th>{t('cost')}</th></tr>
				</thead>
				<tbody>
					{#each logs as l (l.id)}
						<tr>
							<td class="mono">{(l.ts || '').replace('T', ' ').slice(0, 19)}</td>
							<td class="mono">{truncate(l.requested_model, 28)}</td>
							<td>{l.provider_name}</td>
							<td>
								<span class="badge {l.status >= 200 && l.status < 400 ? 'ok' : l.status >= 500 || l.status == 0 ? 'err' : 'warn'}">{l.status}</span>
							</td>
							<td>{l.latency_ms} ms</td>
							<td>{l.tokens_in}/{l.tokens_out}</td>
							<td>{fmtCost(l.cost_usd)}</td>
						</tr>
					{:else}
						<tr><td colspan="7" class="muted">{t('no_data')}</td></tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>
{/if}
