<script lang="ts">
	import { UserRound, ArrowLeft, Activity, Coins, Gauge, AlertTriangle, Timer, KeyRound, RefreshCw, MessagesSquare, ShieldAlert, FileSearch } from '@lucide/svelte';
	import { page } from '$app/state';
	import { api, fmtNum, fmtCost, fmtTs, md } from '#lib/api.js';
	import { t, toast } from '#lib/stores.svelte.js';
	import Stat from '#lib/components/Stat.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import LogContentModal from '#lib/components/LogContentModal.svelte';

	const RANGES = [1, 7, 30, 90];

	let days = $state(30);
	let res = $state<any>(null);
	let loading = $state(true);

	async function reload() {
		loading = true;
		try {
			res = await api.get(`/api/admin/users/${page.params.id}/usage?days=${days}`);
		} catch (e: any) {
			toast(e.message, 'err');
			res = null;
		} finally {
			loading = false;
		}
	}
	$effect(() => { void days; void page.params.id; reload(); });

	// ---- riwayat chat (setiap akses tercatat di audit log) ----
	let convs = $state<any[] | null>(null);
	let convErr = $state('');
	let convSearch = $state('');
	let openConv = $state<any>(null);
	let convLoading = $state(0);
	$effect(() => {
		const id = page.params.id;
		convs = null;
		convErr = '';
		api.get(`/api/admin/users/${id}/conversations`)
			.then((r) => (convs = r.conversations || []))
			.catch((e) => (convErr = e.message));
	});
	const convFiltered = $derived(
		(convs || []).filter((c) => !convSearch || c.title.toLowerCase().includes(convSearch.toLowerCase()))
	);
	async function viewConv(c: any) {
		convLoading = c.id;
		try { openConv = await api.get(`/api/admin/users/${page.params.id}/conversations/${c.id}`); }
		catch (e: any) { toast(e.message, 'err'); }
		convLoading = 0;
	}

	const u = $derived(res?.user);
	const us = $derived(res?.usage);
	const maxReq = $derived(Math.max(1, ...((us?.per_day || []) as any[]).map((d) => d.requests)));
	const keyName = (id: string | number) => {
		const k = (res?.keys || []).find((x: any) => String(x.id) === String(id));
		return k ? `${k.prefix}… ${k.name}` : id ? `#${id}` : '-';
	};
	// ---- pencatatan isi request API (opt-in, disimpan 7 hari) ----
	let openLog = $state<string | null>(null);
	let toggling = $state('');
	async function setUserLogContent(on: boolean) {
		toggling = 'user';
		try {
			await api.put(`/api/admin/users/${page.params.id}/log-content`, { enabled: on });
			res.log_content = on;
			toast(on ? 'Pencatatan isi request user dinyalakan' : 'Pencatatan isi request user dimatikan', 'ok');
		} catch (e: any) { toast(e.message, 'err'); }
		toggling = '';
	}
	async function setKeyLogContent(k: any, on: boolean) {
		toggling = 'key' + k.id;
		try {
			await api.put(`/api/admin/keys/${k.id}/log-content`, { enabled: on });
			k.log_content = on;
		} catch (e: any) { toast(e.message, 'err'); }
		toggling = '';
	}

	const pct = (used: number, limit: number) => (limit > 0 ? Math.min(100, (used / limit) * 100) : 0);
	const statusClass = (s: string) => (+s >= 200 && +s < 400 ? 'ok' : +s >= 500 || +s === 0 ? 'err' : 'warn');
</script>

<svelte:head><title>{u ? u.email : t('users')} — JenderalRouter</title></svelte:head>

<div class="page-head">
	<h1 class="page-title"><UserRound size={20} /> {u ? u.email : 'Detail user'}</h1>
	<div class="row">
		<a class="btn ghost sm" href="/users"><ArrowLeft size={13} /> {t('users')}</a>
		<button class="btn ghost sm" onclick={reload}><RefreshCw size={13} /> Muat ulang</button>
	</div>
</div>

