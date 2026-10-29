// deck.ts — presentasi PowerPoint 16:9 berdesain dari markdown jawaban model
// (pptxgenjs, dimuat dinamis). Tata letak dipilih otomatis per slide:
// judul, pembatas bagian, poin (1/2 kolom), kartu statistik, tabel, kutipan,
// kode, dan penutup. Catatan pembicara dari baris "Catatan pembicara: …".
import { parseMarkdown, inlineRuns, plain, type Block } from '../exporters.js';

const W = 13.333;
const H = 7.5;
const FONT = 'Calibri';
const MONO = 'Consolas';
const C = {
	navy: '0F1E46',
	navy2: '1B2B5E',
	accent: '2F5BEA',
	cyan: '22B8F0',
	ink: '1F2937',
	muted: '64748B',
	soft: 'EEF3FF',
	paper: 'FFFFFF',
	light: 'F5F7FB',
	line: 'D9E0EC'
};

type SlideSpec = { title: string; blocks: Block[]; notes: string[] };

// statistik = nilai pendek yang MENGANDUNG angka di dalam ** ** (mis. **72%**, **3,4 jt**)
const STAT_RE = /^\*\*([^*]{0,6}\d[^*]{0,8})\*\*\s*[—–:-]\s*(.+)$/;
const isStatItem = (b: Block) => b.kind === 'bullet' && STAT_RE.test(b.text.trim());
const CLOSING_RE = /^(terima\s*kasih|thank\s*you|tanya\s*jawab|q\s*&\s*a|sesi\s+tanya)/i;
const NOTE_RE = /^(catatan\s+pembicara|speaker\s+notes?|notes?)\s*:\s*/i;

const SLIDE_PREFIX = /^(?:slide|salindia|halaman)\s*\d+\s*[:.\-–—]\s*/i;
const MARKER_RE = /^\*\*\s*(?:(?:slide|salindia)\s*\d+\s*[:.\-–—]\s*)?([^*]{2,90}?)\s*\*\*\s*:?$/i;
const PLAIN_MARKER_RE = /^(?:slide|salindia)\s*\d+\s*[:.\-–—]\s*(.{2,90})$/i;

/**
 * Normalisasi struktur jawaban model yang tidak baku agar tetap jadi slide:
 * heading level berapa pun (##, ###, # berulang), prefiks "Slide 1:",
 * baris "**Slide 1: Judul**", atau poin tanpa heading sama sekali.
 */
function normalizeSlides(blocks: Block[]): { title: string | null; subtitle: string; blocks: Block[] } {
	let title: string | null = null;
	let subtitle = '';
	const bs = [...blocks];
	const firstH = bs.findIndex((b) => b.kind === 'heading');
	const h1s = bs.filter((b) => b.kind === 'heading' && b.level === 1).length;
	// judul deck: H1 pertama bila hanya satu H1 (H1 berulang = tiap slide)
	if (firstH >= 0 && (bs[firstH] as any).level === 1 && h1s === 1) {
		title = plain((bs[firstH] as any).text);
		bs.splice(firstH, 1);
		const nx = bs[firstH] as Block | undefined;
		if (nx?.kind === 'para' && nx.text.length < 200 && !MARKER_RE.test(nx.text.trim()) && !PLAIN_MARKER_RE.test(nx.text.trim())) {
			subtitle = plain(nx.text.replace(/^[*_]|[*_]$/g, ''));
			bs.splice(firstH, 1);
		}
	}
	// tanpa "# Judul": kalimat pendek pembuka dipakai sebagai judul deck
	if (title === null && bs[0]?.kind === 'para') {
		const t = (bs[0] as any).text.trim() as string;
		if (t.length <= 80 && !MARKER_RE.test(t) && !PLAIN_MARKER_RE.test(t)) {
			title = plain(t.replace(/[:.]$/, ''));
			bs.shift();
		}
	}
	let levels = bs.filter((b) => b.kind === 'heading').map((b) => (b as any).level as number);
	if (levels.length === 0) {
		// tanpa heading: baris penanda tebal / "Slide N:" dijadikan heading
		for (let i = 0; i < bs.length; i++) {
			const b = bs[i];
			if (b.kind !== 'para') continue;
			const t = b.text.trim();
			const m = t.match(MARKER_RE) || t.match(PLAIN_MARKER_RE);
			if (m) bs[i] = { kind: 'heading', level: 2, text: m[1].trim() };
		}
		levels = bs.filter((b) => b.kind === 'heading').map((b) => (b as any).level as number);
	}
	const slideLevel = levels.length ? Math.min(...levels) : 0;
	const out: Block[] = bs.map((b) => {
		if (b.kind !== 'heading') return b;
		if (b.level === slideLevel) return { kind: 'heading', level: 2, text: b.text.replace(SLIDE_PREFIX, '').trim() || b.text };
		// heading lebih dalam = sub-judul di dalam slide
		return { kind: 'heading', level: 3, text: b.text };
	});
	return { title, subtitle, blocks: out };
}

