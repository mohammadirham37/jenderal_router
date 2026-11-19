<script lang="ts">
	import {
		Users, UserPlus, Trash2, KeyRound, Gauge, Copy, X, Activity, Search, ChartLine, RefreshCw, MoreHorizontal,
		ShieldCheck, ShieldOff, Power, Lock, Eye, EyeOff, Wand2, ArrowUpDown, Coins, FileSearch, Check, Plus, UserCog
	} from '@lucide/svelte';
	import { browser } from '$app/env';
	import { goto } from '$app/navigation';
	import { api, fmtNum, fmtCost, fmtTs } from '#lib/api.js';
	import { copyText } from '#lib/clipboard.js';
	import { app, t, toast } from '#lib/stores.svelte.js';
	import Modal from '#lib/components/Modal.svelte';
	import ApiGuide from '#lib/components/ApiGuide.svelte';

	type Row = { user: any; summary: any; can_manage: boolean };

	let rows = $state<Row[]>([]);
	let usageByUser = $state<Record<number, any>>({});
	let loading = $state(true);
	const isSuper = $derived(app.me?.role === 'super_admin');

	async function reload() {
		loading = true;
		try {
			const [res, u] = await Promise.all([api.get('/api/admin/users'), api.get('/api/admin/users/usage?days=7')]);
			rows = res.users || [];
			const map: Record<number, any> = {};
			for (const pu of u.per_user || []) map[pu.user_id] = pu;
			usageByUser = map;
		} catch (e: any) {
			toast(e.message, 'err');
		} finally {
			loading = false;
		}
	}
	$effect(() => { reload(); });

	// ================= filter & urutan =================
	const ROLES = [
		{ k: 'super_admin', label: 'Super admin' },
		{ k: 'admin', label: 'Admin' },
		{ k: 'member', label: 'Member' },
		{ k: 'viewer', label: 'Viewer' }
	];
	const roleLabel = (r: string) => ROLES.find((x) => x.k === r)?.label ?? r;
	let search = $state('');
	let fRole = $state('');
	let fStatus = $state('');
	let sort = $state('recent');
	const SORTS = [
		{ k: 'recent', label: 'Terbaru dibuat' },
		{ k: 'active', label: 'Paling aktif (7 hari)' },
		{ k: 'last', label: 'Terakhir aktif' },
		{ k: 'quota', label: 'Kuota terpakai' },
		{ k: 'email', label: 'Email A–Z' }
	];
	const reqOf = (r: Row) => usageByUser[r.user.id]?.requests ?? 0;
	const roleCount = $derived(rows.reduce((a: Record<string, number>, r) => ((a[r.user.role] = (a[r.user.role] ?? 0) + 1), a), {}));
	const filtered = $derived.by(() => {
		const q = search.trim().toLowerCase();
		const list = rows.filter((r) =>
			(!q || r.user.email.toLowerCase().includes(q)) &&
			(!fRole || r.user.role === fRole || (fRole === 'admin' && r.user.role === 'super_admin')) &&
			(!fStatus || (fStatus === 'active' ? r.user.status === 'active' : r.user.status !== 'active'))
		);
		const by: Record<string, (a: Row, b: Row) => number> = {
			recent: (a, b) => b.user.id - a.user.id,
			active: (a, b) => reqOf(b) - reqOf(a),
			last: (a, b) => (b.summary.last_active ?? '').localeCompare(a.summary.last_active ?? ''),
			quota: (a, b) => (b.summary.quota_pct ?? 0) - (a.summary.quota_pct ?? 0),
			email: (a, b) => a.user.email.localeCompare(b.user.email)
		};
		return [...list].sort(by[sort]);
	});
	const maxReq = $derived(Math.max(1, ...rows.map(reqOf)));
	const totals = $derived.by(() => {
		let req = 0, tok = 0, cost = 0, aktif = 0;
		for (const pu of Object.values(usageByUser) as any[]) {
			req += pu.requests || 0;
			tok += (pu.tokens_in || 0) + (pu.tokens_out || 0);
			cost += pu.cost_usd || 0;
			if ((pu.requests || 0) > 0) aktif++;
		}
		return { req, tok, cost, aktif };
	});
	const nearQuota = $derived(rows.filter((r) => (r.summary.quota_pct ?? 0) >= 90).length);
	const disabled = $derived(rows.filter((r) => r.user.status !== 'active').length);

	// ================= util tampilan =================
	const initials = (email: string) => email.split('@')[0].replace(/[^a-z0-9]/gi, '').slice(0, 2).toUpperCase() || '?';
	const HUES = [218, 199, 262, 160, 28, 340, 188, 45];
	const hue = (s: string) => HUES[[...s].reduce((a, c) => a + c.charCodeAt(0), 0) % HUES.length];
	function ago(ts?: string): string {
		if (!ts) return '';
		const d = new Date(/^\d{4}-\d{2}-\d{2} /.test(ts) ? ts.replace(' ', 'T') + 'Z' : ts);
		const s = (Date.now() - d.getTime()) / 1000;
		if (isNaN(s)) return '';
		if (s < 60) return 'baru saja';
		if (s < 3600) return Math.floor(s / 60) + ' mnt lalu';
		if (s < 86400) return Math.floor(s / 3600) + ' jam lalu';
		if (s < 86400 * 30) return Math.floor(s / 86400) + ' hari lalu';
		return new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'short', year: 'numeric' }).format(d);
	}
	const dateOnly = (ts: string) => {
		const d = new Date(/^\d{4}-\d{2}-\d{2} /.test(ts) ? ts.replace(' ', 'T') + 'Z' : ts);
		return isNaN(d.getTime()) ? ts : new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'short', year: 'numeric' }).format(d);
	};

	// password kuat acak: ≥16 karakter, pasti ada huruf besar, kecil, dan angka
	function genPassword(): string {
		const sets = ['ABCDEFGHJKLMNPQRSTUVWXYZ', 'abcdefghijkmnopqrstuvwxyz', '23456789', '-_.!@#'];
		const all = sets.join('');
		const rnd = (n: number) => crypto.getRandomValues(new Uint32Array(1))[0] % n;
		const chars = sets.map((s) => s[rnd(s.length)]);
		while (chars.length < 16) chars.push(all[rnd(all.length)]);
		for (let i = chars.length - 1; i > 0; i--) { const j = rnd(i + 1); [chars[i], chars[j]] = [chars[j], chars[i]]; }
		return chars.join('');
	}
	const pwRules = (p: string) => [
		{ ok: p.length >= 12, label: 'Minimal 12 karakter' },
		{ ok: /[a-z]/.test(p) && /[A-Z]/.test(p), label: 'Huruf besar & kecil' },
		{ ok: /[0-9]/.test(p), label: 'Angka' }
	];

	// ================= menu aksi per baris =================
	let menuFor = $state<number | null>(null);
	let menuUp = $state(false);
	function openMenu(e: MouseEvent, id: number) {
		e.stopPropagation();
		const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
		menuUp = window.innerHeight - r.bottom < 340;
		menuFor = menuFor === id ? null : id;
	}
	function rowClick(e: MouseEvent, r: Row) {
		if ((e.target as HTMLElement).closest('button, a, input, select')) return;
		goto('/users/' + r.user.id);
	}

	// ================= konfirmasi =================
	let confirmBox = $state<{ title: string; text: string; ok: string; danger?: boolean; run: () => Promise<void> } | null>(null);
	let confirmBusy = $state(false);
	async function doConfirm() {
		if (!confirmBox) return;
		confirmBusy = true;
		try { await confirmBox.run(); confirmBox = null; }
		catch (e: any) { toast(e.message, 'err'); }
		confirmBusy = false;
	}

	async function patchUser(r: Row, body: any, msg: string) {
		await api.patch('/api/admin/users/' + r.user.id, body);
		toast(msg, 'ok');
		reload();
	}
	function askRole(r: Row, role: string) {
		menuFor = null;
		confirmBox = {
			title: 'Ubah peran',
			text: `Ubah peran ${r.user.email} dari ${roleLabel(r.user.role)} menjadi ${roleLabel(role)}?` +
				(role === 'admin' || role === 'super_admin' ? ' Peran admin dapat mengelola gateway dan user lain.' : ''),
			ok: 'Ubah peran',
			run: () => patchUser(r, { role }, `Peran ${r.user.email} kini ${roleLabel(role)} ✓`)
		};
	}
	function askStatus(r: Row) {
		menuFor = null;
		const off = r.user.status === 'active';
		confirmBox = {
			title: off ? 'Nonaktifkan user' : 'Aktifkan user',
			text: off
				? `${r.user.email} tidak bisa masuk dan semua API key-nya berhenti bekerja sampai diaktifkan lagi. Data & riwayatnya tetap tersimpan.`
				: `${r.user.email} bisa masuk dan memakai API key-nya lagi.`,
			ok: off ? 'Nonaktifkan' : 'Aktifkan',
			danger: off,
			run: () => patchUser(r, { status: off ? 'disabled' : 'active' }, off ? 'User dinonaktifkan' : 'User diaktifkan ✓')
		};
	}
	function askDelete(r: Row) {
		menuFor = null;
		confirmBox = {
			title: 'Hapus user',
			text: `Hapus ${r.user.email} secara permanen? Semua API key, sesi, dan riwayat chat-nya ikut terhapus. Tindakan ini tidak bisa dibatalkan.`,
			ok: 'Hapus permanen',
			danger: true,
			run: async () => { await api.del('/api/admin/users/' + r.user.id); toast('User dihapus', 'ok'); reload(); }
		};
	}

	// ================= buat user =================
	let showUser = $state(false);
	let uEmail = $state(''); let uPass = $state(''); let uRole = $state('member'); let uShow = $state(false);
	let uBusy = $state(false);
	let created = $state<{ email: string; pass: string } | null>(null);
	function openCreate() {
		uEmail = ''; uPass = genPassword(); uRole = 'member'; uShow = true; created = null; showUser = true;
	}
	async function createUser() {
		uBusy = true;
		try {
			await api.post('/api/admin/users', { email: uEmail.trim(), password: uPass, role: uRole });
			created = { email: uEmail.trim().toLowerCase(), pass: uPass };
			reload();
		} catch (e: any) { toast(e.message, 'err'); }
		uBusy = false;
	}
	const credText = (email: string, pass: string) =>
		`Akun JenderalRouter\nAlamat: ${browser ? location.origin : ''}\nEmail: ${email}\nPassword: ${pass}\n\nSilakan ganti password setelah masuk.`;
	async function copyCred(email: string, pass: string) {
		(await copyText(credText(email, pass))) ? toast('Kredensial disalin ✓', 'ok') : toast('gagal menyalin', 'err');
	}

	// ================= reset password =================
	let pwFor = $state<Row | null>(null);
	let pwVal = $state(''); let pwShow = $state(true); let pwDone = $state(false); let pwBusy = $state(false);
	function openReset(r: Row) {
		menuFor = null;
		pwFor = r; pwVal = genPassword(); pwShow = true; pwDone = false;
	}
	async function saveReset() {
		if (!pwFor) return;
		pwBusy = true;
		try {
			await api.patch('/api/admin/users/' + pwFor.user.id, { password: pwVal });
			pwDone = true;
			toast('Password diganti — sesi lama user dikeluarkan', 'ok');
		} catch (e: any) { toast(e.message, 'err'); }
		pwBusy = false;
	}

	// ================= API key =================
	let keyFor = $state<any>(null);
	let keys = $state<any[]>([]);
	let keysLoading = $state(false);
	let showKeyForm = $state(false);
	let kName = $state(''); let kAllowed = $state('*'); let kRpm = $state('60'); let kTpm = $state('0');
	let plaintext = $state('');
	const baseURL = (browser ? window.location.origin : '') + '/v1';
	async function openKeys(u: any) {
		menuFor = null;
		keyFor = u; plaintext = ''; kName = ''; kAllowed = '*'; kRpm = '60'; kTpm = '0';
		await loadKeys();
		showKeyForm = !keys.some((k) => !k.revoked_at);
	}
	async function loadKeys() {
		if (!keyFor) return;
		keysLoading = true;
		try { keys = (await api.get(`/api/admin/users/${keyFor.id}/keys`)).keys || []; } catch { keys = []; }
		keysLoading = false;
	}
	async function createKey() {
		if (!keyFor) return;
		try {
			const r = await api.post(`/api/admin/users/${keyFor.id}/keys`, {
				name: kName, allowed_models: kAllowed, rpm: parseInt(kRpm) || 0, tpm: parseInt(kTpm) || 0
			});
			plaintext = r.plaintext;
			loadKeys();
			reload();
		} catch (e: any) { toast(e.message, 'err'); }
	}
	let revealingKey = $state(0);
	async function revealAndCopy(k: any) {
		try {
			revealingKey = k.id;
			const r = await api.get(`/api/admin/keys/${k.id}/reveal`);
			(await copyText(r.plaintext)) ? toast('key disalin ✓', 'ok') : toast('gagal menyalin', 'err');
		} catch (e: any) {
			toast(e.message || 'salinan tidak tersedia — buat ulang key', 'err');
		} finally {
			revealingKey = 0;
		}
	}
	function askDelKey(k: any) {
		confirmBox = {
			title: 'Hapus API key',
			text: `Hapus key ${k.prefix}… (${k.name || 'tanpa nama'})? Aplikasi yang memakai key ini langsung berhenti bekerja.`,
			ok: 'Hapus key',
			danger: true,
			run: async () => { await api.del('/api/admin/keys/' + k.id); await loadKeys(); reload(); }
		};
	}

	// ================= kuota =================
	// Bentuk form satu periode: angka sebagai string agar input kosong = 0 (tanpa batas).
	type QForm = { token: string; req: string; cost: string };
	const PERIODS: { p: 'day' | 'month'; label: string }[] = [{ p: 'day', label: 'Harian' }, { p: 'month', label: 'Bulanan' }];
	const toForm = (l: any): QForm => ({ token: String(l?.token_limit || ''), req: String(l?.request_limit || ''), cost: String(l?.cost_limit_usd || '') });
	const fromForm = (f: QForm) => ({ token_limit: Math.max(0, Math.floor(+f.token || 0)), request_limit: Math.max(0, Math.floor(+f.req || 0)), cost_limit_usd: Math.max(0, +f.cost || 0) });
	const limitText = (l: any) => {
		const parts = [];
		if (l?.token_limit > 0) parts.push(fmtNum(l.token_limit) + ' token');
		if (l?.request_limit > 0) parts.push(fmtNum(l.request_limit) + ' request');
		if (l?.cost_limit_usd > 0) parts.push('$' + l.cost_limit_usd);
		return parts.length ? parts.join(' · ') : 'tanpa batas';
	};

	// kuota default: langsung berlaku untuk semua member & viewer (lama dan baru)
	let showDefault = $state(false);
	let dq = $state<Record<string, QForm>>({ day: toForm(null), month: toForm(null) });
	let dqInfo = $state<any>(null);
	let dqSaving = $state(false);
	async function openDefault() {
		try {
			dqInfo = await api.get('/api/admin/quota-default');
			dq = { day: toForm(dqInfo.default?.day), month: toForm(dqInfo.default?.month) };
			showDefault = true;
		} catch (e: any) { toast(e.message, 'err'); }
	}
	async function saveDefault() {
		dqSaving = true;
		try {
			const r = await api.put('/api/admin/quota-default', { day: fromForm(dq.day), month: fromForm(dq.month) });
			showDefault = false;
			toast(`Kuota default disimpan & diterapkan ke ${r.users_default} user ✓`, 'ok');
			reload();
		} catch (e: any) { toast(e.message, 'err'); }
		dqSaving = false;
	}

	// kuota per user: tiap periode "ikuti default" atau "khusus"
	let qFor = $state<any>(null);
	let qDefault = $state<any>({});
	let qMode = $state<Record<string, 'default' | 'custom'>>({ day: 'default', month: 'default' });
	let qModeOrig: Record<string, 'default' | 'custom'> = {};
	let qf = $state<Record<string, QForm>>({ day: toForm(null), month: toForm(null) });
	async function openQuota(u: any) {
		menuFor = null;
		try {
			const r = await api.get(`/api/admin/users/${u.id}/quota`);
			qDefault = r.default || {};
			for (const { p } of PERIODS) {
				const row = (r.quotas || []).find((q: any) => q.period === p);
				qMode[p] = row?.source === 'custom' ? 'custom' : 'default';
				qModeOrig[p] = qMode[p];
				qf[p] = toForm(row?.source === 'custom' ? row : qDefault[p]);
			}
			qFor = u;
		} catch (e: any) { toast(e.message, 'err'); }
	}
	async function saveQuota() {
		if (!qFor) return;
		try {
			for (const { p } of PERIODS) {
				if (qMode[p] === 'custom') await api.put(`/api/admin/users/${qFor.id}/quota`, { period: p, ...fromForm(qf[p]) });
				else if (qModeOrig[p] === 'custom') await api.del(`/api/admin/users/${qFor.id}/quota?period=${p}`);
			}
			qFor = null;
			toast(t('save') + ' ✓', 'ok');
			reload();
		} catch (e: any) { toast(e.message, 'err'); }
	}

	const ROLE_INFO: Record<string, string> = {
		member: 'Chat & API key untuk dirinya sendiri. Pilihan untuk dosen/mahasiswa.',
		viewer: 'Hanya bisa melihat ringkasan; tidak bisa chat atau memakai API.',
		admin: 'Mengelola provider, model, user biasa, log, dan pengaturan gateway.'
	};
