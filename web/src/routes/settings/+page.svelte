<script lang="ts">
	import { Settings, Save, Database, Rocket, RefreshCw, ScrollText, Copy, Globe } from '@lucide/svelte';
	import { api } from '$lib/api';
	import { copyText } from '$lib/clipboard';
	import { fmtTs } from '$lib/api';
	import { t, toast } from '$lib/stores.svelte';

	// ---- notifikasi ----
	let tgToken = $state(''); let tgChat = $state('');
	async function saveSettings() {
		try {
			await api.put('/api/admin/settings', { notification_telegram_token: tgToken, notification_chat_id: tgChat });
			toast(t('save') + ' ✓', 'ok');
		} catch (e: any) { toast(e.message, 'err'); }
	}

	// ---- Cloudflare Tunnel ----
	let cf = $state<any>(null);
	let cfLoading = $state(false);
	let cfTab = $state<'daftar' | 'server'>('daftar');
	async function loadCf() {
		cfLoading = true;
		try { cf = await api.get('/api/admin/system/cloudflare'); } catch (e: any) { toast(e.message, 'err'); }
		cfLoading = false;
	}
	$effect(() => { loadCf(); });

	function cfGuide(): string {
		if (cfTab === 'daftar') {
			return `YANG HARUS DIDAFTARKAN DI DASHBOARD CLOUDFLARE
=================================================

1) DOMAIN AKTIF DI CLOUDFLARE
   - Masuk dash.cloudflare.com → "Add a domain" → masukkan domain Anda
   - Pilih paket FREE (cukup)
   - Arahkan nameserver domain Anda ke 2 nameserver yang Cloudflare berikan
     (di registrar tempat Anda membeli domain)
   - Tunggu status domain: ACTIVE

2) BUAT TUNNEL (tanpa CLI)
   - Dashboard Cloudflare → menu "Zero Trust" (one-time setup singkat)
   - Networks → Tunnels → "Create a tunnel"
   - Pilih konektor "Cloudflared" → beri nama, mis. jenderal
   - Di langkah "Install and run a connector": pilih Debian / 64-bit
   - SALIN perintah yang ditampilkan — berisi token unik tunnel Anda:
       sudo cloudflared service install <TOKEN-PANJANG>
     (perintah ini sekaligus memasang service otomatis di server)

3) DAFTARKAN HOSTNAME PUBLIK (masih di wizard yang sama)
   - Subdomain : chat        Domain : domainanda.com
   - Path      : (kosongkan)
   - Service   : HTTP  →  URL: 127.0.0.1:20130
   - Klik "Save tunnel"
   → CNAME ke <ID>.cfargotunnel.com + sertifikat SSL dibuat OTOMATIS

4) CEK STATUS
   - Networks → Tunnels → tunnel "jenderal" harus berstatus HEALTHY
   - Buka https://chat.domainanda.com — selesai, tanpa buka port`;
		}
		const addr = cf?.gateway_addr || '127.0.0.1:20130';
		return `PEMASANGAN DI SERVER (metode CLI / lokal-managed)
=================================================

# 1) pasang cloudflared
sudo curl -L --output /usr/local/bin/cloudflared \\
  https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64
sudo chmod +x /usr/local/bin/cloudflared

# 2) autentikasi & buat tunnel (butuh akun Cloudflare + domain)
cloudflared tunnel login
cloudflared tunnel create jenderal

# 3) /etc/cloudflared/config.yml
tunnel: <ID-tunnel>
credentials-file: /root/.cloudflared/<ID-tunnel>.json
ingress:
  - hostname: chat.domainanda.com
    service: http://${addr}
  - service: http_status:404

# 4) daftarkan DNS & jalankan sebagai service
cloudflared tunnel route dns jenderal chat.domainanda.com
sudo cloudflared service install`;
	}
	async function copyCfGuide() {
		(await copyText(cfGuide())) ? toast('panduan disalin ✓', 'ok') : toast('gagal menyalin — salin manual', 'err');
	}

	// ---- backup ----
	function stagePath() {
		const base = upd?.repo_dir ? upd.repo_dir.replace('/src', '') : '/var/lib/jenderalrouter';
		return base + '/updates/jenderalrouter.new';
	}
	function copyStageCommands() {
		const p = stagePath();
		const cmd = [
			'sudo systemctl stop jenderalrouter',
			'sudo install -m 0755 ' + p + ' /usr/local/bin/jenderalrouter',
			'sudo rm -f ' + p,
			'sudo systemctl start jenderalrouter'
		].join('\n');
		copyText(cmd).then((ok) => toast(ok ? 'perintah disalin ✓' : 'gagal menyalin — salin manual', ok ? 'ok' : 'err'));
	}
	function copyHelperCommand() {
		const cmd = 'sudo /usr/local/bin/jenderalrouter install-update-helper';
		copyText(cmd).then((ok) => toast(ok ? 'perintah disalin ✓' : 'gagal menyalin — salin manual', ok ? 'ok' : 'err'));
	}

	// pasang binary hasil stage + restart service lewat helper (tanpa SSH)
	let applying = $state(false);
	async function applyStaged() {
		applying = true;
		try {
			await api.post('/api/admin/system/update/apply', {});
			toast('Memasang binary & me-restart service…', 'info');
		} catch (e: any) {
			toast(e.message, 'err');
			applying = false;
			return;
		}
		// service berhenti sebentar saat helper me-restart — tunggu sampai aktif lagi
		for (let i = 0; i < 60; i++) {
			await new Promise((r) => setTimeout(r, 2000));
			try {
				upd = await api.get('/api/admin/system/update/status', { noRedirect: true });
				if (!upd.staged_binary) break;
			} catch { /* restart sedang berlangsung */ }
		}
		if (upd && !upd.staged_binary) toast('Pembaruan terpasang & service aktif ✓', 'ok');
		else toast('Service belum konfirmasi — cek `systemctl status jenderalrouter` di host', 'err');
		applying = false;
	}

	async function backup() {
		try { await api.post('/api/admin/system/backup'); toast(t('save') + ' ✓', 'ok'); }
		catch (e: any) { toast(e.message, 'err'); }
	}

	// ---- pembaruan ----
	let upd = $state<any>(null);
	let updLoading = $state(false);
	let updating = $state(false);
	let updLog = $state('');
	let job = $state<any>(null);

	async function pollJob() {
		for (;;) {
			try { job = await api.get('/api/admin/system/update/status'); } catch { /* */ }
			const j = job?.job;
			if (!j?.running) break;
			await new Promise((r) => setTimeout(r, 3000));
		}
		if (job?.job?.done) {
			const j = job.job;
			if (j.ok) toast(j.restarted ? 'Pembaruan terpasang & service di-restart ✓' : 'Build selesai — binary distage', 'ok');
			else toast('Pembaruan gagal: ' + (j.error || 'lihat log'), 'err');
		}
		updLoading = true;
		try { upd = await api.get('/api/admin/system/update/status?fetch=auto'); } catch { /* */ }
		updLoading = false;
	}

	async function loadStatus(fetchNew: boolean) {
		updLoading = true;
		try { upd = await api.get('/api/admin/system/update/status?fetch=' + (fetchNew ? '1' : 'auto')); }
		catch (e: any) { toast(e.message, 'err'); }
		updLoading = false;
	}
	$effect(() => {
		loadStatus(true);
		// lanjutkan polling bila job sedang berjalan (mis. halaman dimuat ulang)
		api.get('/api/admin/system/update/status').then((st) => {
			if (st?.job?.running) { job = st; pollJob(); }
		}).catch(() => {});
		(async () => {
			try { const s = await api.get('/api/admin/settings'); tgToken = s.settings.notification_telegram_token || ''; tgChat = s.settings.notification_chat_id || ''; } catch { /* */ }
		})();
	});

	async function update() {
		if (!confirm('Perbarui aplikasi sekarang?\n\nBuild bisa 2–5 menit — log progres tampil live. Bila helper sudo aktif, service akan di-restart otomatis; jika tidak, binary distage dan perintah pemasangan ditampilkan.')) return;
		updating = true;
		updLog = '';
		try {
			await api.post('/api/admin/system/update', {});
			job = { job: { running: true, phase: 'menyiapkan', log: [] } };
			updLog = '⏳ dimulai…';
			await pollJob();
		} catch (e: any) {
			toast(e.message, 'err');
		}
		updating = false;
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
		{#if upd.in_container}
			<p class="muted small" style="margin-bottom:10px">{upd.hint}</p>
		{:else if upd.update_available}
			<div class="kv" style="margin-bottom:10px;padding:10px;border-radius:10px;background:color-mix(in srgb, var(--warn) 12%, transparent);border:1px solid color-mix(in srgb, var(--warn) 35%, transparent)">
				<span class="badge warn"><span class="dot pulse"></span>UPDATE TERSEDIA</span>
				<span class="small">
					{upd.behind_commits} commit di belakang
					{#if upd.remote_subject}&nbsp;· terbaru: <span class="mono">{upd.remote_commit}</span> {upd.remote_subject}{/if}
				</span>
			</div>
		{:else if upd.repo_found}
			<div class="kv" style="margin-bottom:10px">
				<span class="badge ok"><span class="dot"></span>sudah versi terbaru</span>
				<span class="muted small">commit lokal = remote ({upd.current_head})</span>
			</div>
		{/if}
		<div style="margin-bottom:10px">
			<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:210px">Versi terpasang:</span><span class="mono">{upd.version}</span></div>
			<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:210px">Mode:</span><span class="mono">{upd.mode}</span></div>
			{#if upd.hint}
				<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:210px">Info:</span><span>{upd.hint}</span></div>
			{/if}
			{#if !upd.in_container}
				<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:210px">Repo (mirror):</span><span class="mono">{upd.repo_found ? upd.repo_dir + ' [' + (upd.branch || '') + ']' : 'belum di-clone (otomatis)'}</span></div>
			{#if upd.repo_found}
				<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0">
					<span class="muted" style="min-width:210px">Commit lokal / remote:</span>
					<span class="mono">
						{upd.current_head || '—'}
						{#if upd.remote_head}
							/ {upd.remote_head}
							{#if upd.current_head && upd.remote_head !== upd.current_head}<span class="badge warn">beda</span>{:else}<span class="badge ok">sama</span>{/if}
						{/if}
					</span>
				</div>
				{#if upd.remote_subject}
					<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:210px">Commit remote terbaru:</span><span class="mono">{upd.remote_commit}</span> {upd.remote_subject}</div>
				{/if}
			{/if}
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
				{#if upd.busy && upd.job}
					<span class="badge info"><span class="dot pulse"></span>{upd.job.phase || 'berjalan'}</span>
				{/if}
			{/if}
		</div>
		{#if upd.staged_binary}
			<div style="margin-top:12px;padding:12px;border-radius:10px;background:color-mix(in srgb, var(--warn) 12%, transparent);border:1px solid color-mix(in srgb, var(--warn) 40%, transparent)">
				<div class="small" style="font-weight:700;margin-bottom:6px">⚠ Binary baru sudah dibangun tetapi BELUM AKTIF</div>
				{#if upd.sudo_apply_available}
					<p class="muted small" style="margin:0 0 10px">
						Helper pembaruan aktif — pasang binary hasil stage dan restart service langsung dari sini, tanpa SSH.
						(HTTP bisa terputus sesaat saat service di-restart.)
					</p>
					<button class="btn" onclick={applyStaged} disabled={applying}>
						{#if applying}<span class="spinner"></span>{:else}<Rocket size={15} />{/if}
						Terapkan & restart sekarang
					</button>
				{:else}
					<p class="muted small" style="margin:0 0 8px">
						Helper pembaruan belum terpasang. Jalankan <b>sekali saja</b> di host (SSH) — setelah itu semua
						update berikutnya terpasang otomatis dari dashboard, tanpa SSH lagi:
					</p>
					<pre class="logbox" style="margin-bottom:8px">sudo /usr/local/bin/jenderalrouter install-update-helper</pre>
					<div class="row">
						<button class="btn sm" onclick={copyHelperCommand}><Copy size={13} /> Salin perintah</button>
						<span class="muted small">lalu muat ulang halaman ini — tombol "Terapkan" akan muncul di sini</span>
					</div>
					<details style="margin-top:10px">
						<summary class="muted small" style="cursor:pointer">atau pasang binary manual sekali ini (tanpa helper)</summary>
						<pre class="logbox" style="margin:8px 0">sudo systemctl stop jenderalrouter
sudo install -m 0755 {stagePath()} /usr/local/bin/jenderalrouter
sudo rm -f {stagePath()}
sudo systemctl start jenderalrouter</pre>
						<button class="btn sm" onclick={copyStageCommands}>
							<Copy size={13} /> Salin perintah
						</button>
					</details>
				{/if}
			</div>
		{/if}
		{#if (job?.job?.log?.length || updLog)}
			<pre class="logbox" style="margin-top:10px">{job?.job ? job.job.log.join('\n') : updLog}</pre>
		{/if}
	{/if}
</div>

<div class="card">
	<h2><Globe size={15} /> Cloudflare Tunnel</h2>
	<p class="muted small" style="margin:0 0 10px">
		Akses web UI & API dari internet tanpa membuka port — lewat tunnel Cloudflare.
		Streaming chat sudah mengirim heartbeat berkala, jadi batas idle Cloudflare
		(~100 dtk) dan cloudflared (~90 dtk) tidak memutus jawaban panjang.
	</p>
	{#if cfLoading && !cf}
		<span class="muted small"><span class="spinner dark"></span> memeriksa cloudflared di host…</span>
	{:else if cf}
		<div class="kv small" style="margin-bottom:10px">
			<span class="muted" style="min-width:170px">Binary cloudflared:</span>
			<span>{cf.installed ? '✓ terpasang' : '✗ belum terpasang'}</span>
		</div>
		<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0">
			<span class="muted" style="min-width:170px">Service systemd:</span>
			{#if cf.service_state === 'active'}<span class="badge ok"><span class="dot"></span>active</span>
			{:else if cf.service_state}<span class="badge warn">{cf.service_state}</span>
			{:else}<span class="muted">belum ada</span>{/if}
		</div>
		<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0">
			<span class="muted" style="min-width:170px">Proses tunnel:</span>
			{#if cf.running}<span class="badge ok"><span class="dot pulse"></span>berjalan</span>
			{:else}<span class="badge warn">tidak berjalan</span>{/if}
		</div>
		{#if cf.config_found}
			<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0">
				<span class="muted" style="min-width:170px">Hostname:</span>
				<span class="mono">{cf.hostname || '—'}</span>
			</div>
			<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0">
				<span class="muted" style="min-width:170px">Ingress ke:</span>
				<span class="mono">{cf.ingress_service || '—'}</span>
			</div>
		{/if}
		<div class="row" style="margin:10px 0">
			<button class="btn ghost sm" onclick={loadCf}><RefreshCw size={13} /> Periksa ulang</button>
			<span class="grow"></span>
			<button class="btn sm" class:ghost={cfTab !== 'daftar'} onclick={() => (cfTab = 'daftar')}>1. Daftar di Cloudflare</button>
			<button class="btn sm" class:ghost={cfTab !== 'server'} onclick={() => (cfTab = 'server')}>2. Pemasangan di server</button>
			<button class="btn ghost sm" onclick={copyCfGuide}><Copy size={13} /> Salin</button>
		</div>
		<pre class="logbox" style="max-height:340px;overflow:auto">{cfGuide()}</pre>
		<p class="muted small" style="margin:8px 0 0">
			Streaming chat & API aman lewat tunnel (heartbeat otomatis tiap 15 dtk);
			hindari mode non-streaming untuk jawaban &gt;100 dtk — itu batas platform Cloudflare.
			Tunnel juga otomatis membuat DNS (CNAME) + sertifikat SSL — tidak perlu membuka port.
		</p>
	{:else}
		<button class="btn ghost sm" onclick={loadCf}>Periksa host</button>
	{/if}
</div>

<div class="card">
	<h2><ScrollText size={15} /> {t('audit_log')}</h2>
	{#each audit as a (a.id)}
		<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0">
			<span class="muted mono" style="min-width:150px">{fmtTs(a.ts)}</span>
			<span class="mono">{a.action}</span>
			<span class="muted">{a.target}</span>
		</div>
	{:else}
		<span class="muted small">{t('no_data')}</span>
	{/each}
</div>