/** pecah markdown jadi judul deck + daftar slide */
export function planDeck(fallbackTitle: string, md: string) {
	const norm = normalizeSlides(parseMarkdown(md));
	const title = norm.title ?? fallbackTitle;
	const subtitle = norm.subtitle;
	const slides: SlideSpec[] = [];
	let cur: SlideSpec | null = null;
	const loose: Block[] = []; // isi sebelum slide pertama
	for (const b of norm.blocks) {
		if (b.kind === 'heading' && b.level === 2) {
			cur = { title: plain(b.text), blocks: [], notes: [] };
			slides.push(cur);
			continue;
		}
		if (b.kind === 'para' && NOTE_RE.test(b.text)) {
			if (cur) cur.notes.push(plain(b.text.replace(NOTE_RE, '')));
			continue;
		}
		if (b.kind === 'heading') {
			// sub-judul di dalam slide → poin tebal
			(cur ? cur.blocks : loose).push({ kind: 'bullet', level: 0, text: '**' + plain(b.text) + '**' });
			continue;
		}
		(cur ? cur.blocks : loose).push(b);
	}
	// isi tanpa slide (model tidak memakai heading): pecah per 5 butir
	if (loose.length) {
		const chunks: Block[][] = [];
		for (const b of loose) {
			const last = chunks[chunks.length - 1];
			if (!last || last.filter((x) => x.kind !== 'table').length >= 5 || b.kind === 'table') chunks.push([b]);
			else last.push(b);
		}
		const pre = chunks.map((blocks, i) => ({
			title: chunks.length > 1 ? `${title} (${i + 1}/${chunks.length})` : title,
			blocks,
			notes: []
		}));
		slides.unshift(...pre);
	}
	return { title, subtitle, slides };
}

type PptxRun = { text: string; options?: Record<string, unknown> };

/** run inline (tebal/miring/kode) → run pptxgenjs */
function runsOf(text: string, base: Record<string, unknown> = {}): PptxRun[] {
	return inlineRuns(text).map((r) => ({
		text: r.text,
		options: { ...base, bold: r.bold || (base.bold as boolean) || undefined, italic: r.italic || undefined, fontFace: r.code ? MONO : undefined, color: r.link ? C.accent : base.color }
	}));
}

/** daftar poin → satu kotak teks berpoin (paragraf dipisah breakLine) */
function bulletRuns(items: Block[], size: number): PptxRun[] {
	const out: PptxRun[] = [];
	items.forEach((b, idx) => {
		const text = b.kind === 'bullet' || b.kind === 'para' || b.kind === 'quote' ? b.text : '';
		const level = b.kind === 'bullet' ? b.level : 0;
		const rs = runsOf(text, { fontSize: level ? size - 3 : size, color: level ? C.muted : C.ink });
		if (!rs.length) return;
		// opsi paragraf HANYA di run pertama (bullet di run lain memecah paragraf);
		// <a:pPr> ganda yang ditulis pptxgenjs per run dibereskan di fixParagraphProps
		const first = rs[0].options!;
		first.paraSpaceAfter = Math.round(size * 0.75); // poin sedikit → lebih lega
		if (b.kind === 'bullet') {
			first.bullet = b.ordered ? { type: 'number' } : { code: '25CF' };
			first.indentLevel = level;
		}
		if (idx < items.length - 1) rs[rs.length - 1].options = { ...rs[rs.length - 1].options, breakLine: true };
		out.push(...rs);
	});
	return out;
}

