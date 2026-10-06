// Generator file unduhan dari jawaban chat: MD, DOCX, PPTX, XLSX, CSV.
// Semua dibangun mandiri (ZIP metode STORE + OOXML minimal) agar tidak
// menambah dependensi npm — lingkungan build web terbatas.

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
	private chunks: Uint8Array[] = [];
	private central: Uint8Array[] = [];
	private offset = 0;
	private count = 0;
	private when: Date;

	constructor(when = new Date()) {
		this.when = when;
	}

	add(name: string, content: string | Uint8Array) {
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
	| { kind: 'bullet'; text: string; level: number }
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
			blocks.push({ kind: 'heading', level: h[1].length, text: h[2].trim() });
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
			!/^\s*(```|#{1,6}\s|\s*[-*+]\s|>\s)/.test(lines[i]) &&
			!(lines[i].includes('|') && i + 1 < lines.length && /^\s*\|?[\s:|-]+\|[\s:|-]*$/.test(lines[i + 1]))
		) {
			buf.push(lines[i].trim());
			i++;
		}
		blocks.push({ kind: 'para', text: buf.join(' ') });
	}
	return blocks;
}

// token inline: **tebal**, *miring*, `kode`
type Run = { text: string; bold?: boolean; italic?: boolean; code?: boolean };
function inlineRuns(s: string): Run[] {
	const out: Run[] = [];
	const re = /\*\*([^*]+)\*\*|`([^`]+)`|\*([^*]+)\*/g;
	let last = 0;
	let m: RegExpExecArray | null;
	while ((m = re.exec(s))) {
		if (m.index > last) out.push({ text: s.slice(last, m.index) });
		if (m[1] !== undefined) out.push({ text: m[1], bold: true });
		else if (m[2] !== undefined) out.push({ text: m[2], code: true });
		else out.push({ text: m[3], italic: true });
		last = re.lastIndex;
	}
	if (last < s.length) out.push({ text: s.slice(last) });
	return out.filter((r) => r.text);
}

const plain = (s: string) =>
	inlineRuns(s)
		.map((r) => r.text)
		.join('');

// ---- DOCX ----

function docxRuns(text: string, size: number, mono = false): string {
	return inlineRuns(text)
		.map(({ text: t, bold, italic, code }) => {
			const props: string[] = [];
			if (bold) props.push('<w:b/>');
			if (italic) props.push('<w:i/>');
			if (code || mono) props.push('<w:rFonts w:ascii="Courier New" w:hAnsi="Courier New"/><w:sz w:val="20"/>');
			else props.push(`<w:sz w:val="${size}"/>`);
			return `<w:r><w:rPr>${props.join('')}</w:rPr><w:t xml:space="preserve">${xmlEsc(t)}</w:t></w:r>`;
		})
		.join('');
}

function docxPara(text: string, size = 22, extra = '', mono = false): string {
	return `<w:p><w:pPr>${extra}</w:pPr>${docxRuns(text, size, mono) || '<w:r/>'}</w:p>`;
}

function docxTable(header: string[], rows: string[][]): string {
	const border = '<w:top w:val="single" w:sz="4" w:color="BBBBBB"/><w:left w:val="single" w:sz="4" w:color="BBBBBB"/><w:bottom w:val="single" w:sz="4" w:color="BBBBBB"/><w:right w:val="single" w:sz="4" w:color="BBBBBB"/><w:insideH w:val="single" w:sz="4" w:color="BBBBBB"/><w:insideV w:val="single" w:sz="4" w:color="BBBBBB"/>';
	const headCell = (t: string) =>
		`<w:tc><w:tcPr/><w:p><w:pPr><w:spacing w:after="40"/></w:pPr><w:r><w:rPr><w:b/><w:sz w:val="20"/></w:rPr><w:t xml:space="preserve">${xmlEsc(plain(t))}</w:t></w:r></w:p></w:tc>`;
	const bodyCell = (t: string) =>
		`<w:tc><w:tcPr/><w:p><w:pPr><w:spacing w:after="40"/></w:pPr>${docxRuns(plain(t), 20)}</w:p></w:tc>`;
	return `<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/><w:tblBorders>${border}</w:tblBorders></w:tblPr><w:tr>${header
		.map(headCell)
		.join('')}</w:tr>${rows
		.map((r) => `<w:tr>${header.map((_, ci) => bodyCell(r[ci] ?? '')).join('')}</w:tr>`)
		.join('')}</w:tbl>`;
}

export function buildDocx(title: string, src: string): Blob {
	const zip = new ZipWriter();
	const body: string[] = [docxPara(title, 32, '<w:spacing w:after="240"/>')];
	for (const b of parseMarkdown(src)) {
		if (b.kind === 'heading') {
			body.push(docxPara(b.text, b.level === 1 ? 40 : 32, '<w:spacing w:before="240" w:after="120"/>'));
		} else if (b.kind === 'bullet') {
			body.push(docxPara('• ' + b.text, 22, `<w:ind w:left="${360 + 360 * b.level}"/><w:spacing w:after="60"/>`));
		} else if (b.kind === 'quote') {
			body.push(
				`<w:p><w:pPr><w:ind w:left="480"/></w:pPr><w:r><w:rPr><w:i/><w:sz w:val="22"/></w:rPr><w:t xml:space="preserve">${xmlEsc(b.text)}</w:t></w:r></w:p>`
			);
		} else if (b.kind === 'code') {
			for (const line of b.text.split('\n')) body.push(docxPara(line === '' ? ' ' : line, 20, '', true));
		} else if (b.kind === 'table') {
			body.push(docxTable(b.header, b.rows));
			body.push('<w:p/>');
		} else {
			body.push(docxPara(b.text));
		}
	}
	const document = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>${body.join('')}<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr></w:body></w:document>`;
	zip.add(
		'[Content_Types].xml',
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`
	);
	zip.add(
		'_rels/.rels',
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`
	);
	zip.add('word/document.xml', document);
	return zip.finish();
}

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

// ---- PPTX ----

type SlideItem = { text: string; level: number; bold?: boolean };
type Slide = { title: string; items: SlideItem[] };

function planSlides(title: string, src: string): Slide[] {
	const blocks = parseMarkdown(src);
	const sections: { heading: string | null; blocks: Block[] }[] = [{ heading: null, blocks: [] }];
	for (const b of blocks) {
		if (b.kind === 'heading' && b.level <= 2) sections.push({ heading: b.text, blocks: [] });
		else sections[sections.length - 1].blocks.push(b);
	}

	const toItems = (bs: Block[]): SlideItem[] => {
		const items: SlideItem[] = [];
		for (const b of bs) {
			if (b.kind === 'heading') items.push({ text: plain(b.text), level: 0, bold: true });
			else if (b.kind === 'bullet') items.push({ text: plain(b.text), level: b.level });
			else if (b.kind === 'quote') items.push({ text: '"' + plain(b.text) + '"', level: 0 });
			else if (b.kind === 'code')
				b.text.split('\n').slice(0, 8).forEach((l) => items.push({ text: l, level: 1 }));
			else if (b.kind === 'table') {
				items.push({ text: b.header.map(plain).join(' — '), level: 0, bold: true });
				for (const r of b.rows.slice(0, 10))
					items.push({ text: b.header.map((_, ci) => plain(r[ci] ?? '')).join(' — '), level: 1 });
			} else items.push({ text: plain(b.text), level: 0 });
		}
		return items.filter((it) => it.text).slice(0, 30);
	};

	const slides: Slide[] = [{ title, items: [] }];
	for (const s of sections) {
		if (s.heading === null) {
			const items = toItems(s.blocks);
			if (items.length && slides.length === 1) slides[0].items = items;
			continue;
		}
		slides.push({ title: plain(s.heading), items: toItems(s.blocks) });
	}
	return slides;
}

function pptxTxBody(items: SlideItem[]): string {
	const paras = items.length
		? items
				.map(
					(it) =>
						`<a:p><a:pPr lvl="${it.level}"><a:buChar char="•"/></a:pPr><a:r><a:rPr lang="id-ID" sz="1500"${it.bold ? ' b="1"' : ''} dirty="0"/><a:t>${xmlEsc(it.text)}</a:t></a:r></a:p>`
				)
				.join('')
		: '<a:p><a:endParaRPr lang="id-ID"/></a:p>';
	return `<p:txBody><a:bodyPr anchor="t"/><a:lstStyle/>${paras}</p:txBody>`;
}

function pptxShape(id: number, name: string, x: number, y: number, cx: number, cy: number, titleText: string, titleSize: number, body?: string): string {
	const sp = (text: string, size: number) =>
		`<a:p><a:r><a:rPr lang="id-ID" sz="${size}" b="1" dirty="0"/><a:t>${xmlEsc(text)}</a:t></a:r></a:p>`;
	return `<p:sp><p:nvSpPr><p:cNvPr id="${id}" name="${xmlEsc(name)}"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="${x}" y="${y}"/><a:ext cx="${cx}" cy="${cy}"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr><p:txBody><a:bodyPr wrap="square" anchor="t"/><a:lstStyle/>${sp(titleText, titleSize)}</p:txBody></p:sp>${body ?? ''}`;
}

export function buildPptx(title: string, src: string): Blob {
	const slides = planSlides(title, src);
	const zip = new ZipWriter();
	const A = 'http://schemas.openxmlformats.org/drawingml/2006/main';
	const R = 'http://schemas.openxmlformats.org/officeDocument/2006/relationships';
	const P = 'http://schemas.openxmlformats.org/presentationml/2006/main';
	const NS = `xmlns:a="${A}" xmlns:r="${R}" xmlns:p="${P}"`;
	const spTreeHead = `<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>`;

	zip.add(
		'[Content_Types].xml',
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/><Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/><Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/><Override PartName="/ppt/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>${slides
			.map(
				(_, i) =>
					`<Override PartName="/ppt/slides/slide${i + 1}.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`
			)
			.join('')}</Types>`
	);
	zip.add(
		'_rels/.rels',
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/></Relationships>`
	);
	zip.add(
		'ppt/presentation.xml',
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><p:presentation ${NS}><p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst><p:sldIdLst>${slides
			.map((_, i) => `<p:sldId id="${256 + i}" r:id="rId${i + 2}"/>`)
			.join('')}</p:sldIdLst><p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/></p:presentation>`
	);
	zip.add(
		'ppt/_rels/presentation.xml.rels',
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="${R}/slideMaster" Target="slideMasters/slideMaster1.xml"/>${slides
			.map((_, i) => `<Relationship Id="rId${i + 2}" Type="${R}/slide" Target="slides/slide${i + 1}.xml"/>`)
			.join('')}</Relationships>`
	);
	zip.add(
		'ppt/slideMasters/slideMaster1.xml',
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><p:sldMaster ${NS}><p:cSld><p:bg><p:bgPr><a:solidFill><a:srgbClr val="FFFFFF"/></a:solidFill><a:effectLst/></p:bgPr></p:bg><p:spTree>${spTreeHead}</p:spTree></p:cSld><p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/><p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1"/></p:sldLayoutIdLst></p:sldMaster>`
	);
	zip.add(
		'ppt/slideMasters/_rels/slideMaster1.xml.rels',
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="${R}/slideLayout" Target="../slideLayouts/slideLayout1.xml"/><Relationship Id="rId2" Type="${R}/theme" Target="../theme/theme1.xml"/></Relationships>`
	);
	zip.add(
		'ppt/slideLayouts/slideLayout1.xml',
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><p:sldLayout ${NS} type="blank"><p:cSld name="Blank"><p:spTree>${spTreeHead}</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sldLayout>`
	);
	zip.add(
		'ppt/slideLayouts/_rels/slideLayout1.xml.rels',
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="${R}/slideMaster" Target="../slideMasters/slideMaster1.xml"/></Relationships>`
	);
	zip.add(
		'ppt/theme/theme1.xml',
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><a:theme xmlns:a="${A}" name="Tema"><a:themeElements><a:clrScheme name="Tema"><a:dk1><a:sysClr val="windowText" lastClr="000000"/></a:dk1><a:lt1><a:sysClr val="window" lastClr="FFFFFF"/></a:lt1><a:dk2><a:srgbClr val="1F2937"/></a:dk2><a:lt2><a:srgbClr val="F3F4F6"/></a:lt2><a:accent1><a:srgbClr val="4F7CFF"/></a:accent1><a:accent2><a:srgbClr val="0EA5E9"/></a:accent2><a:accent3><a:srgbClr val="10B981"/></a:accent3><a:accent4><a:srgbClr val="F59E0B"/></a:accent4><a:accent5><a:srgbClr val="8B5CF6"/></a:accent5><a:accent6><a:srgbClr val="EF4444"/></a:accent6><a:hlink><a:srgbClr val="2563EB"/></a:hlink><a:folHlink><a:srgbClr val="7C3AED"/></a:folHlink></a:clrScheme><a:fontScheme name="Tema"><a:majorFont><a:latin typeface="Calibri Light"/><a:ea typeface=""/><a:cs typeface=""/></a:majorFont><a:minorFont><a:latin typeface="Calibri"/><a:ea typeface=""/><a:cs typeface=""/></a:minorFont></a:fontScheme><a:fmtScheme name="Tema"><a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:fillStyleLst><a:lnStyleLst><a:ln><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln><a:ln><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln><a:ln><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln></a:lnStyleLst><a:effectStyleLst><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle></a:effectStyleLst><a:bgFillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:bgFillStyleLst></a:fmtScheme></a:themeElements></a:theme>`
	);

	slides.forEach((s, i) => {
		const titleShape = pptxShape(2, 'Judul', 457200, 274638, 11277600, 1325563, s.title, 3000);
		const bodyShape = `<p:sp><p:nvSpPr><p:cNvPr id="3" name="Isi"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="457200" y="1732281"/><a:ext cx="11277600" cy="4846319"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>${pptxTxBody(s.items)}</p:sp>`;
		zip.add(
			`ppt/slides/slide${i + 1}.xml`,
			`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><p:sld ${NS}><p:cSld><p:spTree>${spTreeHead}${titleShape}${bodyShape}</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`
		);
		zip.add(
			`ppt/slides/_rels/slide${i + 1}.xml.rels`,
			`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="${R}/slideLayout" Target="../slideLayouts/slideLayout1.xml"/></Relationships>`
		);
	});
	return zip.finish();
}

// ---- CSV ----

const csvCell = (v: string) => (/[";\n,]/.test(v) ? '"' + v.replace(/"/g, '""') + '"' : v);

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
export function exportConversation(title: string, turns: ChatTurn[], fmt: ExportFormat) {
	const md = conversationMarkdown(title, turns);
	let blob: Blob;
	if (fmt === 'md') blob = new Blob([md], { type: MIME.md });
	else if (fmt === 'docx') blob = buildDocx(title, md);
	else if (fmt === 'pptx') blob = buildPptx(title, md);
	else if (fmt === 'xlsx') blob = buildConversationXlsx(title, turns, md);
	else blob = buildConversationCsv(turns);
	download(`${fileBase(title)}.${fmt}`, blob);
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
export function exportAnswer(title: string, content: string, fmt: ExportFormat) {
	let blob: Blob;
	if (fmt === 'md') blob = new Blob([content], { type: MIME.md });
	else if (fmt === 'docx') blob = buildDocx(title, content);
	else if (fmt === 'pptx') blob = buildPptx(title, content);
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
