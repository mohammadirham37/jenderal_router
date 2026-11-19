<script lang="ts">
	import {
		Activity, Coins, AlertTriangle, Hash, Cpu, MemoryStick, HardDrive, Gauge, KeyRound, Trophy, MessageSquare, Plug,
		RefreshCw, ArrowUpRight, ArrowDownRight, CircleCheck, CircleAlert, Users, UserPlus, Server, Boxes, ScrollText,
		ChartLine, Timer, MessagesSquare, BookOpen, Copy, Zap
	} from '@lucide/svelte';
	import { api, fmtNum, fmtCost, truncate, fmtTs } from '#lib/api.js';
	import { app, t, toast, roleAtLeast } from '#lib/stores.svelte.js';
	import { copyText } from '#lib/clipboard.js';
	import Sparkline from '#lib/components/Sparkline.svelte';
	import TrafficChart from '#lib/components/TrafficChart.svelte';
	import QuotaCard from '#lib/components/QuotaCard.svelte';
	import JenderalCodePromo from '#lib/components/JenderalCodePromo.svelte';

	const isAdmin = $derived(!!app.me && roleAtLeast(app.me.role, 'admin'));

	// ---------- sapaan ----------
	const now = new Date();
	const hour = Number(new Intl.DateTimeFormat('en-GB', { hour: 'numeric', hour12: false, timeZone: 'Asia/Jakarta' }).format(now));
	const greet = hour < 11 ? 'Selamat pagi' : hour < 15 ? 'Selamat siang' : hour < 18 ? 'Selamat sore' : 'Selamat malam';
	const today = new Intl.DateTimeFormat('id-ID', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric', timeZone: 'Asia/Jakarta' }).format(now);
	const firstName = $derived.by(() => {
		const local = (app.me?.email ?? '').split('@')[0].split(/[._-]/)[0];
		return local ? local[0].toUpperCase() + local.slice(1) : '';
	});

	// =================== ADMIN ===================
	const RANGES = [
		{ k: '24h', label: '24 jam', prev: '24 jam sebelumnya' },
		{ k: '7d', label: '7 hari', prev: '7 hari sebelumnya' },
		{ k: '30d', label: '30 hari', prev: '30 hari sebelumnya' },
		{ k: '90d', label: '90 hari', prev: '90 hari sebelumnya' }
	];
	let range = $state('7d');
	let dash = $state<any>(null);
	let dashLoading = $state(false);
	let autoRefresh = $state(true);
	let updatedAt = $state<Date | null>(null);

	async function loadDash() {
		dashLoading = true;
		try {
			dash = await api.get('/api/admin/dashboard?range=' + range);
			updatedAt = new Date();
		} catch (e: any) {
			toast(e.message, 'err');
		} finally {
			dashLoading = false;
		}
	}
	$effect(() => {
		if (!isAdmin) return;
		void range;
		loadDash();
	});
	$effect(() => {
		if (!isAdmin || !autoRefresh) return;
		const id = setInterval(loadDash, 30000);
		return () => clearInterval(id);
	});

	// statistik sistem live (poll 3 dtk) — khusus admin
	let sys = $state<any>(null);
	$effect(() => {
		if (!isAdmin) return;
		const load = () => api.get('/api/admin/system/stats').then((s) => (sys = s)).catch(() => {});
		load();
		const id = setInterval(load, 3000);
		return () => clearInterval(id);
	});

	// user paling aktif (chat + API), filter per model
	let topModel = $state('');
	let top = $state<any>(null);
	let topLoading = $state(false);
	const rangeDays = $derived(({ '24h': 1, '7d': 7, '30d': 30, '90d': 90 } as Record<string, number>)[range] ?? 7);
	$effect(() => {
		if (!isAdmin) return;
		const m = topModel;
		const d = rangeDays;
		topLoading = true;
		api.get(`/api/admin/analytics/top-users?days=${d}&limit=10` + (m ? '&model=' + encodeURIComponent(m) : ''))
			.then((r) => (top = r))
			.catch((e) => toast(e.message, 'err'))
			.finally(() => (topLoading = false));
	});
	const topMax = $derived(Math.max(1, ...((top?.users ?? []) as any[]).map((u) => u.requests)));

	// ---------- KPI ----------
	const cur = $derived(dash?.current ?? {});
	const prev = $derived(dash?.previous ?? {});
	const series = $derived<any[]>(dash?.series ?? []);
	const errRate = (p: any) => (p?.requests ? (p.errors / p.requests) * 100 : 0);
	function delta(c: number, p: number): { txt: string; up: boolean } | null {
		if (!p && !c) return null;
		if (!p) return { txt: 'baru', up: true };
		const d = ((c - p) / p) * 100;
		return { txt: (d >= 0 ? '+' : '') + (Math.abs(d) >= 100 ? d.toFixed(0) : d.toFixed(1)) + '%', up: d >= 0 };
	}
	const kpis = $derived([
		{ key: 'req', label: 'Request', icon: Activity, value: fmtNum(cur.requests), d: delta(cur.requests, prev.requests), goodUp: true,
			spark: series.map((b) => b.requests), color: 'var(--accent)' },
		{ key: 'tok', label: 'Token', icon: Hash, value: fmtNum((cur.tokens_in ?? 0) + (cur.tokens_out ?? 0)),
			d: delta((cur.tokens_in ?? 0) + (cur.tokens_out ?? 0), (prev.tokens_in ?? 0) + (prev.tokens_out ?? 0)), goodUp: true,
			spark: series.map((b) => b.tokens), color: 'var(--accent-2)' },
		{ key: 'cost', label: 'Biaya (USD)', icon: Coins, value: fmtCost(cur.cost_usd), d: delta(cur.cost_usd ?? 0, prev.cost_usd ?? 0), goodUp: false,
			spark: series.map((b) => b.cost_usd), color: 'var(--warn)' },
		{ key: 'err', label: 'Tingkat error', icon: AlertTriangle, value: errRate(cur).toFixed(1) + '%',
			d: prev.requests || cur.requests ? { txt: ((errRate(cur) - errRate(prev)) >= 0 ? '+' : '') + (errRate(cur) - errRate(prev)).toFixed(1) + ' poin', up: errRate(cur) >= errRate(prev) } : null,
			goodUp: false, spark: series.map((b) => (b.requests ? (b.errors / b.requests) * 100 : 0)), color: 'var(--err)' }
	]);
	const rangePrevLabel = $derived(RANGES.find((r) => r.k === range)?.prev ?? '');

	// ---------- grafik lalu lintas ----------
	const METRICS = [
		{ k: 'requests', label: 'Request' },
		{ k: 'tokens', label: 'Token' },
		{ k: 'cost', label: 'Biaya' }
	];
	let metric = $state('requests');
	const fmtLabel = (t: string, hourly: boolean) => {
		if (hourly) return t.slice(11, 16);
		const d = new Date(t + 'T00:00:00');
		return new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'short' }).format(d);
	};
	const chart = $derived.by(() => {
		const hourly = !!dash?.hourly;
		if (metric === 'requests') {
			return {
				series: [{ name: 'Sukses', color: 'var(--accent)' }, { name: 'Gagal', color: 'var(--err)' }],
				points: series.map((b) => ({ label: hourly ? b.t + ' WIB' : fmtLabel(b.t, false), sub: fmtLabel(b.t, hourly), values: [b.requests - b.errors, b.errors] })),
				format: (v: number) => fmtNum(Math.round(v))
			};
		}
		if (metric === 'tokens') {
			return {
				series: [{ name: 'Token', color: 'var(--accent-2)' }],
				points: series.map((b) => ({ label: hourly ? b.t + ' WIB' : fmtLabel(b.t, false), sub: fmtLabel(b.t, hourly), values: [b.tokens] })),
				format: (v: number) => (v >= 1e6 ? (v / 1e6).toFixed(1) + 'jt' : v >= 1e3 ? (v / 1e3).toFixed(0) + 'rb' : String(Math.round(v)))
			};
		}
		return {
			series: [{ name: 'Biaya', color: 'var(--warn)' }],
			points: series.map((b) => ({ label: hourly ? b.t + ' WIB' : fmtLabel(b.t, false), sub: fmtLabel(b.t, hourly), values: [b.cost_usd] })),
			format: (v: number) => '$' + (v < 1 ? v.toFixed(3) : v.toFixed(2))
		};
	});

	// ---------- provider & model ----------
	const STATUS: Record<string, { label: string; cls: string }> = {
		ok: { label: 'Sehat', cls: 'ok' },
		degraded: { label: 'Terganggu', cls: 'warn' },
		down: { label: 'Diputus', cls: 'err' },
		idle: { label: 'Belum dipakai', cls: 'idle' },
		disabled: { label: 'Nonaktif', cls: 'idle' }
	};
	const providers = $derived<any[]>(
		[...(dash?.providers ?? [])].sort((a, b) => {
			const order = ['down', 'degraded', 'ok', 'idle', 'disabled'];
			return order.indexOf(a.status) - order.indexOf(b.status) || b.period.requests - a.period.requests;
		})
	);
	const modelMax = $derived(Math.max(1, ...((dash?.top_models ?? []) as any[]).map((m) => m.requests)));
	const pctOf = (part: number, whole: number) => (whole ? Math.round((part / whole) * 100) : 0);

	// =================== MEMBER ===================
	let mine = $state<any>(null);
	let myAct = $state<any>(null);
	let myLogs = $state<any[]>([]);
	let myKeys = $state<any[]>([]);
	let memberLoading = $state(true);
	$effect(() => {
		if (!app.me || isAdmin) return;
		(async () => {
			try {
				const [u, a, l, k] = await Promise.all([
					api.get('/api/me/usage'),
					api.get('/api/me/activity?days=7'),
					api.get('/api/me/logs?limit=8'),
					api.get('/api/me/keys')
				]);
				mine = u;
				myAct = a;
				myLogs = l.logs || [];
				myKeys = k.keys || [];
			} catch (e: any) {
				toast(e.message, 'err');
			} finally {
				memberLoading = false;
			}
		})();
	});
	const myToday = $derived(mine?.today ?? {});
	const myActiveKeys = $derived(myKeys.filter((k) => !k.revoked_at));
	const baseURL = typeof location !== 'undefined' ? location.origin + '/v1' : '/v1';
	async function copyBase() {
		(await copyText(baseURL)) ? toast('Base URL disalin ✓', 'ok') : toast('gagal menyalin', 'err');
	}
	const myChart = $derived({
		series: [{ name: 'Chat', color: 'var(--accent)' }, { name: 'API', color: 'var(--accent-2)' }],
		points: (myAct?.series ?? []).map((b: any) => ({ label: fmtLabel(b.t, false), sub: fmtLabel(b.t, false), values: [b.chat, b.api] })),
		format: (v: number) => fmtNum(Math.round(v))
	});
