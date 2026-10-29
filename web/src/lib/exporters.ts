// Generator file unduhan dari jawaban chat: MD, DOCX, PPTX, XLSX, CSV.
// Semua dibangun mandiri (ZIP metode STORE + OOXML minimal) agar tidak
// menambah dependensi npm — lingkungan build web terbatas.

import { buildReportDocx } from './docgen/report.js';
import { buildDeckPptx } from './docgen/deck.js';

// ---- ZIP (metode store, tanpa kompresi) ----

const CRC_TABLE = (() => {
	const t = new Uint32Array(256);
	for (let n = 0; n < 256; n++) {
		let c = n;
		for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
		t[n] = c >>> 0;
	}
	return t;
})();

function crc32(data: Uint8Array): number {
	let c = 0xffffffff;
	for (let i = 0; i < data.length; i++) c = CRC_TABLE[(c ^ data[i]) & 0xff] ^ (c >>> 8);
	return (c ^ 0xffffffff) >>> 0;
}

function dosDateTime(d: Date): { time: number; date: number } {
	return {
		time: (d.getHours() << 11) | (d.getMinutes() << 5) | Math.floor(d.getSeconds() / 2),
		date: (((d.getFullYear() - 1980) & 0x7f) << 9) | ((d.getMonth() + 1) << 5) | d.getDate()
	};
}

class ZipWriter {
	private chunks: Uint8Array<ArrayBuffer>[] = [];
	private central: Uint8Array<ArrayBuffer>[] = [];
	private offset = 0;
	private count = 0;
	private when: Date;

	constructor(when = new Date()) {
		this.when = when;
	}

	add(name: string, content: string | Uint8Array<ArrayBuffer>) {
		const data = typeof content === 'string' ? new TextEncoder().encode(content) : content;
		const { time, date } = dosDateTime(this.when);
		const crc = crc32(data);
		const nameBytes = new TextEncoder().encode(name);

		const local = new Uint8Array(30 + nameBytes.length);
		const lv = new DataView(local.buffer);
		lv.setUint32(0, 0x04034b50, true);
		lv.setUint16(4, 20, true); // version needed
		lv.setUint16(6, 0x0800, true); // flag UTF-8
		lv.setUint16(8, 0, true); // method store
		lv.setUint16(10, time, true);
		lv.setUint16(12, date, true);
		lv.setUint32(14, crc, true);
		lv.setUint32(18, data.length, true);
		lv.setUint32(22, data.length, true);
		lv.setUint16(26, nameBytes.length, true);
		lv.setUint16(28, 0, true);
		local.set(nameBytes, 30);

		this.chunks.push(local, data);

		const cen = new Uint8Array(46 + nameBytes.length);
		const cv = new DataView(cen.buffer);
		cv.setUint32(0, 0x02014b50, true);
		cv.setUint16(4, 20, true);
		cv.setUint16(6, 20, true);
		cv.setUint16(8, 0x0800, true);
		cv.setUint16(10, 0, true);
		cv.setUint16(12, time, true);
		cv.setUint16(14, date, true);
		cv.setUint32(16, crc, true);
		cv.setUint32(20, data.length, true);
		cv.setUint32(24, data.length, true);
		cv.setUint16(28, nameBytes.length, true);
		cv.setUint32(42, this.offset, true);
		cen.set(nameBytes, 46);
		this.central.push(cen);

		this.offset += local.length + data.length;
		this.count++;
	}

	finish(): Blob {
		const cdSize = this.central.reduce((n, c) => n + c.length, 0);
		const end = new Uint8Array(22);
		const ev = new DataView(end.buffer);
		ev.setUint32(0, 0x06054b50, true);
		ev.setUint16(8, this.count, true);
		ev.setUint16(10, this.count, true);
		ev.setUint32(12, cdSize, true);
		ev.setUint32(16, this.offset, true);
		return new Blob([...this.chunks, ...this.central, end], {
			type: 'application/octet-stream'
		});
	}
}

// ---- util XML ----

