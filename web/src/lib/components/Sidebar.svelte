<script lang="ts">
	// Sidebar aplikasi: grup menu yang bisa dilipat, lencana status langsung
	// (admin), ringkasan kuota (member), pencarian ⌘K, dan mode rail ciut.
	import { page } from '$app/state';
	import {
		Satellite, Search, ChevronDown, LogOut, PanelLeftClose, PanelLeftOpen, X
	} from '@lucide/svelte';
	import ThemeToggle from '#lib/components/ThemeToggle.svelte';
	import LangToggle from '#lib/components/LangToggle.svelte';
	import { api } from '#lib/api.js';
	import { app, t, roleAtLeast } from '#lib/stores.svelte.js';
	import { GROUPS, visibleFor, isActive, type NavItem } from '#lib/nav.js';

	let {
		open = $bindable(false),
		collapsed = $bindable(false),
		onsearch,
		onlogout
	}: { open?: boolean; collapsed?: boolean; onsearch: () => void; onlogout: () => void } = $props();

	const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform);
	const isAdmin = $derived(!!app.me && roleAtLeast(app.me.role, 'admin'));

	// ---- grup menu (lipatan diingat; grup berisi halaman aktif selalu terbuka) ----
	let folded = $state<Record<string, boolean>>(
		(() => { try { return JSON.parse(localStorage.getItem('jr_nav_folded') || '{}'); } catch { return {}; } })()
	);
	function toggleGroup(key: string) {
		folded[key] = !folded[key];
		try { localStorage.setItem('jr_nav_folded', JSON.stringify(folded)); } catch { /* */ }
	}
	const groups = $derived.by(() => {
		const items = visibleFor(app.me?.role ?? '');
		return GROUPS.map((g) => {
			const list = items.filter((n) => n.group === g.key);
			const hasActive = list.some((n) => isActive(page.url.pathname, n.route));
			return { ...g, items: list, hasActive, folded: !!folded[g.key] && !hasActive };
		}).filter((g) => g.items.length);
	});

	// ---- lencana status (admin): provider bermasalah, error 1 jam, kuota ----
	let status = $state<any>(null);
	$effect(() => {
		if (!isAdmin) return;
		const load = () => {
			if (document.visibilityState !== 'visible') return;
			api.get('/api/admin/nav-status').then((s) => (status = s)).catch(() => {});
		};
		load();
		const id = setInterval(load, 60000);
		return () => clearInterval(id);
	});
	function badge(n: NavItem): { n: number; kind: 'err' | 'warn'; tip: string } | null {
		if (!status || !n.badge) return null;
		if (n.badge === 'providers') {
			const c = status.providers_down + status.providers_degraded;
			return c ? { n: c, kind: status.providers_down ? 'err' : 'warn', tip: `${status.providers_down} diputus · ${status.providers_degraded} terganggu` } : null;
		}
		if (n.badge === 'errors') {
			return status.errors_hour ? { n: status.errors_hour, kind: 'err', tip: `${status.errors_hour} request gagal dalam 1 jam terakhir` } : null;
		}
		return status.quota_alerts ? { n: status.quota_alerts, kind: 'warn', tip: `${status.quota_alerts} user kuotanya ≥90%` } : null;
	}

	// ---- ringkasan kuota (member) ----
	let quota = $state<{ pct: number; label: string } | null>(null);
	$effect(() => {
		if (!app.me || isAdmin) return;
		api.get('/api/me/usage').then((u) => {
			let best: { pct: number; label: string } | null = null;
			for (const q of u.quotas ?? []) {
				for (const [used, lim, name] of [
					[q.used_tokens, q.token_limit, 'token'], [q.used_requests, q.request_limit, 'request'], [q.used_cost_usd, q.cost_limit_usd, 'biaya']
				] as [number, number, string][]) {
					if (lim > 0) {
						const pct = Math.min(100, (used / lim) * 100);
						if (!best || pct > best.pct) best = { pct, label: `Kuota ${name} ${q.period === 'month' ? 'bulanan' : 'harian'}` };
					}
				}
			}
			quota = best;
		}).catch(() => {});
	});

	const initials = $derived((app.me?.email ?? '?').split('@')[0].replace(/[^a-z0-9]/gi, '').slice(0, 2).toUpperCase());
	const ROLE: Record<string, string> = { super_admin: 'Super admin', admin: 'Admin', member: 'Member', viewer: 'Viewer' };
</script>

