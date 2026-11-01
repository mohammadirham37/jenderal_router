<script lang="ts">
	// Grafik batang bertumpuk (mis. sukses/gagal atau chat/API) dengan
	// tooltip saat hover/fokus. Data: daftar titik { label, values[] }.
	type Series = { name: string; color: string };
	type Point = { label: string; sub?: string; values: number[] };
	let {
		points = [],
		series = [],
		format = (v: number) => String(v),
		height = 210
	}: { points?: Point[]; series?: Series[]; format?: (v: number) => string; height?: number } = $props();

	let hover = $state(-1);
	const totals = $derived(points.map((p) => p.values.reduce((a, b) => a + b, 0)));
	const max = $derived(Math.max(1, ...totals));
	// skala sumbu Y yang "bulat"
	const ticks = $derived.by(() => {
		const raw = max / 3;
		const mag = 10 ** Math.floor(Math.log10(raw || 1));
		const step = Math.ceil(raw / mag) * mag;
		return [step * 3, step * 2, step, 0];
	});
	const top = $derived(ticks[0] || 1);
	// tampilkan label sumbu X secukupnya: ±1 label per 56px lebar grafik
	let plotW = $state(600);
	const every = $derived(Math.max(1, Math.ceil(points.length / Math.max(2, Math.floor(plotW / 56)))));
	const empty = $derived(totals.every((v) => v === 0));
</script>

<div class="tc" style="--h:{height}px">
	<div class="y">
		{#each ticks as tk (tk)}<span>{format(tk)}</span>{/each}
	</div>
	<div class="plot" role="list" bind:clientWidth={plotW} onmouseleave={() => (hover = -1)}>
		<div class="gridlines" aria-hidden="true">{#each ticks as tk (tk)}<i></i>{/each}</div>
		{#if empty}<div class="empty muted small">Belum ada data pada rentang ini</div>{/if}
		{#each points as p, i (p.label + i)}
			<!-- bisa difokus keyboard agar tooltip nilai juga terbaca tanpa mouse -->
			<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
			<div class="col" role="listitem" tabindex="0" onmouseenter={() => (hover = i)} onfocus={() => (hover = i)} onblur={() => (hover = -1)}
				aria-label="{p.label}: {series.map((s, k) => `${s.name} ${format(p.values[k] ?? 0)}`).join(', ')}">
				<div class="stack" style="height:{(totals[i] / top) * 100}%">
					{#each series as s, k (s.name)}
						{#if (p.values[k] ?? 0) > 0}
							<div class="seg" style="flex:{p.values[k]};background:{s.color}"></div>
						{/if}
					{/each}
				</div>
				{#if i === points.length - 1 || (i % every === 0 && points.length - 1 - i >= every)}<span class="xl">{p.sub ?? p.label}</span>{/if}
				{#if hover === i}
					<div class="tip" class:left={i > points.length * 0.6}>
						<b>{p.label}</b>
						{#each series as s, k (s.name)}
							<div class="tr"><i style="background:{s.color}"></i>{s.name}<span>{format(p.values[k] ?? 0)}</span></div>
						{/each}
						{#if series.length > 1}<div class="tr total">Total<span>{format(totals[i])}</span></div>{/if}
					</div>
				{/if}
			</div>
		{/each}
	</div>
</div>

<style>
	.tc { display: grid; grid-template-columns: auto 1fr; gap: 8px; height: calc(var(--h) + 22px); }
	.y { display: flex; flex-direction: column; justify-content: space-between; height: var(--h); font-family: var(--mono); font-size: 10.5px; color: var(--muted); text-align: right; }
	.y span { transform: translateY(-50%); }
	.y span:last-child { transform: translateY(-30%); }
	.plot { position: relative; display: flex; align-items: flex-end; gap: 3px; height: var(--h); }
	.gridlines { position: absolute; inset: 0; display: flex; flex-direction: column; justify-content: space-between; pointer-events: none; }
	.gridlines i { border-top: 1px dashed color-mix(in srgb, var(--border) 85%, transparent); }
	.gridlines i:last-child { border-top-style: solid; }
	.empty { position: absolute; inset: 0; display: grid; place-items: center; }
	.col { position: relative; flex: 1; height: 100%; display: flex; flex-direction: column; justify-content: flex-end; outline: none; cursor: default; }
	.col::before { content: ''; position: absolute; inset: 0 -1px; border-radius: 6px; background: transparent; transition: background .12s; }
	.col:hover::before, .col:focus-visible::before { background: color-mix(in srgb, var(--accent) 7%, transparent); }
	.stack { position: relative; display: flex; flex-direction: column-reverse; min-height: 0; border-radius: 5px 5px 2px 2px; overflow: hidden; transition: height .45s cubic-bezier(.2,.7,.2,1); }
	.seg { min-height: 2px; }
	.xl { position: absolute; top: calc(100% + 6px); left: 50%; transform: translateX(-50%); font-family: var(--mono); font-size: 10.5px; color: var(--muted); white-space: nowrap; }
	.tip {
		position: absolute; bottom: calc(100% + 8px); left: 50%; transform: translateX(-30%); z-index: 5; min-width: 150px;
		background: var(--surface); border: 1px solid var(--border-strong); border-radius: 10px; padding: 8px 10px;
		box-shadow: var(--shadow); font-size: 12px; pointer-events: none;
	}
	.tip.left { transform: translateX(-80%); }
	.tip b { display: block; margin-bottom: 4px; font-size: 11.5px; }
	.tr { display: flex; align-items: center; gap: 6px; color: var(--muted); }
	.tr i { width: 8px; height: 8px; border-radius: 2px; }
	.tr span { margin-left: auto; color: var(--text); font-family: var(--mono); font-variant-numeric: tabular-nums; }
	.tr.total { border-top: 1px solid var(--border); margin-top: 4px; padding-top: 4px; }
</style>
