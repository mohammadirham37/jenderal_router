// report.ts — laporan Word (.docx) rapi dari markdown jawaban model, memakai
// docx-js (pedoman skill docx): heading bawaan Word, list lewat numbering
// (bukan karakter •), tabel dengan lebar DXA ganda + shading CLEAR, PageBreak
// di dalam Paragraph, tanpa "\n" di teks. Library dimuat dinamis saat dipakai.
import { parseMarkdown, inlineRuns, plain, type Block } from '../exporters.js';

const FONT = 'Calibri';
const MONO = 'Consolas';
const C = {
	navy: '0F1E46',
	accent: '2F5BEA',
	accent2: '0EA5E9',
	ink: '1F2937',
	muted: '64748B',
	line: 'CBD5E1',
	zebra: 'F3F6FB',
	soft: 'EEF3FF'
};
// A4 potret, margin 2,54 cm → lebar isi 9026 DXA
const PAGE_W = 11906;
const MARGIN = 1440;
const CONTENT_W = PAGE_W - MARGIN * 2;

export type ReportMeta = { title: string; subtitle?: string; author?: string };

/** pisahkan judul (# …) dan subjudul (*…* tepat setelahnya) dari isi */
export function splitMeta(fallbackTitle: string, md: string): { meta: ReportMeta; blocks: Block[] } {
	const blocks = parseMarkdown(md);
	let title = fallbackTitle;
	let subtitle: string | undefined;
	const first = blocks.findIndex((b) => b.kind !== 'para' || b.text.trim() !== '');
	if (blocks[first]?.kind === 'heading' && (blocks[first] as any).level === 1) {
		title = plain((blocks[first] as any).text);
		blocks.splice(first, 1);
		const nx = blocks[first] as Block | undefined;
		if (nx?.kind === 'para' && /^\*[^*].*\*$|^_[^_].*_$/.test(nx.text.trim()) && nx.text.length < 200) {
			subtitle = plain(nx.text.trim().replace(/^[*_]|[*_]$/g, ''));
			blocks.splice(first, 1);
		}
	}
	return { meta: { title, subtitle }, blocks };
}

