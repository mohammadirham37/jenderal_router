// attachments.ts — lampiran chat: gambar (vision) dan dokumen yang teksnya
// diekstrak DI BROWSER (PDF via pdf.js, DOCX/XLSX/PPTX via unzip XML, teks
// polos apa adanya). Server tidak menyimpan file mentah; teks hasil ekstraksi
// dan gambar yang sudah dikecilkan ikut tersimpan di meta pesan user.
import JSZip from 'jszip';

export type AttachmentKind = 'image' | 'pdf' | 'docx' | 'xlsx' | 'pptx' | 'text';

export type Attachment = {
	id: string;
	name: string;
	size: number;
	kind: AttachmentKind;
	dataUrl?: string; // gambar (sudah dikecilkan)
	text?: string; // teks hasil ekstraksi dokumen
	pages?: number; // halaman PDF / slide / sheet
	truncated?: boolean;
	status?: 'reading' | 'ready' | 'error';
	error?: string;
};

export const MAX_ATTACHMENTS = 6;
export const MAX_IMAGES = 4;
export const MAX_FILE_BYTES = 20 << 20;
const MAX_TEXT_CHARS = 60_000; // per file — hemat konteks model
const MAX_IMAGE_SIDE = 1568; // sisi terpanjang gambar yang dikirim ke model
const MAX_PDF_PAGES = 300;

const TEXT_EXT = new Set([
	'txt', 'md', 'markdown', 'csv', 'tsv', 'json', 'xml', 'html', 'htm', 'yaml', 'yml', 'toml', 'ini', 'log', 'sql',
	'go', 'py', 'js', 'mjs', 'ts', 'tsx', 'jsx', 'svelte', 'vue', 'java', 'kt', 'php', 'rb', 'rs', 'c', 'h', 'cpp', 'hpp',
	'cs', 'swift', 'sh', 'bash', 'zsh', 'ps1', 'css', 'scss', 'dart', 'lua', 'r', 'tex', 'srt', 'env', 'gitignore', 'dockerfile'
]);

// atribut accept untuk <input type="file">
export const ACCEPT = 'image/png,image/jpeg,image/webp,image/gif,.pdf,.docx,.xlsx,.pptx,' + [...TEXT_EXT].map((e) => '.' + e).join(',');

export function kindOf(file: File): AttachmentKind | null {
	const ext = file.name.toLowerCase().split('.').pop() ?? '';
	if (/^image\/(png|jpeg|webp|gif)$/.test(file.type)) return 'image';
	if (ext === 'pdf' || file.type === 'application/pdf') return 'pdf';
	if (ext === 'docx' || ext === 'xlsx' || ext === 'pptx') return ext;
	if (TEXT_EXT.has(ext) || file.type.startsWith('text/')) return 'text';
	return null;
}

export function fmtBytes(n: number): string {
	return n >= 1048576 ? (n / 1048576).toFixed(1) + ' MB' : Math.max(1, Math.round(n / 1024)) + ' KB';
}

// readAttachment membaca satu file menjadi lampiran siap kirim (atau melempar error).
export async function readAttachment(file: File, kind: AttachmentKind): Promise<Partial<Attachment>> {
	if (file.size > MAX_FILE_BYTES) throw new Error(`terlalu besar (maks ${fmtBytes(MAX_FILE_BYTES)})`);
	switch (kind) {
		case 'image':
			return { dataUrl: await shrinkImage(file) };
		case 'pdf':
			return finish(await readPdf(file));
		case 'docx':
			return finish({ text: await readDocx(file) });
		case 'xlsx':
			return finish(await readXlsx(file));
		case 'pptx':
			return finish(await readPptx(file));
		default:
			return finish({ text: await file.text() });
	}
}

function finish(r: { text: string; pages?: number }): Partial<Attachment> {
	let text = r.text.replace(/\r\n/g, '\n').replace(/[ \t]+\n/g, '\n').replace(/\n{3,}/g, '\n\n').trim();
	if (!text) throw new Error('tidak ada teks yang bisa dibaca (mungkin hasil scan/gambar)');
	let truncated = false;
	if (text.length > MAX_TEXT_CHARS) {
		text = text.slice(0, MAX_TEXT_CHARS);
		truncated = true;
	}
	return { text, pages: r.pages, truncated };
}

