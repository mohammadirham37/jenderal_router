<script lang="ts">
	import { ChartLine, Activity, Coins, AlertTriangle, Hash, Cpu, MemoryStick, HardDrive, Gauge } from '@lucide/svelte';
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
	let sys = $state<any>(null);

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

	// statistik sistem live (poll 3 dtk)
	$effect(() => {
		const load = () => api.get('/api/admin/system/stats').then((s) => (sys = s)).catch(() => {});
		load();
		const id = setInterval(load, 3000);
		return () => clearInterval(id);
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

	<!-- sistem live: CPU / RAM / Disk / GPU -->
	<div class="card" style="margin-top:14px">
		<h2><Cpu size={15} /> Sistem</h2>
		{#if sys && sys.supported}
			<div class="grid cols-3">
				<div>
					<div class="kv small"><span class="muted">CPU ({sys.cpu_cores} core)</span><span style="font-weight:800">{sys.cpu_percent}%</span></div>
					<div class="sys-spark">
						{#each sys.cpu_history as h}
							<div style="height:{Math.max(5, h)}%"></div>
						{/each}
					</div>
				</div>
				<div>
					<div class="kv small">
						<span class="muted"><MemoryStick size={12} style="vertical-align:-2px" /> RAM</span>
						<span style="font-weight:800">{sys.ram_used_gb} / {sys.ram_total_gb} GB</span>
					</div>
					<div class="sys-bar"><div class="fill" class:hot={sys.ram_used_gb / (sys.ram_total_gb || 1) > 0.85} style="width:{Math.min(100, (sys.ram_used_gb / (sys.ram_total_gb || 1)) * 100)}%"></div></div>
					<div class="muted small" style="margin-top:4px">{(sys.ram_used_gb / (sys.ram_total_gb || 1) * 100).toFixed(0)}% terpakai</div>
				</div>
				<div>
					<div class="kv small">
						<span class="muted"><HardDrive size={12} style="vertical-align:-2px" /> Penyimpanan</span>
						<span style="font-weight:800">{sys.disk_used_gb} / {sys.disk_total_gb} GB</span>
					</div>
					<div class="sys-bar"><div class="fill" class:hot={sys.disk_used_gb / (sys.disk_total_gb || 1) > 0.85} style="width:{Math.min(100, (sys.disk_used_gb / (sys.disk_total_gb || 1)) * 100)}%"></div></div>
					<div class="muted small" style="margin-top:4px">{(sys.disk_used_gb / (sys.disk_total_gb || 1) * 100).toFixed(0)}% terpakai</div>
				</div>
			</div>
			<div class="kv small" style="margin-top:10px">
				<span class="muted"><Gauge size={12} style="vertical-align:-2px" /> GPU:</span>
				{#if sys.gpu}
					<span class="badge info">{sys.gpu.name || 'gpu'}</span>
					{#if sys.gpu.percent !== null}<span>{sys.gpu.percent}%</span>{/if}
					{#if sys.gpu.mem_total_gb}<span class="muted">VRAM {sys.gpu.mem_used_gb}/{sys.gpu.mem_total_gb} GB</span>{/if}
				{:else}
					<span class="muted">tidak terdeteksi</span>
				{/if}
				<span class="muted" style="margin-left:auto">diperbarui {sys.updated_at?.replace('T', ' ').slice(11, 19)} UTC</span>
			</div>
		{:else if sys}
			<span class="muted small">Statistik sistem hanya tersedia di Linux.</span>
		{:else}
			<span class="muted small"><span class="spinner dark"></span> memuat statistik sistem…</span>
		{/if}
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

<style>
	.sys-spark {
		display: flex;
		align-items: flex-end;
		gap: 2px;
		height: 38px;
		margin-top: 6px;
		padding: 3px;
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: 8px;
		overflow: hidden;
	}
	.sys-spark div {
		flex: 1;
		min-width: 2px;
		background: var(--grad-accent);
		border-radius: 1px;
		opacity: 0.85;
	}
	.sys-bar {
		height: 8px;
		margin-top: 6px;
		background: var(--surface-3);
		border-radius: 6px;
		overflow: hidden;
	}
	.sys-bar .fill {
		height: 100%;
		background: var(--grad-accent);
		border-radius: 6px;
		transition: width 0.6s ease;
	}
	.sys-bar .fill.hot {
		background: var(--warn);
	}
</style>
