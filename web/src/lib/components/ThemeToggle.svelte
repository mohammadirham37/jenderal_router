<script lang="ts">
	import { Sun, Moon } from '@lucide/svelte';

	let theme = $state(
		(typeof document !== 'undefined' && document.documentElement.getAttribute('data-theme')) || 'dark'
	);
	function toggle() {
		theme = theme === 'dark' ? 'light' : 'dark';
		localStorage.setItem('jr_theme', theme);
		document.documentElement.setAttribute('data-theme', theme);
	}
</script>

<button class="icon-btn" onclick={toggle} title="Tema gelap/terang">
	{#if theme === 'dark'}<Sun size={15} />{:else}<Moon size={15} />{/if}
</button>
