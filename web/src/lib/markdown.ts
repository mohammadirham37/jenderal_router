// markdown.ts — renderer markdown ringan untuk jawaban model (dipakai dengan
// {@html}). Aman XSS: SEMUA teks di-escape lebih dulu; satu-satunya atribut
// yang dibentuk dari input adalah href link http(s) (sudah ter-escape) dan
// label bahasa blok kode (disaring ke [a-z0-9+#.-]). Tanpa gambar/HTML mentah.

const esc = (x: string) =>
	x.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c] as string);

// Model sering menulis <br> dan entitas HTML (&nbsp; dll) — terutama di sel
// tabel untuk kode multi-baris. Ubah ke karakter biasa DULU; setelah itu
// semuanya tetap di-escape seperti teks lain (aman XSS).
export function decodeSafe(s: string): string {
	return s
		.replace(/<br\s*\/?>/gi, '\n')
		.replace(/&nbsp;/g, '\u00a0')
		.replace(/&lt;/g, '<')
		.replace(/&gt;/g, '>')
		.replace(/&quot;/g, '"')
		.replace(/&#0?39;|&apos;/g, "'")
		.replace(/&amp;/g, '&');
}

export const looksLikeCode = (s: string) =>
	/[{};]|:=|=>|^\s*(func|def|class|import|package|return|if|for|const|let|var)\b|^[\u00a0 ]{2,}\S/m.test(s);

// inline: kode & link diamankan dulu sebagai placeholder agar format
// (*, **, ~~) di dalamnya tidak ikut diubah; baris baru dari <br> → <br>
function inline(src: string): string {
	const text = decodeSafe(src);
	return text.includes('\n') ? text.split('\n').map(inlineOne).join('<br>') : inlineOne(text);
}

// isi sel tabel: kode multi-baris tampil sebagai blok kode kecil
function cellHtml(src: string): string {
	const text = decodeSafe(src);
	if (text.includes('\n') && looksLikeCode(text)) return `<pre class="cell-code"><code>${esc(text)}</code></pre>`;
	return inline(src);
}

function inlineOne(src: string): string {
	const slots: string[] = [];
	const keep = (html: string) => `\u0000${slots.push(html) - 1}\u0000`;
	let s = esc(src);
	s = s.replace(/`([^`]+)`/g, (_, c) => keep(`<code>${c}</code>`));
	s = s.replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, (_, text, url) =>
		keep(`<a href="${url}" target="_blank" rel="noopener noreferrer">${text}</a>`));
	s = s.replace(/(^|[\s(])(https?:\/\/[^\s<]+[^\s<.,;:!?)\]'"])/g, (_, pre, url) =>
		pre + keep(`<a href="${url}" target="_blank" rel="noopener noreferrer">${url}</a>`));
	s = s
		.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
		.replace(/__([^_]+)__/g, '<strong>$1</strong>')
		.replace(/(^|[^*\w])\*([^*\n]+)\*(?!\w)/g, '$1<em>$2</em>')
		.replace(/~~([^~]+)~~/g, '<del>$1</del>');
	return s.replace(/\u0000(\d+)\u0000/g, (_, i) => slots[+i]);
}

const isTableSep = (l: string) => /^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)+\|?\s*$/.test(l);
const cells = (l: string) =>
	l.trim().replace(/^\|/, '').replace(/\|$/, '').split('|').map((c) => c.trim());

export function md(src: string): string {
	const lines = (src || '').replace(/\r\n?/g, '\n').split('\n');
	let html = '';
	let para: string[] = [];
	let list: 'ul' | 'ol' | null = null;
	let quote: string[] = [];

	const flushPara = () => {
		if (para.length) html += `<p>${para.map(inline).join('<br>')}</p>`;
		para = [];
	};
	const closeList = () => {
		if (list) html += `</${list}>`;
		list = null;
	};
	const flushQuote = () => {
		if (quote.length) html += `<blockquote>${md(quote.join('\n'))}</blockquote>`;
		quote = [];
	};
	const flushAll = () => {
		flushPara();
		closeList();
		flushQuote();
	};

	for (let i = 0; i < lines.length; i++) {
		const line = lines[i];
		const fence = line.match(/^\s*```\s*([\w+#.-]*)/);
		if (fence) {
			flushAll();
			const lang = (fence[1] || '').toLowerCase().replace(/[^a-z0-9+#.-]/g, '').slice(0, 20);
			const buf: string[] = [];
			i++;
			while (i < lines.length && !/^\s*```/.test(lines[i])) buf.push(lines[i++]);
			html +=
				`<div class="code-block"><div class="code-head"><span>${lang || 'kode'}</span>` +
				`<button type="button" class="code-copy">Salin</button></div>` +
				`<pre><code>${esc(buf.join('\n'))}</code></pre></div>`;
			continue;
		}
		if (/^\s*>/.test(line)) {
			flushPara();
			closeList();
			quote.push(line.replace(/^\s*>\s?/, ''));
			continue;
		}
		flushQuote();
		const h = line.match(/^(#{1,6})\s+(.*)$/);
		if (h) {
			flushAll();
			const lvl = Math.min(5, h[1].length + 2); // # → h3 … (skala chat)
			html += `<h${lvl}>${inline(h[2].replace(/^#+\s*/, ''))}</h${lvl}>`;
			continue;
		}
		if (/^\s*(?:(?:-\s*){3,}|(?:\*\s*){3,}|(?:_\s*){3,})$/.test(line)) {
			flushAll();
			html += '<hr>';
			continue;
		}
		if (/^\s*\|.*\|\s*$/.test(line) && i + 1 < lines.length && isTableSep(lines[i + 1])) {
			flushAll();
			const head = cells(line);
			html += '<div class="table-scroll"><table><thead><tr>' + head.map((c) => `<th>${cellHtml(c)}</th>`).join('') + '</tr></thead><tbody>';
			i += 2;
			while (i < lines.length && /^\s*\|.*\|\s*$/.test(lines[i])) {
				html += '<tr>' + cells(lines[i]).map((c) => `<td>${cellHtml(c)}</td>`).join('') + '</tr>';
				i++;
			}
			i--;
			html += '</tbody></table></div>';
			continue;
		}
		const ul = line.match(/^\s*[-*+]\s+(.*)$/);
		const ol = line.match(/^\s*\d+[.)]\s+(.*)$/);
		if (ul || ol) {
			flushPara();
			const kind = ul ? 'ul' : 'ol';
			if (list !== kind) {
				closeList();
				html += `<${kind}>`;
				list = kind;
			}
			html += `<li>${inline((ul || ol)![1])}</li>`;
			continue;
		}
		if (line.trim() === '') {
			flushPara();
			closeList();
			continue;
		}
		closeList();
		para.push(line);
	}
	flushAll();
	return html || '<p></p>';
}