// ---- gambar: kecilkan agar hemat token & muat di request ----
async function shrinkImage(file: File): Promise<string> {
	const bmp = await createImageBitmap(file);
	const scale = Math.min(1, MAX_IMAGE_SIDE / Math.max(bmp.width, bmp.height));
	const w = Math.max(1, Math.round(bmp.width * scale));
	const h = Math.max(1, Math.round(bmp.height * scale));
	const canvas = document.createElement('canvas');
	canvas.width = w;
	canvas.height = h;
	const ctx = canvas.getContext('2d');
	if (!ctx) throw new Error('canvas tidak tersedia');
	// latar putih: PNG transparan tetap terbaca setelah jadi JPEG
	ctx.fillStyle = '#fff';
	ctx.fillRect(0, 0, w, h);
	ctx.drawImage(bmp, 0, 0, w, h);
	bmp.close();
	return canvas.toDataURL('image/jpeg', 0.85);
}

// ---- PDF (pdf.js, dimuat hanya saat dibutuhkan) ----
async function readPdf(file: File): Promise<{ text: string; pages: number }> {
	const pdfjs = await import('pdfjs-dist');
	const worker = await import('pdfjs-dist/build/pdf.worker.min.mjs?url');
	pdfjs.GlobalWorkerOptions.workerSrc = worker.default;
	const task = pdfjs.getDocument({ data: new Uint8Array(await file.arrayBuffer()) });
	const doc = await task.promise;
	try {
		const out: string[] = [];
		let chars = 0;
		const n = Math.min(doc.numPages, MAX_PDF_PAGES);
		for (let i = 1; i <= n && chars <= MAX_TEXT_CHARS; i++) {
			const page = await doc.getPage(i);
			const content = await page.getTextContent();
			let s = '';
			for (const it of content.items as any[]) {
				if (typeof it.str !== 'string') continue;
				s += it.str + (it.hasEOL ? '\n' : '');
			}
			out.push(`--- Halaman ${i} ---\n` + s);
			chars += s.length;
		}
		return { text: out.join('\n\n'), pages: doc.numPages };
	} finally {
		task.destroy();
	}
}

// ---- Office Open XML ----
const parseXml = (s: string) => new DOMParser().parseFromString(s, 'application/xml');
const byLocal = (el: Element | Document, name: string) =>
	Array.from(el.getElementsByTagName('*')).filter((e) => e.localName === name);
const childrenLocal = (el: Element, name: string) => Array.from(el.children).filter((e) => e.localName === name);

async function zipText(zip: JSZip, path: string): Promise<string> {
	const f = zip.file(path);
	if (!f) throw new Error('berkas rusak atau bukan format Office');
	return f.async('string');
}

function docxParagraph(p: Element): string {
	let s = '';
	for (const e of Array.from(p.getElementsByTagName('*'))) {
		if (e.localName === 't') s += e.textContent ?? '';
		else if (e.localName === 'tab') s += '\t';
		else if (e.localName === 'br') s += '\n';
	}
	return s;
}

async function readDocx(file: File): Promise<string> {
	const zip = await JSZip.loadAsync(file);
	const xml = parseXml(await zipText(zip, 'word/document.xml'));
	const body = byLocal(xml, 'body')[0];
	if (!body) return '';
	const out: string[] = [];
	for (const el of Array.from(body.children)) {
		if (el.localName === 'p') {
			const t = docxParagraph(el);
			const style = byLocal(el, 'pStyle')[0]?.getAttribute('w:val') ?? '';
			const lvl = /^(?:Heading|Judul)(\d)$/i.exec(style)?.[1];
			out.push(lvl && t.trim() ? '#'.repeat(Math.min(+lvl + 1, 4)) + ' ' + t : t);
		} else if (el.localName === 'tbl') {
			// tabel → markdown agar struktur kolom terbaca model
			const rows = childrenLocal(el, 'tr').map((tr) =>
				childrenLocal(tr, 'tc').map((tc) => byLocal(tc, 'p').map(docxParagraph).join(' ').replace(/\|/g, '/').trim())
			);
			if (!rows.length) continue;
			const cols = Math.max(...rows.map((r) => r.length));
			const line = (r: string[]) => '| ' + Array.from({ length: cols }, (_, i) => r[i] ?? '').join(' | ') + ' |';
			out.push(['', line(rows[0]), '|' + ' --- |'.repeat(cols), ...rows.slice(1).map(line), ''].join('\n'));
		}
	}
	return out.join('\n');
}

const numSort = (a: string, b: string) => (parseInt(a.replace(/\D+/g, '')) || 0) - (parseInt(b.replace(/\D+/g, '')) || 0);

