<script lang="ts">
	// Isi request API + jawaban satu baris log. Setiap pembukaan tercatat di
	// audit log server (log.content.view); isi kedaluwarsa setelah 7 hari.
	import { ShieldAlert, Wrench } from '@lucide/svelte';
	import { api, fmtTs } from '#lib/api.js';
	import Modal from '#lib/components/Modal.svelte';

	let { logId, onclose }: { logId: number | string; onclose: () => void } = $props();

	let data = $state<any>(null);
	let err = $state('');
	$effect(() => {
		data = null;
		err = '';
		api.get(`/api/admin/logs/${logId}/content`)
			.then((r) => (data = r))
			.catch((e) => (err = e.message));
	});
	const c = $derived(data?.content);
	const ROLE: Record<string, string> = { system: 'System', user: 'User', assistant: 'AI', tool: 'Hasil tool', info: 'Info' };
</script>

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

		<div class="conv">
			{#each c.request.messages as m, i (i)}
				<div class="cmsg {m.role}">
					<div class="muted small"><b>{ROLE[m.role] ?? m.role}</b>{#if m.tool_call_id} · <span class="mono">{m.tool_call_id}</span>{/if}</div>
					{#if m.content}<div class="cbody">{m.content}</div>{/if}
					{#each m.tool_calls ?? [] as tc, j (j)}
						<div class="tool"><Wrench size={11} /> <span class="mono">{tc.name}({tc.arguments})</span></div>
					{/each}
				</div>
			{/each}

			<div class="cmsg response">
				<div class="muted small"><b>Jawaban model</b></div>
				{#if c.response.content}<div class="cbody">{c.response.content}</div>{/if}
				{#each c.response.tool_calls ?? [] as tc, j (j)}
					<div class="tool"><Wrench size={11} /> <span class="mono">{tc.name}({tc.arguments})</span></div>
				{/each}
				{#if !c.response.content && !c.response.tool_calls?.length}<span class="muted small">(tidak ada jawaban)</span>{/if}
				{#if c.error}<div class="badge err" style="margin-top:6px">{c.error}</div>{/if}
			</div>
		</div>
	{/if}
</Modal>

<style>
	.meta { display: flex; flex-wrap: wrap; gap: 4px 12px; margin-bottom: 6px; }
	.note { margin: 0 0 10px; }
	.conv { max-height: 65vh; overflow: auto; display: flex; flex-direction: column; gap: 10px; }
	.cmsg { border: 1px solid var(--border); border-radius: 10px; padding: 8px 12px; }
	.cmsg.user { background: var(--surface-2); }
	.cmsg.system, .cmsg.info { border-style: dashed; }
	.cmsg.response { border-color: color-mix(in srgb, var(--accent) 45%, var(--border)); }
	.cbody { margin-top: 4px; font-size: 13px; white-space: pre-wrap; overflow-wrap: anywhere; }
	.tool { margin-top: 4px; font-size: 12px; color: var(--muted); overflow-wrap: anywhere; }
</style>
