<script lang="ts">
	import { Server, Download, Play, Square, TerminalSquare, RefreshCw } from '@lucide/svelte';
	import { api } from '$lib/api';
	import { t, toast } from '$lib/stores.svelte';
	import Modal from '$lib/components/Modal.svelte';

	let status = $state<any>(null);
	let loading = $state(true);
	let installing = $state(false);
	let job = $state<any>(null);
	let busyModel = $state('');

	async function pollJob() {
		for (;;) {
			try {
				job = await api.get('/api/admin/local/install/status');
			} catch { /* */ }
			if (!job?.running) break;
			await new Promise((r) => setTimeout(r, 2500));
		}
		if (job?.done) {
			toast(job.ok ? 'Install LlamaStash selesai ✓' : 'Install gagal: ' + (job.err || 'lihat log'), job.ok ? 'ok' : 'err');
			reload();
		}
	}

	async function reload() {
		loading = true;
		try { status = await api.get('/api/admin/local/status'); }
		catch (e: any) { toast(e.message, 'err'); }
		loading = false;
	}
	$effect(() => {
		reload();
		// bila ada job berjalan (mis. halaman dimuat ulang saat instalasi), lanjutkan polling
		api.get('/api/admin/local/install/status').then((j) => {
			if (j?.running) { job = j; pollJob(); }
		}).catch(() => {});
	});

	async function install() {
		if (!confirm('Install LlamaStash di server ini sekarang?\n\nCatatan: init akan mengunduh model recommended — bisa sangat lama tergantung koneksi. Log progres tampil langsung.')) return;
		installing = true;
		try {
			await api.post('/api/admin/local/install', {});
			job = { running: true, phase: 'menyiapkan', log: [] };
			pollJob();
		} catch (e: any) {
			toast(e.message, 'err');
		}
		installing = false;
	}
	async function toggleModel(name: string, action: 'start' | 'stop') {
		busyModel = name + action;
		try {
			const res = await api.post(`/api/admin/local/models/${encodeURIComponent(name)}/${action}`);
			toast('OK: ' + (res.output || action).slice(0, 80), 'ok');
			reload();
		} catch (e: any) { toast(e.message, 'err'); }
		busyModel = '';
	}
</script>

<svelte:head><title>{t('local')} — JenderalRouter</title></svelte:head>

<div class="page-head">
	<h1 class="page-title"><Server size={20} /> {t('local')}</h1>
	<button class="btn ghost" onclick={reload}><RefreshCw size={15} /></button>
</div>

{#if loading}
	<div class="card"><span class="spinner dark"></span> {t('loading')}…</div>
{:else if status}
	<!-- kartu instalasi (FR-6.8) -->
	<div class="card" style="margin-bottom:14px">
		<h2><TerminalSquare size={15} /> Install LlamaStash</h2>
		{#if job?.running}
			<div class="kv" style="margin:6px 0 10px">
				<span class="spinner dark"></span>
				<span class="badge info"><span class="dot pulse"></span>{job.phase || 'berjalan'}</span>
			</div>
			<pre class="logbox">{job.log.join('\n')}</pre>
		{:else if status.in_container}
			<p class="muted small">{status.hint}</p>
		{:else if status.installed}
			<div class="kv">
				<span class="badge ok"><span class="dot"></span>terpasang</span>
				{#if status.version}<span class="muted small mono">{status.version}</span>{/if}
				<span class="muted small mono">{status.bin}</span>
			</div>
		{:else}
			<p class="muted small">
				Belum terpasang. Tombol ini mengunduh installer resmi, menjalankan
				<code>llamastash init --recommended --json</code>, lalu mendaftarkan provider lokal + modelnya otomatis.
			</p>
			{#if job?.running}
				<div class="kv" style="margin:10px 0">
					<span class="spinner dark"></span>
					<span class="badge info"><span class="dot pulse"></span>{job.phase || 'berjalan'}</span>
					<span class="muted small">log live di bawah — aman meninggalkan halaman ini</span>
				</div>
			{:else}
				<button class="btn" onclick={install} disabled={installing}>
					{#if installing}<span class="spinner"></span>{:else}<Download size={15} />{/if}
					{t('install')} LlamaStash
				</button>
			{/if}
			{#if job && job.log?.length}
				<pre class="logbox" style="margin-top:10px">{job.log.join('\n')}</pre>
			{/if}
			{#if job?.done && !job.ok}
				<p class="small" style="color:var(--err);margin-top:8px">{job.err}</p>
			{/if}
			<p class="muted small" style="margin-top:8px">
				Binary dicari di: PATH, <code>~/.local/bin</code>, <code>/usr/local/bin</code>, <code>/usr/bin</code>, <code>/opt/llamastash/bin</code>.
				Bila Anda memasang manual via SSH, pastikan daemon aktif (<code>llamastash init</code>) lalu tekan ↻.
			</p>
		{/if}
	</div>

	<!-- status daemon -->
	<div class="card">
		<div class="page-head" style="margin-bottom:10px">
			<div>
				<div class="row">
					<strong style="font-size:15px">LlamaStash</strong>
					{#if status.daemon_alive}<span class="badge ok"><span class="dot pulse"></span>{t('alive')}</span>
					{:else}<span class="badge err"><span class="dot"></span>{t('down')}</span>{/if}
					{#if status.latency_ms !== undefined}<span class="muted small">{status.latency_ms} ms</span>{/if}
				</div>
				<div class="muted small mono" style="margin-top:4px">{status.url}</div>
				<div class="muted small">CLI: {status.cli_available ? '✓' : '✗'}{status.installed ? ' · terpasang' : ''}</div>
			</div>
		</div>

		{#if !status.daemon_alive && !status.in_container && status.installed}
			<p class="muted small">
				Daemon tidak merespons. Cek di host: <code>llamastash status</code> atau jalankan ulang
				<code>llamastash init --recommended --json</code>.
			</p>
		{/if}

		{#if (status.models || []).length === 0}
			<p class="muted">{t('no_data')}</p>
		{:else}
			<div class="table-wrap">
				<table>
					<thead><tr><th>Model</th><th>{t('status')}</th><th></th></tr></thead>
					<tbody>
						{#each status.models as m (m.name)}
							<tr>
								<td class="mono">{m.name}</td>
								<td>{#if m.loaded}<span class="badge ok"><span class="dot"></span>loaded</span>{:else}<span class="badge info">idle</span>{/if}</td>
								<td>
									<div class="row">
										<button class="btn sm" disabled={busyModel === m.name + 'start'} onclick={() => toggleModel(m.name, 'start')}>
											{#if busyModel === m.name + 'start'}<span class="spinner"></span>{:else}<Play size={12} />{/if}
											{t('start')}
										</button>
										<button class="btn ghost sm" disabled={busyModel === m.name + 'stop'} onclick={() => toggleModel(m.name, 'stop')}>
											{#if busyModel === m.name + 'stop'}<span class="spinner"></span>{:else}<Square size={12} />{/if}
											{t('stop')}
										</button>
									</div>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
{/if}
