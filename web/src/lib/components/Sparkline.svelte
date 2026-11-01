<script lang="ts">
	// Garis tren mini untuk kartu KPI (SVG murni, tanpa library).
	let { values = [], color = 'var(--accent)', height = 34 }: { values?: number[]; color?: string; height?: number } = $props();
	const W = 120;
	const pts = $derived.by(() => {
		const v = values.length > 1 ? values : [0, ...values, 0];
		const max = Math.max(1, ...v);
		const step = W / (v.length - 1);
		return v.map((y, i) => [i * step, height - 3 - (y / max) * (height - 6)] as const);
	});
	const line = $derived(pts.map(([x, y], i) => `${i ? 'L' : 'M'}${x.toFixed(1)} ${y.toFixed(1)}`).join(' '));
	const area = $derived(`${line} L${W} ${height} L0 ${height} Z`);
	const gid = 'sg' + Math.random().toString(36).slice(2, 8);
</script>

<svg class="spark" viewBox="0 0 {W} {height}" preserveAspectRatio="none" aria-hidden="true" style="height:{height}px">
	<defs>
		<linearGradient id={gid} x1="0" x2="0" y1="0" y2="1">
			<stop offset="0" style="stop-color:{color};stop-opacity:.28" />
			<stop offset="1" style="stop-color:{color};stop-opacity:0" />
		</linearGradient>
	</defs>
	<path d={area} fill="url(#{gid})" />
	<path d={line} fill="none" style="stroke:{color}" stroke-width="1.6" stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke" />
</svg>

<style>
	.spark { display: block; width: 100%; }
</style>
