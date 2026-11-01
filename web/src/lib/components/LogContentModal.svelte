<script lang="ts">
	// Isi request API + jawaban satu baris log, dipecah per pesan dalam
	// dropdown (System/User/AI/Tool) agar mudah dibaca. Setiap pembukaan
	// tercatat di audit log server (log.content.view); isi kedaluwarsa 7 hari.
	import { ShieldAlert, Wrench, Settings2, UserRound, Bot, Info, Paperclip, ChevronsDownUp, ChevronsUpDown } from '@lucide/svelte';
	import { api, fmtTs, fmtNum, md } from '#lib/api.js';
	import { copyText } from '#lib/clipboard.js';
	import Modal from '#lib/components/Modal.svelte';

	let { logId, onclose }: { logId: number | string; onclose: () => void } = $props();

	type Msg = { role: string; content?: string; tool_calls?: { name: string; arguments?: string }[]; tool_call_id?: string };
	type Item = Msg & { key: string; index: number; open: boolean };

	let data = $state<any>(null);
	let err = $state('');
	let items = $state<Item[]>([]);
	let respOpen = $state(true);
	let hidden = $state<Record<string, boolean>>({});

	$effect(() => {
		data = null;
		err = '';
		api.get(`/api/admin/logs/${logId}/content`)
			.then((r) => {
				data = r;
				const msgs: Msg[] = r.content?.request?.messages ?? [];
				const lastUser = msgs.map((m) => m.role).lastIndexOf('user');
				items = msgs.map((m, i) => ({
					...m,
					key: 'm' + i,
					index: i + 1,
					// awalnya terbuka: pesan user terakhir, dan semua pesan bila percakapannya pendek
					open: i === lastUser || (msgs.length <= 3 && m.role !== 'system')
				}));
			})
			.catch((e) => (err = e.message));
	});
	const c = $derived(data?.content);

	const ROLES: Record<string, { label: string; icon: any }> = {
		system: { label: 'System', icon: Settings2 },
		user: { label: 'User', icon: UserRound },
		assistant: { label: 'AI', icon: Bot },
		tool: { label: 'Hasil tool', icon: Wrench },
		info: { label: 'Info', icon: Info }
	};
	const roleOf = (r: string) => ROLES[r] ?? { label: r, icon: Info };
	const counts = $derived(items.reduce((a: Record<string, number>, m) => ((a[m.role] = (a[m.role] ?? 0) + 1), a), {}));
	const visible = $derived(items.filter((m) => !hidden[m.role]));

	const allOpen = $derived(respOpen && visible.every((m) => m.open));
	function setAll(open: boolean) {
		for (const m of items) m.open = open;
		respOpen = open;
	}

	// tombol "Salin" pada blok kode hasil md()
	async function onConvClick(e: MouseEvent) {
		const btn = (e.target as HTMLElement).closest('.code-copy') as HTMLButtonElement | null;
		if (!btn) return;
		const code = btn.closest('.code-block')?.querySelector('code')?.textContent ?? '';
		if (await copyText(code)) {
			btn.textContent = 'Tersalin ✓';
			setTimeout(() => (btn.textContent = 'Salin'), 1500);
		}
	}

	// cuplikan satu baris untuk judul dropdown
	function preview(m: Msg): string {
		const text = (m.content ?? '').replace(LAMPIRAN_RE, (_, attrs) => `[lampiran ${attr(attrs, 'nama') || ''}] `);
		const s = text.replace(/\s+/g, ' ').trim();
		if (s) return s.length > 110 ? s.slice(0, 110) + '…' : s;
		if (m.tool_calls?.length) return m.tool_calls.map((t) => t.name + '(…)').join(', ');
		return '(kosong)';
	}

	// rapikan JSON (argumen tool / hasil tool) bila valid
	function pretty(s: string | undefined): string | null {
		const t = (s ?? '').trim();
		if (!t || (t[0] !== '{' && t[0] !== '[')) return null;
		try { return JSON.stringify(JSON.parse(t), null, 2); } catch { return null; }
	}

	// pecah isi pesan user: teks biasa + blok <lampiran …> (dari chat/klien)
	const LAMPIRAN_RE = /<lampiran ([^>]*)>\n?([\s\S]*?)\n?<\/lampiran>/g;
	const attr = (attrs: string, k: string) => new RegExp(`${k}="([^"]*)"`).exec(attrs)?.[1] ?? '';
	type Part = { kind: 'text'; text: string } | { kind: 'att'; name: string; meta: string; text: string };
	function parts(content: string): Part[] {
		const out: Part[] = [];
		let last = 0;
		for (const m of content.matchAll(LAMPIRAN_RE)) {
			const before = content.slice(last, m.index).trim();
			if (before) out.push({ kind: 'text', text: before });
			const a = m[1];
			const meta = [attr(a, 'jenis'), attr(a, 'halaman') && attr(a, 'halaman') + ' hlm', attr(a, 'slide') && attr(a, 'slide') + ' slide', attr(a, 'sheet') && attr(a, 'sheet') + ' sheet']
				.filter(Boolean).join(' · ');
			out.push({ kind: 'att', name: attr(a, 'nama') || 'lampiran', meta, text: m[2] });
			last = (m.index ?? 0) + m[0].length;
		}
		const rest = content.slice(last).trim();
		if (rest) out.push({ kind: 'text', text: rest });
		return out;
	}