export async function buildDeckPptx(fallbackTitle: string, md: string): Promise<Blob> {
	const { default: PptxGen } = await import('pptxgenjs');
	const pptx = new PptxGen();
	pptx.layout = 'LAYOUT_WIDE';
	const deck = planDeck(fallbackTitle, md);
	pptx.title = deck.title;
	pptx.company = 'JenderalRouter';

	const total = deck.slides.length + 1;
	let sectionNo = 0;

	// ---- master: slide isi ----
	pptx.defineSlideMaster({
		title: 'ISI',
		background: { color: C.paper },
		objects: [
			{ rect: { x: 0, y: 0, w: 0.16, h: H, fill: { color: C.accent } } },
			{ line: { x: 0.6, y: H - 0.55, w: W - 1.2, h: 0, line: { color: C.line, width: 0.75 } } },
			{ text: { text: deck.title, options: { x: 0.6, y: H - 0.5, w: 8, h: 0.35, fontFace: FONT, fontSize: 10, color: C.muted } } }
		],
		slideNumber: { x: W - 1.2, y: H - 0.5, w: 0.6, h: 0.35, fontFace: FONT, fontSize: 10, color: C.muted, align: 'right' }
	});

	const header = (s: any, title: string) => {
		s.addShape(pptx.ShapeType.rect, { x: 0.6, y: 0.5, w: 0.7, h: 0.07, fill: { color: C.cyan }, line: { color: C.cyan } });
		s.addText(title, { x: 0.6, y: 0.62, w: W - 1.2, h: 0.9, fontFace: FONT, fontSize: 30, bold: true, color: C.navy, valign: 'top', fit: 'shrink' });
	};

	// ---- slide judul ----
	{
		const s = pptx.addSlide();
		s.background = { color: C.navy };
		s.addShape(pptx.ShapeType.ellipse, { x: 8.6, y: -1.6, w: 7, h: 7, fill: { color: C.navy2 }, line: { color: C.navy2 } });
		s.addShape(pptx.ShapeType.ellipse, { x: 10.6, y: 3.9, w: 4.2, h: 4.2, fill: { color: C.accent, transparency: 35 }, line: { color: C.accent, transparency: 35 } });
		s.addShape(pptx.ShapeType.rect, { x: 0.9, y: 2.15, w: 1.1, h: 0.09, fill: { color: C.cyan }, line: { color: C.cyan } });
		s.addText(deck.title, { x: 0.9, y: 2.4, w: 9.4, h: 2.0, fontFace: FONT, fontSize: 44, bold: true, color: 'FFFFFF', valign: 'top', fit: 'shrink' });
		if (deck.subtitle)
			s.addText(deck.subtitle, { x: 0.9, y: 4.45, w: 9.0, h: 0.9, fontFace: FONT, fontSize: 20, color: 'B9C7F0', valign: 'top', fit: 'shrink' });
		const today = new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'long', year: 'numeric' }).format(new Date());
		s.addText(today, { x: 0.9, y: 6.3, w: 6, h: 0.4, fontFace: FONT, fontSize: 13, color: '8FA2D8' });
	}

	for (const spec of deck.slides) {
		const bs = spec.blocks;
		const tables = bs.filter((b) => b.kind === 'table') as Extract<Block, { kind: 'table' }>[];
		const quotes = bs.filter((b) => b.kind === 'quote');
		const codes = bs.filter((b) => b.kind === 'code') as Extract<Block, { kind: 'code' }>[];
		const textual = bs.filter((b) => b.kind === 'bullet' || b.kind === 'para' || (b.kind === 'heading' && b.level > 2));
		const stats = bs.filter(isStatItem);
		let s: any;

		if (bs.length === 0 || CLOSING_RE.test(spec.title)) {
			// ---- pembatas bagian / penutup ----
			const closing = CLOSING_RE.test(spec.title);
			s = pptx.addSlide();
			s.background = { color: closing ? C.navy : C.accent };
			s.addShape(pptx.ShapeType.ellipse, { x: -2.2, y: 3.6, w: 6, h: 6, fill: { color: 'FFFFFF', transparency: 92 }, line: { color: 'FFFFFF', transparency: 92 } });
			if (!closing) {
				sectionNo++;
				s.addText(String(sectionNo).padStart(2, '0'), { x: 0.9, y: 1.9, w: 3, h: 1.2, fontFace: FONT, fontSize: 64, bold: true, color: 'BFE9FB' });
			}
			s.addText(spec.title, { x: 0.9, y: closing ? 2.6 : 3.1, w: 11.2, h: 1.4, fontFace: FONT, fontSize: closing ? 48 : 40, bold: true, color: 'FFFFFF', valign: 'top', fit: 'shrink' });
			if (textual.length)
				s.addText(textual.map((b) => plain((b as any).text)).join('\n'), { x: 0.9, y: 4.4, w: 11, h: 1.8, fontFace: FONT, fontSize: 18, color: 'D6E0FF', valign: 'top', fit: 'shrink' });
		} else if (stats.length >= 2 && stats.length <= 4 && stats.length >= textual.length - 1) {
			// ---- kartu angka ----
			s = pptx.addSlide({ masterName: 'ISI' });
			header(s, spec.title);
			const n = stats.length;
			const gap = 0.35;
			const cw = (W - 1.2 - gap * (n - 1)) / n;
			stats.forEach((b, i) => {
				const m = (b as any).text.trim().match(STAT_RE);
				const x = 0.6 + i * (cw + gap);
				s.addShape(pptx.ShapeType.roundRect, { x, y: 2.0, w: cw, h: 3.9, rectRadius: 0.12, fill: { color: C.soft }, line: { color: C.soft } });
				s.addShape(pptx.ShapeType.rect, { x, y: 2.0, w: cw, h: 0.1, fill: { color: i % 2 ? C.cyan : C.accent }, line: { color: i % 2 ? C.cyan : C.accent } });
				s.addText(plain(m[1]), { x: x + 0.3, y: 2.4, w: cw - 0.6, h: 1.4, fontFace: FONT, fontSize: 48, bold: true, color: C.accent, fit: 'shrink' });
				s.addText(plain(m[2]), { x: x + 0.3, y: 3.85, w: cw - 0.6, h: 1.9, fontFace: FONT, fontSize: 16, color: C.ink, valign: 'top', fit: 'shrink' });
			});
			const rest = textual.filter((b) => !isStatItem(b));
			if (rest.length) s.addText(runsOf(plain((rest[0] as any).text), { fontSize: 14, color: C.muted }), { x: 0.6, y: 6.15, w: W - 1.2, h: 0.5, fontFace: FONT });
		} else if (tables.length) {
			// ---- tabel ----
			s = pptx.addSlide({ masterName: 'ISI' });
			header(s, spec.title);
			const t = tables[0];
			const cols = Math.max(t.header.length, ...t.rows.map((r) => r.length));
			const rows = t.rows.slice(0, 9);
			const weight = Array.from({ length: cols }, (_, c) => Math.max(4, Math.min(36, Math.max(plain(t.header[c] ?? '').length, ...rows.map((r) => plain(r[c] ?? '').length)))));
			const sum = weight.reduce((a, b) => a + b, 0);
			const tw = W - 1.2;
			const fs = rows.length > 6 ? 13 : 15;
			let y = 1.65;
			if (textual.length) {
				s.addText(runsOf(plain((textual[0] as any).text), { fontSize: 15, color: C.muted }), { x: 0.6, y, w: tw, h: 0.5, fontFace: FONT });
				y += 0.55;
			}
			s.addTable(
				[
					t.header.concat(Array(cols - t.header.length).fill('')).map((h) => ({ text: plain(h), options: { bold: true, color: 'FFFFFF', fill: { color: C.navy } } })),
					...rows.map((r, ri) => Array.from({ length: cols }, (_, c) => ({ text: plain(r[c] ?? ''), options: { fill: { color: ri % 2 ? 'F3F6FB' : 'FFFFFF' } } })))
				],
				{ x: 0.6, y, w: tw, colW: weight.map((w) => (w / sum) * tw), fontFace: FONT, fontSize: fs, color: C.ink, border: { type: 'solid', pt: 0.75, color: C.line }, margin: 0.08, valign: 'middle', autoPage: false }
			);
			if (t.rows.length > rows.length)
				s.addText(`… ${t.rows.length - rows.length} baris lainnya ada di lampiran/dokumen`, { x: 0.6, y: H - 0.95, w: tw, h: 0.35, fontFace: FONT, fontSize: 11, italic: true, color: C.muted });
		} else if (quotes.length && textual.length <= 1 && !codes.length) {
			// ---- kutipan ----
			s = pptx.addSlide({ masterName: 'ISI' });
			s.background = { color: C.light };
			s.addText('“', { x: 0.9, y: 0.6, w: 2, h: 2, fontFace: 'Georgia', fontSize: 140, color: C.accent, bold: true });
			const q = plain((quotes[0] as any).text);
			const [quoteText, author] = q.split(/\s+[—–]\s+(?=[^—–]+$)/);
			s.addText(quoteText, { x: 1.6, y: 2.0, w: 10.2, h: 2.8, fontFace: FONT, fontSize: 30, italic: true, color: C.navy, valign: 'middle', fit: 'shrink' });
			if (author) s.addText('— ' + author, { x: 1.6, y: 4.9, w: 10.2, h: 0.5, fontFace: FONT, fontSize: 18, color: C.muted });
			s.addText(spec.title, { x: 1.6, y: 5.6, w: 10.2, h: 0.5, fontFace: FONT, fontSize: 14, bold: true, color: C.accent });
		} else {
			// ---- poin (1 atau 2 kolom) + kode opsional ----
			s = pptx.addSlide({ masterName: 'ISI' });
			header(s, spec.title);
			const items = textual.concat(quotes);
			const top = 1.75;
			const bottom = H - 0.75;
			const hasCode = codes.length > 0;
			const twoCol = !hasCode && items.length > 6;
			const size = items.length <= 3 ? 26 : items.length <= 4 ? 24 : items.length <= 6 ? 21 : 18;
			if (twoCol) {
				const half = Math.ceil(items.length / 2);
				const cw = (W - 1.2 - 0.5) / 2;
				s.addText(bulletRuns(items.slice(0, half), size), { x: 0.6, y: top, w: cw, h: bottom - top, fontFace: FONT, valign: 'top', fit: 'shrink' });
				s.addText(bulletRuns(items.slice(half), size), { x: 0.6 + cw + 0.5, y: top, w: cw, h: bottom - top, fontFace: FONT, valign: 'top', fit: 'shrink' });
			} else {
				const textW = hasCode ? 5.6 : W - 1.2;
				if (items.length) s.addText(bulletRuns(items, size), { x: 0.6, y: top, w: textW, h: bottom - top, fontFace: FONT, valign: 'top', fit: 'shrink' });
				if (hasCode) {
					const code = codes[0].text.split('\n').slice(0, 18).join('\n');
					const cx = items.length ? 6.5 : 0.6;
					const cw = W - 0.6 - cx;
					s.addShape(pptx.ShapeType.roundRect, { x: cx, y: top, w: cw, h: bottom - top - 0.1, rectRadius: 0.08, fill: { color: '0B1220' }, line: { color: '0B1220' } });
					s.addText(code, { x: cx + 0.25, y: top + 0.2, w: cw - 0.5, h: bottom - top - 0.5, fontFace: MONO, fontSize: 13, color: 'D7E3FF', valign: 'top', fit: 'shrink' });
				}
			}
		}
		if (spec.notes.length) s.addNotes(spec.notes.join('\n'));
	}
	void total;
	const raw = (await pptx.write({ outputType: 'arraybuffer' })) as ArrayBuffer;
	return fixParagraphProps(raw);
}

