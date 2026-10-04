<script lang="ts">
	import { ChartLine } from '@lucide/svelte';
	import { api, fmtNum, fmtCost } from '$lib/api';
	import { t, toast } from '$lib/stores.svelte';
	import Stat from '$lib/components/Stat.svelte';

	let res = $state<any>(null);
	let loading = $state(true);

	$effect(() => {
		(async () => {
			try {
				res = await api.get('/api/admin/analytics/summary?days=7');
			} catch (e: any) {
				toast(e.message, 'err');
			} finally {
				loading = false;
			}
		})();
	});

	const table = (title: string, head: string[], rows: any[][], key: string) => ({ title, head, rows, key });
	const tables = $derived(
		res
			? [
					table(t('per_day'), [t('time'), t('requests'), t('errors'), t('tokens'), t('cost')],
						(res.per_day || []).map((d: any) => [d.day, d.requests, d.errors, d.tokens_in + d.tokens_out, fmtCost(d.cost_usd)]), 'd'),
					table(t('per_provider'), [t('provider'), t('requests'), t('errors'), t('tokens'), t('cost')],
						(res.per_provider || []).map((p: any) => [p.provider_name, p.requests, p.errors, p.tokens_in + p.tokens_out, fmtCost(p.cost_usd)]), 'p'),
					table(t('per_user'), [t('user'), t('requests'), t('tokens'), t('cost')],
						(res.per_user || []).map((u: any) => [u.email, u.requests, u.tokens_in + u.tokens_out, fmtCost(u.cost_usd)]), 'u')
				]
			: []
	);
</script>

<svelte:head><title>{t('analytics')} — JenderalRouter</title></svelte:head>

<div class="page-head">
	<h1 class="page-title"><ChartLine size={20} /> {t('analytics')}</h1>
</div>

{#if loading}
	<div class="card"><span class="spinner dark"></span> {t('loading')}…</div>
{:else if res}
	<div class="grid cols-4">
		<Stat icon={ChartLine} label={t('requests')} value={fmtNum(res.totals.requests)} />
		<Stat icon={ChartLine} label={t('tokens')} value={fmtNum(res.totals.tokens_in + res.totals.tokens_out)} />
		<Stat icon={ChartLine} label={t('cost')} value={fmtCost(res.totals.cost_usd)} />
		<Stat icon={ChartLine} label="p95 {t('latency')}" value={(res.totals.p95_latency_ms || 0) + ' ms'} />
	</div>

	{#each tables as tb (tb.key)}
		<div class="card" style="margin-top:14px">
			<h2>{tb.title}</h2>
			<div class="table-wrap">
				<table>
					<thead><tr>{#each tb.head as h}<th>{h}</th>{/each}</tr></thead>
					<tbody>
						{#each tb.rows as row}
							<tr>{#each row as cell}<td>{cell}</td>{/each}</tr>
						{:else}
							<tr><td colspan={tb.head.length} class="muted">{t('no_data')}</td></tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	{/each}
{/if}
