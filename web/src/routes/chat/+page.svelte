<script lang="ts">
	import {
		Plus, ArrowUp, Square, Trash2, Sparkles, History, Globe, Activity, Gauge, Download, FileText,
		Presentation, Sheet, Table, Copy, Check, ArrowDown, ChevronRight, Brain, Bot, Search, Paperclip, X, Image as ImageIcon, Upload, Pencil, RotateCcw, Info
	} from '@lucide/svelte';
	import { api, md, fmtTs, fmtNum } from '#lib/api.js';
	import { copyText } from '#lib/clipboard.js';
	import { exportAnswer, exportConversation, detectDocRequest, DOC_SYSTEM, buildDocBlob, docFileName, type ExportFormat, type DocType } from '#lib/exporters.js';
	import { t, toast } from '#lib/stores.svelte.js';
	import Modal from '#lib/components/Modal.svelte';
	import {
		ACCEPT, MAX_ATTACHMENTS, MAX_IMAGES, kindOf, readAttachment, storedAttachments, fmtBytes, kindLabel,
		type Attachment
	} from '#lib/attachments.js';

	type Msg = {
		id?: number; // id baris DB (untuk edit/regenerate)
		role: 'user' | 'assistant' | 'tool'; content: string; provider?: string;
		reasoning?: string; model?: string; ms?: number;
		thinkStart?: number; thinkMs?: number; thinkOpen?: boolean; stopped?: boolean;
		tools?: ToolUse[];
		file?: { name: string; url: string; size: number; type: DocType };
		building?: DocType; // dokumen sedang dirakit
		attachments?: Attachment[]; // lampiran pesan user
		trimmed?: { n: number; window: number }; // pesan lama yang dipangkas server
		doc?: { type: DocType; title?: string; start: number }; // dokumen yang dirakit dari jawaban
	};
	// satu pemanggilan tool dalam giliran assistant (dieksekusi server)
	type ToolUse = {
		id: string; name: string; args: string; round: number; status: 'run' | 'ok' | 'err'; result?: string;
		url?: string; query?: string; hits?: number; doc?: DocType;
	};

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

	const SUGGESTIONS = [
		{ title: 'Cari info terkini', text: 'Apa kabar terbaru soal harga beras di Indonesia? Sertakan sumbernya' },
		{ title: 'Buat presentasi', text: 'Buatkan ppt 6 slide tentang manfaat AI untuk UMKM' },
		{ title: 'Buat laporan Word', text: 'Buatkan laporan kegiatan pelatihan AI untuk dosen, lengkap dengan ringkasan eksekutif, tabel peserta, dan rekomendasi' },
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
		detach();
		const res = await api.post('/api/me/conversations', { title: t('new_chat'), model });
		current = res.conversation.id;
		msgs = [];
		showHistory = false;
		loadConversations();
		inputEl?.focus();
	}
	async function openConversation(id: number) {
		detach();
		current = id;
		try {
			const res = await api.get('/api/me/conversations/' + id);
			msgs = (res.messages || []).map((m: any) => {
				const meta = m.meta || {};
				return {
					id: m.id, role: m.role, content: m.content, provider: m.provider_name, model: m.model ? shortModel(m.model) : undefined,
					reasoning: meta.reasoning, thinkMs: meta.think_ms, tools: meta.tools, stopped: meta.stopped,
					attachments: meta.attachments,
					trimmed: meta.context_trimmed ? { n: meta.context_trimmed, window: meta.context_window } : undefined,
					doc: meta.doc
				};
			});
			showHistory = false;
			scrollToBottom(true);
			// jawaban masih diproses di server → sambung ulang & putar ulang dari awal
			if (res.active_run) {
				msgs.push({ role: 'assistant', content: '' });
				followRun(res.active_run, msgs[msgs.length - 1]);
			}
		} catch (e: any) { toast(e.message, 'err'); }
	}
	async function delConversation(id: number) {
		if (!confirm(t('confirm_delete'))) return;
		await api.del('/api/me/conversations/' + id);
		if (current === id) { detach(); current = null; msgs = []; }
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
	async function doExport(m: Msg, fmt: ExportFormat) {
		try { await exportAnswer(convTitle(), m.content, fmt); }
		catch (e: any) { toast('ekspor gagal: ' + e.message, 'err'); }
	}
	const CONV_EXPORTS: { fmt: ExportFormat; label: string; hint: string }[] = [
		{ fmt: 'md', label: 'MD', hint: 'Markdown seluruh percakapan' },
		{ fmt: 'docx', label: 'DOCX', hint: 'Dokumen Word seluruh percakapan' },
		{ fmt: 'pptx', label: 'PPTX', hint: 'Presentasi ringkas percakapan' },
		{ fmt: 'xlsx', label: 'XLSX', hint: 'Excel — transkrip + semua tabel' },
		{ fmt: 'csv', label: 'CSV', hint: 'CSV transkrip (Peran; Pesan)' }
	];
	async function doExportConv(fmt: ExportFormat) {
		const turns = msgs.filter((m) => m.content && m.role !== 'tool').map((m) => ({ role: m.role, content: m.content, model: m.model }));
		if (!turns.length) return;
		showExport = false;
		try { await exportConversation(convTitle(), turns, fmt); }
		catch (e: any) { toast('ekspor gagal: ' + e.message, 'err'); }
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
	// merakit file dari jawaban (bagian sejak b.doc.start) dengan generator lokal
	async function attachGeneratedFile(b: Msg) {
		if (!b.doc || b.building) return;
		const type = b.doc.type;
		const body = b.content.slice(b.doc.start).trim();
		if (!body) return;
		b.building = type;
		scrollToBottom();
		try {
			const title = firstH1(body) || b.doc.title || convTitle();
			// generator membaca judul (#) & subjudul dari markdown untuk sampul/slide judul
			const blob = await buildDocBlob(type, title, body);
			b.file = { name: docFileName(title, type), url: URL.createObjectURL(blob), size: blob.size, type };
		} catch (e: any) {
			toast('gagal merakit file: ' + e.message, 'err');
		} finally {
			b.building = undefined;
			scrollToBottom();
		}
	}

	// ---- mode "buat file" di composer: otomatis (deteksi dari prompt) atau paksa ----
	const FILE_MODES: { v: 'auto' | 'none' | DocType; label: string }[] = [
		{ v: 'auto', label: 'File: otomatis' },
		{ v: 'docx', label: 'Buat Word' },
		{ v: 'pptx', label: 'Buat PowerPoint' },
		{ v: 'xlsx', label: 'Buat Excel' },
		{ v: 'csv', label: 'Buat CSV' },
		{ v: 'none', label: 'Tanpa file' }
	];
	let fileMode = $state<'auto' | 'none' | DocType>('auto');
	const DOC_LABEL: Record<DocType, string> = { docx: 'dokumen Word', pptx: 'presentasi PowerPoint', xlsx: 'workbook Excel', csv: 'file CSV' };

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

	// ================= lampiran =================
	let pending = $state<Attachment[]>([]);
	let fileEl = $state<HTMLInputElement | null>(null);
	let dragging = $state(false);
	const modelInfo = $derived(models.find((x) => x.id === model));
	const readyAtts = $derived(pending.filter((a) => a.status === 'ready'));
	const readingAtts = $derived(pending.some((a) => a.status === 'reading'));

	function addFiles(files: Iterable<File>) {
		for (const file of files) {
			if (pending.length >= MAX_ATTACHMENTS) { toast(`maksimal ${MAX_ATTACHMENTS} lampiran per pesan`, 'err'); break; }
			const kind = kindOf(file);
			if (!kind) { toast(`${file.name}: jenis file tidak didukung`, 'err'); continue; }
			if (kind === 'image' && pending.filter((a) => a.kind === 'image').length >= MAX_IMAGES) {
				toast(`maksimal ${MAX_IMAGES} gambar per pesan`, 'err');
				continue;
			}
			pending.push({ id: crypto.randomUUID(), name: file.name, size: file.size, kind, status: 'reading' });
			const att = pending[pending.length - 1];
			readAttachment(file, kind)
				.then((r) => Object.assign(att, r, { status: 'ready' }))
				.catch((e) => Object.assign(att, { status: 'error', error: e?.message || String(e) }));
		}
		inputEl?.focus();
	}
	function removeAttachment(id: string) {
		pending = pending.filter((a) => a.id !== id);
	}
	function onPaste(e: ClipboardEvent) {
		const files = Array.from(e.clipboardData?.files ?? []);
		if (files.length) { e.preventDefault(); addFiles(files); }
	}
	function onDragOver(e: DragEvent) {
		if (!e.dataTransfer?.types.includes('Files')) return;
		e.preventDefault();
		dragging = true;
	}
	function onDragLeave(e: DragEvent) {
		if (!(e.currentTarget as HTMLElement).contains(e.relatedTarget as Node)) dragging = false;
	}
	function onDrop(e: DragEvent) {
		e.preventDefault();
		dragging = false;
		if (e.dataTransfer?.files.length) addFiles(e.dataTransfer.files);
	}

	// hentikan jawaban di server; event "done" (stopped) menyusul lewat stream
	async function stop() {
		if (!runId) return;
		try { await api.post(`/api/me/runs/${runId}/cancel`); }
		catch (e: any) { toast(e.message, 'err'); }
	}
	// lepas dari stream yang sedang diikuti (pindah percakapan); run tetap berjalan
	function detach() {
		abort?.abort();
	}

	async function send() {
		const text = input.trim();
		if ((!text && !readyAtts.length) || sending) return;
		if (readingAtts) { toast('tunggu lampiran selesai dibaca', 'err'); return; }
		if (readyAtts.some((a) => a.kind === 'image') && modelInfo?.vision === false) {
			toast(`model ${model} tidak bisa membaca gambar — pilih model vision atau hapus gambar`, 'err');
			return;
		}
		const atts = storedAttachments(pending);
		input = '';
		pending = [];
		await runTurn(text, atts);
	}

	// ================= edit & regenerate =================
	let editIdx = $state(-1);
	let editText = $state('');
	function startEdit(i: number) {
		editIdx = i;
		editText = msgs[i].content;
	}
	// resendFrom memotong riwayat mulai pesan user ke-i (lokal + DB) lalu
	// mengirim ulang dengan teks baru; lampiran pesan asli ikut terbawa
	async function resendFrom(i: number, text: string) {
		const u = msgs[i];
		if (sending || !u || u.role !== 'user') return;
		const atts = u.attachments ?? [];
		if (!text && !atts.length) return;
		if (i < msgs.length - 2 && !confirm('Pesan dan jawaban setelahnya akan dihapus. Lanjutkan?')) return;
		if (current) {
			if (!u.id) { toast('riwayat belum tersinkron — buka ulang percakapan lalu coba lagi', 'err'); return; }
			try { await api.del(`/api/me/conversations/${current}/messages?from=${u.id}`); }
			catch (e: any) { toast('gagal menghapus riwayat: ' + e.message, 'err'); return; }
		}
		msgs = msgs.slice(0, i);
		editIdx = -1;
		await runTurn(text, atts);
	}
	function regenerate(i: number) {
		for (let k = i - 1; k >= 0; k--) {
			if (msgs[k].role === 'user') { resendFrom(k, msgs[k].content); return; }
		}
	}

	// ================= run agen (server) =================
	// Loop model ↔ tool berjalan di server; browser hanya mengikuti event-nya.
	// Menutup tab tidak menghentikan jawaban: saat percakapan dibuka lagi UI
	// menyambung ulang (active_run) dan memutar ulang event dari awal.
	let runId: string | null = null;

	async function runTurn(text: string, atts: Attachment[]) {
		sending = true;
		// mode paksa / deteksi kata kunci → instruksi dokumen sejak awal; selain itu
		// model sendiri yang memutuskan lewat tool create_document
		const forcedDoc: DocType | null = fileMode === 'auto' ? detectDocRequest(text) : fileMode === 'none' ? null : fileMode;
		const docTool = fileMode !== 'none';
		fileMode = 'auto';
		if (!current) {
			try { await newConversation(); }
			catch (e: any) {
				toast('buat percakapan gagal: ' + e.message, 'err');
				sending = false;
				return;
			}
		}
		msgs.push({ role: 'user', content: text, attachments: atts.length ? atts : undefined });
		const userMsg = msgs[msgs.length - 1];
		scrollToBottom(true, true);
		let res: any;
		try {
			res = await api.post(`/api/me/conversations/${current}/runs`, {
				model, content: text, attachments: atts, forced_doc: forcedDoc ?? '', doc_tool: docTool, doc_prompts: DOC_SYSTEM
			});
		} catch (e: any) {
			// pesan belum tersimpan: kembalikan ke composer agar bisa dikirim ulang
			msgs.pop();
			input = text;
			pending = atts.map((a) => ({ ...a, status: 'ready' as const }));
			toast(e.message, 'err');
			sending = false;
			return;
		}
		userMsg.id = res.user_message_id;
		msgs.push({ role: 'assistant', content: '' });
		await followRun(res.run_id, msgs[msgs.length - 1]);
		loadQuota();
		loadConversations();
	}

	// followRun mengikuti event run sampai selesai; koneksi putus disambung
	// ulang dari event terakhir yang sudah diterima.
	async function followRun(id: string, bubble: Msg) {
		runId = id;
		sending = true;
		sendStart = performance.now();
		target = bubble;
		const ctrl = new AbortController();
		abort = ctrl;
		let seen = 0;
		let retries = 0;
		try {
			for (;;) {
				try {
					await api.runEvents(id, seen, ctrl.signal, (ev) => {
						seen++;
						retries = 0;
						applyEvent(bubble, ev);
					});
					break; // [END]: run selesai
				} catch (e: any) {
					if (ctrl.signal.aborted) return;
					if (e?.status === 404 || ++retries > 5) throw e;
					await new Promise((r) => setTimeout(r, 1000 * retries));
				}
			}
		} catch (e: any) {
			flushNow();
			bubble.content += '\n\n**⚠ ' + e.message + '**';
		} finally {
			flushNow();
			if (target === bubble) target = null;
			if (runId === id) {
				runId = null;
				abort = null;
				sending = false;
				inputEl?.focus();
			}
			scrollToBottom();
		}
	}

	// "local/qwen2.5-7b.gguf" → "qwen2.5-7b"
	const shortModel = (m: string) => m.split('/').pop()?.replace(/\.gguf$/i, '') || m;

	function applyEvent(b: Msg, ev: any) {
		switch (ev.t) {
			case 'text': queue('content', ev.v); break;
			case 'reasoning': queue('reasoning', ev.v); break;
			case 'model': b.model = shortModel(ev.model); break;
			case 'trimmed': b.trimmed = { n: ev.n, window: ev.window }; break;
			case 'tool':
				flushNow();
				b.tools = [...(b.tools ?? []), ev.tool];
				scrollToBottom();
				break;
			case 'tool_end': {
				const u = b.tools?.find((x) => x.id === ev.id);
				if (u) Object.assign(u, { status: ev.status, hits: ev.hits, result: ev.error });
				break;
			}
			case 'done':
				flushNow();
				b.content = b.content.trim();
				if (b.thinkStart && !b.thinkMs) b.thinkMs = performance.now() - b.thinkStart;
				b.id = ev.message_id;
				b.stopped = ev.stopped || undefined;
				if (ev.ms) b.ms = Math.round(ev.ms / 100) / 10;
				if (ev.doc) {
					b.doc = ev.doc;
					attachGeneratedFile(b);
				}
				break;
		}
	}

	const TOOL_ICONS: Record<string, any> = { web_search: Search, web_fetch: Globe, create_document: FileText };
	function chipText(u: ToolUse): string {
		const err = u.status === 'err', run = u.status === 'run';
		if (u.name === 'web_search') {
			const q = `"${u.query ?? ''}"`;
			return err ? `Gagal mencari ${q}` : run ? `Mencari ${q}` : `Dicari ${q} · ${u.hits ?? 0} hasil`;
		}
		if (u.name === 'create_document') {
			const what = u.doc && u.doc in DOC_LABEL ? DOC_LABEL[u.doc] : 'dokumen';
			return err ? 'Gagal menyiapkan dokumen' : `Menyiapkan ${what}`;
		}
		const target = u.url || u.name;
		return err ? `Gagal membaca ${target}` : run ? `Membaca ${target}` : `Dibaca ${target}`;
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

	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="chat-shell" ondragover={onDragOver} ondragleave={onDragLeave} ondrop={onDrop}>
		{#if dragging}
			<div class="drop-overlay" aria-hidden="true"><Upload size={26} /><b>Lepaskan file di sini</b><span>Gambar, PDF, Word, Excel, PowerPoint, atau file teks/kode</span></div>
		{/if}
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
						<p class="muted">Model bisa mencari dan membaca web, membuat dokumen Word/PowerPoint/Excel, dan menulis kode.</p>
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
							<div class="user-col">
								{#if m.attachments?.length}
									<div class="att-list sent">
										{#each m.attachments as a (a.id)}
											{#if a.kind === 'image' && a.dataUrl}
												<img class="att-thumb" src={a.dataUrl} alt={a.name} title={a.name} />
											{:else}
												<span class="att-chip" title={a.truncated ? 'Isi dipotong karena terlalu panjang' : a.name}>
													<FileText size={14} />
													<span class="att-name">{a.name}</span>
													<small>{kindLabel(a.kind)}{a.pages ? ` · ${a.pages} ${a.kind === 'pptx' ? 'slide' : a.kind === 'xlsx' ? 'sheet' : 'hlm'}` : ''}</small>
												</span>
											{/if}
										{/each}
									</div>
								{/if}
								{#if editIdx === i}
									<div class="edit-box">
										<!-- svelte-ignore a11y_autofocus -->
										<textarea bind:value={editText} rows="3" autofocus aria-label="Edit pesan"
											onkeydown={(e) => {
												if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) { e.preventDefault(); resendFrom(i, editText.trim()); }
												if (e.key === 'Escape') editIdx = -1;
											}}></textarea>
										<div class="edit-bar">
											<span class="muted">Jawaban setelah pesan ini akan dibuat ulang.</span>
											<button class="btn ghost sm" onclick={() => (editIdx = -1)}>Batal</button>
											<button class="btn sm" onclick={() => resendFrom(i, editText.trim())} disabled={!editText.trim() && !m.attachments?.length}>Kirim</button>
										</div>
									</div>
								{:else}
									{#if m.content}<div class="bubble">{m.content}</div>{/if}
									{#if !sending}
										<div class="user-actions">
											<button class="act" title="Salin pesan" onclick={() => copyMsg(m, i)}>
												{#if copiedIdx === i}<Check size={13} />{:else}<Copy size={13} />{/if}
											</button>
											<button class="act" title="Edit pesan & kirim ulang" onclick={() => startEdit(i)}><Pencil size={13} /></button>
										</div>
									{/if}
								{/if}
							</div>
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
											{@const TIcon = TOOL_ICONS[tl.name] ?? Globe}
											<span class="tool-chip" class:err={tl.status === 'err'} title={tl.status === 'err' ? tl.result : tl.args}>
												<TIcon size={12} />
												{#if tl.status === 'run'}<span class="shimmer">{chipText(tl)}</span>{:else}{chipText(tl)}{/if}
											</span>
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

								{#if m.building}
									{@const BIcon = FILE_ICONS[m.building]}
									<div class="file-card building" aria-live="polite">
										<BIcon size={18} />
										<span class="grow"><b class="shimmer">Menyusun {DOC_LABEL[m.building]}…</b><small>sampul, tata letak, dan gaya diterapkan otomatis</small></span>
										<span class="spinner dark"></span>
									</div>
								{/if}
								{#if m.doc && !m.file && !m.building && !live}
									{@const DIcon = FILE_ICONS[m.doc.type]}
									<button class="file-card make" onclick={() => attachGeneratedFile(m)}>
										<DIcon size={18} />
										<span class="grow"><b>Buat {DOC_LABEL[m.doc.type]}</b><small>rakit file dari jawaban ini</small></span>
										<span class="dl"><Download size={15} /></span>
									</button>
								{/if}
								{#if m.file}
									{@const FileIcon = FILE_ICONS[m.file.type]}
									<a class="file-card" href={m.file.url} download={m.file.name}>
										<FileIcon size={18} />
										<span class="grow"><b>{m.file.name}</b><small>{fmtSize(m.file.size)} — {t('file_ready')}</small></span>
										<span class="dl"><Download size={15} /></span>
									</a>
								{/if}

								{#if m.trimmed && !live}
									<div class="trim-note" title="Riwayat percakapan melebihi context window model, jadi pesan paling awal tidak dikirim. Mulai chat baru untuk topik lain.">
										<Info size={12} /> {m.trimmed.n} pesan awal tidak dikirim ke model karena batas konteks ({m.trimmed.window.toLocaleString('id-ID')} token)
									</div>
								{/if}
								{#if m.content && !live}
									<div class="actions">
										<button class="act" title="Salin jawaban" onclick={() => copyMsg(m, i)}>
											{#if copiedIdx === i}<Check size={14} />{:else}<Copy size={14} />{/if}
										</button>
										{#if i === lastIdx && !sending}
											<button class="act" title="Buat ulang jawaban" onclick={() => regenerate(i)}><RotateCcw size={14} /></button>
										{/if}
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
				{#if pending.length}
					<div class="att-list">
						{#each pending as a (a.id)}
							<span class="att-chip" class:err={a.status === 'error'} class:img={a.kind === 'image' && a.dataUrl}
								title={a.status === 'error' ? `${a.name}: ${a.error}` : a.truncated ? `${a.name} — isi dipotong karena terlalu panjang` : a.name}>
								{#if a.kind === 'image' && a.dataUrl}
									<img src={a.dataUrl} alt="" />
								{:else if a.status === 'reading'}
									<span class="spinner dark"></span>
								{:else if a.kind === 'image'}
									<ImageIcon size={14} />
								{:else}
									<FileText size={14} />
								{/if}
								<span class="att-name">{a.name}</span>
								<small>
									{#if a.status === 'reading'}membaca…{:else if a.status === 'error'}gagal{:else}{fmtBytes(a.size)}{a.pages ? ` · ${a.pages} ${a.kind === 'pptx' ? 'slide' : a.kind === 'xlsx' ? 'sheet' : 'hlm'}` : ''}{a.truncated ? ' · dipotong' : ''}{/if}
								</small>
								<button class="att-x" onclick={() => removeAttachment(a.id)} aria-label={'hapus ' + a.name}><X size={12} /></button>
							</span>
						{/each}
					</div>
				{/if}
				<textarea
					bind:this={inputEl}
					bind:value={input}
					rows="1"
					placeholder="Kirim pesan…"
					aria-label="Pesan"
					onpaste={onPaste}
					onkeydown={(e) => {
						if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) { e.preventDefault(); send(); }
					}}
				></textarea>
				<div class="composer-bar">
					<input bind:this={fileEl} type="file" multiple accept={ACCEPT} hidden
						onchange={(e) => { const el = e.currentTarget; if (el.files) addFiles(el.files); el.value = ''; }} />
					<button class="icon-round" onclick={() => fileEl?.click()} title="Lampirkan file (gambar, PDF, Word, Excel, PowerPoint, teks/kode) — bisa juga seret atau tempel" aria-label="Lampirkan file">
						<Paperclip size={16} />
					</button>
					<select bind:value={model} class="model-select" aria-label="Model">
						{#each models as mo (mo.id)}
							<option value={mo.id}>{mo.id}{mo.local ? ' · lokal' : ''}</option>
						{/each}
					</select>
					<select bind:value={fileMode} class="model-select file-mode" class:forced={fileMode !== 'auto'} aria-label="Buat file dari jawaban"
						title="Otomatis: file dibuat bila pesan meminta dokumen/presentasi/excel">
						{#each FILE_MODES as f (f.v)}<option value={f.v}>{f.label}</option>{/each}
					</select>
					<span class="tool-badge" title="Model bisa mencari di web (web_search), membaca halaman (web_fetch), dan membuat file (create_document)"><Globe size={12} /> web</span>
					<span class="grow"></span>
					{#if sending}
						<button class="send stop" onclick={stop} title="Hentikan jawaban" aria-label="Hentikan"><Square size={13} fill="currentColor" /></button>
					{:else}
						<button class="send" onclick={send} disabled={(!input.trim() && !readyAtts.length) || readingAtts} title={t('send')} aria-label={t('send')}><ArrowUp size={17} /></button>
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
	/* lebar kolom baca: lega untuk tabel & kode, tetap nyaman dibaca */
	.thread { --col: 960px; max-width: var(--col); margin: 0 auto; padding: 26px 28px 40px; }

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
		max-width: min(80%, 760px); padding: 10px 16px; border-radius: 20px 20px 6px 20px;
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
	.tool-chip.err { color: var(--err); border-color: color-mix(in srgb, var(--err) 35%, var(--border)); }

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
		padding: 8px 12px; border-bottom: 1px solid var(--border); text-align: left; vertical-align: top;
		overflow-wrap: normal; word-break: normal; /* tabel melebar & digulir, kata tidak dipotong */
	}
	.answer :global(th) { background: var(--surface-2); font-weight: 700; }
	.answer :global(tr:last-child td) { border-bottom: 0; }
	/* kode multi-baris di dalam sel tabel (model menulis <br>/&nbsp;) */
	.answer :global(.cell-code) {
		margin: 0; padding: 6px 8px; border-radius: 8px; background: var(--bg); border: 1px solid var(--border);
		white-space: pre; overflow-x: auto; max-width: 520px;
	}
	.answer :global(.cell-code code) { background: none; border: 0; padding: 0; font-size: 12px; line-height: 1.55; }
	@media (min-width: 1500px) {
		.thread { --col: 1080px; }
		.composer { max-width: 1024px; }
	}

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
	button.file-card { font: inherit; cursor: pointer; text-align: left; width: 100%; }
	.file-card:hover { border-color: var(--accent); transform: translateY(-1px); }
	.file-card b { font-size: 12.5px; display: block; word-break: break-all; }
	.file-card small { color: var(--muted); font-size: 11px; }
	.file-card .grow { min-width: 0; }
	.file-card .dl { margin-left: auto; color: var(--accent-2); flex-shrink: 0; }
	.file-card.building { border-style: dashed; cursor: default; }
	.file-card.building:hover { transform: none; }
	.file-mode { max-width: 150px; }
	.file-mode.forced { border-color: var(--accent); color: var(--accent-2); }

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
		max-width: 904px; margin: 0 auto; border: 1px solid var(--border-strong); border-radius: 22px;
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
	.composer-bar { display: flex; align-items: center; gap: 8px; margin-top: 6px; min-width: 0; }
	/* dropdown boleh menyusut (teks terpotong) — tombol kirim tidak pernah terdorong keluar */
	.composer-bar .model-select { flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.composer-bar .send, .composer-bar .icon-round, .composer-bar .tool-badge { flex-shrink: 0; }
	.tool-badge { white-space: nowrap; }
	.icon-round {
		all: unset; cursor: pointer; width: 30px; height: 30px; border-radius: 50%; display: grid; place-items: center;
		color: var(--muted); transition: background var(--speed), color var(--speed);
	}
	.icon-round:hover { background: var(--surface-2); color: var(--text); }

	/* ---------- lampiran ---------- */
	.att-list { display: flex; flex-wrap: wrap; gap: 6px; margin: 2px 0 8px; }
	.att-list.sent { justify-content: flex-end; margin: 0 0 6px; }
	.att-chip {
		display: inline-flex; align-items: center; gap: 7px; max-width: 280px; padding: 5px 6px 5px 9px;
		border-radius: 12px; font-size: 12.5px; background: var(--surface-2); border: 1px solid var(--border);
	}
	.att-list.sent .att-chip { padding-right: 10px; }
	.att-chip.err { color: var(--err); border-color: color-mix(in srgb, var(--err) 35%, var(--border)); }
	.att-chip img { width: 28px; height: 28px; object-fit: cover; border-radius: 6px; }
	.att-chip .spinner { width: 13px; height: 13px; }
	.att-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 600; }
	.att-chip small { color: var(--muted); white-space: nowrap; font-size: 11px; }
	.att-x { all: unset; cursor: pointer; display: grid; place-items: center; width: 20px; height: 20px; border-radius: 50%; color: var(--muted); }
	.att-x:hover { background: var(--surface); color: var(--text); }
	.att-thumb { max-width: 220px; max-height: 180px; border-radius: 12px; border: 1px solid var(--border); object-fit: cover; }
	/* lebar dibatasi di kolom (bukan bubble) agar bubble pendek tidak terlipat */
	.user-col { display: flex; flex-direction: column; align-items: flex-end; max-width: min(80%, 760px); min-width: 0; }
	.user-col .bubble { max-width: 100%; }
	.user-actions { display: flex; gap: 2px; margin-top: 4px; opacity: 0; transition: opacity var(--speed); }
	.turn.user:hover .user-actions, .user-actions:focus-within { opacity: 1; }
	@media (hover: none) { .user-actions { opacity: 1; } }
	.edit-box { width: min(760px, 100%); border: 1px solid var(--border-strong); border-radius: 16px; background: var(--surface); padding: 10px 12px; }
	.edit-box textarea { all: unset; display: block; width: 100%; min-height: 60px; white-space: pre-wrap; font-size: 14.5px; line-height: 1.55; }
	.edit-bar { display: flex; align-items: center; gap: 6px; margin-top: 8px; }
	.edit-bar .muted { flex: 1; font-size: 11.5px; }
	.trim-note { display: flex; align-items: center; gap: 6px; font-size: 11.5px; color: var(--muted); margin: 8px 0 0; }
	.drop-overlay {
		position: absolute; inset: 8px; z-index: 30; border-radius: 14px; pointer-events: none;
		display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 6px;
		border: 2px dashed color-mix(in srgb, var(--accent) 60%, transparent);
		background: color-mix(in srgb, var(--surface) 88%, transparent); color: var(--text); font-size: 13px;
	}
	.drop-overlay span { color: var(--muted); font-size: 12px; }
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
		.bubble, .user-col { max-width: 88%; }
		.user-col .bubble { max-width: 100%; }
		.avatar { display: none; }
		.composer-wrap { padding: 0 8px max(8px, env(safe-area-inset-bottom)); }
		.composer { padding: 8px 8px 6px 12px; border-radius: 20px; }
		.composer-bar { gap: 6px; }
		.model-select { max-width: 46%; }
		.file-mode { max-width: 34%; }
		.tool-badge { display: none; }
		/* < 16px membuat iOS Safari memperbesar halaman saat input difokus */
		.composer textarea, .edit-box textarea { font-size: 16px; }
		.att-chip { max-width: 100%; }
		.att-thumb { max-width: 160px; max-height: 140px; }
		.jump { bottom: 120px; }
		.hint { display: none; }
	}
</style>