</script>

<svelte:head><title>{t('users')} — JenderalRouter</title></svelte:head>
<svelte:window onclick={() => (menuFor = null)} onkeydown={(e) => e.key === 'Escape' && (menuFor = null)} />

<div class="page-head">
	<div>
		<h1 class="page-title"><Users size={20} /> {t('users')}</h1>
		<div class="subtitle">{fmtNum(rows.length)} pengguna · {fmtNum(totals.aktif)} aktif dalam 7 hari terakhir</div>
	</div>
	<div class="row">
		<button class="btn ghost" onclick={openDefault} title="Kuota yang otomatis berlaku untuk semua member & viewer, lama maupun baru">
			<Gauge size={16} /> Kuota default
		</button>
		<button class="btn" onclick={openCreate}><UserPlus size={16} /> {t('create_user')}</button>
	</div>
</div>

<!-- ---------- ringkasan ---------- -->
<section class="tiles">
	<div class="tile">
		<div class="tl"><Users size={14} /> Total user</div>
		<div class="tv">{fmtNum(rows.length)}</div>
		<div class="ts">
			{#each ROLES as r (r.k)}{#if roleCount[r.k]}<span class="rp {r.k}">{roleCount[r.k]} {r.label.toLowerCase()}</span>{/if}{/each}
		</div>
	</div>
	<div class="tile">
		<div class="tl"><Activity size={14} /> Aktif 7 hari</div>
		<div class="tv">{fmtNum(totals.aktif)} <span class="of">/ {fmtNum(rows.length)}</span></div>
		<div class="tbar"><div style="width:{rows.length ? (totals.aktif / rows.length) * 100 : 0}%"></div></div>
	</div>
	<div class="tile">
		<div class="tl"><ChartLine size={14} /> Request 7 hari</div>
		<div class="tv">{fmtNum(totals.req)}</div>
		<div class="ts muted">{fmtNum(totals.tok)} token · {fmtCost(totals.cost)}</div>
	</div>
	<button class="tile action" class:warn={nearQuota > 0} onclick={() => { sort = 'quota'; fRole = ''; fStatus = ''; }}
		title="Urutkan berdasarkan kuota terpakai">
		<div class="tl"><Gauge size={14} /> Perlu perhatian</div>
		<div class="tv">{fmtNum(nearQuota + disabled)}</div>
		<div class="ts muted">{nearQuota} kuota ≥90% · {disabled} nonaktif</div>
	</button>
</section>

<!-- ---------- daftar ---------- -->
<div class="card list">
	<div class="toolbar">
		<div class="search">
			<Search size={15} />
			<input bind:value={search} placeholder="Cari email…" aria-label="Cari email" />
			{#if search}<button class="clear" onclick={() => (search = '')} aria-label="Hapus pencarian"><X size={14} /></button>{/if}
		</div>
		<div class="seg" role="tablist" aria-label="Filter peran">
			<button class:on={!fRole} onclick={() => (fRole = '')}>Semua <b>{rows.length}</b></button>
			<button class:on={fRole === 'admin'} onclick={() => (fRole = 'admin')}>Admin <b>{(roleCount.admin ?? 0) + (roleCount.super_admin ?? 0)}</b></button>
			<button class:on={fRole === 'member'} onclick={() => (fRole = 'member')}>Member <b>{roleCount.member ?? 0}</b></button>
			<button class:on={fRole === 'viewer'} onclick={() => (fRole = 'viewer')}>Viewer <b>{roleCount.viewer ?? 0}</b></button>
		</div>
		<select bind:value={fStatus} aria-label="Filter status" class="mini">
			<option value="">Semua status</option>
			<option value="active">Aktif</option>
			<option value="disabled">Nonaktif</option>
		</select>
		<label class="sort"><ArrowUpDown size={14} />
			<select bind:value={sort} aria-label="Urutkan" class="mini">
				{#each SORTS as s (s.k)}<option value={s.k}>{s.label}</option>{/each}
			</select>
		</label>
		<button class="icon-btn" onclick={reload} title="Muat ulang" aria-label="Muat ulang"><span class:spin={loading}><RefreshCw size={14} /></span></button>
	</div>

	{#if loading && !rows.length}
		<div class="empty"><span class="spinner dark"></span> {t('loading')}…</div>
	{:else if !filtered.length}
		<div class="empty">
			<Search size={22} />
			<b>Tidak ada user yang cocok</b>
			<span class="muted small">Coba kata kunci lain atau hapus filter.</span>
			<button class="btn ghost sm" onclick={() => { search = ''; fRole = ''; fStatus = ''; }}>Hapus filter</button>
		</div>
	{:else}
		<div class="thead" aria-hidden="true">
			<span>User</span><span>Peran</span><span>Pemakaian 7 hari</span><span>Kuota</span><span>API key</span><span>Terakhir aktif</span><span></span>
		</div>
		<ul class="rows">
			{#each filtered as r (r.user.id)}
				{@const u = r.user}
				{@const s = r.summary}
				{@const uu = usageByUser[u.id]}
				<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
				<li class="row-u" class:off={u.status !== 'active'} onclick={(e) => rowClick(e, r)}>
					<div class="c-user">
						<span class="av" style="--h:{hue(u.email)}">{initials(u.email)}</span>
						<div class="who">
							<a href="/users/{u.id}" class="email">{u.email}</a>
							<div class="meta">
								{#if u.status !== 'active'}<span class="st off"><Power size={11} /> Nonaktif</span>{/if}
								<span>sejak {dateOnly(u.created_at)}</span>
								{#if s.log_content}<span class="lc" title="Isi request API user ini sedang dicatat"><FileSearch size={11} /> isi dicatat</span>{/if}
							</div>
						</div>
					</div>
					<div class="c-role"><span class="rp {u.role}">{roleLabel(u.role)}</span></div>
					<div class="c-use">
						{#if uu?.requests}
							<div class="num"><b>{fmtNum(uu.requests)}</b> <span class="muted">req</span></div>
							<div class="ubar"><div style="width:{(uu.requests / maxReq) * 100}%"></div></div>
							<div class="sub">{fmtNum((uu.tokens_in || 0) + (uu.tokens_out || 0))} token · {fmtCost(uu.cost_usd)}</div>
						{:else}<span class="muted small">belum ada aktivitas</span>{/if}
					</div>
					<div class="c-quota">
						{#if s.quota_label}
							<div class="num"><b class:hot={s.quota_pct >= 90}>{Math.round(s.quota_pct)}%</b>
								<span class="qs {s.quota_source}">{s.quota_source === 'custom' ? 'khusus' : 'standar'}</span></div>
							<div class="qbar"><div class:warn={s.quota_pct >= 80} class:full={s.quota_pct >= 100} style="width:{s.quota_pct}%"></div></div>
							<div class="sub">{s.quota_label}</div>
						{:else}<span class="muted small">tanpa batas</span>{/if}
					</div>
					<div class="c-keys">
						{#if s.keys_active}<span class="kchip"><KeyRound size={12} /> {s.keys_active} aktif</span>
						{:else}<span class="muted small">—</span>{/if}
					</div>
					<div class="c-last">
						{#if s.last_active}<span title={fmtTs(s.last_active)}>{ago(s.last_active)}</span>{:else}<span class="muted small">belum pernah</span>{/if}
					</div>
					<div class="c-act">
						<button class="kebab" onclick={(e) => openMenu(e, u.id)} aria-label="Aksi untuk {u.email}" aria-expanded={menuFor === u.id}>
							<MoreHorizontal size={17} />
						</button>
						{#if menuFor === u.id}
							<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
							<div class="menu" class:up={menuUp} role="menu" tabindex="-1" onclick={(e) => e.stopPropagation()}>
								<a role="menuitem" href="/users/{u.id}"><ChartLine size={14} /> Lihat detail & riwayat</a>
								{#if r.can_manage}
									<button role="menuitem" onclick={() => openQuota(u)}><Gauge size={14} /> Atur kuota</button>
									<button role="menuitem" onclick={() => openKeys(u)}><KeyRound size={14} /> API key</button>
									<div class="sep"></div>
									<div class="mlabel">Ubah peran</div>
									{#each ROLES.filter((x) => x.k !== u.role && (isSuper || x.k === 'member' || x.k === 'viewer')) as ro (ro.k)}
										<button role="menuitem" onclick={() => askRole(r, ro.k)}><UserCog size={14} /> Jadikan {ro.label}</button>
									{/each}
									<div class="sep"></div>
									<button role="menuitem" onclick={() => openReset(r)}><Lock size={14} /> Reset password</button>
									<button role="menuitem" onclick={() => askStatus(r)}>
										{#if u.status === 'active'}<ShieldOff size={14} /> Nonaktifkan{:else}<ShieldCheck size={14} /> Aktifkan{/if}
									</button>
									{#if isSuper}
										<div class="sep"></div>
										<button role="menuitem" class="danger" onclick={() => askDelete(r)}><Trash2 size={14} /> Hapus user</button>
									{/if}
								{:else}
									<div class="mnote"><Lock size={12} /> Hanya super admin yang bisa mengelola {roleLabel(u.role).toLowerCase()}.</div>
								{/if}
							</div>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
		<div class="foot muted small">Menampilkan {filtered.length} dari {rows.length} user · klik baris untuk melihat detail</div>
	{/if}
</div>

<!-- =================== modal =================== -->
{#if confirmBox}
	<Modal title={confirmBox.title} onclose={() => (confirmBox = null)}>
		<p style="margin:0 0 6px;line-height:1.6">{confirmBox.text}</p>
		<div class="modal-actions">
			<button class="btn ghost" onclick={() => (confirmBox = null)}>{t('cancel')}</button>
			<button class="btn" class:danger={confirmBox.danger} onclick={doConfirm} disabled={confirmBusy}>
				{#if confirmBusy}<span class="spinner"></span>{/if} {confirmBox.ok}
			</button>
		</div>
	</Modal>
{/if}

{#if showUser}
	<Modal title={created ? 'User berhasil dibuat' : t('create_user')} onclose={() => (showUser = false)}>
		{#if created}
			<div class="done"><Check size={18} /> Akun <b>{created.email}</b> siap dipakai.</div>
			<p class="muted small" style="margin:10px 0 6px">Bagikan kredensial ini ke pemiliknya secara pribadi. Password tidak bisa dilihat lagi setelah jendela ini ditutup.</p>
			<pre class="cred">{credText(created.email, created.pass)}</pre>
			<div class="modal-actions">
				<button class="btn ghost" onclick={() => copyCred(created!.email, created!.pass)}><Copy size={14} /> Salin kredensial</button>
				<button class="btn ghost" onclick={openCreate}><Plus size={14} /> Buat lagi</button>
				<button class="btn" onclick={() => (showUser = false)}>Selesai</button>
			</div>
		{:else}
			<form onsubmit={(e) => { e.preventDefault(); createUser(); }}>
				<label for="f-u-email">{t('email')}</label>
				<input id="f-u-email" type="email" bind:value={uEmail} placeholder="nama@unsoed.ac.id" required autocomplete="off" />
				<label for="f-u-pass">{t('password')}</label>
				<div class="pw">
					<input id="f-u-pass" type={uShow ? 'text' : 'password'} bind:value={uPass} autocomplete="new-password" required class="mono" />
					<button type="button" class="icon-btn" onclick={() => (uShow = !uShow)} title={uShow ? 'Sembunyikan' : 'Tampilkan'}>
						{#if uShow}<EyeOff size={14} />{:else}<Eye size={14} />{/if}
					</button>
					<button type="button" class="icon-btn" onclick={() => (uPass = genPassword())} title="Buat password acak yang kuat"><Wand2 size={14} /></button>
				</div>
				<ul class="rules">{#each pwRules(uPass) as rr (rr.label)}<li class:ok={rr.ok}><Check size={11} />{rr.label}</li>{/each}</ul>
				<span class="lbl-r">{t('role')}</span>
				<div class="roles" role="radiogroup" aria-label="Peran">
					{#each ['member', 'viewer', 'admin'] as ro (ro)}
						{#if ro !== 'admin' || isSuper}
							<label class="rc" class:on={uRole === ro}>
								<input type="radio" bind:group={uRole} value={ro} />
								<span><b>{roleLabel(ro)}</b><small>{ROLE_INFO[ro]}</small></span>
							</label>
						{/if}
					{/each}
				</div>
				<p class="muted small" style="margin:10px 0 0">Kuota default (bila diatur) langsung berlaku untuk member & viewer baru.</p>
				<div class="modal-actions">
					<button type="button" class="btn ghost" onclick={() => (showUser = false)}>{t('cancel')}</button>
					<button class="btn" type="submit" disabled={uBusy || !pwRules(uPass).every((x) => x.ok)}>
						{#if uBusy}<span class="spinner"></span>{:else}<UserPlus size={14} />{/if} {t('create_user')}
					</button>
				</div>
			</form>
		{/if}
	</Modal>
{/if}

{#if pwFor}
	<Modal title={`Reset password — ${pwFor.user.email}`} onclose={() => (pwFor = null)}>
		{#if pwDone}
			<div class="done"><Check size={18} /> Password diganti. Semua sesi lama user ini sudah dikeluarkan.</div>
			<pre class="cred">{credText(pwFor.user.email, pwVal)}</pre>
			<div class="modal-actions">
				<button class="btn ghost" onclick={() => copyCred(pwFor!.user.email, pwVal)}><Copy size={14} /> Salin kredensial</button>
				<button class="btn" onclick={() => (pwFor = null)}>Selesai</button>
			</div>
		{:else}
			<p class="muted small" style="margin:0 0 6px">Password baru langsung berlaku dan user dikeluarkan dari semua perangkat.</p>
			<label for="f-pw">Password baru</label>
			<div class="pw">
				<input id="f-pw" type={pwShow ? 'text' : 'password'} bind:value={pwVal} class="mono" autocomplete="new-password" />
				<button type="button" class="icon-btn" onclick={() => (pwShow = !pwShow)} title={pwShow ? 'Sembunyikan' : 'Tampilkan'}>
					{#if pwShow}<EyeOff size={14} />{:else}<Eye size={14} />{/if}
				</button>
				<button type="button" class="icon-btn" onclick={() => (pwVal = genPassword())} title="Buat password acak yang kuat"><Wand2 size={14} /></button>
			</div>
			<ul class="rules">{#each pwRules(pwVal) as rr (rr.label)}<li class:ok={rr.ok}><Check size={11} />{rr.label}</li>{/each}</ul>
			<div class="modal-actions">
				<button class="btn ghost" onclick={() => (pwFor = null)}>{t('cancel')}</button>
				<button class="btn" onclick={saveReset} disabled={pwBusy || !pwRules(pwVal).every((x) => x.ok)}>
					{#if pwBusy}<span class="spinner"></span>{:else}<Lock size={14} />{/if} Ganti password
				</button>
			</div>
		{/if}
	</Modal>
{/if}

{#if keyFor}
	<Modal wide={!!plaintext} title={`${t('api_key')} — ${keyFor.email}`} onclose={() => (keyFor = null)}>
		{#if plaintext}
			<div class="warnbox">⚠ API key hanya tampil sekali — salin sekarang sebelum menutup!</div>
			<div class="kv">
				<code class="mono plain">{plaintext}</code>
				<button class="btn sm" onclick={async () => ((await copyText(plaintext)) ? toast('disalin ✓', 'ok') : toast('gagal menyalin', 'err'))}><Copy size={13} /> Salin key</button>
			</div>
			<div style="border-top:1px solid var(--border);margin:16px 0 12px"></div>
			<div class="small" style="font-weight:700;margin-bottom:10px">Cara menghubungkan aplikasi — bagikan ke {keyFor.email}</div>
			<ApiGuide keyPrefix={plaintext.slice(0, 8)} getKey={() => Promise.resolve(plaintext)} />
			<div class="modal-actions"><button class="btn" onclick={() => (keyFor = null)}>Selesai</button></div>
		{:else}
			{#if keysLoading}
				<span class="spinner dark"></span>
			{:else if keys.length}
				<ul class="klist">
					{#each keys as k (k.id)}
						<li class:revoked={k.revoked_at}>
							<span class="mono">{k.prefix}…</span>
							<span class="kname">{k.name || 'tanpa nama'}<small>dibuat {dateOnly(k.created_at)} · rpm {k.rpm || '∞'} · tpm {k.tpm || '∞'}</small></span>
							{#if k.revoked_at}<span class="badge err">dicabut</span>{:else}<span class="badge ok"><span class="dot"></span>aktif</span>{/if}
							{#if !k.revoked_at && k.has_secret}
								<button class="icon-btn" title="Salin API key lengkap" onclick={() => revealAndCopy(k)}>
									{#if revealingKey === k.id}<span class="spinner dark"></span>{:else}<Copy size={13} />{/if}
								</button>
							{/if}
							<button class="icon-btn" title="Hapus key" onclick={() => askDelKey(k)}><Trash2 size={13} /></button>
						</li>
					{/each}
				</ul>
			{:else}
				<p class="muted small" style="margin:0 0 8px">User ini belum punya API key.</p>
			{/if}

			{#if showKeyForm}
				<div class="kform">
					<div class="small" style="font-weight:700">Buat API key baru</div>
					<label for="f-k-name">{t('name')}</label>
					<input id="f-k-name" bind:value={kName} placeholder="kunci {keyFor.email.split('@')[0]}" />
					<label for="f-k-allowed">Model diizinkan</label>
					<input id="f-k-allowed" bind:value={kAllowed} placeholder="* (semua) atau local/Qwen3-30B-A3B-Q6_K" />
					<p class="muted small" style="margin:4px 0 0">format <span class="mono">prefix/model</span>, pisah dengan koma — <span class="mono">*</span> = semua model</p>
					<div class="row">
						<div class="grow"><label for="f-k-rpm">RPM (request/menit)</label><input id="f-k-rpm" bind:value={kRpm} placeholder="60" /></div>
						<div class="grow"><label for="f-k-tpm">TPM (0 = tanpa batas)</label><input id="f-k-tpm" bind:value={kTpm} placeholder="0" /></div>
					</div>
					<div class="muted small" style="margin-top:8px">Base URL: <code class="mono">{baseURL}</code> — panduan koneksi tampil setelah key dibuat.</div>
				</div>
			{/if}
			<div class="modal-actions">
				<button class="btn ghost" onclick={() => (keyFor = null)}>Tutup</button>
				{#if showKeyForm}
					<button class="btn" onclick={createKey}><KeyRound size={14} /> Buat API key</button>
				{:else}
					<button class="btn" onclick={() => (showKeyForm = true)}><Plus size={14} /> Key baru</button>
				{/if}
			</div>
		{/if}
	</Modal>
{/if}

{#snippet quotaFields(f: QForm, id: string)}
	<div class="qfields">
		<label for="{id}-tok">Token<input id="{id}-tok" type="number" min="0" bind:value={f.token} placeholder="tanpa batas" /></label>
		<label for="{id}-req">Request<input id="{id}-req" type="number" min="0" bind:value={f.req} placeholder="tanpa batas" /></label>
		<label for="{id}-cost">Biaya (USD)<input id="{id}-cost" type="number" min="0" step="0.01" bind:value={f.cost} placeholder="tanpa batas" /></label>
	</div>
{/snippet}

{#if showDefault}
	<Modal title="Kuota default" onclose={() => (showDefault = false)}>
		<p class="muted small" style="margin:0 0 10px">
			Berlaku otomatis untuk <b>semua member &amp; viewer</b>, termasuk user yang dibuat nanti. Admin tidak dikenai.
			User dengan kuota khusus tetap memakai kuota khususnya. Kosongkan semua kolom satu periode untuk menghapusnya.
			Pemakaian berjalan tidak direset saat batas diubah.
		</p>
		{#if dqInfo}
			<p class="small" style="margin:0 0 10px">
				Saat ini: <b>{dqInfo.users_default}</b> user memakai kuota default ·
				<b>{dqInfo.users_custom}</b> user punya kuota khusus.
			</p>
		{/if}
		{#each PERIODS as { p, label } (p)}
			<h3 class="qhead">{label} <span class="muted small">(reset tiap {p === 'day' ? 'tengah malam' : 'awal bulan'} WIB)</span></h3>
			{@render quotaFields(dq[p], 'dq-' + p)}
		{/each}
		<div class="modal-actions">
			<button class="btn ghost" onclick={() => (showDefault = false)}>{t('cancel')}</button>
			<button class="btn" onclick={saveDefault} disabled={dqSaving}>
				{#if dqSaving}<span class="spinner"></span>{/if} Simpan &amp; terapkan ke semua user
			</button>
		</div>
	</Modal>
{/if}

{#if qFor}
	<Modal title={`${t('quota')} — ${qFor.email}`} onclose={() => (qFor = null)}>
		{#if qFor.role !== 'member' && qFor.role !== 'viewer'}
			<p class="muted small" style="margin:0 0 10px">Role {qFor.role} tidak dikenai kuota default — "ikuti default" berarti tanpa batas.</p>
		{/if}
		{#each PERIODS as { p, label } (p)}
			<h3 class="qhead">{label}</h3>
			<div class="row small" style="margin-bottom:6px">
				<label class="qmode"><input type="radio" bind:group={qMode[p]} value="default" /> Ikuti default <span class="muted">({limitText(qDefault[p])})</span></label>
				<label class="qmode"><input type="radio" bind:group={qMode[p]} value="custom" /> Khusus untuk user ini</label>
			</div>
			{#if qMode[p] === 'custom'}{@render quotaFields(qf[p], 'qf-' + p)}{/if}
		{/each}
		<div class="modal-actions">
			<button class="btn ghost" onclick={() => (qFor = null)}>{t('cancel')}</button>
			<button class="btn" onclick={saveQuota}>{t('save')}</button>
		</div>
	</Modal>
{/if}

<style>
	/* ---------- ringkasan ---------- */
	.tiles { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; margin-bottom: 14px; }
	.tile {
		all: unset; box-sizing: border-box; display: flex; flex-direction: column; gap: 6px; padding: 14px 16px; border-radius: 14px;
		background: var(--surface); border: 1px solid var(--border); min-width: 0;
	}
	.tile.action { cursor: pointer; transition: border-color var(--speed), transform var(--speed); }
	.tile.action:hover { border-color: var(--accent); transform: translateY(-1px); }
	.tile.action:focus-visible { outline: 2px solid var(--accent); }
	.tile.warn { border-color: color-mix(in srgb, var(--warn) 45%, var(--border)); }
	.tile.warn .tv { color: var(--warn); }
	.tl { display: flex; align-items: center; gap: 7px; font-size: 12px; font-weight: 650; color: var(--muted); }
	.tl :global(svg) { color: var(--accent); }
	.tv { font-size: 26px; font-weight: 800; letter-spacing: -0.03em; font-variant-numeric: tabular-nums; }
	.of { font-size: 14px; font-weight: 600; color: var(--muted); }
	.ts { display: flex; flex-wrap: wrap; gap: 5px; font-size: 12px; }
	.tbar { height: 5px; border-radius: 999px; background: var(--surface-3); overflow: hidden; margin-top: 4px; }
	.tbar div { height: 100%; background: var(--grad-accent); border-radius: 999px; }

	/* ---------- pill peran ---------- */
	.rp { display: inline-block; padding: 2px 9px; border-radius: 999px; font-size: 11.5px; font-weight: 700; white-space: nowrap; }
	.rp.member { color: var(--accent-2); background: color-mix(in srgb, var(--accent-2) 13%, transparent); }
	.rp.viewer { color: var(--muted); background: var(--surface-3); }
	.rp.admin { color: var(--warn); background: color-mix(in srgb, var(--warn) 14%, transparent); }
	.rp.super_admin { color: var(--err); background: color-mix(in srgb, var(--err) 13%, transparent); }

	/* ---------- toolbar ---------- */
	.list { padding: 0; overflow: visible; }
	.toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; padding: 14px 16px; border-bottom: 1px solid var(--border); }
	.search {
		position: relative; flex: 1 1 220px; display: flex; align-items: center; min-width: 180px;
		border: 1px solid var(--border-strong); border-radius: 10px; background: var(--surface);
	}
	.search:focus-within { border-color: var(--accent); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 15%, transparent); }
	.search > :global(svg) { position: absolute; left: 11px; color: var(--muted); }
	.search input { all: unset; box-sizing: border-box; width: 100%; height: 36px; padding: 0 34px 0 34px; font-size: 13.5px; }
	.clear { all: unset; position: absolute; right: 6px; width: 24px; height: 24px; border-radius: 6px; display: grid; place-items: center; cursor: pointer; color: var(--muted); }
	.clear:hover { background: var(--surface-2); color: var(--text); }
	.seg { display: inline-flex; padding: 3px; border-radius: 10px; background: var(--surface-2); border: 1px solid var(--border); }
	.seg button { all: unset; cursor: pointer; padding: 5px 10px; border-radius: 7px; font-size: 12.5px; font-weight: 650; color: var(--muted); white-space: nowrap; }
	.seg button b { font-family: var(--mono); font-size: 11px; margin-left: 3px; opacity: .75; }
	.seg button:hover { color: var(--text); }
	.seg button.on { background: var(--surface); color: var(--text); box-shadow: var(--shadow-sm); }
	.seg button:focus-visible { outline: 2px solid var(--accent); }
	.mini { width: auto; height: 36px; padding: 0 10px; font-size: 12.5px; margin: 0; }
	.sort { display: inline-flex; align-items: center; gap: 6px; color: var(--muted); margin: 0; }
	.spin { display: grid; animation: spin .8s linear infinite; }
	@keyframes spin { to { transform: rotate(360deg); } }

	/* ---------- tabel ---------- */
	.thead, .row-u {
		display: grid; align-items: center; gap: 14px; padding: 0 16px;
		grid-template-columns: minmax(220px, 2.2fr) 110px minmax(130px, 1.2fr) minmax(130px, 1.2fr) 90px 110px 40px;
	}
	.thead { height: 38px; font-size: 11px; font-weight: 700; letter-spacing: .06em; text-transform: uppercase; color: var(--muted); background: var(--surface-2); }
	.rows { list-style: none; margin: 0; padding: 0; }
	.row-u { min-height: 66px; padding-top: 10px; padding-bottom: 10px; border-top: 1px solid var(--border); cursor: pointer; transition: background var(--speed); }
	.row-u:first-child { border-top: 0; }
	.row-u:hover { background: color-mix(in srgb, var(--accent) 5%, transparent); }
	.row-u.off .c-user, .row-u.off .c-use { opacity: .6; }
	.c-user { display: flex; align-items: center; gap: 11px; min-width: 0; }
	.av {
		flex: 0 0 auto; width: 36px; height: 36px; border-radius: 50%; display: grid; place-items: center;
		font-weight: 800; font-size: 12.5px; color: #fff;
		background: linear-gradient(135deg, hsl(var(--h) 75% 58%), hsl(calc(var(--h) + 30) 80% 48%));
	}
	.who { min-width: 0; }
	.email { display: block; font-weight: 700; font-size: 13.5px; color: var(--text); text-decoration: none; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.email:hover { color: var(--accent-2); }
	.meta { display: flex; flex-wrap: wrap; gap: 4px 10px; font-size: 11.5px; color: var(--muted); margin-top: 2px; }
	.st.off { display: inline-flex; align-items: center; gap: 3px; color: var(--err); font-weight: 700; }
	.lc { display: inline-flex; align-items: center; gap: 3px; color: var(--warn); }
	.num { font-size: 13px; font-variant-numeric: tabular-nums; display: flex; align-items: center; gap: 6px; }
	.num b.hot { color: var(--err); }
	.sub { font-size: 11.5px; color: var(--muted); margin-top: 3px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.ubar, .qbar { height: 5px; border-radius: 999px; background: var(--surface-3); overflow: hidden; margin-top: 5px; }
	.ubar div { height: 100%; border-radius: 999px; background: var(--grad-accent); }
	.qbar div { height: 100%; border-radius: 999px; background: var(--ok); }
	.qbar div.warn { background: var(--warn); }
	.qbar div.full { background: var(--err); }
	.qs { font-size: 10.5px; font-weight: 700; padding: 1px 6px; border-radius: 999px; color: var(--ok); background: color-mix(in srgb, var(--ok) 13%, transparent); }
	.qs.custom { color: var(--accent-2); background: color-mix(in srgb, var(--accent-2) 13%, transparent); }
	.kchip { display: inline-flex; align-items: center; gap: 5px; font-size: 12px; font-weight: 650; }
	.kchip :global(svg) { color: var(--accent); }
	.c-last { font-size: 12.5px; }
	.c-act { position: relative; justify-self: end; }
	.kebab {
		all: unset; cursor: pointer; width: 32px; height: 32px; border-radius: 9px; display: grid; place-items: center; color: var(--muted);
	}
	.kebab:hover, .kebab[aria-expanded='true'] { background: var(--surface-2); color: var(--text); }
	.kebab:focus-visible { outline: 2px solid var(--accent); }
	.menu {
		position: absolute; right: 0; top: calc(100% + 6px); z-index: 30; min-width: 230px; padding: 6px; cursor: default;
		background: var(--surface); border: 1px solid var(--border-strong); border-radius: 12px; box-shadow: var(--shadow);
		animation: pop .12s ease;
	}
	.menu.up { top: auto; bottom: calc(100% + 6px); }
	@keyframes pop { from { opacity: 0; transform: translateY(-4px); } }
	.menu a, .menu button {
		all: unset; box-sizing: border-box; width: 100%; display: flex; align-items: center; gap: 9px; padding: 8px 10px;
		border-radius: 8px; font-size: 13px; cursor: pointer; color: var(--text);
	}
	.menu a:hover, .menu button:hover, .menu a:focus-visible, .menu button:focus-visible { background: var(--surface-2); }
	.menu :global(svg) { color: var(--muted); }
	.menu .danger { color: var(--err); }
	.menu .danger :global(svg) { color: var(--err); }
	.sep { height: 1px; background: var(--border); margin: 5px 4px; }
	.mlabel { font-size: 10.5px; font-weight: 700; letter-spacing: .08em; text-transform: uppercase; color: var(--muted); padding: 4px 10px 2px; }
	.mnote { display: flex; gap: 6px; align-items: center; font-size: 12px; color: var(--muted); padding: 8px 10px; }
	.foot { padding: 12px 16px; border-top: 1px solid var(--border); }
	.empty { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 40px 16px; color: var(--muted); text-align: center; }
	.empty b { color: var(--text); }

	/* ---------- modal ---------- */
	.done { display: flex; align-items: center; gap: 8px; color: var(--ok); font-size: 14px; }
	.cred {
		margin: 6px 0 0; padding: 12px; border-radius: 10px; font-family: var(--mono); font-size: 12.5px; white-space: pre-wrap;
		background: var(--surface-2); border: 1px solid var(--border);
	}
	.pw { display: flex; gap: 6px; align-items: center; }
	.pw input { flex: 1; margin: 0; }
	.pw .icon-btn { width: 36px; height: 36px; flex: 0 0 auto; }
	.rules { list-style: none; padding: 0; margin: 8px 0 0; display: flex; flex-wrap: wrap; gap: 4px 12px; }
	.rules li { display: inline-flex; align-items: center; gap: 4px; font-size: 11.5px; color: var(--muted); }
	.rules li.ok { color: var(--ok); }
	.lbl-r { display: block; font-size: 12.5px; font-weight: 650; margin: 14px 0 6px; }
	.roles { display: grid; gap: 6px; }
	.rc {
		display: flex; gap: 10px; align-items: flex-start; padding: 10px 12px; margin: 0; border-radius: 11px; cursor: pointer;
		border: 1px solid var(--border); transition: border-color var(--speed), background var(--speed);
	}
	.rc input { width: auto; margin: 3px 0 0; accent-color: var(--accent); }
	.rc span { display: flex; flex-direction: column; gap: 2px; }
	.rc small { color: var(--muted); font-size: 12px; }
	.rc.on { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 8%, transparent); }
	.warnbox {
		padding: 10px 12px; border-radius: 10px; margin-bottom: 10px; font-size: 12.5px; font-weight: 700; color: var(--warn);
		background: color-mix(in srgb, var(--warn) 12%, transparent); border: 1px solid color-mix(in srgb, var(--warn) 40%, transparent);
	}
	.plain { background: var(--bg); padding: 10px; border-radius: 8px; word-break: break-all; flex: 1; }
	.klist { list-style: none; margin: 0 0 6px; padding: 0; }
	.klist li { display: flex; align-items: center; gap: 10px; padding: 9px 0; border-top: 1px solid var(--border); font-size: 13px; }
	.klist li:first-child { border-top: 0; }
	.klist li.revoked { opacity: .55; }
	.kname { flex: 1; min-width: 0; display: flex; flex-direction: column; }
	.kname small { color: var(--muted); font-size: 11.5px; }
	.kform { margin-top: 10px; padding: 12px; border-radius: 12px; border: 1px dashed var(--border-strong); }
	.qhead { font-size: 13.5px; margin: 14px 0 6px; }
	.qfields { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; }
	.qfields label { font-size: 12px; color: var(--muted); display: flex; flex-direction: column; gap: 4px; margin: 0; }
	.qmode { display: inline-flex; align-items: center; gap: 6px; cursor: pointer; margin: 0 12px 0 0; }
	.qmode input { width: auto; margin: 0; }

	/* ---------- responsif ---------- */
	@media (max-width: 1200px) {
		.thead, .row-u { grid-template-columns: minmax(200px, 2fr) 100px minmax(120px, 1.2fr) minmax(120px, 1.2fr) 100px 40px; }
		.thead span:nth-child(5), .c-keys { display: none; }
	}
	@media (max-width: 900px) {
		.tiles { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.thead { display: none; }
		.row-u {
			grid-template-columns: 1fr auto; grid-template-areas: 'user act' 'role role' 'use quota';
			gap: 8px 14px; padding: 14px 16px;
		}
		.c-user { grid-area: user; }
		.c-act { grid-area: act; }
		.c-role { grid-area: role; }
		.c-use { grid-area: use; }
		.c-quota { grid-area: quota; }
		.c-keys, .c-last { display: none; }
		.seg { order: 3; width: 100%; overflow-x: auto; }
		.search { flex-basis: 100%; }
		.toolbar > .mini, .sort { flex: 1 1 0; min-width: 0; }
		.sort select { flex: 1; min-width: 0; }
		.seg button { flex: 1; text-align: center; }
	}
	@media (max-width: 560px) {
		.qfields { grid-template-columns: 1fr; }
		.tv { font-size: 22px; }
	}
</style>