<aside class="sb" class:collapsed class:open aria-label="Navigasi utama">
	<div class="sb-brand">
		<a class="logo" href="/dashboard" aria-label="JenderalRouter — Ringkasan">
			<Satellite size={18} /><i class="live" title="Server online"></i>
		</a>
		<div class="title">
			<b>JenderalRouter</b>
			<small>gateway LLM</small>
		</div>
		<button class="close" onclick={() => (open = false)} aria-label="Tutup menu"><X size={18} /></button>
	</div>

	<button class="sb-search" onclick={onsearch} data-tip="Cari menu ({isMac ? '⌘' : 'Ctrl'}+K)" aria-label="Cari menu">
		<Search size={15} />
		<span class="lbl">Cari menu…</span>
		<span class="keys"><kbd>{isMac ? '⌘' : 'Ctrl'}</kbd><kbd>K</kbd></span>
	</button>

	<nav class="sb-nav">
		{#each groups as g (g.key)}
			{#if g.label}
				<button class="sb-group" onclick={() => toggleGroup(g.key)} aria-expanded={!g.folded} disabled={g.hasActive}
					title={g.hasActive ? 'Berisi halaman yang sedang dibuka' : g.folded ? 'Tampilkan' : 'Lipat'}>
					<span>{g.label}</span>
					{#if g.folded}
						{@const sum = g.items.reduce((a, n) => a + (badge(n)?.n ?? 0), 0)}
						{#if sum}<span class="dot-sum"></span>{/if}
					{/if}
					<span class="chev" class:folded={g.folded}><ChevronDown size={13} /></span>
				</button>
			{/if}
			<div class="sb-items" class:folded={g.folded}>
				<div class="inner">
					{#each g.items as n (n.route)}
						{@const b = badge(n)}
						{@const active = isActive(page.url.pathname, n.route)}
						<a href={n.route} class="sb-item" class:active aria-current={active ? 'page' : undefined}
							data-tip={t(n.label) + (b ? ` — ${b.tip}` : '')} onclick={() => (open = false)} tabindex={g.folded ? -1 : 0}>
							<span class="ic"><n.icon size={17} /></span>
							<span class="lbl">{t(n.label)}</span>
							{#if b}<span class="badge-n {b.kind}" title={b.tip}>{b.n > 99 ? '99+' : b.n}</span>{/if}
						</a>
					{/each}
				</div>
			</div>
		{/each}
	</nav>

	<div class="sb-foot">
		{#if quota}
			<a class="quota" href="/dashboard" data-tip="{quota.label} {quota.pct.toFixed(0)}%">
				<div class="qtop"><span>{quota.label}</span><b>{quota.pct.toFixed(0)}%</b></div>
				<div class="qbar"><div class:warn={quota.pct >= 80} class:full={quota.pct >= 100} style="width:{quota.pct}%"></div></div>
			</a>
		{/if}
		<div class="user">
			<span class="avatar-g" title={app.me?.email}>{initials}</span>
			<div class="who">
				<div class="email" title={app.me?.email}>{app.me?.email}</div>
				<span class="role {app.me?.role}">{ROLE[app.me?.role ?? ''] ?? app.me?.role}</span>
			</div>
			<button class="icon-btn" onclick={onlogout} title={t('logout')} aria-label={t('logout')}><LogOut size={15} /></button>
		</div>
		<div class="ctrls">
			<ThemeToggle />
			<LangToggle />
			<span class="grow"></span>
			<button class="icon-btn collapse" onclick={() => (collapsed = !collapsed)}
				title={collapsed ? 'Perluas sidebar' : 'Ciutkan sidebar'} aria-label={collapsed ? 'Perluas sidebar' : 'Ciutkan sidebar'}>
				{#if collapsed}<PanelLeftOpen size={15} />{:else}<PanelLeftClose size={15} />{/if}
			</button>
		</div>
	</div>
</aside>

<style>
	.sb {
		position: sticky; top: 0; z-index: 40; height: 100vh; height: 100dvh;
		width: var(--sidebar-w); flex: 0 0 var(--sidebar-w);
		display: flex; flex-direction: column; gap: 12px; padding: 16px 12px 12px;
		background:
			radial-gradient(420px 260px at 0% 0%, color-mix(in srgb, var(--accent) 9%, transparent), transparent 70%),
			color-mix(in srgb, var(--surface) 90%, transparent);
		backdrop-filter: blur(14px);
		border-right: 1px solid var(--border);
		transition: width .22s cubic-bezier(.2,.7,.2,1), flex-basis .22s cubic-bezier(.2,.7,.2,1), transform .25s cubic-bezier(.2,.7,.3,1);
	}

	/* ---------- merek ---------- */
	.sb-brand { display: flex; align-items: center; gap: 11px; padding: 2px 6px 4px; }
	.logo {
		position: relative; flex: 0 0 auto; width: 36px; height: 36px; border-radius: 11px; display: grid; place-items: center;
		color: #fff; background: var(--grad-accent); box-shadow: 0 6px 18px color-mix(in srgb, var(--accent) 42%, transparent);
	}
	.live {
		position: absolute; right: -2px; bottom: -2px; width: 10px; height: 10px; border-radius: 50%;
		background: var(--ok); border: 2px solid var(--surface);
	}
	.title { display: flex; flex-direction: column; min-width: 0; line-height: 1.15; }
	.title b { font-size: 15px; font-weight: 800; letter-spacing: -0.015em; }
	.title small { font-family: var(--mono); font-size: 10.5px; color: var(--muted); letter-spacing: .04em; }
	.close { display: none; }

	/* ---------- cari ---------- */
	.sb-search {
		all: unset; box-sizing: border-box; display: flex; align-items: center; gap: 9px; height: 36px; padding: 0 10px;
		border-radius: 10px; cursor: pointer; font-size: 13px; color: var(--muted);
		background: var(--surface-2); border: 1px solid var(--border);
		transition: border-color var(--speed), color var(--speed);
	}
	.sb-search:hover { border-color: var(--border-strong); color: var(--text); }
	.sb-search:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
	.sb-search .lbl { flex: 1; }
	.keys { display: inline-flex; gap: 3px; }
	kbd {
		display: inline-grid; place-items: center; min-width: 18px; height: 18px; padding: 0 4px; border-radius: 5px;
		font-family: var(--mono); font-size: 10px; color: var(--muted);
		background: var(--surface); border: 1px solid var(--border); border-bottom-width: 2px;
	}

	/* ---------- menu ---------- */
	.sb-nav { flex: 1; min-height: 0; overflow-y: auto; overflow-x: hidden; margin: 0 -4px; padding: 0 4px; }
	.sb-group {
		all: unset; box-sizing: border-box; width: 100%; display: flex; align-items: center; gap: 6px;
		padding: 14px 10px 6px; font-size: 10.5px; font-weight: 750; letter-spacing: .1em; text-transform: uppercase; color: var(--muted);
		cursor: pointer;
	}
	.sb-group:disabled { cursor: default; }
	.sb-group:hover:not(:disabled) { color: var(--text); }
	.sb-group:focus-visible { outline: 2px solid var(--accent); border-radius: 6px; }
	.sb-group > span:first-child { flex: 1; }
	.chev { display: grid; transition: transform .2s; opacity: .7; }
	.chev.folded { transform: rotate(-90deg); }
	.sb-group:disabled .chev { opacity: .25; }
	.dot-sum { width: 6px; height: 6px; border-radius: 50%; background: var(--err); }

	/* lipat/buka halus via grid-template-rows */
	.sb-items { display: grid; grid-template-rows: 1fr; transition: grid-template-rows .22s ease; }
	.sb-items.folded { grid-template-rows: 0fr; }
	.sb-items .inner { min-height: 0; overflow: hidden; display: flex; flex-direction: column; gap: 2px; }

	.sb-item {
		position: relative; display: flex; align-items: center; gap: 10px; height: 38px; padding: 0 10px; border-radius: 10px;
		color: var(--muted); text-decoration: none; font-size: 13.5px; font-weight: 600;
		transition: background var(--speed), color var(--speed);
	}
	.sb-item .ic { display: grid; place-items: center; flex: 0 0 auto; transition: color var(--speed), transform var(--speed); }
	.sb-item .lbl { flex: 1; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.sb-item:hover { background: var(--surface-2); color: var(--text); }
	.sb-item:hover .ic { transform: translateX(1px); }
	.sb-item:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
	.sb-item.active {
		color: var(--text); font-weight: 700;
		background: linear-gradient(90deg, color-mix(in srgb, var(--accent) 18%, transparent), color-mix(in srgb, var(--accent) 6%, transparent));
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent) 24%, transparent);
	}
	.sb-item.active .ic { color: var(--accent); }
	.sb-item.active::before {
		content: ''; position: absolute; left: 0; top: 9px; bottom: 9px; width: 3px; border-radius: 0 3px 3px 0;
		background: var(--grad-accent);
	}
	.badge-n {
		min-width: 20px; height: 20px; padding: 0 6px; border-radius: 999px; display: inline-grid; place-items: center;
		font-family: var(--mono); font-size: 10.5px; font-weight: 700;
	}
	.badge-n.err { color: #fff; background: var(--err); }
	.badge-n.warn { color: #1b1300; background: var(--warn); }

	/* ---------- kaki ---------- */
	.sb-foot { display: flex; flex-direction: column; gap: 10px; padding-top: 12px; border-top: 1px solid var(--border); }
	.quota { display: block; padding: 9px 11px; border-radius: 11px; text-decoration: none; color: var(--text); background: var(--surface-2); border: 1px solid var(--border); }
	.quota:hover { border-color: var(--border-strong); }
	.qtop { display: flex; justify-content: space-between; font-size: 11.5px; color: var(--muted); }
	.qtop b { color: var(--text); font-family: var(--mono); }
	.qbar { height: 5px; margin-top: 6px; border-radius: 999px; background: var(--surface-3); overflow: hidden; }
	.qbar div { height: 100%; border-radius: 999px; background: var(--grad-accent); }
	.qbar div.warn { background: var(--warn); }
	.qbar div.full { background: var(--err); }

	.user { display: flex; align-items: center; gap: 10px; padding: 4px 4px 0; }
	.avatar-g {
		flex: 0 0 auto; width: 34px; height: 34px; border-radius: 50%; display: grid; place-items: center;
		font-weight: 800; font-size: 12.5px; color: #fff; background: var(--grad-accent);
		box-shadow: 0 0 0 2px var(--surface), 0 0 0 3.5px color-mix(in srgb, var(--accent) 45%, transparent);
	}
	.who { flex: 1; min-width: 0; }
	.email { font-size: 12.5px; font-weight: 650; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.role {
		display: inline-block; margin-top: 2px; padding: 1px 7px; border-radius: 999px; font-size: 10.5px; font-weight: 700;
		color: var(--accent-2); background: color-mix(in srgb, var(--accent-2) 13%, transparent);
	}
	.role.super_admin { color: var(--err); background: color-mix(in srgb, var(--err) 13%, transparent); }
	.role.admin { color: var(--warn); background: color-mix(in srgb, var(--warn) 14%, transparent); }
	.ctrls { display: flex; align-items: center; gap: 6px; }

	/* ---------- mode rail (desktop) ---------- */
	@media (min-width: 901px) {
		.sb.collapsed { width: 72px; flex-basis: 72px; padding: 16px 10px 12px; }
		.collapsed .sb-brand { justify-content: center; padding: 2px 0 4px; }
		.collapsed .title, .collapsed .sb-search .lbl, .collapsed .keys, .collapsed .sb-item .lbl,
		.collapsed .who, .collapsed .user .icon-btn, .collapsed .ctrls > :global(:not(.collapse)), .collapsed .ctrls .grow,
		.collapsed .quota .qtop { display: none; }
		.collapsed .sb-search { justify-content: center; padding: 0; }
		.collapsed .sb-group { justify-content: center; padding: 10px 0 4px; font-size: 0; }
		.collapsed .sb-group::before { content: ''; width: 22px; height: 1px; background: var(--border-strong); }
		.collapsed .sb-group > span { display: none; }
		.collapsed .sb-items.folded { grid-template-rows: 1fr; } /* rail selalu menampilkan semua ikon */
		/* tooltip keluar dari rail — jangan dipotong overflow */
		.collapsed .sb-nav, .collapsed .sb-items .inner { overflow: visible; }
		.collapsed .sb-item { justify-content: center; padding: 0; }
		.collapsed .badge-n { position: absolute; top: 2px; right: 2px; min-width: 16px; height: 16px; padding: 0 4px; font-size: 9.5px; }
		.collapsed .user { justify-content: center; padding: 0; }
		.collapsed .ctrls { justify-content: center; }
		.collapsed .quota { padding: 6px; }
		.collapsed .quota .qbar { margin: 0; }
		/* tooltip label di kanan rail */
		.collapsed [data-tip] { position: relative; }
		.collapsed [data-tip]:hover::after, .collapsed [data-tip]:focus-visible::after {
			content: attr(data-tip); position: absolute; left: calc(100% + 12px); top: 50%; transform: translateY(-50%);
			z-index: 60; white-space: nowrap; padding: 6px 10px; border-radius: 8px; font-size: 12px; font-weight: 600;
			color: var(--text); background: var(--surface); border: 1px solid var(--border-strong); box-shadow: var(--shadow);
			pointer-events: none;
		}
	}

	/* ---------- drawer (HP/tablet) ---------- */
	@media (max-width: 900px) {
		.sb {
			position: fixed; top: 0; bottom: 0; left: 0; z-index: 70;
			width: min(300px, 86vw); flex: none; height: 100dvh;
			background: var(--surface); box-shadow: 0 12px 48px rgb(0 0 0 / .4);
			transform: translateX(-105%); visibility: hidden;
			transition: transform .25s cubic-bezier(.2,.7,.3,1), visibility 0s .25s;
		}
		.sb.open { transform: none; visibility: visible; transition: transform .25s cubic-bezier(.2,.7,.3,1); }
		.close {
			all: unset; margin-left: auto; width: 34px; height: 34px; border-radius: 9px; display: grid; place-items: center;
			color: var(--muted); cursor: pointer;
		}
		.close:hover { background: var(--surface-2); color: var(--text); }
		.collapse { display: none; }
	}
	@media (prefers-reduced-motion: reduce) {
		.sb, .sb-items, .chev { transition: none; }
	}
</style>
