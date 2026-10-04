<script lang="ts">
	import { goto } from '$app/navigation';
	import { Satellite, LogIn, ShieldCheck } from '@lucide/svelte';
	import ThemeToggle from '$lib/components/ThemeToggle.svelte';
	import LangToggle from '$lib/components/LangToggle.svelte';
	import { api, setCSRF } from '$lib/api';
	import { t, toast } from '$lib/stores.svelte';

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
</script>

<svelte:head><title>JenderalRouter — {needsSetup ? t('setup_title') : t('login')}</title></svelte:head>

<div class="auth-screen">
	<div class="card auth-card">
		<div class="auth-logo"><span class="logo"><Satellite size={30} /></span></div>
		<h1 style="text-align:center;font-size:19px;margin:0 0 4px">JenderalRouter</h1>
		<p class="muted small" style="text-align:center;margin:0 0 8px">
			{needsSetup ? t('setup_desc') : 'Gateway LLM self-hosted multi-user'}
		</p>

		<label for="f-email">{t('email')}</label>
		<input id="f-email" type="email" bind:value={email} placeholder="admin@domainanda.com"
			autocomplete="username" onkeydown={(e) => e.key === 'Enter' && submit()} />

		<label for="f-pass">{t('password')}</label>
		<input id="f-pass" type="password" bind:value={pass}
			placeholder={needsSetup ? '≥12 karakter, huruf besar+kecil+angka' : t('password')}
			autocomplete={needsSetup ? 'new-password' : 'current-password'}
			onkeydown={(e) => e.key === 'Enter' && submit()} />

		<div class="form-error">{err}</div>
		<button class="btn" style="width:100%" onclick={submit} disabled={busy || needsSetup === null}>
			{#if busy}<span class="spinner"></span>{:else if needsSetup}<ShieldCheck size={16} />{:else}<LogIn size={16} />{/if}
			{needsSetup ? t('setup_btn') : t('login_btn')}
		</button>

		<div class="row" style="justify-content:center;margin-top:14px">
			<ThemeToggle />
			<LangToggle />
		</div>
	</div>
</div>
