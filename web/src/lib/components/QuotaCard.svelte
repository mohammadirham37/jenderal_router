<script lang="ts">
	// Kartu "Kuota saya" untuk dashboard user: pemakaian vs batas per
	// periode (akun & per API key), waktu reset, dan peringatan bila hampir habis.
	import { Gauge } from '@lucide/svelte';
	import { fmtNum, fmtTs } from '#lib/api.js';

	let { quotas = [] }: { quotas?: any[] } = $props();

	const PERIOD: Record<string, string> = { day: 'Harian', month: 'Bulanan' };
	const usd = (v: number) => '$' + (v || 0).toFixed(2);
	const pct = (used: number, limit: number) => (limit > 0 ? Math.min(100, (used / limit) * 100) : 0);
	const metrics = (q: any) =>
		[
			{ lbl: 'Token', used: q.used_tokens, limit: q.token_limit, fmt: fmtNum },
			{ lbl: 'Request', used: q.used_requests, limit: q.request_limit, fmt: fmtNum },
			{ lbl: 'Biaya', used: q.used_cost_usd, limit: q.cost_limit_usd, fmt: usd }
		].filter((m) => m.limit > 0);
	const exhausted = (q: any) => metrics(q).some((m) => m.used >= m.limit);
	const sorted = $derived(
		[...quotas].sort((a, b) => (a.scope === b.scope ? (a.period === 'day' ? -1 : 1) : a.scope === 'user' ? -1 : 1))
	);
</script>

<div class="card quota-card">
	<h2><Gauge size={15} /> Kuota saya</h2>
	{#if !sorted.length}
		<p class="muted small" style="margin:0">Akun Anda tidak dibatasi kuota.</p>
	{:else}
		<div class="qgrid">
			{#each sorted as q (q.scope + q.key_name + q.period)}
				{@const ms = metrics(q)}
				<div class="qbox" class:hot={exhausted(q)}>
					<div class="kv small">
						<strong>{PERIOD[q.period] ?? q.period}</strong>
						{#if q.scope === 'key'}<span class="muted">· API key {q.key_name}</span>{/if}
						<span class="badge {q.source === 'custom' ? 'info' : 'ok'}" title={q.source === 'custom' ? 'Diatur khusus oleh admin untuk Anda' : 'Kuota standar yang berlaku untuk semua user'}>
							{q.source === 'custom' ? 'khusus' : 'standar'}
						</span>
					</div>
					{#each ms as m (m.lbl)}
						{@const p = pct(m.used, m.limit)}
						<div class="qrow small">
							<span class="qlbl muted">{m.lbl}</span>
							<div class="meter"><div class="fill" class:warn={p >= 80 && p < 100} class:full={p >= 100} style="width:{p}%"></div></div>
							<span class="mono">{m.fmt(m.used)} / {m.fmt(m.limit)}</span>
						</div>
					{:else}
						<div class="muted small" style="margin-top:6px">Tanpa batas</div>
					{/each}
					<div class="muted small reset">
						{#if exhausted(q)}<b class="err">Kuota habis</b> — {/if}
						{#if q.reset_at}reset {fmtTs(q.reset_at)}{/if}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.quota-card { margin-top: 14px; }
	.qgrid { display: grid; grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); gap: 12px; }
	.qbox { border: 1px solid var(--border); border-radius: 12px; padding: 10px 12px; }
	.qbox.hot { border-color: color-mix(in srgb, var(--err) 50%, var(--border)); }
	.qrow { display: flex; align-items: center; gap: 10px; margin-top: 8px; }
	.qlbl { width: 58px; }
	.meter { flex: 1; height: 8px; border-radius: 999px; background: var(--surface-2); overflow: hidden; }
	.fill { height: 100%; background: var(--grad-accent); }
	.fill.warn { background: var(--warn); }
	.fill.full { background: var(--err); }
	.reset { margin-top: 8px; }
	.err { color: var(--err); }
</style>
