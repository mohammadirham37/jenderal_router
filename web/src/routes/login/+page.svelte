<script lang="ts">
	import { goto } from '$app/navigation';
	import {
		Satellite, ShieldCheck, RefreshCw, Mail, Lock, Eye, EyeOff, ArrowRight, AlertCircle, Check,
		MessagesSquare, KeyRound, AppWindow, Cpu
	} from '@lucide/svelte';
	import ThemeToggle from '#lib/components/ThemeToggle.svelte';
	import LangToggle from '#lib/components/LangToggle.svelte';
	import { api, setCSRF } from '#lib/api.js';
	import { app, t, toast } from '#lib/stores.svelte.js';

	const REMEMBER_KEY = 'jr_login_email';

	let needsSetup = $state<boolean | null>(null);
	let version = $state('');
	let email = $state('');
	let pass = $state('');
	let showPass = $state(false);
	let capsOn = $state(false);
	let remember = $state(true);
	let err = $state('');
	let busy = $state(false);
	let emailEl = $state<HTMLInputElement | null>(null);
	let passEl = $state<HTMLInputElement | null>(null);

	// ---- captcha internal (sekali pakai, kedaluwarsa 2 menit) ----
	let capId = $state('');
	let capImg = $state('');
	let capAnswer = $state('');
	let capLoading = $state(false);
	let capTimer: ReturnType<typeof setTimeout> | null = null;
	let lockUntil = $state(0); // epoch ms — tombol masuk nonaktif selama dikunci
	let tick = $state(Date.now());
	async function loadCaptcha() {
		capLoading = true;
		capAnswer = '';
		try {
			const c = await api.get('/api/captcha', { noRedirect: true });
			capId = c.id;
			capImg = c.image;
			if (capTimer) clearTimeout(capTimer);
			capTimer = setTimeout(loadCaptcha, (c.expires_in - 5) * 1000); // segarkan sebelum kedaluwarsa
		} catch (e: any) {
			err = e.message;
		} finally {
			capLoading = false;
		}
	}
	$effect(() => {
		loadCaptcha();
		const id = setInterval(() => (tick = Date.now()), 1000);
		return () => { clearInterval(id); if (capTimer) clearTimeout(capTimer); };
	});
	const lockLeft = $derived(Math.max(0, Math.ceil((lockUntil - tick) / 1000)));

	$effect(() => {
		try {
			const saved = localStorage.getItem(REMEMBER_KEY);
			if (saved) email = saved;
		} catch { /* */ }
		(async () => {
			try {
				const s = await api.get('/api/setup/status', { noRedirect: true });
				needsSetup = s.needs_setup;
				version = s.version || '';
			} catch {
				needsSetup = false;
			}
		})();
	});
	// fokus awal: email kosong → kolom email; email diingat → langsung password
	$effect(() => {
		if (needsSetup === null) return;
		(email ? passEl : emailEl)?.focus();
	});

	// syarat password admin pertama (sama dengan crypto.PasswordStrength)
	const pwRules = $derived([
		{ ok: pass.length >= 12, label: t('pw_len') },
		{ ok: /[a-z]/.test(pass) && /[A-Z]/.test(pass), label: t('pw_case') },
		{ ok: /[0-9]/.test(pass), label: t('pw_digit') }
	]);
	const pwOk = $derived(pwRules.every((r) => r.ok));

	function onPassKey(e: KeyboardEvent) {
		capsOn = e.getModifierState?.('CapsLock') ?? false;
	}

	async function submit() {
		err = '';
		busy = true;
		try {
			const body = { email, password: pass, captcha_id: capId, captcha: capAnswer };
			const res = needsSetup
				? await api.post('/api/setup', body, { noRedirect: true })
				: await api.post('/api/login', body, { noRedirect: true });
			try {
				if (remember && !needsSetup) localStorage.setItem(REMEMBER_KEY, email);
				else localStorage.removeItem(REMEMBER_KEY);
			} catch { /* */ }
			setCSRF(res.csrf);
			app.me = res.user;
			toast(needsSetup ? 'Admin dibuat — selamat datang! 🎉' : 'Selamat datang kembali', 'ok');
			goto('/dashboard', { replace: true });
		} catch (e: any) {
			err = e.message;
			if (e.status === 429) lockUntil = Date.now() + (e.data?.retry_after ?? 60) * 1000;
			loadCaptcha(); // captcha sekali pakai — selalu ganti setelah percobaan
		} finally {
			busy = false;
		}
	}

	// papan rute: klien (kiri) → JenderalRouter → provider (kanan). Koordinat
	// dalam viewBox 640×300; label HTML ditempatkan dengan persen yang sama.
	const HUB = { x: 320, y: 150 };
	const clients = [
		{ label: 'Chat', icon: MessagesSquare, y: 62 },
		{ label: 'API key', icon: KeyRound, y: 150 },
		{ label: 'Aplikasi', icon: AppWindow, y: 238 }
	];
	const providers = [
		{ label: 'openai', y: 40 },
		{ label: 'anthropic', y: 113 },
		{ label: 'gemini', y: 187 },
		{ label: 'lokal · llama.cpp', y: 260 }
	];
	const inPath = (y: number) => `M92 ${y} C 190 ${y}, 220 ${HUB.y}, ${HUB.x - 26} ${HUB.y}`;
	const outPath = (y: number) => `M${HUB.x + 26} ${HUB.y} C 420 ${HUB.y}, 450 ${y}, 532 ${y}`;
