<script lang="ts">
	import { Settings, Save, Database, Rocket, RefreshCw, ScrollText, ShieldCheck } from '@lucide/svelte';
	import { api } from '$lib/api';
	import { t, toast } from '$lib/stores.svelte';

	// ---- notifikasi ----
	let tgToken = $state(''); let tgChat = $state('');
	async function saveSettings() {
		try {
			await api.put('/api/admin/settings', { notification_telegram_token: tgToken, notification_chat_id: tgChat });
			toast(t('save') + ' ✓', 'ok');
		} catch (e: any) { toast(e.message, 'err'); }
	}

	// ---- backup ----
	async function backup() {
		try { await api.post('/api/admin/system/backup'); toast(t('save') + ' ✓', 'ok'); }
		catch (e: any) { toast(e.message, 'err'); }
	}

	// ---- pembaruan ----
	let upd = $state<any>(null);
	let updLoading = $state(false);
	let updating = $state(false);
	let updLog = $state('');

	async function loadStatus(fetchNew: boolean) {
		updLoading = true;
		try { upd = await api.get('/api/admin/system/update/status' + (fetchNew ? '?fetch=1' : '')); }
		catch (e: any) { toast(e.message, 'err'); }
		updLoading = false;
	}
	$effect(() => { loadStatus(false); (async () => {
		try { const s = await api.get('/api/admin/settings'); tgToken = s.settings.notification_telegram_token || ''; tgChat = s.settings.notification_chat_id || ''; } catch { /* */ }
	})(); });

	async function update() {
		if (!confirm('Perbarui aplikasi sekarang?\nBuild bisa 2–5 menit. Bila helper sudo aktif, service akan di-restart otomatis (koneksi sempat terputus).')) return;
		updating = true;
		updLog = '⏳ fetch + build… (jangan tutup halaman ini)';
		try {
			const res = await api.post('/api/admin/system/update', {});
			updLog = res.log || '';
			if (res.error) toast(res.error, 'err');
			else if (res.staged) toast('Binary distage — lihat perintah pemasangan di log', 'ok');
		} catch {
			updLog += '\n(koneksi terputus — kemungkinan service sedang restart, memeriksa status…)';
		}
		// poll hingga selesai
		for (let i = 0; i < 12; i++) {
			await new Promise((r) => setTimeout(r, 3000));
			try {
				const st = await api.get('/api/admin/system/update/status');
				if (!st.busy) { updLog += `\nversi sekarang: ${st.version}`; upd = st; break; }
			} catch { /* tunggu */ }
		}
		updating = false;
		toast('Proses pembaruan selesai', 'ok');
	}

	// ---- audit ----
	let audit = $state<any[]>([]);
	$effect(() => {
		api.get('/api/admin/audit?limit=30').then((r) => (audit = r.audit || [])).catch(() => {});
	});
</script>

<svelte:head><title>{t('settings')} — JenderalRouter</title></svelte:head>

<div class="page-head">
	<h1 class="page-title"><Settings size={20} /> {t('settings')}</h1>
</div>

<div class="card">
	<h2>Notifikasi (v1.1)</h2>
	<label>Telegram bot token</label>
	<input type="password" bind:value={tgToken} placeholder="123456:ABC…" />
	<label>Telegram chat id</label>
	<input bind:value={tgChat} />
	<div class="modal-actions" style="justify-content:flex-start;margin-top:12px">
		<button class="btn" onclick={saveSettings}><Save size={15} /> {t('save')}</button>
	</div>
</div>

<div class="card">
	<h2><Database size={15} /> Backup</h2>
	<p class="muted small">Backup harian otomatis ke <code>data/backups/</code> (retensi 7 salinan).</p>
	<button class="btn" onclick={backup}><Database size={15} /> {t('backup_now')}</button>
</div>

<div class="card">
	<h2><Rocket size={15} /> Pembaruan Aplikasi</h2>
	{#if updLoading && !upd}
		<span class="spinner dark"></span> {t('loading')}…
	{:else if upd}
		<div style="margin-bottom:10px">
			<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:210px">Versi terpasang:</span><span class="mono">{upd.version}</span></div>
			<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:210px">Mode:</span><span class="mono">{upd.mode}</span></div>
			{#if upd.hint}
				<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:210px">Info:</span><span>{upd.hint}</span></div>
			{/if}
			{#if !upd.in_container}
				<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:210px">Repo (mirror):</span><span class="mono">{upd.repo_found ? upd.repo_dir + ' @ ' + (upd.current_head || '') + ' [' + (upd.branch || '') + ']' : 'belum di-clone (otomatis saat update)'}</span></div>
				<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:210px">Ketinggalan commit:</span><span class="mono">{upd.behind_commits ?? '—'}</span></div>
				<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:210px">git / Go:</span><span class="mono">{upd.git_available ? '✓' : '✗'} / {upd.go_available ? '✓' : '✗'}</span></div>
				<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:210px">Restart otomatis:</span><span>{upd.sudo_apply_available ? '✓ helper aktif' : '✗ (stage + perintah manual)'}</span></div>
				{#if upd.staged_binary}<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:210px">Binary ter-stage:</span><span class="badge warn">siap dipasang</span></div>{/if}
				{#if upd.last_update}<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:210px">Update terakhir:</span><span class="mono">{upd.last_update.time} ({upd.last_update.commits} commit)</span></div>{/if}
			{/if}
		</div>
		<div class="row">
			<button class="btn ghost" onclick={() => loadStatus(true)} disabled={updLoading || upd.busy}>
				{#if updLoading}<span class="spinner dark"></span>{:else}<RefreshCw size={15} />{/if}
				{t('check_update')}
			</button>
			{#if !upd.in_container}
				<button class="btn" onclick={update} disabled={updating || upd.busy}>
					{#if updating}<span class="spinner"></span>{:else}<Rocket size={15} />{/if}
					{t('update_now')}
				</button>
			{/if}
		</div>
		{#if updLog}<pre class="logbox" style="margin-top:10px">{updLog}</pre>{/if}
	{/if}
</div>

<div class="card">
	<h2><ScrollText size={15} /> {t('audit_log')}</h2>
	{#each audit as a (a.id)}
		<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0">
			<span class="muted mono" style="min-width:150px">{(a.ts || '').replace('T', ' ').slice(0, 19)}</span>
			<span class="mono">{a.action}</span>
			<span class="muted">{a.target}</span>
		</div>
	{:else}
		<span class="muted small">{t('no_data')}</span>
	{/each}
</div>
