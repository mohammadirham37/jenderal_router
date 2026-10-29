<script lang="ts">
	import { Rocket, RefreshCw, Copy, Undo2, GitCommitHorizontal, CircleCheck, CircleAlert, Wrench } from '@lucide/svelte';
	import { api, fmtTs } from '#lib/api.js';
	import { copyText } from '#lib/clipboard.js';
	import { app, t, toast, roleAtLeast } from '#lib/stores.svelte.js';

	const HELPER_CMD = 'sudo /usr/local/bin/jenderalrouter install-update-helper';

	let upd = $state<any>(null);
	let loading = $state(false);
	let starting = $state(false);
	let applying = $state<'' | 'install' | 'rollback'>('');
	let logEl = $state<HTMLPreElement | null>(null);

	const superAdmin = $derived(roleAtLeast(app.me?.role || '', 'super_admin'));
	const job = $derived(upd?.job);
	const busy = $derived(!!upd?.busy || starting || !!applying);

	const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

	async function load(fetchMode: '0' | '1' | 'auto' = 'auto') {
		loading = true;
		try { upd = await api.get('/api/admin/system/update/status?fetch=' + fetchMode, { noRedirect: true }); }
		catch (e: any) { toast(e.message, 'err'); }
		loading = false;
	}

	$effect(() => {
		(async () => {
			await load('1');
			if (upd?.job?.running) pollJob();
		})();
	});

	// log job ikut bergulir ke bawah
	$effect(() => {
		void job?.log?.length;
		if (logEl) logEl.scrollTop = logEl.scrollHeight;
	});

	async function pollJob() {
		for (;;) {
			try { upd = await api.get('/api/admin/system/update/status', { noRedirect: true }); } catch { /* */ }
			if (!upd?.job?.running) break;
			await sleep(2500);
		}
		const j = upd?.job;
		if (!j?.done) return;
		if (!j.ok) { toast('Pembaruan gagal: ' + (j.error || 'lihat log'), 'err'); return; }
		if (j.restarted) { await waitHelper('install', upd?.apply_requested || ''); return; }
		if (j.staged) toast('Build selesai — binary siap dipasang', 'ok');
		else toast('Sudah versi terbaru', 'ok');
		await load('0');
	}

	// tunggu helper root selesai (service sempat mati saat restart) lalu
	// tampilkan hasilnya: sukses, atau gagal + rollback otomatis
	async function waitHelper(action: 'install' | 'rollback', rid: string) {
		applying = action;
		toast(action === 'install' ? 'Memasang versi baru & me-restart service…' : 'Mengembalikan versi sebelumnya…', 'info');
		let res: any = null;
		for (let i = 0; i < 90 && !res; i++) {
			await sleep(2000);
			try {
				const st = await api.get('/api/admin/system/update/status', { noRedirect: true });
				upd = st;
				// hasil helper dicocokkan lewat id permintaan (file hasil milik root)
				const want = rid || st.apply_requested;
				if (st.apply_result && want && st.apply_result.request === want) res = st.apply_result;
			} catch { /* service sedang restart */ }
		}
		applying = '';
		if (!res) toast('Belum ada konfirmasi dari helper — cek `systemctl status jenderalrouter` di host', 'err');
		else toast(res.message, res.ok ? 'ok' : 'err');
		await load('0');
		// binary baru membawa UI baru — muat ulang agar tab ini tidak menjalankan kode lama
		if (res?.ok) {
			toast('Memuat ulang dashboard ke versi baru…', 'info');
			setTimeout(() => location.reload(), 1800);
		}
	}

	async function startUpdate() {
		if (!confirm('Perbarui aplikasi sekarang?\n\nMirror repo ditarik, binary dibangun ulang (2–5 menit), lalu dipasang. Bila versi baru gagal sehat, versi sebelumnya dipulihkan otomatis.')) return;
		starting = true;
		try {
			await api.post('/api/admin/system/update', {});
			upd = { ...upd, busy: true, job: { running: true, phase: 'menyiapkan', log: ['⏳ dimulai…'] } };
			starting = false;
			await pollJob();
		} catch (e: any) {
			toast(e.message, 'err');
		}
		starting = false;
	}

	async function trigger(action: 'install' | 'rollback') {
		if (action === 'rollback' && !confirm('Kembalikan ke versi sebelumnya?\n\nService di-restart memakai binary backup dari pemasangan terakhir.')) return;
		let rid = '';
		try {
			const r = await api.post('/api/admin/system/update/' + (action === 'install' ? 'apply' : 'rollback'), {});
			rid = r?.request || '';
		} catch (e: any) {
			toast(e.message, 'err');
			return;
		}
		await waitHelper(action, rid);
	}

	async function copy(text: string) {
		(await copyText(text)) ? toast('perintah disalin ✓', 'ok') : toast('gagal menyalin — salin manual', 'err');
	}

	function stagePath() {
		const base = upd?.repo_dir ? upd.repo_dir.replace(/\/src$/, '') : '/var/lib/jenderalrouter';
		return base + '/updates/jenderalrouter.new';
	}
	const manualCmd = $derived(
		[
			'sudo systemctl stop jenderalrouter',
			'sudo install -m 0755 ' + stagePath() + ' /usr/local/bin/jenderalrouter',
			'sudo rm -f ' + stagePath(),
			'sudo systemctl start jenderalrouter'
		].join('\n')
	);
	const applyMode = $derived(
		upd?.apply_mode === 'path_unit' ? '✓ otomatis (unit path systemd)' : upd?.apply_mode === 'sudo' ? '✓ otomatis (helper sudo)' : '✗ belum — perlu helper'
	);