const xmlEsc = (s: string) =>
	s
		.replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F]/g, '')
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;')
		.replace(/"/g, '&quot;');

// ---- parser markdown → blok ----

export type Block =
	| { kind: 'heading'; level: number; text: string }
	| { kind: 'para'; text: string }
	| { kind: 'bullet'; text: string; level: number; ordered?: boolean }
	| { kind: 'quote'; text: string }
	| { kind: 'code'; text: string }
	| { kind: 'table'; header: string[]; rows: string[][] };

export function parseMarkdown(src: string): Block[] {
	const lines = src.replace(/\r\n?/g, '\n').split('\n');
	const blocks: Block[] = [];
	let i = 0;
	while (i < lines.length) {
		const line = lines[i];
		if (/^\s*```/.test(line)) {
			const buf: string[] = [];
			i++;
			while (i < lines.length && !/^\s*```/.test(lines[i])) buf.push(lines[i++]);
			i++; // penutup
			blocks.push({ kind: 'code', text: buf.join('\n') });
			continue;
		}
		const h = line.match(/^(#{1,6})\s+(.*)/);
		if (h) {
			// model kadang menulis "## # Judul" — buang pagar ekstra
			blocks.push({ kind: 'heading', level: h[1].length, text: h[2].replace(/^#+\s+/, '').trim() });
			i++;
			continue;
		}
		// tabel: baris berpipe + baris pemisah berikutnya
		if (line.includes('|') && i + 1 < lines.length && /^\s*\|?[\s:|-]+\|[\s:|-]*$/.test(lines[i + 1])) {
			const cells = (l: string) =>
				l.replace(/^\s*\|/, '').replace(/\|\s*$/, '').split('|').map((c) => c.trim());
			const header = cells(line);
			i += 2;
			const rows: string[][] = [];
			while (i < lines.length && lines[i].includes('|') && lines[i].trim() !== '') {
				rows.push(cells(lines[i]));
				i++;
			}
			blocks.push({ kind: 'table', header, rows });
			continue;
		}
		const b = line.match(/^(\s*)[-*+]\s+(.*)/);
		if (b) {
			blocks.push({ kind: 'bullet', level: Math.min(2, Math.floor(b[1].length / 2)), text: b[2].trim() });
			i++;
			continue;
		}
		const o = line.match(/^(\s*)\d+[.)]\s+(.*)/);
		if (o) {
			blocks.push({ kind: 'bullet', ordered: true, level: Math.min(2, Math.floor(o[1].length / 3)), text: o[2].trim() });
			i++;
			continue;
		}
		if (/^\s*(?:(?:-\s*){3,}|(?:\*\s*){3,}|(?:_\s*){3,})$/.test(line)) {
			i++; // garis pemisah markdown — diabaikan
			continue;
		}
		const q = line.match(/^\s*>\s?(.*)/);
		if (q) {
			blocks.push({ kind: 'quote', text: q[1].trim() });
			i++;
			continue;
		}
		if (line.trim() === '') {
			i++;
			continue;
		}
		const buf: string[] = [];
		while (
			i < lines.length &&
			lines[i].trim() !== '' &&
			!/^\s*(```|#{1,6}\s|\s*[-*+]\s|\d+[.)]\s|>\s)/.test(lines[i]) &&
			!(lines[i].includes('|') && i + 1 < lines.length && /^\s*\|?[\s:|-]+\|[\s:|-]*$/.test(lines[i + 1]))
		) {
			buf.push(lines[i].trim());
			i++;
		}
		blocks.push({ kind: 'para', text: buf.join(' ') });
	}
	return blocks;
}

// token inline: **tebal**, *miring*, `kode`, [teks](https://…)
export type Run = { text: string; bold?: boolean; italic?: boolean; code?: boolean; link?: string };
export function inlineRuns(s: string): Run[] {
	const out: Run[] = [];
	const re = /\*\*([^*]+)\*\*|`([^`]+)`|\*([^*]+)\*|\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g;
	let last = 0;
	let m: RegExpExecArray | null;
	while ((m = re.exec(s))) {
		if (m.index > last) out.push({ text: s.slice(last, m.index) });
		if (m[1] !== undefined) out.push({ text: m[1], bold: true });
		else if (m[2] !== undefined) out.push({ text: m[2], code: true });
		else if (m[3] !== undefined) out.push({ text: m[3], italic: true });
		else out.push({ text: m[4], link: m[5] });
		last = re.lastIndex;
	}
	if (last < s.length) out.push({ text: s.slice(last) });
	return out.filter((r) => r.text);
}

export const plain = (s: string) =>
	inlineRuns(s)
		.map((r) => r.text)
		.join('');

// ---- DOCX & PPTX: lihat docgen/report.ts dan docgen/deck.ts ----

// ---- XLSX ----

const colLetter = (i: number): string => {	let s = '';
	i++;
	while (i > 0) {
		s = String.fromCharCode(64 + ((i - 1) % 26) + 1) + s;
		i = Math.floor((i - 1) / 26);
	}
	return s;
};

const NUM_RE = /^-?\d+(\.\d+)?$/;

// string berisi angka murni → number agar jadi sel numerik di Excel
const asNumber = (s: string): string | number => (NUM_RE.test(s) ? Number(s) : s);

function xlsxSheet(rows: (string | number)[][]): string {
	const body = rows
		.map((r, ri) => {
			const cells = r
				.map((v, ci) => {
					const ref = colLetter(ci) + (ri + 1);
					if (typeof v === 'number') return `<c r="${ref}"><v>${v}</v></c>`;
					return `<c r="${ref}" t="inlineStr"><is><t xml:space="preserve">${xmlEsc(String(v))}</t></is></c>`;
				})
				.join('');
			return `<row r="${ri + 1}">${cells}</row>`;
		})
		.join('');
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>${body}</sheetData></worksheet>`;
}

const sheetName = (name: string, used: Set<string>): string => {
	let base = name.replace(/[*?:\\/[\]]/g, ' ').slice(0, 28).trim() || 'Sheet';
	let n = base,
		k = 2;
	while (used.has(n)) n = `${base} ${k++}`;
	used.add(n);
	return n;
};

type Sheet = { name: string; rows: (string | number)[][] };

// sheet per tabel markdown (judul dari heading terdekat di atasnya)
function tableSheets(blocks: Block[], used: Set<string>): Sheet[] {
	const out: Sheet[] = [];
	let no = 0;
	let lastHeading = '';
	for (const b of blocks) {
		if (b.kind === 'heading') lastHeading = b.text;
		if (b.kind !== 'table') continue;
		no++;
		const rows: (string | number)[][] = [[plain(lastHeading) || `Tabel ${no}`]];
		rows.push(b.header.map(plain));
		for (const r of b.rows) rows.push(b.header.map((_, ci) => asNumber(r[ci] ?? '')));
		out.push({ name: sheetName(`Tabel${no} ${plain(lastHeading)}`, used), rows });
	}
	return out;
}

function xlsxZip(sheets: Sheet[]): Blob {
	const zip = new ZipWriter();
	const ct = [
		'<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>',
		'<Default Extension="xml" ContentType="application/xml"/>',
		'<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>',
		...sheets.map(
			(_, i) =>
				`<Override PartName="/xl/worksheets/sheet${i + 1}.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`
		)
	];
	zip.add(
		'[Content_Types].xml',
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">${ct.join('')}</Types>`
	);
	zip.add(
		'_rels/.rels',
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`
	);
	zip.add(
		'xl/workbook.xml',
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>${sheets
			.map((s, i) => `<sheet name="${xmlEsc(s.name)}" sheetId="${i + 1}" r:id="rId${i + 1}"/>`)
			.join('')}</sheets></workbook>`
	);
	zip.add(
		'xl/_rels/workbook.xml.rels',
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">${sheets
			.map(
				(_, i) =>
					`<Relationship Id="rId${i + 1}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet${i + 1}.xml"/>`
			)
			.join('')}</Relationships>`
	);
	sheets.forEach((s, i) => zip.add(`xl/worksheets/sheet${i + 1}.xml`, xlsxSheet(s.rows)));
	return zip.finish();
}

export function buildXlsx(title: string, src: string): Blob {
	const blocks = parseMarkdown(src);
	const used = new Set<string>();
	const tables = tableSheets(blocks, used);
	if (tables.length > 0) return xlsxZip(tables);

	const rows: (string | number)[][] = [[title]];
	for (const b of blocks) {
		if (b.kind === 'heading') rows.push(['', plain(b.text)]);
		else if (b.kind === 'bullet') rows.push(['• ' + plain(b.text)]);
		else if (b.kind === 'code') b.text.split('\n').forEach((l) => rows.push([l]));
		else if (b.kind === 'table') {
			rows.push(b.header.map(plain));
			b.rows.forEach((r) => rows.push(r.map(plain)));
		} else if (b.kind === 'quote' || b.kind === 'para') rows.push([plain(b.text)]);
	}
	return xlsxZip([{ name: sheetName('Catatan', used), rows }]);
}

// ---- CSV ----

// sel yang diawali = + - @ (atau tab/CR) dibaca Excel/Sheets sebagai rumus —
// isi jawaban model bisa disusupi (prompt injection), jadi dinetralkan dengan '
const csvSafe = (v: string) => (/^[=+\-@\t\r]/.test(v) ? "'" + v : v);
const csvCell = (raw: string) => {
	const v = csvSafe(raw);
	return /[";\n,']/.test(v) ? '"' + v.replace(/"/g, '""') + '"' : v;
};

export function buildCsv(title: string, src: string): Blob {
	const blocks = parseMarkdown(src);
	const tables = blocks.filter((b): b is Extract<Block, { kind: 'table' }> => b.kind === 'table');
	let text = '';
	if (tables.length > 0) {
		// tabel dengan baris terbanyak
		const t = tables.reduce((a, b) => (b.rows.length > a.rows.length ? b : a));
		text = [t.header, ...t.rows].map((r) => r.map(csvCell).join(';')).join('\r\n');
	} else {
		text = blocks
			.map((b) => {
				if (b.kind === 'bullet') return '• ' + plain(b.text);
				if (b.kind === 'code') return b.text;
				if (b.kind === 'table') return b.header.map(plain).join(';');
				return plain(b.text);
			})
			.filter((l) => l !== '')
			.join('\r\n');
	}
	// BOM agar Excel menampilkan UTF-8 dengan benar
	return new Blob(['\uFEFF' + text], { type: 'text/csv;charset=utf-8' });
}

// ---- ekspor seluruh percakapan ----

export type ChatTurn = { role: 'user' | 'assistant' | 'tool'; content: string; model?: string };

const roleLabel = (role: string): string =>
	role === 'user' ? 'Anda' : role === 'assistant' ? 'Assistant' : role;

// transkrip sebagai markdown: judul + satu section per pesan, sehingga
// builder docx/pptx yang ada bisa dipakai ulang apa adanya
function conversationMarkdown(title: string, turns: ChatTurn[]): string {
	const parts = [`# ${title}`, ''];
	for (const t of turns) {
		if (!t.content) continue;
		parts.push(t.role === 'assistant' && t.model ? `## Assistant — ${t.model}` : `## ${roleLabel(t.role)}`);
		parts.push('');
		parts.push(t.content);
		parts.push('');
	}
	return parts.join('\n');
}

export function buildConversationXlsx(title: string, turns: ChatTurn[], md: string): Blob {
	const used = new Set<string>();
	const sheets: Sheet[] = [
		{
			name: sheetName('Transkrip', used),
			rows: [[title], ['Peran', 'Pesan'], ...turns.filter((t) => t.content).map((t) => [roleLabel(t.role), t.content])]
		}
	];
	sheets.push(...tableSheets(parseMarkdown(md), used));
	return xlsxZip(sheets);
}

export function buildConversationCsv(turns: ChatTurn[]): Blob {
	const lines = ['Peran;Pesan'];
	for (const t of turns) {
		if (!t.content) continue;
		lines.push([roleLabel(t.role), t.content].map(csvCell).join(';'));
	}
	return new Blob(['\uFEFF' + lines.join('\r\n')], { type: 'text/csv;charset=utf-8' });
}

// Ekspor seluruh percakapan (semua pesan user + assistant) ke satu file.
export async function exportConversation(title: string, turns: ChatTurn[], fmt: ExportFormat) {
	const md = conversationMarkdown(title, turns);
	let blob: Blob;
	if (fmt === 'md') blob = new Blob([md], { type: MIME.md });
	else if (fmt === 'docx') blob = await buildReportDocx(title, md);
	else if (fmt === 'pptx') blob = await buildDeckPptx(title, md);
	else if (fmt === 'xlsx') blob = buildConversationXlsx(title, turns, md);
	else blob = buildConversationCsv(turns);
	download(`${fileBase(title)}.${fmt}`, blob);
}

// ---- mode dokumen: prompt user → file jadi (ala "skill doc") ----

export type DocType = 'pptx' | 'docx' | 'xlsx' | 'csv';

// instruksi per format yang disisipkan sebagai system message agar model
// menyusun konten yang bisa dikonversi rapi menjadi file
const DOC_COMMON =
	'Jawabanmu akan DIKONVERSI OTOMATIS menjadi file oleh aplikasi, jadi tulis HANYA isi dokumen dalam markdown — ' +
	'tanpa kalimat pembuka ("Berikut…"), tanpa penutup ("Semoga membantu"), tanpa menyebut bahwa ini markdown/file, ' +
	'tanpa blok ```markdown. Gunakan Bahasa Indonesia baku kecuali pengguna meminta bahasa lain. ' +
	'Isi harus substantif, spesifik, akurat, dan siap dipakai — bukan kerangka kosong atau placeholder seperti [isi di sini].';

export const DOC_SYSTEM: Record<DocType, string> = {
	pptx:
		'PERAN: kamu desainer presentasi profesional. ' + DOC_COMMON +
		'\nSTRUKTUR WAJIB:' +
		'\n1. Baris 1: "# <Judul Presentasi>" (maks 8 kata). Baris 2: "*<subjudul satu kalimat>*".' +
		'\n2. Setiap slide diawali "## <Judul Slide>" — judul berupa pesan/insight, bukan label (contoh: "Biaya turun 30% setelah otomasi", bukan "Biaya").' +
		'\n3. Jumlah slide 8–12 kecuali pengguna menentukan. Alur: pembuka/masalah → isi bertahap → kesimpulan → "## Terima kasih".' +
		'\n4. Slide isi: 3–5 poin "- ", tiap poin maks 12 kata; boleh sub-poin "  - " (2 spasi). Awali poin dengan **frasa kunci** lalu penjelasan singkat.' +
		'\nTATA LETAK KHUSUS (pakai minimal 2 jenis agar variatif):' +
		'\n- Slide angka/statistik: 2–4 poin berformat "- **72%** — penjelasan singkat" (angka/nilai pendek di dalam ** **).' +
		'\n- Slide perbandingan/data: satu tabel markdown (maks 6 baris × 4 kolom), boleh 1 kalimat pengantar di atasnya.' +
		'\n- Slide kutipan: satu baris "> kutipan — Nama Tokoh".' +
		'\n- Slide pembatas bagian: "## <Nama Bagian>" tanpa isi apa pun.' +
		'\n- Contoh kode (bila topik teknis): satu blok ``` pendek (maks 15 baris) + 2–3 poin penjelasan.' +
		'\n5. Catatan pembicara (opsional, dianjurkan): di akhir slide tulis paragraf "Catatan pembicara: <apa yang diucapkan>".' +
		'\nDILARANG: paragraf panjang di slide, lebih dari 6 poin per slide, emoji berlebihan, heading level 3+.',
	docx:
		'PERAN: kamu penulis laporan profesional. ' + DOC_COMMON +
		'\nSTRUKTUR WAJIB:' +
		'\n1. Baris 1: "# <Judul Dokumen>". Baris 2: "*<subjudul/konteks satu kalimat>*". Sampul, daftar isi, nomor halaman, dan penomoran bab dibuat otomatis — JANGAN menulis daftar isi atau nomor bab sendiri.' +
		'\n2. Bab memakai "## <Judul Bab>", subbab "### <Judul Subbab>". Untuk laporan umumnya: Ringkasan Eksekutif, Pendahuluan/Latar Belakang, (bab inti sesuai topik), Kesimpulan, Rekomendasi. Sesuaikan bila pengguna meminta jenis lain (surat, proposal, notulen, SOP, makalah).' +
		'\n3. Paragraf utuh 3–5 kalimat, bahasa formal dan jelas. Gunakan **tebal** untuk istilah kunci.' +
		'\n4. Daftar memakai "- " atau "1. " (langkah berurutan). Data/perbandingan WAJIB dalam tabel markdown dengan header jelas dan satuan.' +
		'\n5. Kutipan, catatan penting, atau definisi pakai "> ".' +
		'\n6. Panjang menyesuaikan permintaan; laporan standar 800–1500 kata.' +
		'\nDILARANG: heading level 4+, HTML, emoji, garis "---", kata "Berikut adalah".',
	xlsx:
		'Pengguna meminta file Excel. ' + DOC_COMMON +
		' Sajikan data sebagai satu atau beberapa tabel markdown dengan baris header "| Kol1 | Kol2 |" dan pemisah "|---|---|". ' +
		'Beri heading "## <Nama Sheet>" di atas setiap tabel. Isi sel berupa nilai murni (angka tanpa titik ribuan dan tanpa satuan; satuan ditaruh di header). Tanpa narasi.',
	csv:
		'Pengguna meminta file CSV. ' + DOC_COMMON +
		' Sajikan TEPAT SATU tabel markdown dengan baris header dan pemisah "|---|". Isi sel berupa nilai murni tanpa karakter pipe. Tanpa narasi.'
};

const DOC_PATTERNS: [RegExp, DocType][] = [
	[/\b(pptx?|power\s*point|presentasi\w*|slide\w*|deck|salindia)\b/i, 'pptx'],
	[/\b(xlsx?|excel|spreadsheet|lembar\s+kerja)\b/i, 'xlsx'],
	[/\bcsv\b/i, 'csv'],
	[/\b(docx?|word|dokumen\w*|laporan|makalah|proposal|notulen|surat|sop|artikel|esai|essay|modul|rpp|report)\b/i, 'docx']
];

// Deteksi permintaan pembuatan file dari prompt user: harus ada kata kerja
// buat + jenis dokumen, agar pertanyaan biasa ("apa itu ppt") tidak kena.
export function detectDocRequest(text: string): DocType | null {
	if (!/\b(buat\w*|bikin\w*|susun\w*|siapkan|tuliskan|tulis|rancang\w*|generate|create|make|draft\w*)\b/i.test(text)) return null;
	for (const [re, type] of DOC_PATTERNS) if (re.test(text)) return type;
	return null;
}

// Rakit blob file dari konten markdown sesuai tipe dokumen.
export async function buildDocBlob(type: DocType, title: string, content: string): Promise<Blob> {
	if (type === 'pptx') return buildDeckPptx(title, content);
	if (type === 'docx') return buildReportDocx(title, content);
	if (type === 'xlsx') return buildXlsx(title, content);
	return buildCsv(title, content);
}

// nama file attachment dari judul (dipakai kartu unduh di chat)
export function docFileName(title: string, ext: string): string {
	return `${slugify(title)}.${ext}`;
}

// ---- unduhan ----

export type ExportFormat = 'md' | 'docx' | 'pptx' | 'xlsx' | 'csv';

const slugify = (s: string) =>
	s
		.toLowerCase()
		.replace(/[^a-z0-9]+/g, '-')
		.replace(/^-+|-+$/g, '')
		.slice(0, 40) || 'jawaban';

function fileBase(title: string): string {
	const d = new Date();
	const pad = (n: number) => String(n).padStart(2, '0');
	return `${slugify(title)}-${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}`;
}

const MIME: Record<ExportFormat, string> = {
	md: 'text/markdown;charset=utf-8',
	docx: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
	pptx: 'application/vnd.openxmlformats-officedocument.presentationml.presentation',
	xlsx: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
	csv: 'text/csv;charset=utf-8'
};

// Ekspor satu jawaban chat ke format yang diminta dan unduh sebagai file.
export async function exportAnswer(title: string, content: string, fmt: ExportFormat) {
	let blob: Blob;
	if (fmt === 'md') blob = new Blob([content], { type: MIME.md });
	else if (fmt === 'docx') blob = await buildReportDocx(title, content);
	else if (fmt === 'pptx') blob = await buildDeckPptx(title, content);
	else if (fmt === 'xlsx') blob = buildXlsx(title, content);
	else blob = buildCsv(title, content);
	download(`${fileBase(title)}.${fmt}`, blob);
}

export function download(name: string, blob: Blob) {
	const url = URL.createObjectURL(blob);
	const a = document.createElement('a');
	a.href = url;
	a.download = name;
	a.click();
	setTimeout(() => URL.revokeObjectURL(url), 5000);
}
