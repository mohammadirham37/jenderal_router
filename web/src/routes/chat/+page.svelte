<script lang="ts">
	import {
		Plus, ArrowUp, Square, Trash2, Sparkles, History, Globe, Activity, Gauge, Download, FileText,
		Presentation, Sheet, Table, Copy, Check, ArrowDown, ChevronRight, Brain, Bot
	} from '@lucide/svelte';
	import { api, md, fmtTs, fmtNum } from '#lib/api.js';
	import { copyText } from '#lib/clipboard.js';
	import { exportAnswer, exportConversation, detectDocRequest, DOC_SYSTEM, buildDocBlob, docFileName, type ExportFormat, type DocType } from '#lib/exporters.js';
	import { t, toast } from '#lib/stores.svelte.js';
	import Modal from '#lib/components/Modal.svelte';

	type Msg = {
		role: 'user' | 'assistant' | 'tool'; content: string; provider?: string;
		reasoning?: string; model?: string; ms?: number;
		thinkStart?: number; thinkMs?: number; thinkOpen?: boolean; stopped?: boolean;
		tools?: { name: string; args: string; url?: string }[];
		file?: { name: string; url: string; size: number; type: DocType };
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
	let inputEl = $state<HTMLTextAreaElement | null>(null);
	let showHistory = $state(false);
	let showExport = $state(false);
	let abort: AbortController | null = null;

	// ---- ringkasan kuota (toolbar) — constraint paling menekan ----
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
				properties: { url: { type: 'string', description: 'URL lengkap halaman, mulai dengan http atau https' } },
				required: ['url']
			}
		}
	}];
	const MAX_TOOL_ROUNDS = 4;

	const SUGGESTIONS = [
		{ title: 'Ringkas halaman web', text: 'Ringkas isi https://go.dev/doc/ dalam 5 poin' },
		{ title: 'Buat presentasi', text: 'Buatkan ppt 6 slide tentang manfaat AI untuk UMKM' },
		{ title: 'Jelaskan konsep', text: 'Jelaskan perbedaan TCP dan UDP dengan tabel' },
		{ title: 'Tulis kode', text: 'Tulis fungsi Go untuk membaca file CSV dan menghitung rata-rata kolom' }
	];

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

	$effect(() => {
		if (model) try { localStorage.setItem('jr_chat_model', model); } catch { /* */ }
	});

	async function loadConversations() {
		try {
			const res = await api.get('/api/me/conversations');
			conversations = res.conversations || [];
			if (current === null && conversations.length > 0) await openConversation(conversations[0].id);
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
		inputEl?.focus();
	}
	async function openConversation(id: number) {
		current = id;
		try {
			const res = await api.get('/api/me/conversations/' + id);
			msgs = (res.messages || []).map((m: any) => ({
				role: m.role, content: m.content, provider: m.provider_name, model: m.model || undefined
			}));
			showHistory = false;
			scrollToBottom(true);
		} catch (e: any) { toast(e.message, 'err'); }
	}
	async function delConversation(id: number) {
		if (!confirm(t('confirm_delete'))) return;
		await api.del('/api/me/conversations/' + id);
		if (current === id) { current = null; msgs = []; }
		loadConversations();
	}

	// ================= scroll pintar =================
	// Ikut ke bawah HANYA bila pengguna memang berada di dasar. Begitu ia
	// menggulir ke atas (saat model masih berpikir/menjawab), auto-scroll
	// berhenti dan muncul tombol "ke bawah".
	let stick = $state(true);
	function onScroll() {
		if (!msgsEl) return;
		const dist = msgsEl.scrollHeight - msgsEl.scrollTop - msgsEl.clientHeight;
		stick = dist < 72;
	}
	function onWheel(e: WheelEvent) {
		if (e.deltaY < 0) stick = false; // niat naik → lepas seketika, tanpa menunggu event scroll
	}
	let touchY = 0;
	function onTouchStart(e: TouchEvent) { touchY = e.touches[0]?.clientY ?? 0; }
	function onTouchMove(e: TouchEvent) {
		if ((e.touches[0]?.clientY ?? 0) > touchY + 4) stick = false;
	}
	function scrollToBottom(force = false, smooth = false) {
		if (force) stick = true;
		if (!stick) return;
		requestAnimationFrame(() => {
			if (msgsEl && stick) msgsEl.scrollTo({ top: msgsEl.scrollHeight, behavior: smooth ? 'smooth' : 'auto' });
		});
	}

	// ================= streaming per frame =================
	// token ditampung lalu diterapkan sekali per frame — render markdown tidak
	// diulang ratusan kali per detik, animasi tetap halus.
	let pend = { content: '', reasoning: '' };
	let target: Msg | null = null;
	let frame = 0;
	function flush() {
		frame = 0;
		if (!target) return;
		if (pend.reasoning) {
			target.thinkStart ??= performance.now();
			target.reasoning = (target.reasoning ?? '') + pend.reasoning;
		}
		if (pend.content) {
			if (target.thinkStart && !target.thinkMs) target.thinkMs = performance.now() - target.thinkStart;
			target.content += pend.content;
		}
		pend = { content: '', reasoning: '' };
		scrollToBottom();
	}
	function queue(field: 'content' | 'reasoning', text: string) {
		pend[field] += text;
		if (!frame) frame = requestAnimationFrame(flush);
	}
	function flushNow() {
		if (frame) cancelAnimationFrame(frame);
		flush();
	}

	// detik berjalan untuk label "Sedang berpikir… N dtk"
	let now = $state(performance.now());
	let sendStart = $state(0);
	$effect(() => {
		if (!sending) return;
		const id = setInterval(() => (now = performance.now()), 250);
		return () => clearInterval(id);
	});
	const secs = (ms: number) => Math.max(1, Math.round(ms / 1000));

	// ================= ekspor =================
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
		try { exportAnswer(convTitle(), m.content, fmt); }
		catch (e: any) { toast('ekspor gagal: ' + e.message, 'err'); }
	}
	const CONV_EXPORTS: { fmt: ExportFormat; label: string; hint: string }[] = [
		{ fmt: 'md', label: 'MD', hint: 'Markdown seluruh percakapan' },
		{ fmt: 'docx', label: 'DOCX', hint: 'Dokumen Word seluruh percakapan' },
		{ fmt: 'pptx', label: 'PPTX', hint: 'Presentasi ringkas percakapan' },
		{ fmt: 'xlsx', label: 'XLSX', hint: 'Excel — transkrip + semua tabel' },
		{ fmt: 'csv', label: 'CSV', hint: 'CSV transkrip (Peran; Pesan)' }
	];
	function doExportConv(fmt: ExportFormat) {
		const turns = msgs.filter((m) => m.content && m.role !== 'tool').map((m) => ({ role: m.role, content: m.content, model: m.model }));
		if (!turns.length) return;
		try { exportConversation(convTitle(), turns, fmt); }
		catch (e: any) { toast('ekspor gagal: ' + e.message, 'err'); }
		showExport = false;
	}

	// ---- file jadi: prompt user → dokumen terlampir di jawaban ----
	const FILE_ICONS = { pptx: Presentation, docx: FileText, xlsx: Sheet, csv: Table };
	function fmtSize(n: number): string {
		return n > 1048576 ? (n / 1048576).toFixed(1) + ' MB' : Math.max(1, Math.round(n / 1024)) + ' KB';
	}
	function firstH1(s: string): string {
		const m = s.match(/^#{1,3}\s+(.+)/m);
		return m ? m[1].replace(/^#+\s+/, '').trim() : '';
	}
	function attachGeneratedFile(type: DocType) {
		const bubble = [...msgs].reverse().find((m) => m.role === 'assistant' && m.content.trim());
		if (!bubble) return;
		try {
			const title = firstH1(bubble.content) || convTitle();
			const content = bubble.content.replace(/^\s*#{1,3}\s+[^\n]*\n+/, '');
			const blob = buildDocBlob(type, title, content);
			bubble.file = { name: docFileName(title, type), url: URL.createObjectURL(blob), size: blob.size, type };
			scrollToBottom();
		} catch (e: any) {
			toast('gagal merakit file: ' + e.message, 'err');
		}
	}

	// ---- salin: pesan & blok kode (delegasi klik; HTML jawaban dari md()) ----
	let copiedIdx = $state(-1);
	async function copyMsg(m: Msg, i: number) {
		if (await copyText(m.content)) {
			copiedIdx = i;
			setTimeout(() => (copiedIdx = copiedIdx === i ? -1 : copiedIdx), 1500);
		} else toast('gagal menyalin', 'err');
	}
	// tombol Salin di blok kode berasal dari HTML md() → dipasang lewat delegasi
	$effect(() => {
		const el = msgsEl;
		if (!el) return;
		el.addEventListener('click', onMsgsClick);
		return () => el.removeEventListener('click', onMsgsClick);
	});
	async function onMsgsClick(e: MouseEvent) {
		const btn = (e.target as HTMLElement).closest('.code-copy') as HTMLButtonElement | null;
		if (!btn) return;
		const code = btn.closest('.code-block')?.querySelector('code')?.textContent ?? '';
		if (await copyText(code)) {
			btn.textContent = 'Tersalin ✓';
			setTimeout(() => (btn.textContent = 'Salin'), 1500);
		}
	}

	// ---- composer: tinggi mengikuti isi ----
	function autosize() {
		if (!inputEl) return;
		inputEl.style.height = 'auto';
		inputEl.style.height = Math.min(inputEl.scrollHeight, 220) + 'px';
	}
	$effect(() => {
		void input;
		autosize();
	});
	function useSuggestion(text: string) {
		input = text;
		inputEl?.focus();
	}

	function stop() {
		abort?.abort();
	}

	async function send() {
		const text = input.trim();
		if (!text || sending) return;
		sending = true;
		input = '';
		const docType = detectDocRequest(text);
		if (!current) {
			try { await newConversation(); }
			catch (e: any) {
				toast('buat percakapan gagal: ' + e.message, 'err');
				sending = false;
				return;
			}
		}
		msgs.push({ role: 'user', content: text });
		scrollToBottom(true, true);

		const history: any[] = [];
		if (docType) history.push({ role: 'system', content: DOC_SYSTEM[docType] });
		for (const m of msgs) {
			if (m.role === 'user') history.push({ role: 'user', content: m.content });
			else if (m.role === 'assistant' && m.content) history.push({ role: 'assistant', content: m.content });
		}

		const started = performance.now();
		sendStart = started;
		let quotaSeen = false;
		abort = new AbortController();
		const signal = abort.signal;

		try {
			for (let round = 0; round <= MAX_TOOL_ROUNDS; round++) {
				msgs.push({ role: 'assistant', content: '' });
				const bubble = msgs[msgs.length - 1];
				target = bubble;
				scrollToBottom();

				let finish = '';
				const pending: Record<number, ToolCall> = {};

				await api.chatStream(
					{ model, messages: history, tools: TOOLS, signal },
					(delta) => queue('content', delta),
					() => { if (!quotaSeen) { quotaSeen = true; loadQuota(); } },
					(extra) => {
						if (extra.reasoning) queue('reasoning', extra.reasoning);
						if (extra.model) bubble.model = extra.model.split('/').pop()?.replace(/\.gguf$/i, '') || extra.model;
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
						}
						if (d?.finish_reason) finish = d.finish_reason;
					}
				);
				flushNow();
				if (bubble.thinkStart && !bubble.thinkMs) bubble.thinkMs = performance.now() - bubble.thinkStart;

				const calls = Object.values(pending).filter((tc) => tc.name);
				if (finish !== 'tool_calls' || calls.length === 0) break;

				bubble.tools = (bubble.tools ?? []).concat(
					calls.map((tc) => {
						let u = '';
						try { u = JSON.parse(tc.args || '{}').url || ''; } catch { /* */ }
						return { name: tc.name, args: tc.args, url: u };
					})
				);
				history.push({
					role: 'assistant',
					content: bubble.content || null,
					tool_calls: calls.map((tc) => ({ id: tc.id, type: 'function', function: { name: tc.name, arguments: tc.args } }))
				});
				for (const tc of calls) {
					let result = '';
					const meta = bubble.tools?.[bubble.tools.length - 1];
					if (tc.name === 'web_fetch') {
						let url = '';
						try { url = JSON.parse(tc.args || '{}').url || ''; } catch { /* */ }
						try {
							const r = await api.post('/api/me/tools/webfetch', { url });
							result = 'Judul: ' + (r.title || '-') + '\n' + r.text;
						} catch (e: any) {
							result = 'ERROR: ' + e.message;
						}
						if (meta) meta.url = url;
					} else {
						result = 'ERROR: tool "' + tc.name + '" tidak dikenal';
					}
					history.push({ role: 'tool', tool_call_id: tc.id, content: result });
				}
				scrollToBottom();
			}
			await saveTurn(text);
			if (docType) attachGeneratedFile(docType);
			loadQuota();
			loadConversations();
		} catch (e: any) {
			flushNow();
			const last = msgs[msgs.length - 1];
			if (e?.name === 'AbortError') {
				if (last && last.role === 'assistant') last.stopped = true;
			} else if (last && last.role === 'assistant') last.content += '\n\n**⚠ ' + e.message + '**';
			else toast(e.message, 'err');
			// simpan best-effort: user + jawaban parsial agar tidak hilang saat refresh
			if (current) try { await saveTurn(text); } catch { /* */ }
		} finally {
			target = null;
			abort = null;
			const dur = Math.round((performance.now() - started) / 100) / 10;
			for (const m of msgs) if (m.role === 'assistant' && !m.ms) m.ms = dur;
			sending = false;
			scrollToBottom();
			inputEl?.focus();
		}
	}
	async function saveTurn(text: string) {
		await api.post(`/api/me/conversations/${current}/messages`, { role: 'user', content: text });
		const la = [...msgs].reverse().find((m) => m.role === 'assistant');
		await api.post(`/api/me/conversations/${current}/messages`, {
			role: 'assistant', content: la?.content || '', model: la?.model || model
		});
	}

	const lastIdx = $derived(msgs.length - 1);
</script>

<svelte:head><title>{t('chat')} — JenderalRouter</title></svelte:head>

<div class="chat-page">
	<div class="chat-toolbar">
		<button class="btn ghost sm" onclick={() => (showHistory = true)}><History size={14} /> <span class="lbl">{t('history')}</span></button>
		<button class="btn ghost sm" onclick={newConversation}><Plus size={14} /> <span class="lbl">{t('new_chat')}</span></button>
		<div class="export-wrap">
			<button class="btn ghost sm" onclick={() => (showExport = !showExport)} disabled={!msgs.some((m) => m.content)} title={t('export_conv')}>
				<Download size={14} /> <span class="lbl">{t('export_conv')}</span>
			</button>
			{#if showExport}
				<div class="menu-backdrop" aria-hidden="true" onclick={() => (showExport = false)}></div>
				<div class="export-menu" role="menu">
					{#each CONV_EXPORTS as ex (ex.fmt)}
						<button role="menuitem" onclick={() => doExportConv(ex.fmt)}><b>{ex.label}</b><span>{ex.hint}</span></button>
					{/each}
				</div>
			{/if}
		</div>
		<span class="grow"></span>
		{#if quotaInfo}
			<span class="badge info quota-chip req" title="Total request Anda hari ini (reset tengah malam WIB)">
				<Activity size={11} /> {fmtNum(quotaInfo.totalReq)} req hari ini
			</span>
			{#if quotaInfo.unlimited}
				<span class="badge ok quota-chip" title="Akun Anda tidak memiliki batas kuota"><Gauge size={11} /> kuota: unlimited</span>
			{:else if quotaInfo.tightest}
				<span
					class="badge {quotaInfo.tightest.pct >= 100 ? 'err' : quotaInfo.tightest.pct >= 80 ? 'warn' : 'ok'} quota-chip"
					title={quotaInfo.list.map((c) => `${c.unit} ${periodLabel(c.period)}: ${fmtNum(c.used)} / ${fmtNum(c.limit)} terpakai`).join('\n')}
				>
					<Gauge size={11} /> sisa {quotaInfo.tightest.left} ({periodLabel(quotaInfo.tightest.period)})
				</span>
			{/if}
		{/if}
	</div>

	<div class="chat-shell">
		<div
			class="scroller"
			bind:this={msgsEl}
			onscroll={onScroll}
			onwheel={onWheel}
			ontouchstart={onTouchStart}
			ontouchmove={onTouchMove}
			role="log"
			aria-live="polite"
			aria-busy={sending}
		>
			<div class="thread">
				{#if msgs.length === 0}
					<div class="welcome">
						<div class="orb" aria-hidden="true"><Sparkles size={22} /></div>
						<h2>Apa yang bisa saya bantu?</h2>
						<p class="muted">Model bisa membaca halaman web, membuat dokumen Word/PowerPoint/Excel, dan menulis kode.</p>
						<div class="suggest">
							{#each SUGGESTIONS as s, i (s.title)}
								<button style="--d:{i * 60}ms" onclick={() => useSuggestion(s.text)}>
									<b>{s.title}</b><span>{s.text}</span>
								</button>
							{/each}
						</div>
					</div>
				{/if}

				{#each msgs as m, i (i)}
					{#if m.role === 'user'}
						<div class="turn user">
							<div class="bubble">{m.content}</div>
						</div>
					{:else if m.role === 'assistant'}
						{@const live = sending && i === lastIdx}
						{@const thinkingLive = live && !!m.reasoning && !m.content}
						<div class="turn assistant" class:live>
							<div class="avatar" class:pulse={live} aria-hidden="true"><Bot size={15} /></div>
							<div class="body">
								{#if live && !m.content && !m.reasoning && !m.tools?.length}
									<div class="waiting" aria-label="menunggu jawaban">
										<span class="dots"><i></i><i></i><i></i></span>
										<span class="shimmer">{now - sendStart > 2500 ? 'Menyiapkan jawaban…' : 'Menghubungkan ke model…'}</span>
									</div>
								{/if}

								{#if m.reasoning}
									<div class="think" class:open={m.thinkOpen} class:live={thinkingLive}>
										<button class="think-head" onclick={() => (m.thinkOpen = !m.thinkOpen)} aria-expanded={!!m.thinkOpen}>
											<Brain size={14} />
											{#if thinkingLive}
												<span class="shimmer">Sedang berpikir… {m.thinkStart ? secs(now - m.thinkStart) : 1} dtk</span>
											{:else}
												<span>Berpikir selama {secs(m.thinkMs ?? 0)} dtk</span>
											{/if}
											<span class="chev"><ChevronRight size={14} /></span>
										</button>
										{#if m.thinkOpen}
											<div class="think-full">{m.reasoning}</div>
										{:else if thinkingLive}
											<!-- pratinjau: baris terakhir penalaran, memudar di atas -->
											<div class="think-peek"><div>{m.reasoning.slice(-600)}</div></div>
										{/if}
									</div>
								{/if}

								{#if m.tools?.length}
									<div class="tool-chips">
										{#each m.tools as tl}
											<span class="tool-chip" title={tl.args}><Globe size={12} /> Membaca {tl.url || tl.name}</span>
										{/each}
									</div>
								{/if}

								{#if m.content}
									<div class="answer" class:streaming={live}>
										<!-- md() meng-escape seluruh teks sebelum membentuk HTML -->
										{@html md(m.content)}
									</div>
								{/if}
								{#if m.stopped}<div class="stopped">Dihentikan</div>{/if}

								{#if m.file}
									{@const FileIcon = FILE_ICONS[m.file.type]}
									<a class="file-card" href={m.file.url} download={m.file.name}>
										<FileIcon size={18} />
										<span class="grow"><b>{m.file.name}</b><small>{fmtSize(m.file.size)} — {t('file_ready')}</small></span>
										<span class="dl"><Download size={15} /></span>
									</a>
								{/if}

								{#if m.content && !live}
									<div class="actions">
										<button class="act" title="Salin jawaban" onclick={() => copyMsg(m, i)}>
											{#if copiedIdx === i}<Check size={14} />{:else}<Copy size={14} />{/if}
										</button>
										<span class="exports">
											{#each EXPORTS as ex (ex.fmt)}
												<button title={ex.hint} onclick={() => doExport(m, ex.fmt)}>{ex.label}</button>
											{/each}
										</span>
										<span class="meta">
											{#if m.model}<span title="model yang melayani jawaban ini">{m.model}</span>{/if}
											{#if m.ms}<span title="durasi jawaban">{m.ms.toLocaleString('id-ID')} dtk</span>{/if}
											{#if m.provider}<span>{m.provider}</span>{/if}
										</span>
									</div>
								{/if}
							</div>
						</div>
					{/if}
				{/each}
			</div>
		</div>

		{#if !stick && msgs.length > 0}
			<button class="jump" onclick={() => scrollToBottom(true, true)} aria-label="Gulir ke pesan terbaru">
				<ArrowDown size={16} />
				{#if sending}<span>jawaban baru</span>{/if}
			</button>
		{/if}

		<div class="composer-wrap">
			<div class="composer">
				<textarea
					bind:this={inputEl}
					bind:value={input}
					rows="1"
					placeholder="Kirim pesan…"
					aria-label="Pesan"
					onkeydown={(e) => {
						if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) { e.preventDefault(); send(); }
					}}
				></textarea>
				<div class="composer-bar">
					<select bind:value={model} class="model-select" aria-label="Model">
						{#each models as mo (mo.id)}
							<option value={mo.id}>{mo.id}{mo.local ? ' · lokal' : ''}</option>
						{/each}
					</select>
					<span class="tool-badge" title="Model bisa memanggil web_fetch untuk membaca halaman web"><Globe size={12} /> web</span>
					<span class="grow"></span>
					{#if sending}
						<button class="send stop" onclick={stop} title="Hentikan jawaban" aria-label="Hentikan"><Square size={13} fill="currentColor" /></button>
					{:else}
						<button class="send" onclick={send} disabled={!input.trim()} title={t('send')} aria-label={t('send')}><ArrowUp size={17} /></button>
					{/if}
				</div>
			</div>
			<div class="hint">Enter untuk kirim · Shift+Enter baris baru · Jawaban AI bisa keliru, periksa info penting.</div>
		</div>
	</div>
</div>

{#if showHistory}
	<Modal title={t('history')} onclose={() => (showHistory = false)}>
		<button class="btn" style="width:100%;margin-bottom:12px" onclick={newConversation}><Plus size={15} /> {t('new_chat')}</button>
		{#each conversations as c (c.id)}
			<div class="conv-item" class:active={current === c.id}>
				<button class="grow" style="all:unset;cursor:pointer;display:block" onclick={() => openConversation(c.id)}>
					<div class="small" style="font-weight:700">{c.title || t('new_chat')}</div>
					<div class="muted" style="font-size:10.5px">{fmtTs(c.updated_at)}</div>
				</button>
				<button class="icon-btn" onclick={() => delConversation(c.id)} aria-label="hapus percakapan"><Trash2 size={13} /></button>
			</div>
		{:else}
			<p class="muted small">Belum ada percakapan.</p>
		{/each}
	</Modal>
{/if}

<style>
	.chat-page { display: flex; flex-direction: column; height: calc(100dvh - 66px); min-height: 480px; }
	.chat-toolbar { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin-bottom: 8px; }
	.quota-chip { cursor: help; white-space: nowrap; }

	/* ---------- cangkang ---------- */
	.chat-shell {
		position: relative; flex: 1; min-height: 0; display: flex; flex-direction: column;
		border: 1px solid var(--border); border-radius: 18px; overflow: hidden;
		background:
			radial-gradient(700px 260px at 50% -80px, color-mix(in srgb, var(--accent) 7%, transparent), transparent 70%),
			color-mix(in srgb, var(--surface) 92%, transparent);
	}
	.scroller { flex: 1; overflow-y: auto; overscroll-behavior: contain; scroll-padding-bottom: 24px; }
	.thread { max-width: 780px; margin: 0 auto; padding: 26px 22px 40px; }

	/* ---------- sambutan ---------- */
	.welcome { text-align: center; padding: 8vh 0 2vh; animation: rise 420ms cubic-bezier(.2,.7,.2,1); }
	.welcome h2 { font-size: 24px; letter-spacing: -0.02em; margin: 14px 0 6px; font-weight: 750; }
	.welcome p { margin: 0 auto 22px; max-width: 460px; font-size: 13.5px; }
	.orb {
		width: 48px; height: 48px; margin: 0 auto; border-radius: 16px; display: grid; place-items: center;
		color: #fff; background: var(--grad-accent);
		box-shadow: 0 10px 30px color-mix(in srgb, var(--accent) 40%, transparent);
	}
	.suggest { display: grid; grid-template-columns: repeat(2, 1fr); gap: 10px; text-align: left; }
	.suggest button {
		all: unset; cursor: pointer; padding: 12px 14px; border-radius: 14px;
		border: 1px solid var(--border); background: var(--surface);
		display: flex; flex-direction: column; gap: 3px;
		animation: rise 460ms cubic-bezier(.2,.7,.2,1) both; animation-delay: var(--d);
		transition: border-color var(--speed), transform var(--speed), background var(--speed);
	}
	.suggest button:hover { border-color: color-mix(in srgb, var(--accent) 55%, var(--border)); transform: translateY(-1px); background: var(--surface-2); }
	.suggest b { font-size: 13px; }
	.suggest span { font-size: 12px; color: var(--muted); line-height: 1.45; }

	/* ---------- giliran ---------- */
	.turn { display: flex; gap: 12px; margin-bottom: 26px; animation: rise 260ms cubic-bezier(.2,.7,.2,1); }
	.turn.user { justify-content: flex-end; }
	.bubble {
		max-width: min(78%, 620px); padding: 10px 16px; border-radius: 20px 20px 6px 20px;
		background: var(--surface-3); border: 1px solid var(--border);
		white-space: pre-wrap; overflow-wrap: anywhere; font-size: 14.5px; line-height: 1.6;
	}
	.avatar {
		flex: 0 0 auto; width: 30px; height: 30px; border-radius: 10px; display: grid; place-items: center;
		color: #fff; background: var(--grad-accent); margin-top: 1px;
	}
	.avatar.pulse { animation: breathe 1.8s ease-in-out infinite; }
	.body { flex: 1; min-width: 0; }

	/* ---------- menunggu & berpikir ---------- */
	.waiting { display: flex; align-items: center; gap: 10px; height: 30px; color: var(--muted); font-size: 13.5px; }
	.dots { display: inline-flex; gap: 4px; }
	.dots i { width: 6px; height: 6px; border-radius: 50%; background: var(--accent-2); animation: bounce 1.2s infinite ease-in-out; }
	.dots i:nth-child(2) { animation-delay: .15s; }
	.dots i:nth-child(3) { animation-delay: .3s; }
	.shimmer {
		background: linear-gradient(90deg, var(--muted) 0%, var(--muted) 35%, var(--text) 50%, var(--muted) 65%, var(--muted) 100%);
		background-size: 250% 100%; -webkit-background-clip: text; background-clip: text; color: transparent;
		animation: shimmer 2.2s linear infinite;
	}
	.think { margin: 2px 0 12px; }
	.think-head {
		all: unset; cursor: pointer; display: inline-flex; align-items: center; gap: 7px;
		color: var(--muted); font-size: 13px; font-weight: 600; padding: 4px 0;
		transition: color var(--speed);
	}
	.think-head:hover { color: var(--text); }
	.chev { display: inline-flex; transition: transform 220ms ease; }
	.think.open .chev { transform: rotate(90deg); }
	.think-peek {
		margin-top: 6px; max-height: 92px; overflow: hidden; display: flex; flex-direction: column-reverse;
		padding-left: 12px; border-left: 2px solid color-mix(in srgb, var(--accent) 45%, var(--border));
		-webkit-mask-image: linear-gradient(to bottom, transparent 0, #000 45%);
		mask-image: linear-gradient(to bottom, transparent 0, #000 45%);
	}
	.think-peek div, .think-full {
		white-space: pre-wrap; font-size: 12.5px; line-height: 1.6; color: var(--muted); overflow-wrap: anywhere;
	}
	.think-full {
		margin-top: 6px; padding: 2px 0 2px 12px; max-height: 340px; overflow-y: auto;
		border-left: 2px solid var(--border-strong); animation: fade 200ms ease;
	}

	.tool-chips { display: flex; flex-wrap: wrap; gap: 6px; margin: 0 0 10px; }
	.tool-chip {
		display: inline-flex; align-items: center; gap: 6px; max-width: 100%;
		padding: 4px 10px; border-radius: 999px; font-size: 12px; color: var(--muted);
		background: var(--surface-2); border: 1px solid var(--border);
		white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
	}

	/* ---------- jawaban (markdown) ---------- */
	.answer { font-size: 15px; line-height: 1.72; overflow-wrap: anywhere; }
	.answer :global(p) { margin: 0 0 12px; }
	.answer :global(h3), .answer :global(h4), .answer :global(h5) { margin: 20px 0 8px; line-height: 1.35; letter-spacing: -0.01em; }
	.answer :global(h3) { font-size: 19px; }
	.answer :global(h4) { font-size: 16.5px; }
	.answer :global(h5) { font-size: 15px; }
	.answer :global(> :first-child) { margin-top: 0; }
	.answer :global(ul), .answer :global(ol) { margin: 0 0 12px; padding-left: 22px; }
	.answer :global(li) { margin: 4px 0; }
	.answer :global(li::marker) { color: var(--muted); }
	.answer :global(a) { color: var(--accent-2); text-underline-offset: 3px; }
	.answer :global(strong) { font-weight: 700; }
	.answer :global(hr) { border: 0; border-top: 1px solid var(--border); margin: 18px 0; }
	.answer :global(blockquote) {
		margin: 0 0 12px; padding: 2px 0 2px 14px; color: var(--muted);
		border-left: 3px solid color-mix(in srgb, var(--accent) 50%, var(--border));
	}
	.answer :global(code) {
		font-family: var(--mono); font-size: 0.86em; padding: 1px 6px; border-radius: 6px;
		background: var(--surface-3); border: 1px solid var(--border);
	}
	.answer :global(.code-block) { margin: 4px 0 14px; border: 1px solid var(--border); border-radius: 12px; overflow: hidden; background: var(--bg); }
	.answer :global(.code-head) {
		display: flex; justify-content: space-between; align-items: center; padding: 6px 12px;
		font-size: 11.5px; color: var(--muted); background: var(--surface-2); border-bottom: 1px solid var(--border);
		font-family: var(--mono);
	}
	.answer :global(.code-copy) {
		all: unset; cursor: pointer; font-family: var(--font); font-size: 11.5px; color: var(--muted);
		padding: 2px 8px; border-radius: 6px; transition: color var(--speed), background var(--speed);
	}
	.answer :global(.code-copy:hover) { color: var(--text); background: var(--surface-3); }
	.answer :global(pre) { margin: 0; padding: 12px 14px; overflow-x: auto; }
	.answer :global(pre code) { background: none; border: 0; padding: 0; font-size: 13px; line-height: 1.6; }
	.answer :global(.table-scroll) { overflow-x: auto; margin: 4px 0 14px; border: 1px solid var(--border); border-radius: 12px; }
	.answer :global(table) { border-collapse: collapse; width: 100%; font-size: 13.5px; }
	.answer :global(th), .answer :global(td) {
		padding: 8px 12px; border-bottom: 1px solid var(--border); text-align: left;
		overflow-wrap: normal; word-break: normal; /* tabel melebar & digulir, kata tidak dipotong */
	}
	.answer :global(th) { background: var(--surface-2); font-weight: 700; }
	.answer :global(tr:last-child td) { border-bottom: 0; }

	/* kursor ketik di akhir jawaban yang sedang mengalir */
	.answer.streaming :global(> :last-child::after) {
		content: ''; display: inline-block; width: 8px; height: 1.05em; margin-left: 3px; vertical-align: -0.15em;
		border-radius: 2px; background: var(--accent-2); animation: caret 1s steps(2) infinite;
	}
	.answer.streaming :global(> .code-block:last-child::after),
	.answer.streaming :global(> .table-scroll:last-child::after) { display: none; }
	.stopped { font-size: 12px; color: var(--muted); font-style: italic; margin-top: -4px; }

	/* ---------- aksi per jawaban ---------- */
	.actions { display: flex; align-items: center; gap: 6px; margin-top: 2px; min-height: 28px; flex-wrap: wrap; }
	.act {
		all: unset; cursor: pointer; display: grid; place-items: center; width: 28px; height: 28px; border-radius: 8px;
		color: var(--muted); transition: color var(--speed), background var(--speed);
	}
	.act:hover { color: var(--text); background: var(--surface-2); }
	.exports { display: inline-flex; gap: 4px; opacity: 0; transition: opacity var(--speed); }
	.turn:hover .exports, .exports:focus-within { opacity: 1; }
	.exports button {
		all: unset; cursor: pointer; padding: 2px 8px; border-radius: 999px; font-size: 10.5px; font-weight: 700;
		color: var(--muted); border: 1px solid var(--border); transition: color var(--speed), border-color var(--speed);
	}
	.exports button:hover { color: var(--accent-2); border-color: var(--accent); }
	.meta { margin-left: auto; display: inline-flex; gap: 10px; font-size: 11px; color: var(--muted); opacity: .75; }
	.meta span + span::before { content: '·'; margin-right: 10px; }

	.file-card {
		display: flex; align-items: center; gap: 10px; margin: 6px 0 10px; padding: 11px 13px; max-width: 460px;
		border: 1px solid color-mix(in srgb, var(--accent) 35%, var(--border));
		background: color-mix(in srgb, var(--accent) 7%, var(--surface));
		border-radius: 14px; color: var(--text); text-decoration: none;
		transition: border-color var(--speed), transform var(--speed);
	}
	.file-card:hover { border-color: var(--accent); transform: translateY(-1px); }
	.file-card b { font-size: 12.5px; display: block; word-break: break-all; }
	.file-card small { color: var(--muted); font-size: 11px; }
	.file-card .grow { min-width: 0; }
	.file-card .dl { margin-left: auto; color: var(--accent-2); flex-shrink: 0; }

	/* ---------- tombol ke bawah ---------- */
	.jump {
		all: unset; cursor: pointer; position: absolute; left: 50%; bottom: 128px; transform: translateX(-50%);
		display: inline-flex; align-items: center; gap: 6px; padding: 7px 12px; border-radius: 999px;
		font-size: 12px; font-weight: 600; color: var(--text); z-index: 5;
		background: color-mix(in srgb, var(--surface) 92%, transparent); backdrop-filter: blur(10px);
		border: 1px solid var(--border-strong); box-shadow: var(--shadow-sm), 0 8px 24px rgb(0 0 0 / .18);
		animation: pop 200ms cubic-bezier(.2,.7,.2,1);
	}
	.jump:hover { border-color: var(--accent); }

	/* ---------- composer ---------- */
	.composer-wrap {
		padding: 0 18px 12px; position: relative;
		background: linear-gradient(to top, color-mix(in srgb, var(--surface) 96%, transparent) 70%, transparent);
	}
	.composer-wrap::before {
		content: ''; position: absolute; left: 0; right: 0; top: -28px; height: 28px; pointer-events: none;
		background: linear-gradient(to top, color-mix(in srgb, var(--surface) 92%, transparent), transparent);
	}
	.composer {
		max-width: 780px; margin: 0 auto; border: 1px solid var(--border-strong); border-radius: 22px;
		background: var(--surface); padding: 10px 10px 8px 16px;
		box-shadow: 0 6px 28px rgb(0 0 0 / .14);
		transition: border-color var(--speed), box-shadow var(--speed);
	}
	.composer:focus-within {
		border-color: color-mix(in srgb, var(--accent) 60%, var(--border-strong));
		box-shadow: 0 0 0 4px color-mix(in srgb, var(--accent) 14%, transparent), 0 6px 28px rgb(0 0 0 / .14);
	}
	.composer textarea {
		all: unset; display: block; width: 100%; resize: none; min-height: 24px; max-height: 220px;
		overflow-y: auto; font-size: 15px; line-height: 1.55; padding: 4px 0; white-space: pre-wrap;
	}
	.composer textarea::placeholder { color: var(--muted); }
	.composer-bar { display: flex; align-items: center; gap: 8px; margin-top: 6px; }
	.model-select {
		width: auto; max-width: 260px; font-size: 12px; padding: 4px 8px; border-radius: 999px;
		background: var(--surface-2); border: 1px solid var(--border);
	}
	.tool-badge { display: inline-flex; align-items: center; gap: 4px; font-size: 11.5px; color: var(--muted); cursor: help; }
	.send {
		all: unset; cursor: pointer; width: 34px; height: 34px; border-radius: 50%; display: grid; place-items: center;
		color: #fff; background: var(--grad-accent); transition: transform var(--speed), opacity var(--speed), filter var(--speed);
	}
	.send:hover:not(:disabled) { transform: scale(1.06); filter: brightness(1.08); }
	.send:disabled { opacity: .35; cursor: default; }
	.send.stop { background: var(--text); color: var(--surface); }
	.hint { text-align: center; font-size: 11px; color: var(--muted); margin-top: 7px; opacity: .8; }

	/* ---------- toolbar menu ---------- */
	.export-wrap { position: relative; }
	.export-menu {
		position: absolute; top: calc(100% + 6px); left: 0; z-index: 40; min-width: 290px;
		display: flex; flex-direction: column; gap: 2px; background: var(--surface);
		border: 1px solid var(--border); border-radius: 12px; padding: 6px; box-shadow: 0 12px 32px rgb(0 0 0 / 0.18);
	}
	.export-menu button {
		all: unset; cursor: pointer; display: flex; gap: 8px; align-items: baseline; padding: 8px 10px;
		border-radius: 8px; font-size: 12.5px; transition: background var(--speed);
	}
	.export-menu button:hover { background: var(--surface-2); }
	.export-menu b { font-size: 11px; color: var(--accent-2); min-width: 36px; }
	.export-menu span { color: var(--muted); }
	.menu-backdrop { position: fixed; inset: 0; z-index: 39; background: transparent; }

	.conv-item { display: flex; align-items: center; gap: 8px; padding: 9px 11px; border-radius: 10px; cursor: pointer; transition: background var(--speed); }
	.conv-item:hover { background: var(--surface-2); }
	.conv-item.active { background: var(--grad-accent-soft); border: 1px solid color-mix(in srgb, var(--accent) 30%, transparent); }

	/* ---------- animasi ---------- */
	@keyframes rise { from { opacity: 0; transform: translateY(8px); } to { opacity: 1; transform: none; } }
	@keyframes fade { from { opacity: 0; } to { opacity: 1; } }
	@keyframes pop { from { opacity: 0; transform: translate(-50%, 6px) scale(.96); } to { opacity: 1; transform: translateX(-50%); } }
	@keyframes shimmer { from { background-position: 100% 0; } to { background-position: -150% 0; } }
	@keyframes bounce { 0%, 80%, 100% { transform: translateY(0); opacity: .45; } 40% { transform: translateY(-5px); opacity: 1; } }
	@keyframes breathe { 0%, 100% { box-shadow: 0 0 0 0 color-mix(in srgb, var(--accent) 45%, transparent); } 50% { box-shadow: 0 0 0 7px transparent; } }
	@keyframes caret { 50% { opacity: 0; } }
	@media (prefers-reduced-motion: reduce) {
		.turn, .welcome, .suggest button, .jump { animation: none; }
		.shimmer, .dots i, .avatar.pulse, .answer.streaming :global(> :last-child::after) { animation: none; }
	}

	@media (max-width: 640px) {
		.chat-page { height: calc(100dvh - 110px); }
		.chat-toolbar { flex-wrap: nowrap; }
		.chat-toolbar .lbl, .quota-chip.req { display: none; }
		.thread { padding: 18px 12px 30px; }
		.suggest { grid-template-columns: 1fr; }
		.bubble { max-width: 88%; }
		.avatar { display: none; }
		.composer-wrap { padding: 0 8px 8px; }
		.model-select { max-width: 150px; }
		.jump { bottom: 120px; }
		.hint { display: none; }
	}
</style>
