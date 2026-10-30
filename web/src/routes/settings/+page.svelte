<script lang="ts">
	import { Settings, Save, Database, Rocket, RefreshCw, ScrollText, Copy, Globe, FileSearch } from '@lucide/svelte';
	import { api } from '#lib/api.js';
	import { copyText } from '#lib/clipboard.js';
	import { fmtTs } from '#lib/api.js';
	import { t, toast, app, roleAtLeast } from '#lib/stores.svelte.js';

	// ---- pencatatan isi request API (global) ----
	let logContent = $state(false);
	let logPromptsEnv = $state(false);
	const superAdmin = $derived(roleAtLeast(app.me?.role || '', 'super_admin'));
	async function setLogContent(on: boolean) {
		try {
			await api.put('/api/admin/settings', { log_content: on ? 'true' : 'false' });
			logContent = on;
			toast(on ? 'Isi semua request API kini dicatat (7 hari)' : 'Pencatatan isi global dimatikan', 'ok');
		} catch (e: any) { toast(e.message, 'err'); }
	}

	// ---- notifikasi ----
	let tgToken = $state(''); let tgChat = $state(''); let tgTokenSet = $state(false);
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
	async function backup() {
		try { await api.post('/api/admin/system/backup'); toast(t('save') + ' ✓', 'ok'); }
		catch (e: any) { toast(e.message, 'err'); }
	}

	$effect(() => {
		(async () => {
			try { const s = await api.get('/api/admin/settings'); tgTokenSet = s.settings.notification_telegram_token_set === 'true'; tgChat = s.settings.notification_chat_id || ''; logContent = s.settings.log_content === 'true'; logPromptsEnv = s.settings.log_prompts === 'true'; } catch { /* */ }
		})();
	});

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
	<label for="f-tg-token">Telegram bot token</label>
	<input id="f-tg-token" type="password" bind:value={tgToken} placeholder={tgTokenSet ? 'tersimpan — isi untuk mengganti' : '123456:ABC…'} autocomplete="off" />
	<label for="f-tg-chat">Telegram chat id</label>
	<input id="f-tg-chat" bind:value={tgChat} />
	<div class="modal-actions" style="justify-content:flex-start;margin-top:12px">
		<button class="btn" onclick={saveSettings}><Save size={15} /> {t('save')}</button>
	</div>
</div>

<div class="card">
	<h2><FileSearch size={15} /> Pencatatan isi request API</h2>
	<p class="muted small" style="margin:0 0 10px">
		Saklar global: isi request &amp; jawaban <b>semua</b> API key dicatat. Untuk user/key tertentu saja, gunakan saklar di
		halaman detail user. Isi disimpan 7 hari lalu dikosongkan otomatis; rahasia yang dikenali disamarkan; setiap pembukaan
		tercatat di audit log.
	</p>
	{#if logPromptsEnv}
		<div class="badge warn">Dipaksa menyala oleh env JR_LOG_PROMPTS di server.</div>
	{:else}
		<label class="kv" style="cursor:pointer">
			<input type="checkbox" style="width:auto;margin:0" checked={logContent} disabled={!superAdmin}
				onchange={(e) => setLogContent(e.currentTarget.checked)} />
			<span>Catat isi semua request API</span>
			{#if !superAdmin}<span class="muted small">(hanya super admin yang bisa mengubah)</span>{/if}
		</label>
	{/if}
</div>

<div class="card">
	<h2><Database size={15} /> Backup</h2>
	<p class="muted small">Backup harian otomatis ke <code>data/backups/</code> (retensi 7 salinan).</p>
	<button class="btn" onclick={backup}><Database size={15} /> {t('backup_now')}</button>
</div>

<div class="card">
	<h2><Rocket size={15} /> Pembaruan Aplikasi</h2>
	<p class="muted small" style="margin:0 0 10px">Tarik versi terbaru, build, pasang, dan rollback kini ada di menu tersendiri.</p>
	<a class="btn ghost sm" href="/update"><Rocket size={13} /> Buka {t('system_update')}</a>
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