</script>

<svelte:head><title>{t('dashboard')} — JenderalRouter</title></svelte:head>

<header class="dash-head">
	<div class="hello-wrap">
		<div class="date">{today}</div>
		<h1 class="hello">{greet}{firstName ? ',' : ''} <em>{firstName}</em></h1>
	</div>
	{#if isAdmin}
		<div class="controls">
			<div class="seg" role="tablist" aria-label="Rentang waktu">
				{#each RANGES as r (r.k)}
					<button role="tab" aria-selected={range === r.k} class:on={range === r.k} onclick={() => (range = r.k)}>{r.label}</button>
				{/each}
			</div>
			<button class="icon-ctl" onclick={loadDash} title="Muat ulang" aria-label="Muat ulang">
				<span class:spin={dashLoading}><RefreshCw size={15} /></span>
			</button>
			<label class="auto" title="Muat ulang otomatis tiap 30 detik">
				<input type="checkbox" bind:checked={autoRefresh} /> Auto
			</label>
		</div>
	{/if}
</header>

{#if isAdmin}
	<nav class="quick" aria-label="Pintasan">
		<a href="/users"><UserPlus size={15} /> Kelola user</a>
		<a href="/providers"><Server size={15} /> Provider</a>
		<a href="/models"><Boxes size={15} /> Model & harga</a>
		<a href="/logs?status=5xx"><ScrollText size={15} /> Log error</a>
		<a href="/analytics"><ChartLine size={15} /> Analitik</a>
		<span class="grow"></span>
		{#if updatedAt}<span class="upd">diperbarui {updatedAt.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit', second: '2-digit' })}</span>{/if}
	</nav>
	<div class="promo"><JenderalCodePromo variant="strip" dismissKey="jr_jc_admin_dash" /></div>

	{#if !dash}
		<div class="card"><span class="spinner dark"></span> {t('loading')}…</div>
	{:else}
		<!-- ---------- peringatan ---------- -->
		{#if dash.alerts.length}
			<section class="alerts">
				{#each dash.alerts as a, i (i)}
					<a class="alert {a.level}" href={a.link || '#'}>
						<CircleAlert size={16} /><span>{a.text}</span><ArrowUpRight size={14} class="go" />
					</a>
				{/each}
			</section>
		{:else}
			<div class="allgood"><CircleCheck size={16} /> Semua sistem normal — tidak ada provider bermasalah, lonjakan error, atau kuota user yang hampir habis.</div>
		{/if}

		<!-- ---------- KPI ---------- -->
		<section class="kpis">
			{#each kpis as k (k.key)}
				{@const good = k.d ? k.d.up === k.goodUp : true}
				<div class="kpi card">
					<div class="kpi-top"><k.icon size={15} /> {k.label}</div>
					<div class="kpi-val">{k.value}</div>
					<div class="kpi-delta">
						{#if k.d}
							<span class="chip" class:good class:bad={!good}>
								{#if k.d.up}<ArrowUpRight size={12} />{:else}<ArrowDownRight size={12} />{/if}{k.d.txt}
							</span>
							<span class="muted">vs {rangePrevLabel}</span>
						{:else}<span class="muted">belum ada pembanding</span>{/if}
					</div>
					<div class="kpi-spark"><Sparkline values={k.spark} color={k.color} /></div>
				</div>
			{/each}
		</section>
		<div class="minis">
			<span><Users size={13} /> <b>{fmtNum(cur.active_users)}</b> user aktif dari {fmtNum(dash.users_total)}</span>
			<span><KeyRound size={13} /> <b>{fmtNum(dash.keys_active)}</b> API key aktif</span>
			<span><Timer size={13} /> latensi rata-rata <b>{Math.round(cur.avg_latency_ms || 0)} ms</b> · p95 <b>{fmtNum(cur.p95_latency_ms)} ms</b></span>
			<span><AlertTriangle size={13} /> <b>{fmtNum(cur.errors)}</b> request gagal</span>
		</div>

		<!-- ---------- lalu lintas + provider ---------- -->
		<section class="row2">
			<div class="card">
				<div class="card-head">
					<h2><Activity size={15} /> Lalu lintas</h2>
					<div class="seg sm">
						{#each METRICS as m (m.k)}
							<button class:on={metric === m.k} onclick={() => (metric = m.k)}>{m.label}</button>
						{/each}
					</div>
				</div>
				<TrafficChart points={chart.points} series={chart.series} format={chart.format} />
				{#if metric === 'requests'}
					<div class="legend"><span><i style="background:var(--accent)"></i>Sukses</span><span><i style="background:var(--err)"></i>Gagal</span></div>
				{/if}
			</div>

			<div class="card">
				<div class="card-head">
					<h2><Server size={15} /> Kesehatan provider</h2>
					<a class="more" href="/providers">Kelola <ArrowUpRight size={13} /></a>
				</div>
				<ul class="plist">
					{#each providers as p (p.id)}
						{@const st = STATUS[p.status] ?? STATUS.idle}
						<li>
							<span class="dot {st.cls}" title={st.label}></span>
							<div class="pmain">
								<div class="pname">{p.name} <span class="muted mono">{p.type}</span></div>
								<div class="psub muted">
									{#if p.period.requests}
										{fmtNum(p.period.requests)} req · error {pctOf(p.period.errors, p.period.requests)}% · {Math.round(p.period.avg_latency_ms)} ms
									{:else}tidak ada request{/if}
									{#if p.period.last_error}<span class="perr" title="error terakhir {fmtTs(p.period.last_error_at)}"> · {p.period.last_error}</span>{/if}
								</div>
							</div>
							<span class="pstat {st.cls}">{st.label}</span>
						</li>
					{:else}
						<li class="muted small">Belum ada provider. <a href="/providers">Tambahkan sekarang</a>.</li>
					{/each}
				</ul>
			</div>
		</section>

		<!-- ---------- model + error ---------- -->
		<section class="row2 even">
			<div class="card">
				<div class="card-head"><h2><Boxes size={15} /> Model teratas</h2></div>
				{#each dash.top_models as m (m.model)}
					<div class="mrow">
						<div class="mtop">
							<span class="mono mname" title={m.model}>{truncate(m.model, 34)}</span>
							<span class="mnum"><b>{fmtNum(m.requests)}</b> <span class="muted">· {pctOf(m.requests, cur.requests)}%</span></span>
						</div>
						<div class="mbar"><div style="width:{(m.requests / modelMax) * 100}%"></div></div>
						<div class="muted msub">
							{fmtNum(m.tokens)} token · {Math.round(m.avg_latency_ms)} ms
							{#if m.errors}<span class="perr"> · {m.errors} gagal</span>{/if}
						</div>
					</div>
				{:else}
					<p class="muted small">{t('no_data')}</p>
				{/each}
			</div>

			<div class="card">
				<div class="card-head">
					<h2><CircleAlert size={15} /> Error terbaru</h2>
					<a class="more" href="/logs?status=5xx">Semua log <ArrowUpRight size={13} /></a>
				</div>
				<ul class="elist">
					{#each dash.recent_errors as e (e.id)}
						<li>
							<span class="ecode">{e.status || 'ERR'}</span>
							<div class="emain">
								<div><span class="mono">{truncate(e.model, 30)}</span> <span class="muted">via {e.via} · {e.provider || '—'}</span></div>
								<div class="muted esub">{e.error_code || 'gagal'} · {e.email} · {fmtTs(e.ts)}</div>
							</div>
						</li>
					{:else}
						<li class="none"><CircleCheck size={15} /> Tidak ada error pada rentang ini</li>
					{/each}
				</ul>
			</div>
		</section>

		<!-- ---------- user aktif ---------- -->
		<div class="card">
			<div class="card-head">
				<h2><Trophy size={15} /> User paling aktif</h2>
				<div class="row">
					<select bind:value={topModel} style="width:auto;min-width:200px" aria-label="Filter model">
						<option value="">Semua model</option>
						{#each top?.models ?? [] as m (m.model)}
							<option value={m.model}>{m.model} — {fmtNum(m.requests)} req · {m.users} user</option>
						{/each}
					</select>
					{#if topLoading}<span class="spinner dark"></span>{/if}
				</div>
			</div>
			<div class="table-wrap">
				<table>
					<thead>
						<tr><th>#</th><th>{t('user')}</th><th>{t('requests')}</th><th>Chat / API</th><th>{t('tokens')}</th><th>{t('cost')}</th><th>Model favorit</th><th>Terakhir</th></tr>
					</thead>
					<tbody>
						{#each top?.users ?? [] as u, i (u.user_id)}
							<tr>
								<td class="muted">{i + 1}</td>
								<td><a href="/users/{u.user_id}">{u.email}</a></td>
								<td style="min-width:130px">
									<b>{fmtNum(u.requests)}</b>
									<div class="rank-bar"><div style="width:{(u.requests / topMax) * 100}%"></div></div>
								</td>
								<td class="small">
									<span class="badge info" title="chat dashboard"><MessageSquare size={11} /> {fmtNum(u.chat_requests)}</span>
									<span class="badge" title="lewat API key"><Plug size={11} /> {fmtNum(u.api_requests)}</span>
								</td>
								<td class="mono small">{fmtNum(u.tokens_in + u.tokens_out)}</td>
								<td>{fmtCost(u.cost_usd)}</td>
								<td class="mono small">{truncate(u.top_model, 28)}</td>
								<td class="mono small">{fmtTs(u.last_used)}</td>
							</tr>
						{:else}
							<tr><td colspan="8" class="muted">{t('no_data')}</td></tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>

		<!-- ---------- sistem ---------- -->
		<div class="card">
			<div class="card-head">
				<h2><Cpu size={15} /> Server</h2>
				{#if sys?.supported}<span class="muted small">diperbarui {fmtTs(sys.updated_at)}</span>{/if}
			</div>
			{#if sys && sys.supported}
				<div class="sysgrid">
					<div>
						<div class="kv small"><span class="muted">CPU ({sys.cpu_cores} core)</span><b>{sys.cpu_percent}%</b></div>
						<div class="sys-spark">{#each sys.cpu_history as h}<div style="height:{Math.max(5, h)}%"></div>{/each}</div>
					</div>
					{#each [
						{ icon: MemoryStick, label: 'RAM', used: sys.ram_used_gb, total: sys.ram_total_gb },
						{ icon: HardDrive, label: 'Penyimpanan', used: sys.disk_used_gb, total: sys.disk_total_gb }
					] as r (r.label)}
						{@const p = (r.used / (r.total || 1)) * 100}
						<div>
							<div class="kv small"><span class="muted"><r.icon size={12} /> {r.label}</span><b>{r.used} / {r.total} GB</b></div>
							<div class="sys-bar"><div class="fill" class:hot={p > 85} style="width:{Math.min(100, p)}%"></div></div>
							<div class="muted small" style="margin-top:4px">{p.toFixed(0)}% terpakai</div>
						</div>
					{/each}
					<div>
						<div class="kv small"><span class="muted"><Gauge size={12} /> GPU</span>
							{#if sys.gpu}<b>{sys.gpu.percent ?? '—'}%</b>{:else}<span class="muted">tidak ada</span>{/if}</div>
						<div class="muted small" style="margin-top:8px">
							{#if sys.gpu}{sys.gpu.name || 'gpu'}{#if sys.gpu.mem_total_gb} · VRAM {sys.gpu.mem_used_gb}/{sys.gpu.mem_total_gb} GB{/if}
							{:else}Model lokal berjalan di CPU{/if}
						</div>
					</div>
				</div>
			{:else if sys}
				<span class="muted small">Statistik server hanya tersedia di Linux.</span>
			{:else}
				<span class="muted small"><span class="spinner dark"></span> memuat statistik server…</span>
			{/if}
		</div>
	{/if}

{:else if app.me}
	<!-- =================== MEMBER =================== -->
	<nav class="mquick" aria-label="Pintasan">
		<a class="qa primary" href="/chat">
			<span class="qi"><MessagesSquare size={19} /></span>
			<span><b>Mulai chat</b><small>Tanya, ringkas dokumen, buat file</small></span>
			<ArrowUpRight size={16} class="go" />
		</a>
		<a class="qa" href="/usage">
			<span class="qi"><BookOpen size={19} /></span>
			<span><b>API key & panduan</b><small>Hubungkan aplikasi Anda</small></span>
			<ArrowUpRight size={16} class="go" />
		</a>
		<button class="qa" onclick={copyBase}>
			<span class="qi"><Copy size={19} /></span>
			<span><b>Salin Base URL</b><small class="mono">{baseURL}</small></span>
			<Zap size={16} class="go" />
		</button>
	</nav>

	<div class="promo"><JenderalCodePromo dismissKey="jr_jc_member_dash" guideHref="/usage#panduan" /></div>

	{#if memberLoading}
		<div class="card"><span class="spinner dark"></span> {t('loading')}…</div>
	{:else}
		<section class="kpis three">
			<div class="kpi card">
				<div class="kpi-top"><Activity size={15} /> {t('requests_today')}</div>
				<div class="kpi-val">{fmtNum(myToday.requests ?? 0)}</div>
				<div class="kpi-delta muted">reset tengah malam WIB</div>
			</div>
			<div class="kpi card">
				<div class="kpi-top"><Hash size={15} /> {t('tokens_today')}</div>
				<div class="kpi-val">{fmtNum((myToday.tokens_in ?? 0) + (myToday.tokens_out ?? 0))}</div>
				<div class="kpi-delta muted">masuk {fmtNum(myToday.tokens_in ?? 0)} · keluar {fmtNum(myToday.tokens_out ?? 0)}</div>
			</div>
			<div class="kpi card">
				<div class="kpi-top"><ChartLine size={15} /> 7 hari terakhir</div>
				<div class="kpi-val">{fmtNum(myAct?.total?.requests ?? 0)} <span class="unit">request</span></div>
				<div class="kpi-delta muted">chat {fmtNum(myAct?.total?.chat ?? 0)} · API {fmtNum(myAct?.total?.api ?? 0)}</div>
			</div>
		</section>

		<div class="card">
			<div class="card-head">
				<h2><Activity size={15} /> Aktivitas 7 hari</h2>
				<div class="legend"><span><i style="background:var(--accent)"></i>Chat</span><span><i style="background:var(--accent-2)"></i>API</span></div>
			</div>
			<TrafficChart points={myChart.points} series={myChart.series} format={myChart.format} height={170} />
		</div>

		<QuotaCard quotas={mine?.quotas ?? []} />

		<section class="row2 even" style="margin-top:14px">
			<div class="card">
				<div class="card-head">
					<h2><KeyRound size={15} /> API key</h2>
					<a class="more" href="/usage">Kelola <ArrowUpRight size={13} /></a>
				</div>
				{#each myActiveKeys as k (k.id)}
					<div class="keyrow"><span class="mono">{k.prefix}…</span><span>{k.name}</span><span class="badge ok"><span class="dot"></span>aktif</span></div>
				{:else}
					<p class="muted small" style="margin:0">Belum punya API key aktif. <a href="/usage">Buat sekarang</a> untuk memakai model dari aplikasi Anda.</p>
				{/each}
			</div>
			<div class="card">
				<div class="card-head"><h2><ScrollText size={15} /> Aktivitas terbaru</h2></div>
				<ul class="elist">
					{#each myLogs as l (l.id)}
						<li>
							<span class="ecode" class:ok={l.status >= 200 && l.status < 400}>{l.status}</span>
							<div class="emain">
								<div class="mono">{truncate(l.requested_model, 34)}</div>
								<div class="muted esub">{fmtTs(l.ts)} · {l.latency_ms} ms · {fmtNum(+l.tokens_in + +l.tokens_out)} token</div>
							</div>
						</li>
					{:else}
						<li class="none muted">Belum ada aktivitas — mulai dari <a href="/chat">Chat</a>.</li>
					{/each}
				</ul>
			</div>
		</section>
	{/if}
{/if}

<style>
	/* ---------- kepala ---------- */
	.dash-head {
		display: flex; align-items: flex-end; justify-content: space-between; gap: 14px; flex-wrap: wrap;
		margin-bottom: 14px;
	}
	.date { font-family: var(--mono); font-size: 11.5px; letter-spacing: .06em; text-transform: uppercase; color: var(--muted); }
	.hello { margin: 4px 0 0; font-size: clamp(26px, 3vw, 36px); font-weight: 780; letter-spacing: -0.035em; line-height: 1.1; }
	.hello em {
		font-family: var(--font-display); font-style: italic; font-weight: 400; letter-spacing: 0; font-size: 1.12em;
		background: linear-gradient(100deg, var(--accent), var(--accent-2)); -webkit-background-clip: text; background-clip: text; color: transparent;
	}
	.controls { display: flex; align-items: center; gap: 8px; }
	.seg { display: inline-flex; padding: 3px; border-radius: 11px; background: var(--surface-2); border: 1px solid var(--border); }
	.seg button {
		all: unset; cursor: pointer; padding: 6px 12px; border-radius: 8px; font-size: 12.5px; font-weight: 650; color: var(--muted);
		transition: background var(--speed), color var(--speed);
	}
	.seg button:hover { color: var(--text); }
	.seg button.on { background: var(--surface); color: var(--text); box-shadow: var(--shadow-sm); }
	.seg button:focus-visible { outline: 2px solid var(--accent); }
	.seg.sm button { padding: 4px 10px; font-size: 12px; }
	.icon-ctl {
		all: unset; cursor: pointer; width: 34px; height: 34px; border-radius: 10px; display: grid; place-items: center;
		border: 1px solid var(--border); color: var(--muted); background: var(--surface);
	}
	.icon-ctl:hover { color: var(--text); border-color: var(--border-strong); }
	.spin { display: grid; animation: spin .8s linear infinite; }
	@keyframes spin { to { transform: rotate(360deg); } }
	.auto { display: inline-flex; align-items: center; gap: 6px; font-size: 12px; color: var(--muted); margin: 0; cursor: pointer; }
	.auto input { width: auto; margin: 0; accent-color: var(--accent); }

	.quick { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; margin-bottom: 14px; }
	.quick a {
		display: inline-flex; align-items: center; gap: 7px; padding: 6px 12px; border-radius: 999px; font-size: 12.5px; font-weight: 600;
		text-decoration: none; color: var(--text); background: var(--surface); border: 1px solid var(--border);
		transition: border-color var(--speed), transform var(--speed);
	}
	.quick a:hover { border-color: var(--accent); transform: translateY(-1px); }
	.quick a :global(svg) { color: var(--accent); }
	.upd { font-family: var(--mono); font-size: 11px; color: var(--muted); }

	.promo { margin-bottom: 14px; }

	/* ---------- peringatan ---------- */
	.alerts { display: grid; gap: 6px; margin-bottom: 14px; }
	.alert {
		display: flex; align-items: center; gap: 10px; padding: 10px 14px; border-radius: 12px; text-decoration: none; font-size: 13px;
		border: 1px solid; transition: transform var(--speed);
	}
	.alert:hover { transform: translateX(2px); }
	.alert.err { color: var(--err); background: color-mix(in srgb, var(--err) 9%, transparent); border-color: color-mix(in srgb, var(--err) 35%, transparent); }
	.alert.warn { color: var(--warn); background: color-mix(in srgb, var(--warn) 9%, transparent); border-color: color-mix(in srgb, var(--warn) 35%, transparent); }
	.alert span { flex: 1; color: var(--text); }
	.alert :global(.go) { opacity: .6; }
	.allgood {
		display: flex; align-items: center; gap: 10px; padding: 10px 14px; margin-bottom: 14px; border-radius: 12px; font-size: 13px;
		color: var(--ok); background: color-mix(in srgb, var(--ok) 8%, transparent); border: 1px solid color-mix(in srgb, var(--ok) 28%, transparent);
	}

	/* ---------- KPI ---------- */
	.kpis { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 14px; }
	.kpis.three { grid-template-columns: repeat(3, minmax(0, 1fr)); margin-bottom: 14px; }
	/* grid mengatur jarak — batalkan margin .card + .card global */
	.kpis > .card, .row2 > .card { margin-top: 0; }
	.kpi { position: relative; overflow: hidden; padding-bottom: 0; display: flex; flex-direction: column; }
	.kpis.three .kpi { padding-bottom: 16px; }
	.kpi-top { display: flex; align-items: center; gap: 7px; font-size: 12.5px; font-weight: 650; color: var(--muted); }
	.kpi-top :global(svg) { color: var(--accent); }
	.kpi-val { font-size: 30px; font-weight: 800; letter-spacing: -0.03em; margin-top: 8px; font-variant-numeric: tabular-nums; }
	.unit { font-size: 14px; font-weight: 600; color: var(--muted); letter-spacing: 0; }
	.kpi-delta { display: flex; align-items: center; gap: 8px; margin-top: 6px; font-size: 12px; min-height: 22px; }
	.chip { display: inline-flex; align-items: center; gap: 3px; padding: 2px 8px; border-radius: 999px; font-weight: 700; font-family: var(--mono); font-size: 11.5px; }
	.chip.good { color: var(--ok); background: color-mix(in srgb, var(--ok) 13%, transparent); }
	.chip.bad { color: var(--err); background: color-mix(in srgb, var(--err) 13%, transparent); }
	.kpi-spark { margin: 10px -18px 0; }
	.minis { display: flex; flex-wrap: wrap; gap: 8px 22px; margin: 12px 2px 14px; font-size: 12.5px; color: var(--muted); }
	.minis span { display: inline-flex; align-items: center; gap: 6px; }
	.minis b { color: var(--text); font-variant-numeric: tabular-nums; }

	/* ---------- tata letak ---------- */
	.row2 { display: grid; grid-template-columns: minmax(0, 1.65fr) minmax(0, 1fr); gap: 14px; margin-bottom: 14px; }
	.row2.even { grid-template-columns: repeat(2, minmax(0, 1fr)); }
	.row2 > .card { margin: 0; }
	.card-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-bottom: 14px; flex-wrap: wrap; }
	.card-head h2 { margin: 0; }
	.more { display: inline-flex; align-items: center; gap: 3px; font-size: 12px; text-decoration: none; color: var(--muted); }
	.more:hover { color: var(--accent-2); }
	.legend { display: flex; gap: 14px; font-size: 12px; color: var(--muted); margin-top: 8px; }
	.card-head .legend { margin-top: 0; }
	.legend span { display: inline-flex; align-items: center; gap: 6px; }
	.legend i { width: 9px; height: 9px; border-radius: 3px; }

	/* ---------- provider ---------- */
	.plist { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }
	.plist li { display: flex; align-items: center; gap: 11px; padding: 10px 0; border-top: 1px solid var(--border); }
	.plist li:first-child { border-top: 0; padding-top: 0; }
	.dot { width: 9px; height: 9px; border-radius: 50%; flex: 0 0 auto; }
	.dot.ok { background: var(--ok); box-shadow: 0 0 0 4px color-mix(in srgb, var(--ok) 16%, transparent); }
	.dot.warn { background: var(--warn); box-shadow: 0 0 0 4px color-mix(in srgb, var(--warn) 18%, transparent); }
	.dot.err { background: var(--err); box-shadow: 0 0 0 4px color-mix(in srgb, var(--err) 18%, transparent); animation: blink 1.4s infinite; }
	.dot.idle { background: var(--border-strong); }
	@keyframes blink { 50% { opacity: .35; } }
	.pmain { flex: 1; min-width: 0; }
	.pname { font-weight: 650; font-size: 13.5px; }
	.pname .mono { font-size: 11px; margin-left: 4px; }
	.psub { font-size: 12px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.perr { color: var(--err); }
	.pstat { font-size: 11.5px; font-weight: 700; white-space: nowrap; }
	.pstat.ok { color: var(--ok); }
	.pstat.warn { color: var(--warn); }
	.pstat.err { color: var(--err); }
	.pstat.idle { color: var(--muted); }

	/* ---------- model ---------- */
	.mrow + .mrow { margin-top: 12px; }
	.mtop { display: flex; justify-content: space-between; gap: 10px; font-size: 13px; }
	.mname { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.mnum { white-space: nowrap; font-variant-numeric: tabular-nums; }
	.mbar { height: 6px; border-radius: 999px; background: var(--surface-3); margin-top: 6px; overflow: hidden; }
	.mbar div { height: 100%; border-radius: 999px; background: var(--grad-accent); transition: width .5s cubic-bezier(.2,.7,.2,1); }
	.msub { font-size: 11.5px; margin-top: 4px; }

	/* ---------- error / aktivitas ---------- */
	.elist { list-style: none; margin: 0; padding: 0; }
	.elist li { display: flex; gap: 11px; align-items: flex-start; padding: 9px 0; border-top: 1px solid var(--border); }
	.elist li:first-child { border-top: 0; padding-top: 0; }
	.ecode {
		flex: 0 0 auto; min-width: 42px; text-align: center; padding: 2px 6px; border-radius: 7px; font-family: var(--mono); font-size: 11.5px; font-weight: 700;
		color: var(--err); background: color-mix(in srgb, var(--err) 12%, transparent);
	}
	.ecode.ok { color: var(--ok); background: color-mix(in srgb, var(--ok) 12%, transparent); }
	.emain { min-width: 0; font-size: 13px; }
	.esub { font-size: 11.5px; margin-top: 2px; overflow-wrap: anywhere; }
	.elist .none { display: flex; align-items: center; gap: 8px; color: var(--ok); font-size: 13px; }
	.elist .none.muted { color: var(--muted); }

	.rank-bar { height: 5px; margin-top: 4px; background: var(--surface-3); border-radius: 4px; overflow: hidden; }
	.rank-bar div { height: 100%; background: var(--grad-accent); }

	/* ---------- server ---------- */
	.sysgrid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 18px; }
	.sysgrid :global(svg) { vertical-align: -2px; }
	.sys-spark { display: flex; align-items: flex-end; gap: 2px; height: 38px; margin-top: 6px; padding: 3px; background: var(--surface-2); border: 1px solid var(--border); border-radius: 8px; overflow: hidden; }
	.sys-spark div { flex: 1; min-width: 2px; background: var(--grad-accent); border-radius: 1px; opacity: .85; }
	.sys-bar { height: 8px; margin-top: 8px; background: var(--surface-3); border-radius: 6px; overflow: hidden; }
	.sys-bar .fill { height: 100%; background: var(--grad-accent); border-radius: 6px; transition: width .6s ease; }
	.sys-bar .fill.hot { background: var(--warn); }

	/* ---------- member ---------- */
	.mquick { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; margin-bottom: 14px; }
	.qa {
		all: unset; box-sizing: border-box; cursor: pointer; display: flex; align-items: center; gap: 12px; padding: 14px 16px; border-radius: 14px;
		background: var(--surface); border: 1px solid var(--border); text-decoration: none; color: var(--text);
		transition: border-color var(--speed), transform var(--speed), box-shadow var(--speed);
	}
	.qa:hover { border-color: var(--accent); transform: translateY(-2px); box-shadow: var(--shadow); }
	.qa:focus-visible { outline: 2px solid var(--accent); }
	.qa > span:nth-child(2) { flex: 1; min-width: 0; display: flex; flex-direction: column; }
	.qa b { font-size: 14px; }
	.qa small { font-size: 12px; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.qi { width: 40px; height: 40px; border-radius: 12px; display: grid; place-items: center; flex: 0 0 auto; color: var(--accent); background: var(--grad-accent-soft); }
	.qa.primary { background: var(--grad-accent); border-color: transparent; color: #fff; box-shadow: 0 10px 26px color-mix(in srgb, var(--accent) 35%, transparent); }
	.qa.primary small { color: rgb(255 255 255 / .82); }
	.qa.primary .qi { background: rgb(255 255 255 / .18); color: #fff; }
	.qa :global(.go) { opacity: .55; flex: 0 0 auto; }
	.keyrow { display: flex; align-items: center; gap: 10px; padding: 8px 0; border-top: 1px solid var(--border); font-size: 13px; }
	.keyrow:first-of-type { border-top: 0; padding-top: 0; }
	.keyrow span:nth-child(2) { flex: 1; }

	@media (max-width: 1100px) {
		.kpis { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.row2, .row2.even { grid-template-columns: 1fr; }
		.sysgrid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
	}
	@media (max-width: 640px) {
		.controls { width: 100%; }
		.seg { flex: 1; }
		.seg button { flex: 1; text-align: center; padding: 6px 4px; }
		.kpis, .kpis.three { grid-template-columns: 1fr 1fr; gap: 10px; }
		.kpis.three .kpi:last-child { grid-column: span 2; }
		.kpi-val { font-size: 24px; }
		.kpi-delta .muted { display: none; }
		.mquick { grid-template-columns: 1fr; }
		.sysgrid { grid-template-columns: 1fr; }
		.upd { display: none; }
	}
</style>
