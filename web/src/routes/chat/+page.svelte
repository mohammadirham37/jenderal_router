<script lang="ts">
	import { Plus, Send, Trash2, X, Bot, Sparkles, History, Globe, Activity, Gauge, Download, FileText, File, Presentation, Sheet } from '@lucide/svelte';
	import { api, md, fmtTs, fmtNum } from '$lib/api';
	import { exportAnswer, exportConversation, type ExportFormat } from '$lib/exporters';
	import { t, toast } from '$lib/stores.svelte';
	import Modal from '$lib/components/Modal.svelte';

	type Msg = {
		role: 'user' | 'assistant' | 'tool'; content: string; provider?: string;
		reasoning?: string; model?: string; ms?: number;
		tools?: { name: string; args: string; url?: string }[];
	};
	type ToolCall = { id: string; name: string; args: string };

	let models = $state<any[]>([]);
	let model = $state('');
	let conversations = $state<any[]>([]);
	let current = $state<number | null>(null);
	let msgs = $state<Msg[]>([]);
	let input = $state('');
	let sending = $state(false);
	let quota = $state<any>(null);
	let msgsEl = $state<HTMLElement | null>(null);
	let showHistory = $state(false);
	let showExport = $state(false);

	// ringkasan kuota utk toolbar: constraint paling menekan yang ditampilkan;
	// limit 0 (atau tanpa baris kuota) = unlimited
	const quotaInfo = $derived.by(() => {
		if (!quota) return null;
		const list: { period: string; unit: 'req' | 'token' | 'usd'; used: number; limit: number; pct: number; left: string }[] = [];
		for (const q of quota.quotas ?? []) {
			const dims: ['req' | 'token' | 'usd', number, number, (v: number) => string][] = [
				['req', q.used_requests ?? 0, q.request_limit ?? 0, (v) => fmtNum(v)],
				['token', q.used_tokens ?? 0, q.token_limit ?? 0, (v) => fmtNum(v)],
				['usd', q.used_cost_usd ?? 0, q.cost_limit_usd ?? 0, (v) => '$' + v.toFixed(2)]
			];
			for (const [unit, used, limit, fmt] of dims) {
				if (limit > 0) {
					const pct = Math.round((used / limit) * 100);
					list.push({ period: q.period, unit, used, limit, pct, left: fmt(Math.max(0, limit - used)) + (unit === 'usd' ? '' : ' ' + unit) });
				}
			}
		}
		list.sort((a, b) => b.pct - a.pct);
		return { list, tightest: list[0] ?? null, unlimited: list.length === 0, totalReq: quota.today?.requests ?? 0 };
	});
	const periodLabel = (p: string) => (p === 'month' ? 'bulanan' : 'harian');

	// tool bawaan: web_fetch — dieksekusi sisi server (guard SSRF aktif)
	const TOOLS = [{
		type: 'function',
		function: {
			name: 'web_fetch',
			description: 'Mengambil isi sebuah halaman web publik. Gunakan saat pengguna menyebutkan URL/link atau membutuhkan informasi terkini dari web.',
			parameters: {
				type: 'object',
				properties: {
					url: { type: 'string', description: 'URL lengkap halaman, mulai dengan http atau https' }
				},
				required: ['url']
			}
		}
	}];
	const MAX_TOOL_ROUNDS = 4;

	$effect(() => {
		(async () => {
			try {
				const [m, q] = await Promise.all([api.get('/api/me/models'), api.get('/api/me/usage')]);
				models = m.models || [];
				model = models[0]?.id ?? '';
				const saved = typeof localStorage !== 'undefined' ? localStorage.getItem('jr_chat_model') : null;
				if (saved && models.some((x) => x.id === saved)) model = saved;
				quota = q;
			} catch (e: any) {
				toast(e.message, 'err');
			}
			loadConversations();
		})();
	});

	// ingat pilihan model antar-kunjungan
	$effect(() => {
		if (model) try { localStorage.setItem('jr_chat_model', model); } catch { /* */ }
	});

	async function loadConversations() {
		try {
			const res = await api.get('/api/me/conversations');
			conversations = res.conversations || [];
			// pulihkan sesi: buka percakapan terakhir bila belum ada yang terbuka
			if (current === null && conversations.length > 0) {
				await openConversation(conversations[0].id);
			}
		} catch { /* */ }
	}
	async function loadQuota() {
		try { quota = await api.get('/api/me/usage'); } catch { /* */ }
	}

	async function newConversation() {
		const res = await api.post('/api/me/conversations', { title: t('new_chat'), model });
		current = res.conversation.id;
		msgs = [];
		showHistory = false;
		loadConversations();
	}
	async function openConversation(id: number) {
		current = id;
		try {
			const res = await api.get('/api/me/conversations/' + id);
			msgs = (res.messages || []).map((m: any) => ({
				role: m.role, content: m.content, provider: m.provider_name,
				model: m.model || undefined
			}));
			showHistory = false;
			scrollBottom();
		} catch (e: any) { toast(e.message, 'err'); }
	}
	async function delConversation(id: number) {
		if (!confirm(t('confirm_delete'))) return;
		await api.del('/api/me/conversations/' + id);
		if (current === id) { current = null; msgs = []; }
		loadConversations();
	}

	function scrollBottom() {
		setTimeout(() => msgsEl?.scrollTo({ top: msgsEl.scrollHeight }), 30);
	}

	// ---- ekspor jawaban ke file (md/docx/pptx/xlsx/csv) ----
	const EXPORTS: { fmt: ExportFormat; label: string; hint: string }[] = [
		{ fmt: 'md', label: 'MD', hint: 'Unduh sebagai Markdown' },
		{ fmt: 'docx', label: 'DOCX', hint: 'Unduh sebagai dokumen Word' },
		{ fmt: 'pptx', label: 'PPTX', hint: 'Unduh sebagai presentasi PowerPoint' },
		{ fmt: 'xlsx', label: 'XLSX', hint: 'Unduh sebagai workbook Excel' },
		{ fmt: 'csv', label: 'CSV', hint: 'Unduh tabel jawaban sebagai CSV' }
	];
	function convTitle(): string {
		const c = conversations.find((x) => x.id === current);
		if (c?.title) return c.title;
		const first = msgs.find((m) => m.role === 'user')?.content;
		return first ? first.slice(0, 60) : 'Jawaban';
	}
	function doExport(m: Msg, fmt: ExportFormat) {
		try {
			exportAnswer(convTitle(), m.content, fmt);
		} catch (e: any) {
			toast('ekspor gagal: ' + e.message, 'err');
		}
	}

	// ---- ekspor seluruh percakapan dari toolbar ----
	const CONV_EXPORTS: { fmt: ExportFormat; label: string; hint: string }[] = [
		{ fmt: 'md', label: 'MD', hint: 'Markdown seluruh percakapan' },
		{ fmt: 'docx', label: 'DOCX', hint: 'Dokumen Word seluruh percakapan' },
		{ fmt: 'pptx', label: 'PPTX', hint: 'Presentasi ringkas percakapan' },
		{ fmt: 'xlsx', label: 'XLSX', hint: 'Excel — transkrip + semua tabel' },
		{ fmt: 'csv', label: 'CSV', hint: 'CSV transkrip (Peran; Pesan)' }
	];
	function doExportConv(fmt: ExportFormat) {
		const turns = msgs
			.filter((m) => m.content && m.role !== 'tool')
			.map((m) => ({ role: m.role, content: m.content, model: m.model }));
		if (!turns.length) return;
		try {
			exportConversation(convTitle(), turns, fmt);
		} catch (e: any) {
			toast('ekspor gagal: ' + e.message, 'err');
		}
		showExport = false;
	}

	async function send() {
		const text = input.trim();
		if (!text || sending) return;
		sending = true;
		input = '';
		// percakapan baru bila belum ada yang terpilih — wajib sebelum push pesan
		if (!current) {
			try {
				await newConversation();
			} catch (e: any) {
				toast('buat percakapan gagal: ' + e.message, 'err');
				sending = false;
				return;
			}
		}
		msgs.push({ role: 'user', content: text });
		msgs = [...msgs];

		// riwayat awal: semua pesan yang sudah ada (tanpa bubble kosong)
		const history: any[] = [];
		for (const m of msgs) {
			if (m.role === 'user') history.push({ role: 'user', content: m.content });
			else if (m.role === 'assistant' && m.content) history.push({ role: 'assistant', content: m.content });
		}

		const started = performance.now();
		let quotaSeen = false;

		try {
			// loop tool-calling: maksimal N ronde web_fetch
			for (let round = 0; round <= MAX_TOOL_ROUNDS; round++) {
				msgs.push({ role: 'assistant', content: '' });
				const bubble = msgs[msgs.length - 1];
				msgs = [...msgs];
				scrollBottom();

				let finish = '';
				const pending: Record<number, ToolCall> = {};

				await api.chatStream(
					{ model, messages: history, tools: TOOLS },
					(delta) => {
						bubble.content += delta;
						msgs = [...msgs];
						scrollBottom();
					},
					() => {
						if (!quotaSeen) { quotaSeen = true; loadQuota(); }
					},
					(extra) => {
						if (extra.reasoning) {
							bubble.reasoning = (bubble.reasoning ?? '') + extra.reasoning;
							msgs = [...msgs];
							scrollBottom();
						}
						if (extra.model) {
							bubble.model = extra.model.split('/').pop()?.replace(/\.gguf$/i, '') || extra.model;
						}
					},
					(chunk) => {
						const d = chunk.choices?.[0]?.delta;
						if (d?.tool_calls) {
							for (const tc of d.tool_calls) {
								const idx = tc.index ?? 0;
								pending[idx] ??= { id: '', name: '', args: '' };
								if (tc.id) pending[idx].id = tc.id;
								if (tc.function?.name) pending[idx].name += tc.function.name;
								if (tc.function?.arguments) pending[idx].args += tc.function.arguments;
							}
							msgs = [...msgs];
						}
						if (d?.finish_reason) finish = d.finish_reason;
					}
				);

				const calls = Object.values(pending).filter((t) => t.name);
				if (finish !== 'tool_calls' || calls.length === 0) break; // jawaban selesai

				// model meminta tool → jalankan di server lalu lanjutkan
				bubble.tools = (bubble.tools ?? []).concat(
					calls.map((t) => {
						let u = '';
						try { u = JSON.parse(t.args || '{}').url || ''; } catch { /* */ }
						return { name: t.name, args: t.args, url: u };
					})
				);
				msgs = [...msgs];
				history.push({
					role: 'assistant',
					content: bubble.content || null,
					tool_calls: calls.map((t) => ({
						id: t.id, type: 'function', function: { name: t.name, arguments: t.args }
					}))
				});

				for (const t of calls) {
					let result = '';
					const meta = bubble.tools?.[bubble.tools.length - 1];
					if (t.name === 'web_fetch') {
						let url = '';
						try { url = JSON.parse(t.args || '{}').url || ''; } catch { /* */ }
						try {
							const r = await api.post('/api/me/tools/webfetch', { url });
							result = 'Judul: ' + (r.title || '-') + '\n' + r.text;
						} catch (e: any) {
							result = 'ERROR: ' + e.message;
						}
						if (meta) meta.url = url;
					} else {
						result = 'ERROR: tool "' + t.name + '" tidak dikenal';
					}
					history.push({ role: 'tool', tool_call_id: t.id, content: result });
				}
				msgs = [...msgs];
				scrollBottom();
			}

			await api.post(`/api/me/conversations/${current}/messages`, { role: 'user', content: text });
			const lastAssistant = [...msgs].reverse().find((m) => m.role === 'assistant');
			await api.post(`/api/me/conversations/${current}/messages`, {
				role: 'assistant', content: lastAssistant?.content || '', model: lastAssistant?.model || model
			});
			loadQuota();
			loadConversations();
		} catch (e: any) {
			const last = msgs[msgs.length - 1];
			if (last && last.role === 'assistant') last.content += '\n\n**⚠ ' + e.message + '**';
			else toast(e.message, 'err');
			// simpan best-effort: user + jawaban parsial agar tidak hilang saat refresh
			if (current) {
				try {
					await api.post(`/api/me/conversations/${current}/messages`, { role: 'user', content: text });
					const la = [...msgs].reverse().find((m) => m.role === 'assistant');
					await api.post(`/api/me/conversations/${current}/messages`, {
						role: 'assistant', content: la?.content || '', model: la?.model || model
					});
				} catch { /* */ }
			}
			msgs = [...msgs];
		} finally {
			const dur = Math.round((performance.now() - started) / 100) / 10;
			for (const m of msgs) if (m.role === 'assistant' && !m.ms) m.ms = dur;
			msgs = [...msgs];
			sending = false;
			scrollBottom();
		}
	}