</script>

{#snippet toolCalls(calls: { name: string; arguments?: string }[] | undefined)}
	{#each calls ?? [] as tc, j (j)}
		{@const pj = pretty(tc.arguments)}
		<div class="tcall">
			<div class="tname"><Wrench size={12} /> <span class="mono">{tc.name}</span></div>
			{#if pj}<pre class="code">{pj}</pre>
			{:else if tc.arguments}<pre class="code">{tc.arguments}</pre>{/if}
		</div>
	{/each}
{/snippet}

{#snippet body(m: Msg)}
	{#if m.role === 'assistant'}
		<!-- md() meng-escape HTML sebelum merender markdown -->
		{#if m.content}<div class="md-render">{@html md(m.content)}</div>{/if}
	{:else if m.role === 'tool'}
		{@const pj = pretty(m.content)}
		{#if pj}<pre class="code">{pj}</pre>{:else}<div class="plain">{m.content}</div>{/if}
	{:else if m.content}
		{#each parts(m.content) as p, j (j)}
			{#if p.kind === 'text'}
				<div class="plain">{p.text}</div>
			{:else}
				<details class="att">
					<summary><Paperclip size={12} /> <b>{p.name}</b> <span class="muted">{p.meta}{p.meta ? ' · ' : ''}{fmtNum(p.text.length)} karakter</span></summary>
					<div class="plain">{p.text}</div>
				</details>
			{/if}
		{/each}
	{/if}
	{@render toolCalls(m.tool_calls)}
{/snippet}

<Modal wide title={`Isi request #${logId}`} {onclose}>
	{#if err}
		<p class="muted">{err}</p>
	{:else if !data}
		<span class="spinner dark"></span>
	{:else}
		<div class="muted small meta">
			<span>{data.user_email}</span>
			{#if data.key_prefix}<span class="mono">{data.key_prefix}… {data.key_name}</span>{/if}
			<span>{fmtTs(data.ts)}</span>
			<span class="mono">{c.request.model}{c.request.stream ? ' · stream' : ''}</span>
			{#if c.request.tools?.length}<span><Wrench size={11} /> {c.request.tools.join(', ')}</span>{/if}
		</div>
		<p class="muted small note">
			<ShieldAlert size={12} /> Akses ini tercatat di audit log. Rahasia yang dikenali (API key, token, kata sandi, nomor kartu)
			sudah disamarkan.
			{#if data.expires_at}Isi dihapus otomatis {fmtTs(data.expires_at)}.{/if}
			{#if c.truncated}<b>Sebagian isi dipotong karena batas ukuran log.</b>{/if}
		</p>

		<div class="toolbar">
			{#each Object.entries(counts) as [role, n] (role)}
				{@const R = roleOf(role)}
				<button class="chip {role}" class:off={hidden[role]} onclick={() => (hidden[role] = !hidden[role])}
					title={hidden[role] ? 'Tampilkan' : 'Sembunyikan'}>
					<R.icon size={12} /> {R.label} <b>{n}</b>
				</button>
			{/each}
			<span class="grow"></span>
			<button class="btn ghost sm" onclick={() => setAll(!allOpen)}>
				{#if allOpen}<ChevronsDownUp size={13} /> Tutup semua{:else}<ChevronsUpDown size={13} /> Buka semua{/if}
			</button>
		</div>

		<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
		<div class="conv" onclick={onConvClick}>
			{#each visible as m (m.key)}
				{@const R = roleOf(m.role)}
				<details class="msg {m.role}" bind:open={m.open}>
					<summary>
						<span class="badge-role {m.role}"><R.icon size={12} /> {R.label}</span>
						<span class="idx muted">#{m.index}</span>
						{#if !m.open}<span class="prev muted">{preview(m)}</span>{:else}<span class="grow"></span>{/if}
						{#if m.tool_call_id}<span class="mono muted small">{m.tool_call_id}</span>{/if}
						<span class="size muted">{fmtNum((m.content ?? '').length)} kar</span>
					</summary>
					<div class="mbody">{@render body(m)}</div>
				</details>
			{/each}

			<details class="msg response" bind:open={respOpen}>
				<summary>
					<span class="badge-role response"><Bot size={12} /> Jawaban model</span>
					{#if !respOpen}<span class="prev muted">{preview(c.response)}</span>{:else}<span class="grow"></span>{/if}
					<span class="size muted">{fmtNum((c.response.content ?? '').length)} kar</span>
				</summary>
				<div class="mbody">
					{@render body({ role: 'assistant', ...c.response })}
					{#if !c.response.content && !c.response.tool_calls?.length}<span class="muted small">(tidak ada jawaban)</span>{/if}
					{#if c.error}<div class="badge err" style="margin-top:6px">{c.error}</div>{/if}
				</div>
			</details>
		</div>
	{/if}
</Modal>

<style>
	.meta { display: flex; flex-wrap: wrap; gap: 4px 12px; margin-bottom: 6px; }
	.note { margin: 0 0 10px; }
	.toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; margin-bottom: 10px; }
	.chip {
		all: unset; cursor: pointer; display: inline-flex; align-items: center; gap: 5px; font-size: 12px;
		padding: 3px 10px; border-radius: 999px; border: 1px solid var(--border); background: var(--surface-2);
	}
	.chip.off { opacity: .45; text-decoration: line-through; }
	.conv { max-height: 65vh; overflow: auto; display: flex; flex-direction: column; gap: 8px; padding-right: 2px; }

	.msg { border: 1px solid var(--border); border-left-width: 3px; border-radius: 10px; background: var(--surface); }
	.msg.system { border-left-color: var(--muted); }
	.msg.user { border-left-color: var(--accent); }
	.msg.assistant, .msg.response { border-left-color: var(--ok, #22c55e); }
	.msg.tool { border-left-color: var(--warn); }
	.msg.info { border-style: dashed; }
	.msg.response { border-color: color-mix(in srgb, var(--accent) 45%, var(--border)); border-left-color: var(--ok, #22c55e); }
	summary {
		display: flex; align-items: center; gap: 8px; padding: 8px 12px; cursor: pointer; list-style: none;
		font-size: 12.5px; min-width: 0;
	}
	summary::-webkit-details-marker { display: none; }
	summary::before { content: '▸'; color: var(--muted); transition: transform .15s; }
	details[open] > summary::before { transform: rotate(90deg); }
	details[open] > summary { border-bottom: 1px solid var(--border); }
	.badge-role { display: inline-flex; align-items: center; gap: 4px; font-weight: 700; white-space: nowrap; }
	.idx { font-size: 11px; }
	.prev { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.size { font-size: 11px; white-space: nowrap; margin-left: auto; }
	.grow { flex: 1; }
	.mbody { padding: 10px 12px; font-size: 13px; overflow-wrap: anywhere; }
	.plain { white-space: pre-wrap; }
	.plain + .plain, .att + .plain, .plain + .att, .att + .att { margin-top: 8px; }
	.code {
		margin: 6px 0 0; padding: 8px 10px; border-radius: 8px; background: var(--surface-2); font-size: 12px;
		white-space: pre-wrap; overflow-wrap: anywhere; max-height: 360px; overflow: auto;
	}
	.tcall { margin-top: 8px; }
	.tname { display: flex; align-items: center; gap: 5px; font-size: 12px; color: var(--muted); }
	.att { border: 1px solid var(--border); border-radius: 8px; background: var(--surface-2); }
	.att summary { padding: 6px 10px; font-size: 12px; }
	.att .plain { padding: 8px 10px; max-height: 320px; overflow: auto; font-size: 12.5px; }
</style>
