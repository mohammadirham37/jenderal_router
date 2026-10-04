<script lang="ts">
	import { goto } from '$app/navigation';
	import { Satellite, LogIn, ShieldCheck } from '@lucide/svelte';
	import ThemeToggle from '$lib/components/ThemeToggle.svelte';
	import LangToggle from '$lib/components/LangToggle.svelte';
	import { api, setCSRF } from '$lib/api';
	import { app, t, toast } from '$lib/stores.svelte';

	let needsSetup = $state<boolean | null>(null);
	let email = $state('');
	let pass = $state('');
	let err = $state('');
	let busy = $state(false);

	$effect(() => {
		(async () => {
			try {
				const s = await api.get('/api/setup/status', { noRedirect: true });
				needsSetup = s.needs_setup;
			} catch {
				needsSetup = false;
			}
		})();
	});

	async function submit() {
		err = '';
		busy = true;
		try {
			const res = needsSetup
				? await api.post('/api/setup', { email, password: pass }, { noRedirect: true })
				: await api.post('/api/login', { email, password: pass }, { noRedirect: true });
			setCSRF(res.csrf);
			app.me = res.user;
			toast(needsSetup ? 'Admin dibuat — selamat datang! 🎉' : 'Selamat datang kembali', 'ok');
			goto('/dashboard', { replaceState: true });
		} catch (e: any) {
			err = e.message;
		} finally {
			busy = false;
		}
	}

	// Peta rute: hub kiri, node provider kanan (posisi %, selaras dengan path SVG).
	const nodes = [
		{ name: 'openai', x: 86, y: 10.5 },
		{ name: 'anthropic', x: 86, y: 36.8 },
		{ name: 'gemini', x: 86, y: 63.2 },
		{ name: 'llamastash', x: 86, y: 87.4 }
	];
</script>

<svelte:head><title>JenderalRouter — {needsSetup ? t('setup_title') : t('login')}</title></svelte:head>

