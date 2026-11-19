<script lang="ts">
	import { Sun, Moon } from '@lucide/svelte';
	import { app, toggleTheme } from '#lib/stores.svelte.js';
</script>

<button class="icon-btn" onclick={toggleTheme} title="Tema gelap/terang" aria-label="Ganti tema gelap/terang">
	{#if app.theme === 'dark'}<Sun size={15} />{:else}<Moon size={15} />{/if}
</button>
