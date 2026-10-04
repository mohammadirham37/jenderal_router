<script lang="ts">
	import { X } from '@lucide/svelte';
	import { t } from '$lib/stores.svelte';

	let {
		title,
		onclose,
		children
	}: { title: string; onclose: () => void; children: import('svelte').Snippet } = $props();

	function backdropClick(e: MouseEvent) {
		if (e.target === e.currentTarget) onclose();
	}
	function keydown(e: KeyboardEvent) {
		if (e.key === 'Escape') onclose();
	}
</script>

<svelte:window on:keydown={keydown} />

<div class="modal-back" onclick={backdropClick} role="presentation">
	<div class="modal" role="dialog" aria-modal="true" aria-label={title}>
		<div class="row" style="justify-content:space-between;margin-bottom:6px">
			<h2 style="margin:0">{title}</h2>
			<button class="icon-btn" onclick={onclose} aria-label={t('cancel')}><X size={16} /></button>
		</div>
		{@render children()}
	</div>
</div>
