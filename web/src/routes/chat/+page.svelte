<script lang="ts">
	import { MessageSquare, Plus, Send, Trash2, X, Bot, Sparkles } from '@lucide/svelte';
	import { api, md } from '$lib/api';
	import { t, toast } from '$lib/stores.svelte';

	type Msg = { role: 'user' | 'assistant'; content: string; provider?: string };

	let models = $state<any[]>([]);
	let model = $state('');
	let conversations = $state<any[]>([]);
	let current = $state<number | null>(null);
	let msgs = $state<Msg[]>([]);
	let input = $state('');
	let sending = $state(false);
	let quota = $state<any>(null);
	let msgsEl = $state<HTMLElement | null>(null);

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

	// ingat pilihan model antar-kunjungan
	$effect(() => {
		if (model) try { localStorage.setItem('jr_chat_model', model); } catch { /* */ }
	});

	async function loadConversations() {
		try {
			const res = await api.get('/api/me/conversations');
			conversations = res.conversations || [];
			// pulihkan sesi: buka percakapan terakhir bila belum ada yang terbuka
			if (current === null && conversations.length > 0) {
				await openConversation(conversations[0].id);
			}
		} catch { /* */ }
	}
	async function loadQuota() {
		try { quota = await api.get('/api/me/usage'); } catch { /* */ }
	}

	async function newConversation() {
		const res = await api.post('/api/me/conversations', { title: t('new_chat'), model });
		current = res.conversation.id;
		msgs = [];
		loadConversations();
	}
	async function openConversation(id: number) {
		current = id;
		try {
			const res = await api.get('/api/me/conversations/' + id);
			msgs = (res.messages || []).map((m: any) => ({ role: m.role, content: m.content, provider: m.provider_name }));
			scrollBottom();
		} catch (e: any) { toast(e.message, 'err'); }
	}
	async function delConversation(id: number) {
		if (!confirm(t('confirm_delete'))) return;
		await api.del('/api/me/conversations/' + id);
		if (current === id) { current = null; msgs = []; }
		loadConversations();
	}

	function scrollBottom() {
		setTimeout(() => msgsEl?.scrollTo({ top: msgsEl.scrollHeight }), 30);
	}

	async function send() {
		const text = input.trim();
		if (!text || sending) return;
		sending = true;
		try {
			if (!current) await newConversation();
		} catch (e: any) {
			toast('buat percakapan gagal: ' + e.message, 'err');
			sending = false;
			return;
		}
		input = '';
		msgs.push({ role: 'user', content: text });
		const history = msgs.map((m) => ({ role: m.role, content: m.content }));
		// ambil referensi PROXY dari array $state — memutasi objek mentah
		// tidak memicu reaktivitas Svelte 5
		msgs.push({ role: 'assistant', content: '' });
		const bubble = msgs[msgs.length - 1];
		scrollBottom();

		try {
			(globalThis as any).__deltas = 0;
			(globalThis as any).__deltaText = '';
			await api.chatStream(
				{ model, messages: history },
				(delta) => {
					bubble.content += delta;
					msgs = [...msgs]; // paksa reaktivitas array
					scrollBottom();
				},
				() => loadQuota()
			);
			if (!bubble.content) bubble.content = '(kosong)';
			await api.post(`/api/me/conversations/${current}/messages`, { role: 'user', content: text });
			await api.post(`/api/me/conversations/${current}/messages`, { role: 'assistant', content: bubble.content, model });
			// judul dari pesan pertama bila masih default — supaya daftar di sidebar bermakna
			const conv = conversations.find((c) => c.id === current);
			if (conv && (!conv.title || conv.title === t('new_chat') || conv.title === 'Chat baru' || conv.title === 'New chat')) {
				const title = text.length > 48 ? text.slice(0, 48) + '…' : text;
				try { await api.patch(`/api/me/conversations/${current}`, { title }); } catch { /* */ }
			}
			loadQuota();
			loadConversations();
		} catch (e: any) {
			bubble.content += `\n\n**⚠ ${e.message}**`;
		} finally {
			sending = false;
			scrollBottom();
		}
	}
</script>

<svelte:head><title>{t('chat')} — JenderalRouter</title></svelte:head>

<div class="page-head">
	<h1 class="page-title"><MessageSquare size={20} /> {t('chat')}</h1>
	<div class="row" style="max-width:320px;width:100%">
		<select bind:value={model} class="grow">
			{#each models as m (m.id)}
				<option value={m.id}>{m.id}{m.local ? ' · lokal' : ''}</option>
			{/each}
		</select>
	</div>
</div>

<div class="chat-layout">
	<div class="card" style="display:flex;flex-direction:column">
		<button class="btn" style="width:100%;margin-bottom:10px" onclick={newConversation}>
			<Plus size={15} /> {t('new_chat')}
		</button>
		<div style="overflow-y:auto;max-height:42vh">
			{#each conversations as c (c.id)}
				<div class="conv-item {current === c.id ? 'active' : ''}" onclick={() => openConversation(c.id)} onkeydown={(e) => e.key === 'Enter' && openConversation(c.id)} role="button" tabindex="0">
					<span class="small" style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap">{c.title || '…'}</span>
					<button class="icon-btn" style="width:22px;height:22px" onclick={(e) => { e.stopPropagation(); delConversation(c.id); }}>
						<X size={12} />
					</button>
				</div>
			{/each}
		</div>
		<div class="muted small" style="margin-top:auto;border-top:1px solid var(--border);padding-top:10px">
			<div style="font-weight:700;margin-bottom:4px">{t('quota_left')}</div>
			{#each quota?.quotas ?? [] as q (q.period)}
				<div class="mono">{q.period}: {q.used_tokens}/{q.token_limit}</div>
			{/each}
			{#if quota?.today}
				<div class="muted" style="margin-top:4px">{t('requests')}: {quota.today.requests}</div>
			{/if}
		</div>
	</div>

	<div class="card chat-pane">
		<div class="chat-msgs" bind:this={msgsEl}>
			{#if msgs.length === 0}
				<div class="kv" style="justify-content:center;padding:40px 0;color:var(--muted)">
					<Sparkles size={18} /> Pilih model, lalu mulai mengobrol — streaming + riwayat tersimpan
				</div>
			{/if}
			{#each msgs as m, i (i)}
				<div class="msg {m.role}">
					{#if m.role === 'assistant'}
						<div class="row" style="margin-bottom:4px;color:var(--muted);font-size:11px"><Bot size={12} /> assistant</div>
					{/if}
					<!-- eslint-disable-next-line svelte/no-at-html-tags -->
					{@html md(m.content)}
					{#if m.provider}
						<div class="meta"><Sparkles size={11} /> {t('answered_by')}: {m.provider}</div>
					{/if}
				</div>
			{/each}
		</div>
		<div class="chat-input-row">
			<textarea
				bind:value={input}
				placeholder="Tulis pesan… (Enter = kirim, Shift+Enter = baris baru)"
				onkeydown={(e) => {
					if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); }
				}}
			></textarea>
			<button class="btn" onclick={send} disabled={sending || !input.trim()}>
				{#if sending}<span class="spinner"></span>{:else}<Send size={16} />{/if}
				{t('send')}
			</button>
		</div>
	</div>
</div>