/**
 * pptxgenjs menulis satu <a:pPr> untuk SETIAP run dalam paragraf; run ke-2 dst
 * tanpa bullet memberi <a:buNone/> yang menimpa bullet, dan pPr ganda tidak
 * sah menurut skema (PowerPoint bisa meminta "repair"). Sisakan pPr pertama.
 */
export async function fixParagraphProps(buf: ArrayBuffer): Promise<Blob> {
	const { default: JSZip } = await import('jszip');
	const zip = await JSZip.loadAsync(buf);
	const pPr = /<a:pPr\b[^>]*\/>|<a:pPr\b[^>]*>[\s\S]*?<\/a:pPr>/g;
	for (const name of Object.keys(zip.files)) {
		if (!/^ppt\/(slides|slideMasters|slideLayouts|notesSlides)\/[^/]+\.xml$/.test(name)) continue;
		const xml = await zip.file(name)!.async('string');
		const fixed = xml.replace(/<a:p>([\s\S]*?)<\/a:p>/g, (_, inner: string) => {
			let seen = false;
			return '<a:p>' + inner.replace(pPr, (m) => (seen ? '' : ((seen = true), m))) + '</a:p>';
		});
		if (fixed !== xml) zip.file(name, fixed);
	}
	const out = await zip.generateAsync({ type: 'arraybuffer', compression: 'DEFLATE' });
	return new Blob([out], { type: 'application/vnd.openxmlformats-officedocument.presentationml.presentation' });
}
