<script lang="ts">
	import '@fontsource-variable/plus-jakarta-sans';
	import '@fontsource-variable/jetbrains-mono';
	import '@fontsource/instrument-serif/latin-400.css';
	import '@fontsource/instrument-serif/latin-400-italic.css';
	import '../app.css';
	import { page, updated } from '$app/state';
	import { goto } from '$app/navigation';
	import { Satellite, Menu, Search, RefreshCw } from '@lucide/svelte';
	import { browser } from '$app/env';
	import { api, setCSRF, resetCSRF, setUnauthorizedHandler } from '#lib/api.js';
	import { app } from '#lib/stores.svelte.js';
	import Sidebar from '#lib/components/Sidebar.svelte';
	import CommandPalette from '#lib/components/CommandPalette.svelte';
	import Toasts from '#lib/components/Toasts.svelte';

	let { children } = $props();

	let menuOpen = $state(false);
	let paletteOpen = $state(false);
	// ⌘K / Ctrl+K membuka palet perintah dari halaman mana pun
	function onGlobalKey(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k' && app.me) {
			e.preventDefault();
			menuOpen = false;
			paletteOpen = !paletteOpen;
		}
	}
	// collapse sidebar di desktop — preferensi diingat antar-kunjungan
	let collapsed = $state(typeof localStorage !== 'undefined' && localStorage.getItem('jr_sidebar') === '1');
	$effect(() => {
		try { localStorage.setItem('jr_sidebar', collapsed ? '1' : '0'); } catch { /* */ }
	});

	setUnauthorizedHandler(() => {
		resetCSRF();
		app.me = null;
		goto('/login');
	});

	$effect(() => {
		// bootstrap sesi sekali saat layout terpasang (hard load)
		(async () => {
			try {
				const res = await api.get('/api/me', { noRedirect: true });
				app.me = res.user;
				setCSRF(res.csrf);
				if (page.url.pathname === '/') goto('/dashboard', { replace: true });
			} catch {
				goto('/login', { replace: true });
			}
		})();
	});

	async function logout() {
		try { await api.post('/api/logout', {}); } catch { /* */ }
		app.me = null;
		resetCSRF();
		goto('/login');
	}

</script>

{#if page.url.pathname === '/login'}
	{@render children()}
{:else if app.me}
	<div class="shell">
		{#if menuOpen}
			<div
				class="backdrop"
				onclick={() => menuOpen = false}
				aria-hidden="true"
			></div>
		{/if}
		<Sidebar bind:open={menuOpen} bind:collapsed onsearch={() => { menuOpen = false; paletteOpen = true; }} onlogout={logout} />
		<main class="main">
			<header class="topbar">
				<button class="icon-btn burger" aria-label="Buka menu" onclick={() => (menuOpen = true)}><Menu size={20} /></button>
				<span class="brand-mini">
					<span class="logo"><Satellite size={15} /></span>
					JenderalRouter
				</span>
				<span class="grow"></span>
				<button class="icon-btn burger" aria-label="Cari menu" onclick={() => (paletteOpen = true)}><Search size={18} /></button>
			</header>
			{#if updated.current}
				<div class="update-banner" role="status">
					<RefreshCw size={15} />
					<span class="grow"><b>Versi baru dashboard sudah terpasang.</b> Muat ulang agar fitur terbaru aktif (tab ini masih menjalankan versi lama).</span>
					<button class="btn sm" onclick={() => location.reload()}>Muat ulang</button>
				</div>
			{/if}
			{@render children()}
		</main>
	</div>
{:else}
	<div class="auth-screen"><div class="spinner dark"></div></div>
{/if}
<svelte:window onfocus={() => { if (!updated.current) updated.check().catch(() => {}); }} onkeydown={onGlobalKey} />
{#if app.me}<CommandPalette bind:open={paletteOpen} onlogout={logout} />{/if}
<Toasts />
<style>
	.update-banner {
		display: flex; align-items: center; gap: 10px; margin: 0 0 14px; padding: 10px 14px;
		border-radius: 12px; font-size: 13px; animation: rise .3s ease;
		background: color-mix(in srgb, var(--accent) 12%, var(--surface));
		border: 1px solid color-mix(in srgb, var(--accent) 45%, var(--border));
	}
	@keyframes rise { from { opacity: 0; transform: translateY(-4px); } to { opacity: 1; transform: none; } }
</style>