{#if u}
	<div class="kv" style="margin-bottom:12px">
		<span class="badge {u.role === 'super_admin' ? 'err' : 'info'}">{u.role}</span>
		{#if u.status === 'active'}<span class="badge ok"><span class="dot"></span>active</span>
		{:else}<span class="badge warn">{u.status}</span>{/if}
		<span class="muted small">dibuat {fmtTs(u.created_at)}</span>
		<span class="grow"></span>
		<span class="muted small">Rentang:</span>
		{#each RANGES as r (r)}
			<button class="btn sm" class:ghost={days !== r} onclick={() => (days = r)}>{r} hari</button>
		{/each}
	</div>
{/if}

{#if loading && !res}
	<div class="card"><span class="spinner dark"></span> {t('loading')}…</div>
{:else if res}
	<div class="grid cols-4">
		<Stat icon={Activity} label="Request ({days} hari)" value={fmtNum(us.totals.requests)} />
		<Stat icon={Coins} label="Token ({days} hari)" value={fmtNum(us.totals.tokens_in + us.totals.tokens_out)}
			sub="{fmtNum(us.totals.tokens_in)} in · {fmtNum(us.totals.tokens_out)} out" />
		<Stat icon={Coins} label="Biaya ({days} hari)" value={fmtCost(us.totals.cost_usd)} />
		<Stat icon={AlertTriangle} label={t('error_rate')} value={(us.error_rate * 100).toFixed(1) + '%'}
			sub="{fmtNum(us.totals.errors)} error" />
	</div>
	<div class="grid cols-4" style="margin-top:14px">
		<Stat icon={Timer} label="p50 {t('latency')}" value={(us.totals.p50_latency_ms || 0) + ' ms'} />
		<Stat icon={Timer} label="p95 {t('latency')}" value={(us.totals.p95_latency_ms || 0) + ' ms'} />
		<Stat icon={KeyRound} label="API key aktif" value={String((res.keys || []).filter((k: any) => !k.revoked_at).length)}
			sub="{(res.keys || []).length} total" />
		<Stat icon={Activity} label="Model dipakai" value={String(us.per_model.length)} />
	</div>

	<div class="card" style="margin-top:14px">
		<h2>Request {t('per_day')}</h2>
		<div class="bar-chart">
			{#each us.per_day as d (d.day)}
				<div class="bar" style="height:{Math.max(4, (d.requests / maxReq) * 100)}%"
					data-tip="{d.day}: {d.requests} req · {fmtNum(d.tokens_in + d.tokens_out)} token · {fmtCost(d.cost_usd)}"></div>
			{:else}
				<span class="muted small">{t('no_data')}</span>
			{/each}
		</div>
	</div>

	<div class="card" style="margin-top:14px">
		<h2><Gauge size={16} /> {t('quota')}</h2>
		{#each res.quotas as q (q.id)}
			<div class="quota">
				<div class="kv small">
					<strong>{q.period === 'day' ? 'Harian' : 'Bulanan'}</strong>
					<span class="grow"></span>
					{#if q.reset_at}<span class="muted">reset {fmtTs(q.reset_at)}</span>{/if}
				</div>
				{#each [
					{ lbl: 'Token', used: q.used_tokens, limit: q.token_limit, fmt: fmtNum },
					{ lbl: 'Request', used: q.used_requests, limit: q.request_limit, fmt: fmtNum },
					{ lbl: 'Biaya', used: q.used_cost, limit: q.cost_limit_usd, fmt: fmtCost }
				] as m (m.lbl)}
					<div class="qrow small">
						<span class="qlbl muted">{m.lbl}</span>
						<div class="meter"><div class="fill" class:hot={pct(m.used, m.limit) >= 90} style="width:{pct(m.used, m.limit)}%"></div></div>
						<span class="mono">{m.fmt(m.used)} / {m.limit > 0 ? m.fmt(m.limit) : '∞'}</span>
					</div>
				{/each}
			</div>
		{:else}
			<div class="muted small">Belum ada kuota — pemakaian tanpa batas. Atur dari halaman {t('users')}.</div>
		{/each}
	</div>

	<div class="card" style="margin-top:14px">
		<h2>Per model</h2>
		<div class="table-wrap">
			<table>
				<thead><tr><th>{t('model')}</th><th>{t('provider')}</th><th>{t('requests')}</th><th>{t('errors')}</th><th>{t('tokens')} in/out</th><th>{t('cost')}</th><th>Rata-rata {t('latency')}</th></tr></thead>
				<tbody>
					{#each us.per_model as m (m.model)}
						<tr>
							<td class="mono">{m.model}</td>
							<td>{m.provider_name}</td>
							<td>{fmtNum(m.requests)}</td>
							<td>{#if m.errors > 0}<span class="badge err">{m.errors}</span>{:else}0{/if}</td>
							<td>{fmtNum(m.tokens_in)} / {fmtNum(m.tokens_out)}</td>
							<td>{fmtCost(m.cost_usd)}</td>
							<td>{Math.round(m.avg_latency_ms)} ms</td>
						</tr>
					{:else}
						<tr><td colspan="7" class="muted">{t('no_data')}</td></tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>

	<div class="card" style="margin-top:14px">
		<h2>Per API key</h2>
		<div class="table-wrap">
			<table>
				<thead><tr><th>{t('api_key')}</th><th>{t('requests')}</th><th>{t('errors')}</th><th>{t('tokens')} in/out</th><th>{t('cost')}</th><th>Terakhir dipakai</th></tr></thead>
				<tbody>
					{#each us.per_key as k (k.key_id)}
						<tr>
							<td>
								{#if k.prefix}<span class="mono">{k.prefix}…</span> <span class="muted small">{k.name}</span>
								{:else if k.key_id}<span class="muted">#{k.key_id} (terhapus)</span>
								{:else}<span class="muted">sesi dashboard</span>{/if}
							</td>
							<td>{fmtNum(k.requests)}</td>
							<td>{#if k.errors > 0}<span class="badge err">{k.errors}</span>{:else}0{/if}</td>
							<td>{fmtNum(k.tokens_in)} / {fmtNum(k.tokens_out)}</td>
							<td>{fmtCost(k.cost_usd)}</td>
							<td class="mono">{fmtTs(k.last_used)}</td>
						</tr>
					{:else}
						<tr><td colspan="6" class="muted">{t('no_data')}</td></tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>

	<div class="card" style="margin-top:14px">
		<h2><FileSearch size={16} /> Pencatatan isi request API</h2>
		<p class="muted small" style="margin:0 0 10px">
			Bila menyala, isi request &amp; jawaban lewat API key dicatat (rahasia yang dikenali disamarkan), bisa dilihat dari
			tabel log, dan dikosongkan otomatis setelah 7 hari. User melihat pemberitahuan di halaman API-nya.
		</p>
		{#if res.log_content_global}
			<div class="badge warn" style="margin-bottom:10px">Saklar global menyala — isi semua request API sedang dicatat.</div>
		{/if}
		<label class="switch-row">
			<input type="checkbox" checked={res.log_content} disabled={toggling === 'user'}
				onchange={(e) => setUserLogContent(e.currentTarget.checked)} />
			<span><b>Semua API key milik user ini</b></span>
		</label>
		{#each (res.keys || []).filter((k: any) => !k.revoked_at) as k (k.id)}
			<label class="switch-row">
				<input type="checkbox" checked={k.log_content} disabled={toggling === 'key' + k.id || res.log_content}
					onchange={(e) => setKeyLogContent(k, e.currentTarget.checked)} />
				<span>Hanya key <span class="mono">{k.prefix}…</span> {k.name}</span>
				{#if res.log_content}<span class="muted small">(tercakup saklar user)</span>{/if}
			</label>
		{/each}
	</div>

	<div class="card" style="margin-top:14px">
		<h2>{t('recent_logs')} <span class="muted small">(50 terakhir)</span></h2>
		<div class="table-wrap">
			<table>
				<thead><tr><th>{t('time')}</th><th>{t('api_key')}</th><th>{t('requested')}</th><th>{t('provider')}</th><th>{t('status')}</th><th>{t('latency')}</th><th>{t('tokens')}</th><th>{t('cost')}</th><th>Isi</th></tr></thead>
				<tbody>
					{#each res.recent_logs as l (l.id)}
						<tr>
							<td class="mono">{fmtTs(l.ts)}</td>
							<td class="mono small">{keyName(l.key_id)}</td>
							<td class="mono">{l.requested_model}</td>
							<td>{l.provider_name}</td>
							<td><span class="badge {statusClass(l.status)}">{l.status}{l.error_code ? ' · ' + l.error_code : ''}</span></td>
							<td>{l.latency_ms} ms</td>
							<td>{l.tokens_in}/{l.tokens_out}</td>
							<td>{fmtCost(l.cost_usd)}</td>
							<td>
								{#if l.has_content}
									<button class="btn ghost sm" onclick={() => (openLog = l.id)} title="Lihat isi request & jawaban"><FileSearch size={13} /></button>
								{/if}
							</td>
						</tr>
					{:else}
						<tr><td colspan="9" class="muted">{t('no_data')}</td></tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>

	<div class="card" style="margin-top:14px">
		<div class="kv" style="margin-bottom:8px">
			<h2 style="margin:0"><MessagesSquare size={16} /> Riwayat chat</h2>
			{#if convs}<span class="muted small">{convs.length} percakapan</span>{/if}
			<span class="grow"></span>
			<input style="max-width:240px" bind:value={convSearch} placeholder="cari judul…" aria-label="cari judul percakapan" />
		</div>
		<p class="muted small" style="margin:0 0 10px"><ShieldAlert size={12} style="vertical-align:-2px" /> Isi chat bersifat pribadi — setiap kali dibuka tercatat di audit log.</p>
		{#if convErr}
			<span class="muted small">{convErr}</span>
		{:else if !convs}
			<span class="spinner dark"></span>
		{:else}
			<div class="table-wrap">
				<table>
					<thead><tr><th>Judul</th><th>{t('model')}</th><th>Pesan</th><th>Dibuat</th><th>Terakhir aktif</th><th></th></tr></thead>
					<tbody>
						{#each convFiltered as c (c.id)}
							<tr>
								<td>{c.title}</td>
								<td class="mono small">{c.model || '—'}</td>
								<td>{c.message_count}</td>
								<td class="mono small">{fmtTs(c.created_at)}</td>
								<td class="mono small">{fmtTs(c.updated_at)}</td>
								<td>
									<button class="btn ghost sm" onclick={() => viewConv(c)} disabled={convLoading === c.id}>
										{#if convLoading === c.id}<span class="spinner dark"></span>{/if} Buka
									</button>
								</td>
							</tr>
						{:else}
							<tr><td colspan="6" class="muted">Belum ada percakapan.</td></tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
{/if}


{#if openConv}
	<Modal wide title={openConv.conversation.title} onclose={() => (openConv = null)}>
		<div class="muted small" style="margin-bottom:10px">
			{u?.email} · <span class="mono">{openConv.conversation.model || '—'}</span> · dibuat {fmtTs(openConv.conversation.created_at)}
		</div>
		<div class="conv">
			{#each openConv.messages as m (m.id)}
				<div class="cmsg {m.role}">
					<div class="muted small">
						<b>{m.role === 'user' ? 'User' : m.role === 'assistant' ? 'AI' : 'System'}</b>
						{#if m.model}· <span class="mono">{m.model}</span>{/if} · {fmtTs(m.created_at)}
					</div>
					{#if m.role === 'assistant'}
						<!-- md() meng-escape HTML sebelum merender markdown -->
						<div class="cbody md">{@html md(m.content)}</div>
					{:else}
						<div class="cbody plain">{m.content}</div>
					{/if}
				</div>
			{:else}
				<span class="muted small">Percakapan kosong.</span>
			{/each}
		</div>
	</Modal>
{/if}
{#if openLog}<LogContentModal logId={openLog} onclose={() => (openLog = null)} />{/if}

<style>
	.switch-row { display: flex; align-items: center; gap: 8px; padding: 6px 0; border-top: 1px solid var(--border); cursor: pointer; font-size: 13.5px; }
	.switch-row:first-of-type { border-top: 0; }
	.switch-row input { width: auto; margin: 0; }
	.quota { padding: 8px 0; border-top: 1px solid var(--border); }
	.quota:first-of-type { border-top: 0; }
	.qrow { display: flex; align-items: center; gap: 10px; margin-top: 6px; }
	.qlbl { width: 64px; }
	.meter { flex: 1; height: 8px; border-radius: 999px; background: var(--surface-2); overflow: hidden; }
	.fill { height: 100%; background: var(--grad-accent); }
	.fill.hot { background: var(--err); }
	.conv { max-height: 65vh; overflow: auto; display: flex; flex-direction: column; gap: 10px; }
	.cmsg { border: 1px solid var(--border); border-radius: 10px; padding: 8px 12px; }
	.cmsg.user { background: var(--surface-2); }
	.cbody { margin-top: 4px; font-size: 13.5px; overflow-wrap: anywhere; }
	.cbody.plain { white-space: pre-wrap; }
</style>