export async function buildReportDocx(fallbackTitle: string, md: string): Promise<Blob> {
	const d = await import('docx');
	const {
		Document, Packer, Paragraph, TextRun, ExternalHyperlink, HeadingLevel, AlignmentType, Table, TableRow,
		TableCell, WidthType, ShadingType, BorderStyle, LevelFormat, Header, Footer, PageNumber, PageBreak,
		VerticalAlign, TableLayoutType
	} = d;
	const { meta, blocks } = splitMeta(fallbackTitle, md);

	// ---- run inline: tebal/miring/kode/link ----
	const runs = (text: string, opt: { size?: number; color?: string; bold?: boolean; italic?: boolean } = {}) =>
		inlineRuns(text).map((r) => {
			const base = {
				text: r.text,
				bold: opt.bold || r.bold,
				italics: opt.italic || r.italic,
				size: opt.size,
				color: opt.color,
				font: r.code ? MONO : undefined,
				shading: r.code ? { type: ShadingType.CLEAR, color: 'auto', fill: 'EEF1F6' } : undefined
			};
			return r.link
				? new ExternalHyperlink({ link: r.link, children: [new TextRun({ ...base, style: 'Hyperlink' })] })
				: new TextRun(base);
		});

	// ---- penomoran heading otomatis (1, 1.1, 1.1.1) bila model belum menomori ----
	const counters = [0, 0, 0];
	const alreadyNumbered = blocks.some((b) => b.kind === 'heading' && /^\d+(\.\d+)*[.)]?\s/.test(b.text));
	const numberHeading = (level: number, text: string) => {
		const lv = Math.min(3, Math.max(1, level - 1)) - 1; // ## → 0
		counters[lv]++;
		for (let k = lv + 1; k < counters.length; k++) counters[k] = 0;
		if (alreadyNumbered) return plain(text);
		return counters.slice(0, lv + 1).join('.') + '  ' + plain(text);
	};

	// ---- isi ----
	const body: (InstanceType<typeof Paragraph> | InstanceType<typeof Table>)[] = [];
	const toc: { level: number; text: string }[] = [];
	let listInstance = 0;
	let prevWasList = false;

	const cellPara = (text: string, header: boolean, align: (typeof AlignmentType)[keyof typeof AlignmentType]) =>
		new Paragraph({
			alignment: align,
			spacing: { before: 40, after: 40 },
			children: runs(text, { size: 19, bold: header, color: header ? 'FFFFFF' : C.ink })
		});

	const table = (header: string[], rows: string[][]) => {
		const cols = Math.max(header.length, ...rows.map((r) => r.length));
		// lebar kolom proporsional panjang isi (min 900 DXA), jumlah = lebar isi
		const weight = Array.from({ length: cols }, (_, c) =>
			Math.max(4, Math.min(40, Math.max(plain(header[c] ?? '').length, ...rows.map((r) => plain(r[c] ?? '').length))))
		);
		const total = weight.reduce((a, b) => a + b, 0);
		let widths = weight.map((w) => Math.max(900, Math.round((w / total) * CONTENT_W)));
		const scale = CONTENT_W / widths.reduce((a, b) => a + b, 0);
		widths = widths.map((w) => Math.floor(w * scale));
		widths[widths.length - 1] += CONTENT_W - widths.reduce((a, b) => a + b, 0);
		const numeric = Array.from({ length: cols }, (_, c) =>
			rows.length > 0 && rows.every((r) => /^[-+]?[\d.,]+%?$|^Rp\s?[\d.,]+$/.test(plain(r[c] ?? '').trim()) || !plain(r[c] ?? '').trim())
		);
		const border = { style: BorderStyle.SINGLE, size: 4, color: C.line };
		const borders = { top: border, bottom: border, left: border, right: border };
		const cell = (text: string, c: number, header: boolean, zebra: boolean) =>
			new TableCell({
				width: { size: widths[c], type: WidthType.DXA },
				borders,
				verticalAlign: VerticalAlign.CENTER,
				margins: { top: 60, bottom: 60, left: 110, right: 110 },
				shading: header
					? { type: ShadingType.CLEAR, color: 'auto', fill: C.navy }
					: zebra
						? { type: ShadingType.CLEAR, color: 'auto', fill: C.zebra }
						: undefined,
				children: [cellPara(text, header, numeric[c] && !header ? AlignmentType.RIGHT : AlignmentType.LEFT)]
			});
		return new Table({
			width: { size: CONTENT_W, type: WidthType.DXA },
			columnWidths: widths,
			layout: TableLayoutType.FIXED,
			rows: [
				new TableRow({ tableHeader: true, children: Array.from({ length: cols }, (_, c) => cell(header[c] ?? '', c, true, false)) }),
				...rows.map((r, ri) => new TableRow({ children: Array.from({ length: cols }, (_, c) => cell(r[c] ?? '', c, false, ri % 2 === 1)) }))
			]
		});
	};

	for (const b of blocks) {
		const isList = b.kind === 'bullet';
		if (isList && !prevWasList) listInstance++;
		prevWasList = isList;
		switch (b.kind) {
			case 'heading': {
				if (b.level === 1) {
					body.push(new Paragraph({ heading: HeadingLevel.HEADING_1, children: runs(b.text) }));
					toc.push({ level: 1, text: plain(b.text) });
					break;
				}
				const text = numberHeading(b.level, b.text);
				const hl = b.level <= 2 ? HeadingLevel.HEADING_1 : b.level === 3 ? HeadingLevel.HEADING_2 : HeadingLevel.HEADING_3;
				body.push(new Paragraph({ heading: hl, children: [new TextRun(text)] }));
				if (b.level <= 3) toc.push({ level: b.level <= 2 ? 1 : 2, text });
				break;
			}
			case 'para':
				body.push(new Paragraph({ children: runs(b.text), alignment: AlignmentType.JUSTIFIED }));
				break;
			case 'bullet':
				body.push(new Paragraph({
					numbering: { reference: b.ordered ? 'nomor' : 'butir', level: b.level, instance: listInstance },
					spacing: { after: 60 },
					children: runs(b.text)
				}));
				break;
			case 'quote':
				body.push(new Paragraph({
					indent: { left: 360, right: 360 },
					spacing: { before: 120, after: 160 },
					border: { left: { style: BorderStyle.SINGLE, size: 24, color: C.accent, space: 10 } },
					shading: { type: ShadingType.CLEAR, color: 'auto', fill: C.soft },
					children: runs(b.text, { italic: true, color: C.navy })
				}));
				break;
			case 'code':
				b.text.split('\n').forEach((line, li, all) =>
					body.push(new Paragraph({
						spacing: { before: li === 0 ? 120 : 0, after: li === all.length - 1 ? 160 : 0, line: 260 },
						shading: { type: ShadingType.CLEAR, color: 'auto', fill: 'F4F6FA' },
						indent: { left: 200, right: 200 },
						children: [new TextRun({ text: line || ' ', font: MONO, size: 18, color: '334155' })]
					}))
				);
				break;
			case 'table':
				body.push(table(b.header, b.rows));
				body.push(new Paragraph({ spacing: { after: 120 }, children: [] }));
				break;
		}
	}

	// ---- sampul ----
	const today = new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'long', year: 'numeric' }).format(new Date());
	const cover = [
		new Paragraph({ spacing: { before: 2600 }, children: [] }),
		new Paragraph({
			spacing: { after: 160 },
			border: { bottom: { style: BorderStyle.SINGLE, size: 36, color: C.accent, space: 1 } },
			children: [new TextRun({ text: 'LAPORAN', bold: true, size: 20, color: C.accent, characterSpacing: 120 })]
		}),
		new Paragraph({
			spacing: { before: 240, after: 200, line: 300 },
			children: [new TextRun({ text: meta.title, bold: true, size: 52, color: C.navy })]
		}),
		...(meta.subtitle
			? [new Paragraph({ spacing: { after: 400 }, children: [new TextRun({ text: meta.subtitle, size: 28, color: C.muted })] })]
			: []),
		new Paragraph({ spacing: { before: 3200 }, children: [new TextRun({ text: today, size: 22, color: C.ink, bold: true })] }),
		new Paragraph({ children: [new TextRun({ text: 'Disusun dengan bantuan JenderalRouter AI', size: 18, color: C.muted })] }),
		new Paragraph({ children: [new PageBreak()] })
	];

	// ---- daftar isi statis (terbaca di Word, LibreOffice, Google Docs) ----
	const tocParas =
		toc.length >= 3
			? [
					new Paragraph({
						spacing: { after: 240 },
						border: { bottom: { style: BorderStyle.SINGLE, size: 6, color: C.line, space: 4 } },
						children: [new TextRun({ text: 'Daftar Isi', bold: true, size: 32, color: C.navy })]
					}),
					...toc.map((e) => new Paragraph({
						indent: { left: e.level === 1 ? 0 : 480 },
						spacing: { after: e.level === 1 ? 100 : 60 },
						children: [new TextRun({ text: e.text, bold: e.level === 1, size: e.level === 1 ? 23 : 21, color: e.level === 1 ? C.ink : C.muted })]
					})),
					new Paragraph({ children: [new PageBreak()] })
				]
			: [];

	const bulletLevels = ['•', '◦', '▪'].map((ch, lv) => ({
		level: lv,
		format: LevelFormat.BULLET,
		text: ch,
		alignment: AlignmentType.LEFT,
		style: { paragraph: { indent: { left: 540 + lv * 360, hanging: 270 } } }
	}));
	const numberLevels = ['%1.', '%2)', '%3.'].map((fmt, lv) => ({
		level: lv,
		format: lv === 1 ? LevelFormat.LOWER_LETTER : LevelFormat.DECIMAL,
		text: fmt,
		alignment: AlignmentType.LEFT,
		style: { paragraph: { indent: { left: 540 + lv * 360, hanging: 300 } } }
	}));

	const doc = new Document({
		creator: 'JenderalRouter',
		title: meta.title,
		description: meta.subtitle ?? '',
		styles: {
			default: {
				document: { run: { font: FONT, size: 22, color: C.ink }, paragraph: { spacing: { after: 140, line: 288 } } }
			},
			paragraphStyles: [
				{
					id: 'Heading1', name: 'Heading 1', basedOn: 'Normal', next: 'Normal', quickFormat: true,
					run: { font: FONT, size: 32, bold: true, color: C.navy },
					paragraph: {
						spacing: { before: 420, after: 160 }, outlineLevel: 0, keepNext: true,
						border: { bottom: { style: BorderStyle.SINGLE, size: 6, color: C.line, space: 4 } }
					}
				},
				{
					id: 'Heading2', name: 'Heading 2', basedOn: 'Normal', next: 'Normal', quickFormat: true,
					run: { font: FONT, size: 26, bold: true, color: C.accent },
					paragraph: { spacing: { before: 300, after: 120 }, outlineLevel: 1, keepNext: true }
				},
				{
					id: 'Heading3', name: 'Heading 3', basedOn: 'Normal', next: 'Normal', quickFormat: true,
					run: { font: FONT, size: 23, bold: true, color: C.ink },
					paragraph: { spacing: { before: 220, after: 80 }, outlineLevel: 2, keepNext: true }
				}
			],
			characterStyles: [
				{ id: 'Hyperlink', name: 'Hyperlink', basedOn: 'DefaultParagraphFont', run: { color: C.accent, underline: {} } }
			]
		},
		numbering: {
			config: [
				{ reference: 'butir', levels: bulletLevels },
				{ reference: 'nomor', levels: numberLevels }
			]
		},
		sections: [
			{
				properties: {
					page: { size: { width: PAGE_W, height: 16838 }, margin: { top: MARGIN, bottom: MARGIN, left: MARGIN, right: MARGIN } },
					titlePage: true // sampul tanpa header/footer
				},
				headers: {
					default: new Header({
						children: [
							new Paragraph({
								alignment: AlignmentType.RIGHT,
								border: { bottom: { style: BorderStyle.SINGLE, size: 4, color: C.line, space: 4 } },
								children: [new TextRun({ text: meta.title, size: 16, color: C.muted })]
							})
						]
					}),
					first: new Header({ children: [new Paragraph({ children: [] })] })
				},
				footers: {
					default: new Footer({
						children: [
							new Paragraph({
								alignment: AlignmentType.CENTER,
								children: [
									new TextRun({ children: ['Halaman ', PageNumber.CURRENT, ' dari ', PageNumber.TOTAL_PAGES], size: 16, color: C.muted })
								]
							})
						]
					}),
					first: new Footer({ children: [new Paragraph({ children: [] })] })
				},
				children: [...cover, ...tocParas, ...body]
			}
		]
	});
	return Packer.toBlob(doc);
}