</script>

<svelte:head><title>{t('system_update')} — JenderalRouter</title></svelte:head>

<div class="page-head">
	<h1 class="page-title"><Rocket size={20} /> {t('system_update')}</h1>
	<div class="row">
		<button class="btn ghost" onclick={() => load('1')} disabled={loading || busy}>
			{#if loading}<span class="spinner dark"></span>{:else}<RefreshCw size={15} />{/if}
			{t('check_update')}
		</button>
		{#if upd && !upd.in_container}
			<button class="btn" onclick={startUpdate} disabled={!superAdmin || busy || !upd.update_available}
				title={superAdmin ? '' : 'hanya super_admin'}>
				{#if starting || upd.busy}<span class="spinner"></span>{:else}<Rocket size={15} />{/if}
				{t('update_now')}
			</button>
		{/if}
	</div>
</div>

{#if !upd}
	<div class="card"><span class="spinner dark"></span> {t('loading')}…</div>
{:else if upd.in_container}
	<div class="card"><p class="muted small" style="margin:0">{upd.hint}</p></div>
{:else}
	{#if upd.helper_outdated}
		<div class="note warn">
			<Wrench size={16} />
			<div class="grow">
				<b>Helper pembaruan versi lama</b> — belum mendukung backup & rollback otomatis. Perbarui sekali di host:
				<pre class="logbox" style="margin:8px 0">{HELPER_CMD}</pre>
				<button class="btn sm" onclick={() => copy(HELPER_CMD)}><Copy size={13} /> Salin perintah</button>
			</div>
		</div>
	{/if}

	{#if upd.apply_result && !applying}
		{@const r = upd.apply_result}
		<div class="note {r.ok ? 'ok' : 'err'}">
			{#if r.ok}<CircleCheck size={16} />{:else}<CircleAlert size={16} />{/if}
			<div class="grow">
				<b>{r.action === 'rollback' ? 'Rollback' : 'Pemasangan'} terakhir {r.ok ? 'berhasil' : 'gagal'}</b> — {r.message}
				<span class="muted small"> · {fmtTs(r.time)}</span>
			</div>
		</div>
	{/if}

	{#if applying}
		<div class="note info">
			<span class="spinner dark"></span>
			<div class="grow">
				{applying === 'install' ? 'Memasang versi baru' : 'Mengembalikan versi sebelumnya'} — service di-restart,
				dashboard bisa terputus sebentar. Halaman ini menunggu konfirmasi helper…
			</div>
		</div>
	{/if}

	<div class="grid cols-2">
		<div class="card">
			<h2>Status</h2>
			<div class="kv small" style="margin-bottom:10px">
				{#if upd.busy}<span class="badge info"><span class="dot pulse"></span>{job?.phase || 'berjalan'}</span>
				{:else if upd.update_available}<span class="badge warn"><span class="dot pulse"></span>{upd.behind_commits} commit baru tersedia</span>
				{:else if upd.repo_found}<span class="badge ok"><span class="dot"></span>sudah versi terbaru</span>
				{/if}
				{#if upd.staged_binary}<span class="badge warn">binary siap dipasang</span>{/if}
			</div>
			<div class="rows">
				<div><span class="muted">Versi berjalan</span><span class="mono">{upd.version}</span></div>
				<div><span class="muted">Repo (mirror)</span><span class="mono">{upd.repo_found ? `${upd.repo_dir} [${upd.branch}]` : 'belum di-clone (otomatis)'}</span></div>
				{#if upd.repo_found}
					<div><span class="muted">Commit mirror / remote</span><span class="mono">{upd.current_head || '—'} / {upd.remote_head || '—'}</span></div>
				{/if}
				<div><span class="muted">git / Go</span><span class="mono">{upd.git_available ? '✓' : '✗'} / {upd.go_available ? '✓' : '✗ (diunduh otomatis)'}</span></div>
				<div><span class="muted">Pasang & restart</span><span>{applyMode}</span></div>
				<div><span class="muted">Backup versi sebelumnya</span><span>{upd.backup_available ? '✓ tersedia' : '—'}</span></div>
				{#if upd.last_update}
					<div><span class="muted">Update terakhir</span><span class="mono">{fmtTs(upd.last_update.time)} ({upd.last_update.commits} commit)</span></div>
				{/if}
				{#if upd.last_error}
					<div><span class="muted">Galat terakhir</span><span class="badge err">{upd.last_error}</span></div>
				{/if}
			</div>

			{#if upd.backup_available && upd.apply_mode && superAdmin}
				<div class="row" style="margin-top:12px">
					<button class="btn ghost sm" onclick={() => trigger('rollback')} disabled={busy}>
						<Undo2 size={13} /> Kembalikan versi sebelumnya
					</button>
				</div>
			{/if}
		</div>

		<div class="card">
			<h2><GitCommitHorizontal size={16} /> {upd.update_available ? 'Yang akan masuk' : 'Riwayat terbaru'}</h2>
			{#if upd.update_available}
				{#each upd.pending_commits as c (c.hash)}
					<div class="commit">
						<span class="badge warn mono">{c.hash}</span>
						<span class="grow">{c.subject}</span>
						<span class="muted small">{c.author} · {fmtTs(c.date)}</span>
					</div>
				{/each}
			{:else}
				{#each upd.recent_commits || [] as c (c.hash)}
					<div class="commit">
						<span class="badge mono">{c.hash}</span>
						<span class="grow">{c.subject}</span>
						<span class="muted small">{fmtTs(c.date)}</span>
					</div>
				{:else}
					<span class="muted small">{t('no_data')}</span>
				{/each}
			{/if}
		</div>
	</div>

	{#if upd.staged_binary && !upd.busy}
		<div class="card" style="margin-top:14px">
			<h2>Binary baru sudah dibangun tetapi belum aktif</h2>
			{#if upd.apply_mode}
				<p class="muted small" style="margin:0 0 10px">
					Pasang dan restart dari sini. Binary lama dibackup; bila versi baru gagal sehat, otomatis dikembalikan.
				</p>
				<button class="btn" onclick={() => trigger('install')} disabled={!superAdmin || busy}>
					<Rocket size={15} /> Terapkan & restart sekarang
				</button>
			{:else}
				<p class="muted small" style="margin:0 0 8px">
					Helper pembaruan belum terpasang. Jalankan <b>sekali saja</b> di host — setelah itu semua
					pembaruan berikutnya terpasang dari menu ini tanpa SSH:
				</p>
				<pre class="logbox" style="margin-bottom:8px">{HELPER_CMD}</pre>
				<button class="btn sm" onclick={() => copy(HELPER_CMD)}><Copy size={13} /> Salin perintah</button>
				<details style="margin-top:10px">
					<summary class="muted small" style="cursor:pointer">atau pasang manual sekali ini</summary>
					<pre class="logbox" style="margin:8px 0">{manualCmd}</pre>
					<button class="btn sm" onclick={() => copy(manualCmd)}><Copy size={13} /> Salin perintah</button>
				</details>
			{/if}
		</div>
	{/if}

	{#if job?.log?.length}
		<div class="card" style="margin-top:14px">
			<h2>
				Log pembaruan
				{#if job.running}<span class="badge info"><span class="dot pulse"></span>{job.phase}</span>
				{:else if job.ok}<span class="badge ok">selesai</span>
				{:else if job.done}<span class="badge err">gagal</span>{/if}
			</h2>
			<pre class="logbox" style="max-height:360px;overflow:auto" bind:this={logEl}>{job.log.join('\n')}</pre>
		</div>
	{/if}
{/if}

<style>
	.note {
		display: flex; gap: 10px; align-items: flex-start;
		padding: 12px; border-radius: 10px; margin-bottom: 14px; border: 1px solid;
	}
	.note.warn { background: color-mix(in srgb, var(--warn) 12%, transparent); border-color: color-mix(in srgb, var(--warn) 40%, transparent); }
	.note.ok { background: color-mix(in srgb, var(--ok) 10%, transparent); border-color: color-mix(in srgb, var(--ok) 35%, transparent); }
	.note.err { background: color-mix(in srgb, var(--err) 10%, transparent); border-color: color-mix(in srgb, var(--err) 35%, transparent); }
	.note.info { background: var(--surface-2); border-color: var(--border); }
	.rows > div { display: flex; gap: 10px; padding: 5px 0; border-bottom: 1px solid var(--border); font-size: 13px; }
	.rows > div > span:first-child { min-width: 180px; }
	.commit { display: flex; gap: 10px; align-items: baseline; padding: 6px 0; border-bottom: 1px solid var(--border); font-size: 13px; }
	.commit:last-child { border-bottom: 0; }
	@media (max-width: 700px) {
		.commit { flex-wrap: wrap; }
		.rows > div { flex-direction: column; gap: 2px; }
	}
</style>
