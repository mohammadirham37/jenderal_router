<script lang="ts">
	import { Link2, Plus, Trash2, X } from '@lucide/svelte';
	import { api } from '$lib/api';
	import { t, toast } from '$lib/stores.svelte';
	import Modal from '$lib/components/Modal.svelte';

	let combos = $state<any[]>([]);
	let models = $state<any[]>([]);
	let loading = $state(true);
	let showAdd = $state(false);

	async function reload() {
		loading = true;
		try {
			const [c, m] = await Promise.all([api.get('/api/admin/combos'), api.get('/api/admin/models')]);
			combos = c.combos || [];
			models = m.models || [];
		} catch (e: any) {
			toast(e.message, 'err');
		} finally {
			loading = false;
		}
	}
	$effect(() => { reload(); });

	let name = $state('');
	let desc = $state('');
	let steps = $state<number[]>([]);
	let adding = $state(false);

	function openAdd() {
		name = ''; desc = ''; steps = [];
		showAdd = true;
	}
	const modelById = (id: number) => models.find((m) => m.id === id);

	async function create() {
		adding = true;
		try {
			await api.post('/api/admin/combos', { name, description: desc, model_ids: steps });
			showAdd = false;
			toast(t('save') + ' ✓', 'ok');
			reload();
		} catch (e: any) {
			toast(e.message, 'err');
		} finally {
			adding = false;
		}
	}
	async function del(c: any) {
		if (!confirm(t('confirm_delete'))) return;
		try { await api.del('/api/admin/combos/' + c.id); reload(); }
		catch (e: any) { toast(e.message, 'err'); }
	}
</script>

<svelte:head><title>{t('combos')} — JenderalRouter</title></svelte:head>

<div class="page-head">
	<h1 class="page-title"><Link2 size={20} /> {t('combos')}</h1>
	<button class="btn" onclick={openAdd}><Plus size={16} /> {t('create')}</button>
</div>

{#if loading}
	<div class="card"><span class="spinner dark"></span> {t('loading')}…</div>
{:else}
	{#each combos as c (c.id)}
		<div class="card" style="margin-bottom:12px">
			<div class="page-head" style="margin-bottom:8px">
				<div>
					<strong class="mono" style="font-size:15px">{c.name}</strong>
					<div class="muted small">{c.description}</div>
				</div>
				<button class="btn danger sm" onclick={() => del(c)}><Trash2 size={13} /> {t('delete')}</button>
			</div>
			<div class="row">
				{#each c.steps as s, i (s.position)}
					<span class="badge info">{i + 1}. {s.public_id || s.model_id}</span>
					{#if i < c.steps.length - 1}<span class="muted">→</span>{/if}
				{/each}
			</div>
		</div>
	{:else}
		<div class="card muted">{t('no_data')} — combo = rantai model fallback berurutan (mis. Sonnet → GLM → lokal)</div>
	{/each}
{/if}

{#if showAdd}
	<Modal title={t('combo_name')} onclose={() => (showAdd = false)}>
		<label>{t('combo_name')}</label>
		<input bind:value={name} placeholder="coding-hemat" />
		<label>Deskripsi</label>
		<input bind:value={desc} placeholder="Sonnet → GLM → lokal" />
		<label>{t('combos')}: langkah (urut)</label>
		<div class="row">
			<select bind:value={steps[steps.length]} style="flex:1" onchange={(e) => { steps.push(Number((e.currentTarget as HTMLSelectElement).value)); steps = steps; }}>
				<option value="" disabled selected>pilih model…</option>
				{#each models as m (m.id)}
					<option value={m.id}>{m.public_id}</option>
				{/each}
			</select>
		</div>
		<div style="margin-top:10px">
			{#each steps as id, i (i + '-' + id)}
				<div class="kv" style="margin-bottom:5px">
					<span class="badge info">{i + 1}</span>
					<span class="mono">{modelById(id)?.public_id ?? id}</span>
					<button class="btn ghost sm" onclick={() => { steps.splice(i, 1); steps = steps; }}><X size={12} /></button>
				</div>
			{/each}
		</div>
		<div class="modal-actions">
			<button class="btn ghost" onclick={() => (showAdd = false)}>{t('cancel')}</button>
			<button class="btn" onclick={create} disabled={adding || !name || steps.length === 0}>
				{#if adding}<span class="spinner"></span>{/if}
				{t('create')}
			</button>
		</div>
	</Modal>
{/if}