async function readPptx(file: File): Promise<{ text: string; pages: number }> {
	const zip = await JSZip.loadAsync(file);
	const slides = Object.keys(zip.files).filter((p) => /^ppt\/slides\/slide\d+\.xml$/.test(p)).sort(numSort);
	const out: string[] = [];
	for (const [i, path] of slides.entries()) {
		const xml = parseXml(await zipText(zip, path));
		const paras = byLocal(xml, 'p')
			.map((p) => byLocal(p, 't').map((t) => t.textContent ?? '').join(''))
			.filter((t) => t.trim());
		const notesPath = `ppt/notesSlides/notesSlide${path.match(/(\d+)\.xml$/)?.[1]}.xml`;
		let notes = '';
		if (zip.file(notesPath)) {
			notes = byLocal(parseXml(await zipText(zip, notesPath)), 't').map((t) => t.textContent ?? '').join(' ').trim();
		}
		out.push(`## Slide ${i + 1}\n` + paras.join('\n') + (notes ? `\nCatatan pembicara: ${notes}` : ''));
	}
	return { text: out.join('\n\n'), pages: slides.length };
}

async function readXlsx(file: File): Promise<{ text: string; pages: number }> {
	const zip = await JSZip.loadAsync(file);
	const shared: string[] = zip.file('xl/sharedStrings.xml')
		? byLocal(parseXml(await zipText(zip, 'xl/sharedStrings.xml')), 'si').map((si) => byLocal(si, 't').map((t) => t.textContent ?? '').join(''))
		: [];
	// nama sheet dari workbook.xml (urutan sama dengan sheetN pada kebanyakan berkas)
	const names = zip.file('xl/workbook.xml')
		? byLocal(parseXml(await zipText(zip, 'xl/workbook.xml')), 'sheet').map((s) => s.getAttribute('name') ?? '')
		: [];
	const sheets = Object.keys(zip.files).filter((p) => /^xl\/worksheets\/sheet\d+\.xml$/.test(p)).sort(numSort);
	const out: string[] = [];
	let chars = 0;
	for (const [i, path] of sheets.entries()) {
		const xml = parseXml(await zipText(zip, path));
		const lines: string[] = [];
		for (const row of byLocal(xml, 'row')) {
			const cells: string[] = [];
			for (const c of childrenLocal(row, 'c')) {
				const col = colIndex(c.getAttribute('r') ?? '');
				const t = c.getAttribute('t');
				let v = '';
				if (t === 's') v = shared[parseInt(childrenLocal(c, 'v')[0]?.textContent ?? '-1')] ?? '';
				else if (t === 'inlineStr') v = byLocal(c, 't').map((x) => x.textContent ?? '').join('');
				else v = childrenLocal(c, 'v')[0]?.textContent ?? '';
				if (col >= 0) cells[col] = v;
				else cells.push(v);
			}
			if (cells.some((x) => x && x.trim())) lines.push(Array.from(cells, (x) => csvCell(x ?? '')).join(','));
			chars += lines[lines.length - 1]?.length ?? 0;
			if (chars > MAX_TEXT_CHARS) break;
		}
		out.push(`## Sheet: ${names[i] || 'Sheet' + (i + 1)}\n` + lines.join('\n'));
		if (chars > MAX_TEXT_CHARS) break;
	}
	return { text: out.join('\n\n'), pages: sheets.length };
}

// "C12" → 2
function colIndex(ref: string): number {
	const m = /^([A-Z]+)/.exec(ref);
	if (!m) return -1;
	let n = 0;
	for (const ch of m[1]) n = n * 26 + (ch.charCodeAt(0) - 64);
	return n - 1;
}
const csvCell = (s: string) => (/[",\n]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s);

// ---- label ----
const KIND_LABEL: Record<AttachmentKind, string> = {
	image: 'gambar', pdf: 'PDF', docx: 'Word', xlsx: 'Excel', pptx: 'PowerPoint', text: 'teks'
};
export const kindLabel = (k: AttachmentKind) => KIND_LABEL[k];

// bentuk tersimpan di meta pesan (tanpa status sementara)
export function storedAttachments(atts: Attachment[]): Attachment[] {
	return atts
		.filter((a) => a.status !== 'error' && a.status !== 'reading')
		.map(({ id, name, size, kind, dataUrl, text, pages, truncated }) => ({ id, name, size, kind, dataUrl, text, pages, truncated }));
}
