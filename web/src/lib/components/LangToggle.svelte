<script lang="ts">
	import { Languages } from '@lucide/svelte';
	import { app, toggleLang } from '$lib/stores.svelte';
</script>

<button class="icon-btn" onclick={toggleLang} title="Bahasa / Language"
	style="width:auto;padding:0 9px;font-weight:700;font-size:11px">
	<Languages size={14} /> {app.lang.toUpperCase()}
</button>