</script>

<svelte:head><title>JenderalRouter — {needsSetup ? t('setup_title') : t('login')}</title></svelte:head>

<div class="auth">
	<!-- ============ panggung kiri ============ -->
	<section class="stage">
		<div class="grain" aria-hidden="true"></div>
		<div class="aurora a1" aria-hidden="true"></div>
		<div class="aurora a2" aria-hidden="true"></div>

		<header class="stage-top rise">
			<span class="brand"><span class="logo"><Satellite size={18} /></span>JenderalRouter</span>
			<span class="row"><ThemeToggle /><LangToggle /></span>
		</header>

		<div class="copy">
			<div class="eyebrow rise d1">
				<span class="live"><i></i>{t('auth_online')}{version ? ' · ' + version : ''}</span>
				<span>{t('auth_eyebrow')}</span>
			</div>
			<h1 class="rise d2">
				{t('auth_h1a')} <em>{t('auth_h1em')}</em> {t('auth_h1b')}
			</h1>
			<p class="sub rise d3">{t('auth_sub')}</p>
		</div>

		<div class="board rise d4" aria-hidden="true">
			<svg viewBox="0 0 640 300" preserveAspectRatio="xMidYMid meet">
				<defs>
					<linearGradient id="wire" x1="0" x2="1">
						<stop offset="0" style="stop-color: var(--accent-2); stop-opacity: .15" />
						<stop offset=".5" style="stop-color: var(--accent); stop-opacity: .6" />
						<stop offset="1" style="stop-color: var(--accent-2); stop-opacity: .15" />
					</linearGradient>
				</defs>
				{#each clients as c, i (c.label)}
					<path class="wire" d={inPath(c.y)} />
					<circle class="pkt" r="3">
						<animateMotion dur="{2.6 + i * 0.4}s" begin="-{i * 0.9}s" repeatCount="indefinite" path={inPath(c.y)} />
						<animate attributeName="opacity" values="0;1;1;0" keyTimes="0;.12;.85;1" dur="{2.6 + i * 0.4}s" begin="-{i * 0.9}s" repeatCount="indefinite" />
					</circle>
				{/each}
				{#each providers as p, i (p.label)}
					<path class="wire" d={outPath(p.y)} />
					<circle class="pkt out" r="3">
						<animateMotion dur="{2.4 + i * 0.35}s" begin="-{i * 0.7 + 0.4}s" repeatCount="indefinite" path={outPath(p.y)} />
						<animate attributeName="opacity" values="0;1;1;0" keyTimes="0;.12;.85;1" dur="{2.4 + i * 0.35}s" begin="-{i * 0.7 + 0.4}s" repeatCount="indefinite" />
					</circle>
				{/each}
			</svg>
			{#each clients as c (c.label)}
				<span class="node client" style="left:{(52 / 640) * 100}%;top:{(c.y / 300) * 100}%"><c.icon size={13} />{c.label}</span>
			{/each}
			<span class="hub" style="left:{(HUB.x / 640) * 100}%;top:{(HUB.y / 300) * 100}%">
				<i class="ring"></i><i class="ring r2"></i><Satellite size={20} />
			</span>
			{#each providers as p (p.label)}
				<span class="node prov" style="left:{(574 / 640) * 100}%;top:{(p.y / 300) * 100}%"><Cpu size={12} />{p.label}</span>
			{/each}
		</div>

		<ul class="feats rise d5">
			<li><Check size={14} />{t('auth_feat1')}</li>
			<li><Check size={14} />{t('auth_feat2')}</li>
			<li><Check size={14} />{t('auth_feat3')}</li>
		</ul>
	</section>

	<!-- ============ panel formulir ============ -->
	<section class="panel">
		<div class="form-wrap rise d2">
			{#if needsSetup}
				<span class="mode-chip"><ShieldCheck size={14} /> first run</span>
			{/if}
			<h2>{needsSetup ? t('setup_title') : t('auth_welcome')}</h2>
			<p class="lead">{needsSetup ? t('setup_desc') : t('login_continue')}</p>

			{#if needsSetup === false}
				<a href="/google/auth" class="g-btn">
					<svg width="18" height="18" viewBox="0 0 48 48" aria-hidden="true">
						<path fill="#EA4335" d="M24 9.5c3.54 0 6.71 1.22 9.21 3.6l6.85-6.85C35.9 2.38 30.47 0 24 0 14.62 0 6.51 5.38 2.56 13.22l7.98 6.19C12.43 13.72 17.74 9.5 24 9.5z"/>
						<path fill="#4285F4" d="M46.98 24.55c0-1.57-.15-3.09-.38-4.55H24v9.02h12.94c-.58 2.96-2.26 5.48-4.78 7.18l7.73 6c4.51-4.18 7.09-10.36 7.09-17.65z"/>
						<path fill="#FBBC05" d="M10.53 28.59c-.48-1.45-.76-2.99-.76-4.59s.27-3.14.76-4.59l-7.98-6.19C.92 16.46 0 20.12 0 24c0 3.88.92 7.54 2.56 10.78l7.97-6.19z"/>
						<path fill="#34A853" d="M24 48c6.48 0 11.93-2.13 15.89-5.81l-7.73-6c-2.15 1.45-4.92 2.3-8.16 2.3-6.26 0-11.57-4.22-13.47-9.91l-7.98 6.19C6.51 42.62 14.62 48 24 48z"/>
					</svg>
					{t('auth_google')}
				</a>
				<p class="hint">{t('auth_google_hint')}</p>
				<div class="divider"><span>{t('auth_or_email')}</span></div>
			{/if}

			<form onsubmit={(e) => { e.preventDefault(); submit(); }}>
				<label class="lbl" for="f-email">{t('email')}</label>
				<div class="field">
					<Mail size={16} />
					<input id="f-email" bind:this={emailEl} type="email" bind:value={email}
						placeholder="nama@unsoed.ac.id" autocomplete="username" required />
				</div>

				<label class="lbl" for="f-pass">{t('password')}</label>
				<div class="field">
					<Lock size={16} />
					<input id="f-pass" bind:this={passEl} type={showPass ? 'text' : 'password'} bind:value={pass}
						placeholder={needsSetup ? '••••••••••••' : t('password')}
						autocomplete={needsSetup ? 'new-password' : 'current-password'} required minlength={8}
						onkeydown={onPassKey} onkeyup={onPassKey} onblur={() => (capsOn = false)} />
					<button type="button" class="eye" onclick={() => (showPass = !showPass)}
						aria-label={showPass ? t('auth_hide_pw') : t('auth_show_pw')} title={showPass ? t('auth_hide_pw') : t('auth_show_pw')}>
						{#if showPass}<EyeOff size={16} />{:else}<Eye size={16} />{/if}
					</button>
				</div>
				{#if capsOn}<div class="caps"><AlertCircle size={13} /> {t('auth_caps')}</div>{/if}
				{#if needsSetup}
					<ul class="pw-rules">
						{#each pwRules as r (r.label)}
							<li class:ok={r.ok}><Check size={12} />{r.label}</li>
						{/each}
					</ul>
				{/if}

				<label class="lbl" for="f-captcha">{t('auth_captcha')}</label>
				<div class="captcha">
					<span class="cap-img">
						{#if capImg}<img src={capImg} alt="Kode captcha — ketik karakter pada gambar" width="150" height="52" />{/if}
					</span>
					<button type="button" class="cap-new" onclick={loadCaptcha} title={t('auth_captcha_new')} aria-label={t('auth_captcha_new')}>
						<span class:spin={capLoading}><RefreshCw size={15} /></span>
					</button>
					<input id="f-captcha" bind:value={capAnswer} placeholder={t('auth_captcha_ph')} autocomplete="off"
						autocapitalize="characters" spellcheck="false" maxlength={8} required
						oninput={() => (capAnswer = capAnswer.toUpperCase())} />
				</div>

				{#if !needsSetup}
					<div class="opts">
						<label class="check"><input type="checkbox" bind:checked={remember} /> {t('auth_remember')}</label>
					</div>
				{/if}

				{#if err}
					<div class="alert" role="alert"><AlertCircle size={16} /><span>{err}{#if lockLeft > 0} — coba lagi {lockLeft} dtk{/if}</span></div>
				{/if}

				<button class="submit" type="submit"
					disabled={busy || needsSetup === null || lockLeft > 0 || !capId || (needsSetup && !pwOk)}>
					{#if busy}<span class="spinner"></span>{:else if needsSetup}<ShieldCheck size={17} />{/if}
					<span>{needsSetup ? t('setup_btn') : t('login_btn')}</span>
					{#if !busy && !needsSetup}<ArrowRight size={17} class="arrow" />{/if}
				</button>
			</form>

			{#if !needsSetup}<p class="foot">{t('auth_forgot')}</p>{/if}
		</div>
	</section>
</div>

<style>
	.auth {
		min-height: 100vh;
		min-height: 100dvh;
		display: grid;
		grid-template-columns: minmax(0, 1.3fr) minmax(400px, 500px);
	}

	/* ---------- panggung ---------- */
	.stage {
		position: relative;
		overflow: hidden;
		display: flex;
		flex-direction: column;
		padding: 28px clamp(28px, 4vw, 64px) 34px;
		isolation: isolate;
	}
	/* grid titik, memudar ke tepi */
	.stage::before {
		content: '';
		position: absolute;
		inset: 0;
		z-index: -2;
		background-image: radial-gradient(color-mix(in srgb, var(--text) 16%, transparent) 1px, transparent 1.3px);
		background-size: 22px 22px;
		mask-image: radial-gradient(ellipse 75% 70% at 45% 55%, #000 25%, transparent 78%);
		-webkit-mask-image: radial-gradient(ellipse 75% 70% at 45% 55%, #000 25%, transparent 78%);
	}
	.grain {
		position: absolute;
		inset: -50%;
		z-index: -1;
		opacity: 0.05;
		pointer-events: none;
		background-image: url("data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='160' height='160'><filter id='n'><feTurbulence type='fractalNoise' baseFrequency='.9' numOctaves='3' stitchTiles='stitch'/></filter><rect width='100%' height='100%' filter='url(%23n)'/></svg>");
	}
	.aurora {
		position: absolute;
		z-index: -3;
		border-radius: 50%;
		filter: blur(70px);
		opacity: 0.55;
		pointer-events: none;
	}
	.a1 {
		width: 520px; height: 420px; left: -120px; top: 18%;
		background: color-mix(in srgb, var(--accent) 38%, transparent);
		animation: drift 18s ease-in-out infinite alternate;
	}
	.a2 {
		width: 460px; height: 380px; right: -140px; bottom: -80px;
		background: color-mix(in srgb, var(--accent-2) 30%, transparent);
		animation: drift 22s ease-in-out infinite alternate-reverse;
	}
	@keyframes drift {
		from { transform: translate3d(0, 0, 0) scale(1); }
		to { transform: translate3d(60px, -40px, 0) scale(1.12); }
	}
	:global([data-theme='light']) .aurora { opacity: 0.35; }

	.stage-top { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
	.brand { display: inline-flex; align-items: center; gap: 10px; font-weight: 800; font-size: 15.5px; letter-spacing: -0.015em; }
	.logo, .hub {
		background: var(--grad-accent);
		color: #fff;
		display: grid;
		place-items: center;
		box-shadow: 0 6px 22px color-mix(in srgb, var(--accent) 45%, transparent);
	}
	.logo { width: 34px; height: 34px; border-radius: 11px; }

	.copy { margin-top: clamp(28px, 7vh, 72px); }
	.eyebrow {
		display: flex; flex-wrap: wrap; align-items: center; gap: 10px 14px;
		font-family: var(--mono); font-size: 11.5px; letter-spacing: 0.08em; text-transform: uppercase; color: var(--muted);
	}
	.live {
		display: inline-flex; align-items: center; gap: 7px; padding: 3px 10px 3px 8px; border-radius: 999px;
		border: 1px solid color-mix(in srgb, var(--ok) 40%, var(--border)); color: var(--ok);
		background: color-mix(in srgb, var(--ok) 9%, transparent); text-transform: none; letter-spacing: 0.02em;
	}
	.live i { width: 7px; height: 7px; border-radius: 50%; background: var(--ok); box-shadow: 0 0 0 0 var(--ok); animation: beacon 2.2s infinite; }
	@keyframes beacon {
		0% { box-shadow: 0 0 0 0 color-mix(in srgb, var(--ok) 60%, transparent); }
		70%, 100% { box-shadow: 0 0 0 8px transparent; }
	}
	h1 {
		margin: 18px 0 0;
		font-size: clamp(38px, 5.2vw, 72px);
		line-height: 1.02;
		letter-spacing: -0.04em;
		font-weight: 780;
		max-width: 12.5ch;
		text-wrap: balance;
	}
	h1 em {
		font-family: var(--font-display);
		font-style: italic;
		font-weight: 400;
		letter-spacing: -0.01em;
		font-size: 1.12em;
		padding-right: 0.06em;
		background: linear-gradient(100deg, var(--accent) 10%, var(--accent-2) 90%);
		-webkit-background-clip: text;
		background-clip: text;
		color: transparent;
	}
	.sub { margin: 18px 0 0; max-width: 46ch; color: var(--muted); font-size: 15px; line-height: 1.65; }

	.board { position: relative; width: min(100%, 680px); aspect-ratio: 640 / 300; margin: clamp(18px, 4vh, 40px) 0 0; }
	.board svg { position: absolute; inset: 0; width: 100%; height: 100%; overflow: visible; }
	.wire { fill: none; stroke: url(#wire); stroke-width: 1.4; }
	.pkt { fill: var(--accent-2); filter: drop-shadow(0 0 5px var(--accent-2)); }
	.pkt.out { fill: var(--accent); filter: drop-shadow(0 0 5px var(--accent)); }
	.node {
		position: absolute; transform: translate(-50%, -50%);
		display: inline-flex; align-items: center; gap: 6px; white-space: nowrap;
		font-family: var(--mono); font-size: 11.5px; color: var(--text);
		padding: 5px 11px; border-radius: 999px;
		background: color-mix(in srgb, var(--surface) 82%, transparent);
		border: 1px solid var(--border-strong);
		backdrop-filter: blur(6px);
		box-shadow: var(--shadow-sm);
	}
	.node :global(svg) { color: var(--accent-2); }
	.node.prov { color: var(--muted); }
	.hub { position: absolute; transform: translate(-50%, -50%); width: 52px; height: 52px; border-radius: 16px; }
	.ring {
		position: absolute; inset: -10px; border-radius: 22px;
		border: 1px solid color-mix(in srgb, var(--accent) 55%, transparent);
		animation: pulse 2.8s ease-out infinite;
	}
	.ring.r2 { animation-delay: 1.4s; }
	@keyframes pulse {
		from { transform: scale(0.85); opacity: 0.9; }
		to { transform: scale(1.5); opacity: 0; }
	}

	.feats {
		list-style: none; padding: 0; margin: auto 0 0; padding-top: 26px;
		display: flex; flex-wrap: wrap; gap: 10px 26px;
		font-size: 13px; color: var(--muted);
	}
	.feats li { display: inline-flex; align-items: center; gap: 8px; }
	.feats :global(svg) { color: var(--ok); }

	/* ---------- panel formulir ---------- */
	.panel {
		display: grid;
		place-items: center;
		padding: 40px clamp(24px, 3.4vw, 52px);
		background: color-mix(in srgb, var(--surface) 72%, transparent);
		border-left: 1px solid var(--border);
		backdrop-filter: blur(12px);
	}
	.form-wrap { width: 100%; max-width: 380px; }
	.mode-chip {
		display: inline-flex; align-items: center; gap: 6px; margin-bottom: 14px;
		font-family: var(--mono); font-size: 11px; font-weight: 700; color: var(--accent-2);
		background: var(--grad-accent-soft); border: 1px solid var(--border); border-radius: 999px; padding: 4px 11px;
	}
	h2 {
		font-family: var(--font-display);
		font-weight: 400;
		font-size: 42px;
		line-height: 1;
		letter-spacing: -0.01em;
		margin: 0;
	}
	.lead { color: var(--muted); margin: 10px 0 24px; font-size: 14px; line-height: 1.55; }

	.g-btn {
		display: flex; align-items: center; justify-content: center; gap: 11px;
		height: 48px; border-radius: 12px; text-decoration: none;
		background: #fff; color: #1f2937; font-weight: 700; font-size: 14px;
		border: 1px solid #dadce0;
		box-shadow: 0 1px 2px rgb(0 0 0 / .08), 0 6px 18px rgb(0 0 0 / .12);
		transition: transform var(--speed), box-shadow var(--speed);
	}
	.g-btn:hover { transform: translateY(-1px); box-shadow: 0 2px 4px rgb(0 0 0 / .1), 0 10px 26px rgb(0 0 0 / .16); }
	.g-btn:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
	.hint { font-size: 11.5px; color: var(--muted); text-align: center; margin: 8px 0 0; }
	.divider { display: flex; align-items: center; gap: 12px; margin: 22px 0 8px; color: var(--muted); font-size: 11.5px; }
	.divider::before, .divider::after { content: ''; flex: 1; height: 1px; background: var(--border); }

	.lbl { display: block; font-size: 12.5px; font-weight: 650; margin: 14px 0 6px; color: var(--text); }
	.field {
		position: relative; display: flex; align-items: center;
		border: 1px solid var(--border-strong); border-radius: 12px; background: var(--surface);
		transition: border-color var(--speed), box-shadow var(--speed);
	}
	.field:focus-within {
		border-color: var(--accent);
		box-shadow: 0 0 0 4px color-mix(in srgb, var(--accent) 16%, transparent);
	}
	.field > :global(svg) { position: absolute; left: 13px; color: var(--muted); pointer-events: none; }
	.field input {
		all: unset; box-sizing: border-box; width: 100%; height: 46px; padding: 0 44px 0 40px;
		font-size: 14.5px; color: var(--text);
	}
	.field input::placeholder { color: color-mix(in srgb, var(--muted) 75%, transparent); }
	.eye {
		all: unset; position: absolute; right: 6px; width: 34px; height: 34px; border-radius: 9px;
		display: grid; place-items: center; cursor: pointer; color: var(--muted);
	}
	.eye:hover { color: var(--text); background: var(--surface-2); }
	.eye:focus-visible { outline: 2px solid var(--accent); }
	.caps { display: flex; align-items: center; gap: 6px; margin-top: 6px; font-size: 12px; color: var(--warn); }

	.pw-rules { list-style: none; padding: 0; margin: 8px 0 0; display: flex; flex-wrap: wrap; gap: 6px 14px; }
	.pw-rules li { display: inline-flex; align-items: center; gap: 5px; font-size: 12px; color: var(--muted); }
	.pw-rules li :global(svg) { opacity: 0.35; }
	.pw-rules li.ok { color: var(--ok); }
	.pw-rules li.ok :global(svg) { opacity: 1; }

	.captcha { display: flex; gap: 8px; align-items: stretch; }
	.cap-img {
		flex: 0 0 auto; width: 132px; height: 46px; border-radius: 12px; overflow: hidden; line-height: 0;
		border: 1px solid var(--border-strong); background: #e8ecf8;
	}
	.cap-img img { width: 100%; height: 100%; object-fit: cover; }
	.cap-new {
		all: unset; flex: 0 0 auto; width: 40px; border-radius: 12px; cursor: pointer;
		display: grid; place-items: center; color: var(--muted);
		border: 1px solid var(--border-strong); background: var(--surface);
	}
	.cap-new:hover { color: var(--text); border-color: var(--accent); }
	.cap-new:focus-visible { outline: 2px solid var(--accent); }
	.spin { display: grid; animation: spin 0.8s linear infinite; }
	@keyframes spin { to { transform: rotate(360deg); } }
	.captcha input {
		flex: 1; min-width: 0; height: 46px; box-sizing: border-box; margin: 0;
		border-radius: 12px; text-align: center; letter-spacing: 0.3em; font-family: var(--mono); font-weight: 700; font-size: 15px;
	}
	.captcha input::placeholder { letter-spacing: normal; font-family: var(--font); font-weight: 400; font-size: 13px; }

	.opts { display: flex; align-items: center; justify-content: space-between; margin-top: 14px; }
	.check { display: inline-flex; align-items: center; gap: 8px; font-size: 13px; color: var(--muted); cursor: pointer; margin: 0; }
	.check input { width: 16px; height: 16px; margin: 0; accent-color: var(--accent); }

	.alert {
		display: flex; gap: 9px; align-items: flex-start; margin-top: 14px; padding: 10px 12px; border-radius: 11px;
		font-size: 13px; color: var(--err);
		background: color-mix(in srgb, var(--err) 10%, transparent);
		border: 1px solid color-mix(in srgb, var(--err) 35%, transparent);
	}
	.alert :global(svg) { flex: 0 0 auto; margin-top: 1px; }

	.submit {
		all: unset; box-sizing: border-box; width: 100%; height: 48px; margin-top: 18px; border-radius: 12px; cursor: pointer;
		display: flex; align-items: center; justify-content: center; gap: 9px;
		color: #fff; font-weight: 750; font-size: 14.5px; letter-spacing: 0.01em;
		background: var(--grad-accent);
		box-shadow: 0 8px 24px color-mix(in srgb, var(--accent) 38%, transparent);
		transition: transform var(--speed), box-shadow var(--speed), filter var(--speed);
	}
	.submit:hover:not(:disabled) { filter: brightness(1.07); box-shadow: 0 12px 30px color-mix(in srgb, var(--accent) 48%, transparent); }
	.submit:hover:not(:disabled) :global(.arrow) { transform: translateX(3px); }
	.submit :global(.arrow) { transition: transform var(--speed); }
	.submit:active:not(:disabled) { transform: translateY(1px); }
	.submit:disabled { opacity: 0.5; cursor: not-allowed; box-shadow: none; }
	.submit:focus-visible { outline: 2px solid var(--accent-2); outline-offset: 3px; }
	.foot { margin: 18px 0 0; text-align: center; font-size: 12px; color: var(--muted); }

	/* ---------- animasi masuk ---------- */
	@keyframes rise {
		from { opacity: 0; transform: translateY(14px); }
		to { opacity: 1; transform: none; }
	}
	.rise { animation: rise 0.7s cubic-bezier(0.2, 0.75, 0.25, 1) both; }
	.d1 { animation-delay: 0.06s; }
	.d2 { animation-delay: 0.14s; }
	.d3 { animation-delay: 0.22s; }
	.d4 { animation-delay: 0.32s; }
	.d5 { animation-delay: 0.42s; }

	/* ---------- layar kecil ---------- */
	@media (max-width: 1100px) {
		.auth { grid-template-columns: minmax(0, 1fr) minmax(380px, 440px); }
		.feats { display: none; }
	}
	@media (max-width: 900px) {
		.auth { grid-template-columns: 1fr; grid-template-rows: auto 1fr; }
		.stage { padding: 16px 18px 22px; }
		.copy { margin-top: 22px; }
		h1 { font-size: clamp(30px, 8.5vw, 42px); max-width: 14ch; }
		.sub { font-size: 13.5px; margin-top: 10px; }
		.board, .feats { display: none; }
		.panel { border-left: none; border-top: 1px solid var(--border); padding: 26px 18px 34px; place-items: start center; }
		h2 { font-size: 34px; }
		.lead { margin-bottom: 18px; }
	}
	@media (max-width: 380px) {
		.cap-img { width: 110px; }
	}
	@media (prefers-reduced-motion: reduce) {
		.rise, .aurora, .ring, .live i { animation: none; }
		.pkt { display: none; }
	}
</style>
