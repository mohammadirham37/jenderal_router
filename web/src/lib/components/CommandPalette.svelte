<script lang="ts">
	// Palet perintah (⌘K / Ctrl+K): lompat ke halaman mana pun atau jalankan
	// aksi cepat hanya dengan keyboard.
	import { goto } from '$app/navigation';
	import { Search, CornerDownLeft, SunMoon, Languages, LogOut, ArrowRight } from '@lucide/svelte';
	import { app, t, toggleLang, toggleTheme } from '#lib/stores.svelte.js';
	import { GROUPS, visibleFor } from '#lib/nav.js';

	let { open = $bindable(false), onlogout }: { open?: boolean; onlogout: () => void } = $props();

	let q = $state('');
	let sel = $state(0);
	let inputEl = $state<HTMLInputElement | null>(null);
	let listEl = $state<HTMLElement | null>(null);

	type Entry = { id: string; label: string; hint: string; icon: any; run: () => void; text: string };
	const groupLabel = (g: string) => GROUPS.find((x) => x.key === g)?.label || 'Menu';
	const entries = $derived.by((): Entry[] => {
		const pages = visibleFor(app.me?.role ?? '').map((n) => ({
			id: n.route, label: t(n.label), hint: groupLabel(n.group), icon: n.icon,
			run: () => goto(n.route), text: `${t(n.label)} ${n.route} ${n.keywords ?? ''} ${groupLabel(n.group)}`.toLowerCase()
		}));
		const actions: Entry[] = [
			{ id: 'theme', label: 'Ganti tema gelap/terang', hint: 'Aksi', icon: SunMoon, run: toggleTheme, text: 'tema theme gelap terang dark light' },
			{ id: 'lang', label: 'Ganti bahasa (ID/EN)', hint: 'Aksi', icon: Languages, run: toggleLang, text: 'bahasa language english indonesia' },
			{ id: 'logout', label: t('logout'), hint: 'Aksi', icon: LogOut, run: onlogout, text: 'keluar logout sign out' }
		];
		return [...pages, ...actions];
	});
	const results = $derived.by(() => {
		const terms = q.toLowerCase().trim().split(/\s+/).filter(Boolean);
		return terms.length ? entries.filter((e) => terms.every((w) => e.text.includes(w))) : entries;
	});

	$effect(() => {
		if (open) {
			q = '';
			sel = 0;
			queueMicrotask(() => inputEl?.focus());
		}
	});
	$effect(() => {
		void q;
		sel = 0;
	});
	$effect(() => {
		listEl?.querySelector<HTMLElement>(`[data-i="${sel}"]`)?.scrollIntoView({ block: 'nearest' });
	});

	function run(e: Entry | undefined) {
		if (!e) return;
		open = false;
		e.run();
	}
	function onKey(e: KeyboardEvent) {
		if (e.key === 'ArrowDown') { e.preventDefault(); sel = Math.min(results.length - 1, sel + 1); }
		else if (e.key === 'ArrowUp') { e.preventDefault(); sel = Math.max(0, sel - 1); }
		else if (e.key === 'Enter') { e.preventDefault(); run(results[sel]); }
		else if (e.key === 'Escape') { e.preventDefault(); open = false; }
	}
</script>

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<div class="cp-backdrop" onclick={() => (open = false)}>
		<div class="cp" role="dialog" aria-modal="true" aria-label="Cari menu" tabindex="-1" onclick={(e) => e.stopPropagation()}>
			<div class="cp-input">
				<Search size={17} />
				<input bind:this={inputEl} bind:value={q} onkeydown={onKey} placeholder="Cari halaman atau aksi…"
					aria-label="Cari" aria-controls="cp-list" aria-activedescendant={results[sel] ? 'cp-' + sel : undefined} />
				<kbd>Esc</kbd>
			</div>
			<ul class="cp-list" id="cp-list" role="listbox" bind:this={listEl}>
				{#each results as r, i (r.id)}
					<li id="cp-{i}" role="option" aria-selected={i === sel} data-i={i}>
						<button class:on={i === sel} onclick={() => run(r)} onmousemove={() => (sel = i)}>
							<span class="ic"><r.icon size={16} /></span>
							<span class="lbl">{r.label}</span>
							<span class="hint">{r.hint}</span>
							{#if i === sel}<ArrowRight size={14} class="go" />{/if}
						</button>
					</li>
				{:else}
					<li class="none">Tidak ada yang cocok dengan “{q}”.</li>
				{/each}
			</ul>
			<div class="cp-foot">
				<span><kbd>↑</kbd><kbd>↓</kbd> pilih</span>
				<span><kbd><CornerDownLeft size={11} /></kbd> buka</span>
				<span><kbd>Esc</kbd> tutup</span>
			</div>
		</div>
	</div>
{/if}

<style>
	.cp-backdrop {
		position: fixed; inset: 0; z-index: 200; display: grid; place-items: start center; padding: 12vh 16px 16px;
		background: rgb(4 8 18 / .55); backdrop-filter: blur(3px); animation: fade .12s ease;
	}
	.cp {
		width: min(560px, 100%); border-radius: 16px; overflow: hidden;
		background: var(--surface); border: 1px solid var(--border-strong); box-shadow: 0 24px 64px rgb(0 0 0 / .45);
		animation: pop .16s cubic-bezier(.2,.8,.2,1);
	}
	@keyframes fade { from { opacity: 0; } }
	@keyframes pop { from { opacity: 0; transform: translateY(-8px) scale(.98); } }
	.cp-input { display: flex; align-items: center; gap: 10px; padding: 14px 16px; border-bottom: 1px solid var(--border); color: var(--muted); }
	.cp-input input { all: unset; flex: 1; font-size: 15px; color: var(--text); }
	.cp-input input::placeholder { color: var(--muted); }
	kbd {
		display: inline-flex; align-items: center; justify-content: center; min-width: 20px; height: 20px; padding: 0 5px;
		border-radius: 5px; font-family: var(--mono); font-size: 10.5px; color: var(--muted);
		background: var(--surface-2); border: 1px solid var(--border); border-bottom-width: 2px;
	}
	.cp-list { list-style: none; margin: 0; padding: 6px; max-height: min(420px, 55vh); overflow-y: auto; }
	.cp-list button {
		all: unset; box-sizing: border-box; width: 100%; display: flex; align-items: center; gap: 11px;
		padding: 9px 10px; border-radius: 10px; cursor: pointer; font-size: 13.5px;
	}
	.cp-list button.on { background: color-mix(in srgb, var(--accent) 13%, transparent); }
	.ic { width: 30px; height: 30px; border-radius: 9px; display: grid; place-items: center; color: var(--accent); background: var(--surface-2); flex: 0 0 auto; }
	.on .ic { background: color-mix(in srgb, var(--accent) 18%, var(--surface)); }
	.lbl { flex: 1; font-weight: 600; }
	.hint { font-size: 11.5px; color: var(--muted); }
	.cp-list :global(.go) { color: var(--accent); }
	.none { padding: 18px 12px; text-align: center; color: var(--muted); font-size: 13px; }
	.cp-foot { display: flex; gap: 16px; padding: 9px 14px; border-top: 1px solid var(--border); font-size: 11.5px; color: var(--muted); }
	.cp-foot span { display: inline-flex; align-items: center; gap: 4px; }
</style>
