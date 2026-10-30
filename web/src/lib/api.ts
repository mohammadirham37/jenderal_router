// api.ts — klien HTTP dashboard: sesi cookie + CSRF double-submit.
let csrfToken = '';

export function setCSRF(t: string) {
	csrfToken = t || '';
	sessionStoreSet('jr_csrf', csrfToken);
}
export function resetCSRF() {
	csrfToken = '';
	sessionStorage.removeItem('jr_csrf');
}
export function getCSRF() {
	return csrfToken;
}
function sessionStoreSet(k: string, v: string) {
	try { sessionStorage.setItem(k, v); } catch { /* ignore */ }
}
try { csrfToken = sessionStorage.getItem('jr_csrf') || ''; } catch { /* ignore */ }

export class ApiError extends Error {
	status: number;
	data: any;
	constructor(status: number, message: string, data?: any) {
		super(message);
		this.status = status;
		this.data = data;
	}
}

let onUnauthorized: (() => void) | null = null;
export function setUnauthorizedHandler(fn: () => void) {
	onUnauthorized = fn;
}

async function request(method: string, path: string, body?: any, opts: any = {}) {
	const headers: Record<string, string> = { 'Content-Type': 'application/json' };
	if (csrfToken) headers['X-CSRF-Token'] = csrfToken;
	const res = await fetch(path, {
		method,
		headers,
		credentials: 'same-origin',
		body: body !== undefined ? JSON.stringify(body) : undefined
	});
	let data: any = null;
	try { data = await res.json(); } catch { /* bukan JSON */ }
	if (!res.ok) {
		if (res.status === 401 && !opts.noRedirect && !path.startsWith('/v1')) {
			onUnauthorized?.();
		}
		throw new ApiError(res.status, (data && (data.error || data.message)) || `HTTP ${res.status}`, data);
	}
	return data;
}

export const api = {
	get: (p: string, opts?: any) => request('GET', p, undefined, opts),
	post: (p: string, b?: any, opts?: any) => request('POST', p, b, opts),
	put: (p: string, b?: any, opts?: any) => request('PUT', p, b, opts),
	patch: (p: string, b?: any, opts?: any) => request('PATCH', p, b, opts),
	del: (p: string, opts?: any) => request('DELETE', p, undefined, opts),

	// runEvents mengikuti event run agen chat (SSE) mulai indeks from sampai
	// [END]; melempar ApiError (dengan status) bila run tidak ditemukan.
	async runEvents(id: string, from: number, signal: AbortSignal, onEvent: (ev: any) => void) {
		const res = await fetch(`/api/me/runs/${id}/events?from=${from}`, { credentials: 'same-origin', signal });
		if (!res.ok || !res.body) {
			let msg = `HTTP ${res.status}`;
			try { const j = await res.json(); msg = j.error || msg; } catch { /* */ }
			throw new ApiError(res.status, msg);
		}
		const reader = res.body.getReader();
		const dec = new TextDecoder();
		let buf = '';
		for (;;) {
			const { done, value } = await reader.read();
			if (done) throw new Error('koneksi ke server terputus');
			buf += dec.decode(value, { stream: true });
			const parts = buf.split('\n\n');
			buf = parts.pop() ?? '';
			for (const part of parts) {
				for (const line of part.split('\n')) {
					if (!line.startsWith('data:')) continue;
					const payload = line.slice(5).trim();
					if (payload === '[END]') return;
					onEvent(JSON.parse(payload));
				}
			}
		}
	},

	// streaming chat playground (konsumsi SSE gaya OpenAI)
	async chatStream(
		opts: { model: string; messages: any[]; conversationId?: number | null; temperature?: number; maxTokens?: number; tools?: any[]; signal?: AbortSignal },
		onDelta: (text: string) => void,
		onMeta?: (usage: any) => void,
		onExtra?: (e: { reasoning?: string; model?: string; trimmed?: { n: number; window: number } }) => void,
		onChunk?: (j: any) => void
	) {
		const res = await fetch('/api/me/chat', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken },
			credentials: 'same-origin',
			body: JSON.stringify({ model: opts.model, messages: opts.messages, conversation_id: opts.conversationId ?? undefined, temperature: opts.temperature, max_tokens: opts.maxTokens, tools: opts.tools, stream: true }),
			signal: opts.signal
		});
		if (!res.ok || !res.body) {
			let msg = `HTTP ${res.status}`;
			try { const j = await res.json(); msg = j.error || msg; } catch { /* */ }
			throw new Error(msg);
		}
		const reader = res.body.getReader();
		const dec = new TextDecoder();
		let buf = '';
		for (;;) {
			const { done, value } = await reader.read();
			if (done) break;
			buf += dec.decode(value, { stream: true });
			const parts = buf.split('\n\n');
			buf = parts.pop() ?? '';
			for (const ev of parts) {
				for (const line of ev.split('\n')) {
					if (!line.startsWith('data:')) continue;
					const payload = line.slice(5).trim();
					if (payload === '[DONE]') return;
					try {
						const j = JSON.parse(payload);
						if (j.error) throw new Error(j.error.message || 'upstream error');
						if (onChunk) onChunk(j);
						const delta = j.choices?.[0]?.delta;
						if (delta?.reasoning_content && onExtra) onExtra({ reasoning: delta.reasoning_content });
						if (delta?.content) onDelta(delta.content);
						if (j.reasoning_content && onExtra) onExtra({ reasoning: j.reasoning_content });
						if (j.served_model && onExtra) onExtra({ model: j.served_model });
						if (j.context_trimmed && onExtra) onExtra({ trimmed: { n: j.context_trimmed, window: j.context_window } });
						if (j.usage && onMeta) onMeta(j.usage);
					} catch (e: any) {
						if (e.message && !/JSON/.test(e.message)) throw e;
					}
				}
			}
		}
	}
};

// ---- formatter util (dipakai lintas halaman) ----
export function fmtNum(n: number | undefined | null): string {
	if (n === undefined || n === null) return '0';
	return new Intl.NumberFormat('id-ID').format(n);
}
export function fmtCost(v: any): string {
	return '$' + (parseFloat(v || 0) || 0).toFixed(4);
}
export function fmtMs(v: any): string {
	return (v || 0) + ' ms';
}
export function truncate(s: string, n: number): string {
	return s.length <= n ? s : s.slice(0, n) + '…';
}

// waktu server (UTC RFC3339) → tampilan WIB (Asia/Jakarta)
const TZ = 'Asia/Jakarta';
export function fmtTs(ts: string | undefined | null): string {
	if (!ts) return '';
	// datetime('now') SQLite = UTC tanpa zona ("2026-10-29 10:00:00") — tanpa
	// penanda Z browser membacanya sebagai waktu lokal (meleset 7 jam di WIB)
	const iso = /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(\.\d+)?$/.test(ts) ? ts.replace(' ', 'T') + 'Z' : ts;
	const d = new Date(iso);
	if (isNaN(d.getTime())) return ts;
	const p = new Intl.DateTimeFormat('en-GB', {
		timeZone: TZ,
		year: 'numeric', month: '2-digit', day: '2-digit',
		hour: '2-digit', minute: '2-digit', second: '2-digit',
		hour12: false
	}).formatToParts(d);
	const g = (t: string) => p.find((x) => x.type === t)?.value ?? '';
	return `${g('year')}-${g('month')}-${g('day')} ${g('hour')}:${g('minute')}:${g('second')} WIB`;
}

// markdown mini → html (heading, bold, italic, code, list, link)
export { md } from './markdown.js';
