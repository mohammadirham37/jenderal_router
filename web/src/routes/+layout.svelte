<script lang="ts">
	import '../app.css';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		Satellite, LayoutDashboard, MessageSquare, Cable, BrainCircuit, Link2, Users,
		ScrollText, ChartLine, Server, Settings, LogOut
	} from '@lucide/svelte';
	import ThemeToggle from '$lib/components/ThemeToggle.svelte';
	import LangToggle from '$lib/components/LangToggle.svelte';
	import { api, setCSRF, resetCSRF, setUnauthorizedHandler } from '$lib/api';
	import { app, t, toast, roleAtLeast } from '$lib/stores.svelte';
	import Toasts from '$lib/components/Toasts.svelte';

	const NAV = [
		{ route: '/dashboard', icon: LayoutDashboard, label: 'dashboard', min: 'viewer', section: 'main' },
		{ route: '/chat', icon: MessageSquare, label: 'chat', min: 'member', section: 'main' },
		{ route: '/providers', icon: Cable, label: 'providers', min: 'viewer', section: 'gateway' },
		{ route: '/models', icon: BrainCircuit, label: 'models', min: 'viewer', section: 'gateway' },
		{ route: '/combos', icon: Link2, label: 'combos', min: 'viewer', section: 'gateway' },
		{ route: '/users', icon: Users, label: 'users', min: 'admin', section: 'kelola' },
		{ route: '/logs', icon: ScrollText, label: 'logs', min: 'viewer', section: 'observabilitas' },
		{ route: '/analytics', icon: ChartLine, label: 'analytics', min: 'viewer', section: 'observabilitas' },
		{ route: '/local', icon: Server, label: 'local', min: 'viewer', section: 'sistem' },
		{ route: '/settings', icon: Settings, label: 'settings', min: 'admin', section: 'sistem' }
	];
	const sections: Record<string, string> = {
		main: t('dashboard'),
		gateway: 'Gateway',
		kelola: 'Kelola',
		observabilitas: 'Observabilitas',
		sistem: 'Sistem'
	};

	let { children } = $props();

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
				if (page.url.pathname === '/') goto('/dashboard', { replaceState: true });
			} catch {
				goto('/login', { replaceState: true });
			}
		})();
	});

	async function logout() {
		try { await api.post('/api/logout', {}); } catch { /* */ }
		app.me = null;
		resetCSRF();
		goto('/login');
	}

	const initials = $derived(
		(app.me?.email ?? '?')
			.split('@')[0]
			.slice(0, 2)
			.toUpperCase()
	);
</script>

{#if page.url.pathname === '/login'}
	{@render children()}
{:else if app.me}
	<div class="shell">
		<aside class="sidebar">
			<div class="brand">
				<span class="logo"><Satellite size={19} /></span>
				<span>JenderalRouter</span>
			</div>
			<nav class="nav">
				{#each NAV as item, i}
					{#if roleAtLeast(app.me.role, item.min)}
						{#if item.section !== 'main' && (i === 0 || NAV[i - 1].section !== item.section)}
							<div class="nav-section">{sections[item.section]}</div>
						{/if}
						<a href={item.route} class={page.url.pathname === item.route ? 'active' : ''}>
							<item.icon size={17} />
							{t(item.label)}
						</a>
					{/if}
				{/each}
			</nav>
			<div class="sidebar-foot">
				<div class="user-chip">
					<span class="avatar">{initials}</span>
					<span class="grow small" style="min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">
						{app.me.email}
						<div class="muted" style="font-size:10.5px">{app.me.role}</div>
					</span>
					<button class="icon-btn" onclick={logout} title={t('logout')}><LogOut size={15} /></button>
				</div>
				<div class="row" style="justify-content:space-between">
					<ThemeToggle />
					<LangToggle />
				</div>
			</div>
		</aside>
		<main class="main">
			{@render children()}
		</main>
	</div>
{:else}
	<div class="auth-screen"><div class="spinner dark"></div></div>
{/if}
<Toasts />