<div class="auth">
	<section class="auth-side">
		<div class="topbar rise">
			<span class="brand"><span class="logo"><Satellite size={19} /></span>JenderalRouter</span>
			<span class="row"><ThemeToggle /><LangToggle /></span>
		</div>

		<div class="side-copy rise d1">
			<h1>{t('auth_headline')}</h1>
			<p>{t('auth_sub')}</p>
		</div>

		<div class="map-wrap rise d2" aria-hidden="true">
			<div class="route-map">
				<svg viewBox="0 0 560 380" preserveAspectRatio="none">
					<path class="route" d="M112 190 C 220 190, 310 40, 448 40" />
					<path class="route" d="M112 190 C 230 190, 320 140, 448 140" />
					<path class="route" d="M112 190 C 230 190, 320 240, 448 240" />
					<path class="route" d="M112 190 C 220 190, 310 332, 448 332" />
					<circle class="pkt" r="3.2">
						<animateMotion dur="3.2s" begin="0s" repeatCount="indefinite" path="M112 190 C 220 190, 310 40, 448 40" />
						<animate attributeName="opacity" values="0;1;1;0" keyTimes="0;0.1;0.82;1" dur="3.2s" begin="0s" repeatCount="indefinite" />
					</circle>
					<circle class="pkt" r="3.2">
						<animateMotion dur="2.8s" begin="-1.1s" repeatCount="indefinite" path="M112 190 C 230 190, 320 140, 448 140" />
						<animate attributeName="opacity" values="0;1;1;0" keyTimes="0;0.1;0.82;1" dur="2.8s" begin="-1.1s" repeatCount="indefinite" />
					</circle>
					<circle class="pkt" r="3.2">
						<animateMotion dur="2.9s" begin="-2s" repeatCount="indefinite" path="M112 190 C 230 190, 320 240, 448 240" />
						<animate attributeName="opacity" values="0;1;1;0" keyTimes="0;0.1;0.82;1" dur="2.9s" begin="-2s" repeatCount="indefinite" />
					</circle>
					<circle class="pkt" r="3.2">
						<animateMotion dur="3.5s" begin="-0.6s" repeatCount="indefinite" path="M112 190 C 220 190, 310 332, 448 332" />
						<animate attributeName="opacity" values="0;1;1;0" keyTimes="0;0.1;0.82;1" dur="3.5s" begin="-0.6s" repeatCount="indefinite" />
					</circle>
					<circle class="pkt pkt-b" r="2.6">
						<animateMotion dur="2.8s" begin="-2.3s" repeatCount="indefinite" path="M112 190 C 230 190, 320 140, 448 140" />
						<animate attributeName="opacity" values="0;1;1;0" keyTimes="0;0.1;0.82;1" dur="2.8s" begin="-2.3s" repeatCount="indefinite" />
					</circle>
				</svg>
				<span class="hub" style="left:14.3%;top:50%"><Satellite size={17} /></span>
				{#each nodes as n}
					<span class="node" style="left:{n.x}%;top:{n.y}%">{n.name}</span>
				{/each}
			</div>
		</div>
	</section>

	<section class="auth-panel">
		<div class="auth-card rise d3">
			{#if needsSetup}
				<span class="mode-chip"><ShieldCheck size={15} /> first run</span>
			{/if}
			<h2>{needsSetup ? t('setup_title') : t('login')}</h2>
			<p class="muted small card-sub">
				{needsSetup ? t('setup_desc') : t('login_continue')}
			</p>

			<form onsubmit={(e) => { e.preventDefault(); submit(); }}>
				<label for="f-email">{t('email')}</label>
				<input id="f-email" type="email" bind:value={email} placeholder="admin@domainanda.com"
					autocomplete="username" required />

				<label for="f-pass">{t('password')}</label>
				<input id="f-pass" type="password" bind:value={pass}
					placeholder={needsSetup ? '≥12 karakter, huruf besar+kecil+angka' : t('password')}
					autocomplete={needsSetup ? 'new-password' : 'current-password'} required minlength={8} />

				<div class="form-error" role="alert">{err}</div>
				<button class="btn auth-submit" type="submit" disabled={busy || needsSetup === null}>
					{#if busy}<span class="spinner"></span>{:else if needsSetup}<ShieldCheck size={16} />{:else}<LogIn size={16} />{/if}
					{needsSetup ? t('setup_btn') : t('login_btn')}
				</button>
			</form>
		</div>
	</section>
</div>

<style>
	.auth {
		min-height: 100vh;
		display: grid;
		grid-template-columns: minmax(0, 1.15fr) minmax(380px, 460px);
	}
	.auth-side {
		display: flex;
		flex-direction: column;
		padding: 26px 48px 30px 44px;
		min-width: 0;
	}
	.topbar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 10px;
	}
	.brand {
		display: inline-flex;
		align-items: center;
		gap: 10px;
		font-weight: 800;
		font-size: 15px;
		letter-spacing: -0.01em;
	}
	.brand .logo,
	.hub {
		background: var(--grad-accent);
		color: #fff;
		display: grid;
		place-items: center;
		border-radius: 10px;
		box-shadow: 0 4px 16px rgb(79 124 255 / 0.4);
	}
	.brand .logo { width: 32px; height: 32px; }

	.side-copy { margin-top: 34px; padding-top: 12px; }
	.side-copy h1 {
		font-size: clamp(26px, 3.1vw, 38px);
		line-height: 1.12;
		letter-spacing: -0.025em;
		font-weight: 800;
		margin: 0;
		max-width: 15ch;
	}
	.side-copy p {
		color: var(--muted);
		font-size: 14px;
		line-height: 1.65;
		margin: 14px 0 0;
		max-width: 42ch;
	}

	.map-wrap { flex: 1; display: grid; place-items: center; padding: 26px 0 4px; min-height: 250px; }
	.route-map { position: relative; width: min(100%, 620px); aspect-ratio: 560 / 380; }
	.route-map svg { position: absolute; inset: 0; width: 100%; height: 100%; }
	.route {
		fill: none;
		stroke: var(--border-strong);
		stroke-width: 1.2;
		opacity: 0.75;
	}
	.pkt { fill: var(--accent-2); }
	.pkt-b { fill: var(--accent); }
	.hub {
		position: absolute;
		width: 38px;
		height: 38px;
		border-radius: 12px;
		transform: translate(-50%, -50%);
	}
	.node {
		position: absolute;
		transform: translate(-50%, -50%);
		background: var(--surface-2);
		border: 1px solid var(--border);
		color: var(--muted);
		font-family: var(--mono);
		font-size: 11px;
		padding: 4px 10px;
		border-radius: 999px;
		white-space: nowrap;
		box-shadow: var(--shadow-sm);
	}

	.auth-panel {
		display: grid;
		place-items: center;
		padding: 36px 34px;
		background: color-mix(in srgb, var(--surface) 55%, transparent);
		border-left: 1px solid var(--border);
	}
	.auth-card {
		width: 100%;
		max-width: 400px;
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		padding: 30px 28px 26px;
		box-shadow: var(--shadow);
	}
	.mode-chip {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		font-family: var(--mono);
		font-size: 10.5px;
		font-weight: 700;
		color: var(--accent-2);
		background: var(--grad-accent-soft);
		border: 1px solid var(--border);
		border-radius: 999px;
		padding: 3px 10px;
		margin-bottom: 12px;
	}
	.auth-card h2 {
		font-size: 19px;
		font-weight: 800;
		letter-spacing: -0.02em;
		margin: 0;
	}
	.card-sub { margin: 5px 0 4px; line-height: 1.55; }
	.auth-card label:first-of-type { margin-top: 10px; }
	.auth-submit { width: 100%; margin-top: 6px; padding: 11px 15px; font-size: 13.5px; }
	.auth :global(button:focus-visible) {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}

	@keyframes rise {
		from { opacity: 0; transform: translateY(10px); }
		to { opacity: 1; transform: none; }
	}
	.rise { animation: rise 0.55s cubic-bezier(0.2, 0.7, 0.3, 1) both; }
	.d1 { animation-delay: 0.07s; }
	.d2 { animation-delay: 0.14s; }
	.d3 { animation-delay: 0.2s; }

	@media (max-width: 960px) {
		.auth { grid-template-columns: 1fr; grid-template-rows: auto 1fr; }
		.auth-side { padding: 16px 18px 0; }
		.side-copy { margin-top: 0; padding-top: 18px; }
		.side-copy h1 { font-size: 19px; max-width: none; }
		.side-copy p, .map-wrap { display: none; }
		.auth-panel {
			border-left: none;
			border-top: 1px solid var(--border);
			padding: 26px 18px;
			place-items: start center;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.rise { animation: none; }
		.pkt { display: none; }
	}
</style>
