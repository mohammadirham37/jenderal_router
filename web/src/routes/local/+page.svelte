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
		// lanjutkan polling unduhan model yang mungkin masih berjalan di latar
		api.get('/api/admin/local/models/download/status').then((j) => {
			if (j?.running) { dlJob = j; pollDlJob(); }
		}).catch(() => {});
	});

	// muat rekomendasi begitu status terpasang diketahui
	$effect(() => {
		if (status?.installed && !status?.in_container && recs.length === 0 && !recsLoading) loadRecs();
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

	// ---- unduh model (rekomendasi hardware-aware + repo HF kustom) ----
	let recs = $state<any[]>([]);
	let hw = $state<any>(null);
	let recsLoading = $state(false);
	let dlJob = $state<any>(null);
	let dlStarting = $state(false);
	let customRepo = $state('');

	function fmtB(b: number): string {
		if (b >= 1e9) return (b / 1e9).toFixed(b >= 1e10 ? 0 : 1) + 'B';
		if (b >= 1e6) return Math.round(b / 1e6) + 'M';
		return String(b);
	}

	async function loadRecs() {
		recsLoading = true;
		try {
			const r = await api.get('/api/admin/local/models/recommendations');
			recs = r.recommendations || [];
			hw = r.hardware || null;
		} catch { recs = []; hw = null; }
		recsLoading = false;
	}

	async function pollDlJob() {
		for (;;) {
			try { dlJob = await api.get('/api/admin/local/models/download/status'); } catch { /* */ }
			if (!dlJob?.running) break;
			await new Promise((r) => setTimeout(r, 2500));
		}
		if (dlJob?.done) {
			toast(dlJob.ok ? 'Unduhan model selesai ✓ — sinkronkan model di atas bila perlu' : 'Unduhan gagal: ' + (dlJob.err || 'lihat log'), dlJob.ok ? 'ok' : 'err');
			reload();
		}
	}

	async function downloadModel(repo: string, file: string) {
		if (!repo) { toast('isi repo dulu (owner/repo)', 'err'); return; }
		dlStarting = true;
		try {
			await api.post('/api/admin/local/models/download', { repo, file });
			customRepo = '';
			dlJob = { running: true, phase: 'menyiapkan unduhan', log: [] };
			pollDlJob();
		} catch (e: any) { toast(e.message, 'err'); }
		dlStarting = false;
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

	<!-- unduh model (rekomendasi hardware-aware) -->
	{#if !status.in_container && status.installed}
		<div class="card" style="margin-bottom:14px">
			<h2><Download size={15} /> Unduh Model</h2>
			{#if dlJob?.running}
				<div class="kv" style="margin:6px 0 10px">
					<span class="spinner dark"></span>
					<span class="badge info"><span class="dot pulse"></span>{dlJob.phase || 'mengunduh'}</span>
					<span class="muted small">aman meninggalkan halaman ini — unduhan lanjut di latar</span>
				</div>
				<pre class="logbox">{dlJob.log?.join('\n') || ''}</pre>
			{:else}
				{#if recsLoading}
					<p class="muted small"><span class="spinner dark"></span> memuat rekomendasi untuk hardware ini…</p>
				{:else if recs.length}
					{#if hw}
						<p class="muted small" style="margin:0 0 10px">
							Hardware: {hw.gpu_backend} · RAM {hw.ram_total_gb} GB — rekomendasi terurut dari yang terbaik.
						</p>
					{/if}
					<div class="table-wrap">
						<table>
							<thead><tr><th>Model</th><th>Ukuran</th><th></th></tr></thead>
							<tbody>
								{#each recs as m (m.id)}
									<tr>
										<td>
											<div class="small" style="font-weight:700">{m.repo}</div>
											<div class="muted small mono">{m.file}</div>
											<div class="muted small">
												{#if m.moe && m.params_active_b}MoE {fmtB(m.params_active_b)} aktif · {/if}
												{m.justification}
											</div>
										</td>
										<td class="small">
											{m.weights_gb} GB
											{#if m.peak_gb}<div class="muted small">peak ±{m.peak_gb} GB</div>{/if}
											{#if m.bench}<div class="muted small">skor {m.bench}</div>{/if}
										</td>
										<td>
											<button class="btn sm" disabled={dlStarting || !!dlJob?.running}
												onclick={() => downloadModel(m.repo, m.file)}>
												{#if dlStarting}<span class="spinner"></span>{:else}<Download size={12} />{/if}
												Unduh
											</button>
										</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{:else}
					<p class="muted small">Rekomendasi tidak tersedia (CLI llamastash tidak ditemukan atau gagal dijalankan).</p>
				{/if}
				<div class="row" style="margin-top:10px">
					<input style="max-width:360px" bind:value={customRepo}
						placeholder="repo HF kustom: owner/repo[:file.gguf]" />
					<button class="btn ghost sm" disabled={dlStarting || !!dlJob?.running || !customRepo}
						onclick={() => downloadModel(customRepo, '')}>
						<Download size={13} /> Unduh
					</button>
				</div>
			{/if}
			{#if dlJob?.done && !dlJob.ok}
				<p class="small" style="color:var(--err);margin-top:8px">{dlJob.err}</p>
			{/if}
		</div>
	{/if}

	<!-- status daemon -->
	<div class="card">
		<div class="page-head" style="margin-bottom:10px">
			<div>
				<div class="row">
					<strong style="font-size:15px">LlamaStash</strong>
					{#if status.daemon_alive}<span class="badge ok"><span class="dot pulse"></span>{t('alive')}</span>
					{:else}<span class="badge err"><span class="dot"></span>{t('down')}</span>{/if}
					{#if status.version}<span class="badge info">{status.version}</span>{/if}
					{#if status.latency_ms !== undefined}<span class="muted small">{status.latency_ms} ms</span>{/if}
				</div>
				<div class="muted small mono" style="margin-top:4px">
					{status.proxy_listen || status.url}{#if status.proxy_auth && status.proxy_auth !== 'none'} · auth: {status.proxy_auth}{:else} · keyless loopback{/if}
				</div>
				<div class="muted small">
					{#if status.daemon_build}daemon {status.daemon_build} (pid {status.daemon_pid}) · {/if}
					CLI: {status.cli_available ? '✓' : '✗'}
					{#if status.ui_url}· <a href={status.ui_url} target="_blank" rel="noopener">Web UI ↗</a>{/if}
				</div>
				{#if status.listen_mismatch}
					<p class="small" style="color:var(--warn);margin-top:6px">⚠ {status.hint}</p>
				{/if}
				{#if status.host}
					<div class="kv small muted" style="margin-top:6px">
						<span>CPU {status.host.cpu_pct}%</span>
						<span>RAM {status.host.ram_used_gb}/{status.host.ram_total_gb} GB</span>
						{#if status.host.gpu_mem_total}<span>GPU ({status.host.gpu_backend}) {status.host.gpu_mem_total} GB</span>
						{:else}<span>GPU: {status.host.gpu_backend}</span>{/if}
					</div>
				{/if}
				{#if status.models_discovered !== undefined}
					<div class="muted small">model: {status.models_loaded} dimuat / {status.models_discovered} ditemukan</div>
				{/if}
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