</script>

<svelte:head><title>{t('chat')} — JenderalRouter</title></svelte:head>

<div class="chat-page">
	<div class="chat-toolbar">
		<button class="btn ghost sm" onclick={() => (showHistory = true)}>
			<History size={14} /> {t('history')}
		</button>
		<div class="export-wrap">
			<button
				class="btn ghost sm"
				onclick={() => (showExport = !showExport)}
				disabled={!msgs.some((m) => m.content)}
				title={t('export_conv')}
			>
				<Download size={14} /> {t('export_conv')}
			</button>
			{#if showExport}
				<div class="menu-backdrop" aria-hidden="true" onclick={() => (showExport = false)}></div>
				<div class="export-menu" role="menu">
					{#each CONV_EXPORTS as ex (ex.fmt)}
						<button role="menuitem" onclick={() => doExportConv(ex.fmt)}>
							<b>{ex.label}</b><span>{ex.hint}</span>
						</button>
					{/each}
				</div>
			{/if}
		</div>
		<button class="btn sm" onclick={newConversation}><Plus size={14} /> {t('new_chat')}</button>
		<select bind:value={model} class="grow model-select">
			{#each models as m (m.id)}
				<option value={m.id}>{m.id}{m.local ? ' · lokal' : ''}</option>
			{/each}
		</select>
		<span class="badge info tool-badge" title="Model bisa memanggil web_fetch untuk membaca halaman web">
			<Globe size={11} /> web_fetch
		</span>
		{#if quotaInfo}
			<span class="badge info quota-chip" title="Total request Anda hari ini (reset tengah malam WIB)">
				<Activity size={11} /> {fmtNum(quotaInfo.totalReq)} req hari ini
			</span>
			{#if quotaInfo.unlimited}
				<span class="badge ok quota-chip" title="Akun Anda tidak memiliki batas kuota">
					<Gauge size={11} /> Sisa kuota: unlimited
				</span>
			{:else if quotaInfo.tightest}
				<span
					class="badge {quotaInfo.tightest.pct >= 100 ? 'err' : quotaInfo.tightest.pct >= 80 ? 'warn' : 'ok'} quota-chip"
					title={quotaInfo.list.map((c) => `${c.unit} ${periodLabel(c.period)}: ${fmtNum(c.used)} / ${fmtNum(c.limit)} terpakai`).join('\n')}
				>
					<Gauge size={11} /> Sisa kuota: {quotaInfo.tightest.left} ({periodLabel(quotaInfo.tightest.period)})
				</span>
			{/if}
		{/if}
	</div>

	<div class="card chat-pane">
		<div class="chat-msgs" bind:this={msgsEl}>
			{#if msgs.length === 0}
				<div class="kv" style="justify-content:center;padding:40px 0;color:var(--muted)">
					<Sparkles size={18} /> Model bisa membuka halaman web — coba: <span class="mono">ringkas https://example.com</span>
				</div>
			{/if}
			{#each msgs as m, i (i)}
				<div class="msg {m.role}">
					{#if m.role === 'assistant'}
						<div class="row" style="margin-bottom:4px;color:var(--muted);font-size:11px"><Bot size={12} /> assistant</div>
					{/if}
					{#if m.role === 'assistant' && m.reasoning}
						<details class="think" open={sending && !m.content}>
							<summary>💡 Thinking</summary>
							<div class="think-body">{m.reasoning}</div>
						</details>
					{/if}
					{#if m.tools?.length}
						<div class="tool-chips">
							{#each m.tools as tl}
								<span class="badge info" title={tl.args}>
									<Globe size={11} /> {tl.name}{tl.url ? ': ' + tl.url : ''}
								</span>
							{/each}
						</div>
					{/if}
					<!-- eslint-disable-next-line svelte/no-at-html-tags -->
					{@html md(m.content)}
						{#if m.role === 'assistant' && (m.provider || m.model || m.ms)}
							<div class="meta">
								{#if m.model}<span title="model yang melayani jawaban ini"><Bot size={11} /> {m.model}</span>{/if}
								{#if m.ms}<span title="durasi streaming jawaban"><span class="dot"></span> {m.ms.toLocaleString('id-ID')} dtk</span>{/if}
								{#if m.provider}<span><Sparkles size={11} /> {t('answered_by')}: {m.provider}</span>{/if}
							</div>
						{/if}
						{#if m.role === 'assistant' && m.content && (!sending || i < msgs.length - 1)}
							<div class="export-row">
								<Download size={11} />
								{#each EXPORTS as ex (ex.fmt)}
									<button title={ex.hint} onclick={() => doExport(m, ex.fmt)}>{ex.label}</button>
								{/each}
							</div>
						{/if}
				</div>
			{/each}
		</div>
		<div class="chat-input-row">
			<textarea
				bind:value={input}
				placeholder="Tulis pesan… (Enter = kirim, Shift+Enter = baris baru)"
				onkeydown={(e) => {
					if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); }
				}}
			></textarea>
			<button class="btn" onclick={send} disabled={sending || !input.trim()}>
				{#if sending}<span class="spinner"></span>{:else}<Send size={16} />{/if}
				{t('send')}
			</button>
		</div>
	</div>
</div>

{#if showHistory}
	<Modal title={t('history')} onclose={() => (showHistory = false)}>
		<button class="btn" style="width:100%;margin-bottom:12px" onclick={newConversation}>
			<Plus size={15} /> {t('new_chat')}
		</button>
		{#each conversations as c (c.id)}
			<div class="conv-item" class:active={current === c.id}>
				<button class="grow" style="all:unset;cursor:pointer;display:block" onclick={() => openConversation(c.id)}>
					<div class="small" style="font-weight:700">{c.title || t('new_chat')}</div>
					<div class="muted" style="font-size:10.5px">{fmtTs(c.updated_at)}</div>
				</button>
				<button class="icon-btn" onclick={() => delConversation(c.id)}>
					<Trash2 size={13} />
				</button>
			</div>
		{:else}
			<p class="muted small">Belum ada percakapan.</p>
		{/each}
	</Modal>
{/if}

<style>
	.chat-page {
		display: flex;
		flex-direction: column;
		height: calc(100vh - 158px);
		min-height: 460px;
	}
	.chat-toolbar {
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
		margin-bottom: 10px;
	}
	.model-select { max-width: 340px; min-width: 200px; }
	.tool-badge { cursor: help; }
	.quota-chip { cursor: help; white-space: nowrap; }
	.export-row {
		display: flex; align-items: center; gap: 5px; flex-wrap: wrap;
		margin: 6px 0 2px; color: var(--muted);
	}
	.export-row button {
		all: unset; cursor: pointer; padding: 1px 8px;
		border: 1px solid var(--border); border-radius: 999px;
		font-size: 10px; font-weight: 700; color: var(--muted);
		transition: color var(--speed), border-color var(--speed), background var(--speed);
	}
	.export-row button:hover {
		color: var(--accent-2); border-color: var(--accent);
		background: color-mix(in srgb, var(--accent) 10%, transparent);
	}
	.export-wrap { position: relative; }
	.export-menu {
		position: absolute; top: calc(100% + 6px); left: 0; z-index: 40;
		min-width: 290px; display: flex; flex-direction: column; gap: 2px;
		background: var(--surface); border: 1px solid var(--border);
		border-radius: 12px; padding: 6px;
		box-shadow: 0 12px 32px rgb(0 0 0 / 0.18);
	}
	.export-menu button {
		all: unset; cursor: pointer; display: flex; gap: 8px; align-items: baseline;
		padding: 8px 10px; border-radius: 8px; font-size: 12.5px;
		transition: background var(--speed);
	}
	.export-menu button:hover { background: var(--surface-2); }
	.export-menu b { font-size: 11px; color: var(--accent-2); min-width: 36px; }
	.export-menu span { color: var(--muted); }
	.menu-backdrop { position: fixed; inset: 0; z-index: 39; background: transparent; }
	.chat-pane {
		flex: 1;
		display: flex;
		flex-direction: column;
		min-height: 0;
		overflow: hidden;
	}
	.chat-msgs { flex: 1; overflow-y: auto; padding: 6px 2px; }
	.tool-chips { display: flex; flex-wrap: wrap; gap: 6px; margin: 2px 0 8px; }
	.think {
		margin: 2px 0 8px;
		border: 1px dashed var(--border-strong);
		border-radius: 10px;
		overflow: hidden;
		font-size: 12.5px;
	}
	.think summary {
		cursor: pointer;
		padding: 6px 10px;
		color: var(--muted);
		font-weight: 700;
		user-select: none;
		background: var(--surface-2);
	}
	.think summary:hover { color: var(--text); }
	.think-body {
		padding: 8px 12px;
		white-space: pre-wrap;
		color: var(--muted);
		max-height: 260px;
		overflow-y: auto;
		line-height: 1.55;
	}
	.msg.assistant .think { background: color-mix(in srgb, var(--surface-2) 60%, transparent); }
	.conv-item {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 9px 11px;
		border-radius: 10px;
		cursor: pointer;
		transition: background var(--speed);
	}
	.conv-item:hover { background: var(--surface-2); }
	.conv-item.active { background: var(--grad-accent-soft); border: 1px solid color-mix(in srgb, var(--accent) 30%, transparent); }
	@media (max-width: 640px) {
		.chat-page { height: auto; min-height: 60dvh; }
		.chat-msgs { max-height: 52dvh; }
		.model-select { max-width: none; flex: 1; }
	}
</style>
