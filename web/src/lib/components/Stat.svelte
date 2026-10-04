<script lang="ts">
	import { fmtNum } from '$lib/api';

	let {
		icon: Icon,
		label,
		value,
		sub
	}: { icon: any; label: string; value: string; sub?: string } = $props();
</script>

<div class="card stat">
	<span class="lbl"><Icon size={14} /> {label}</span>
	<span class="val">{value}</span>
	{#if sub}<span class="muted small">{sub}</span>{/if}
</div>
