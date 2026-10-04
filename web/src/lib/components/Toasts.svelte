<script lang="ts">
	import { X } from '@lucide/svelte';
	import { app, dismissToast } from '$lib/stores.svelte';
</script>

<div class="toasts">
	{#each app.toasts as t (t.id)}
		<div class="toast {t.kind}">
			<span class="grow">{t.msg}</span>
			<button class="icon-btn" style="width:24px;height:24px" onclick={() => dismissToast(t.id)} aria-label="tutup">
				<X size={13} />
			</button>
		</div>
	{/each}
</div>
