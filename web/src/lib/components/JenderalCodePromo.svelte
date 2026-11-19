<script lang="ts">
	// Promosi JenderalCode (code.jenderalcorp.com) — agen AI coding desktop yang
	// bisa langsung memakai API key JenderalRouter (provider "Custom
	// OpenAI-compatible"). variant: card (lengkap) | strip (ringkas).
	import { untrack } from 'svelte';
	import { ArrowUpRight, ShieldCheck, History, Plug, X, Download } from '@lucide/svelte';

	let {
		variant = 'card',
		dismissKey = '',
		guideHref = ''
	}: { variant?: 'card' | 'strip'; dismissKey?: string; guideHref?: string } = $props();

	const URL_JC = 'https://code.jenderalcorp.com';
	// kunci sembunyikan dibaca sekali saat komponen dibuat
	let hidden = $state(untrack(() => !!dismissKey && typeof localStorage !== 'undefined' && localStorage.getItem(dismissKey) === '1'));
	function dismiss() {
		hidden = true;
		try { localStorage.setItem(dismissKey, '1'); } catch { /* */ }
	}
</script>

{#snippet mark(size: number)}
	<span class="mark" style="width:{size}px;height:{size}px" aria-hidden="true">
		<svg viewBox="0 0 24 24" width={size * 0.55} height={size * 0.55} fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
			<path d="M8 7 3 12l5 5" /><path d="m16 7 5 5-5 5" /><path d="m13.5 4-3 16" />
		</svg>
	</span>
{/snippet}

{#if !hidden}
	{#if variant === 'strip'}
		<aside class="strip">
			{@render mark(30)}
			<div class="s-txt">
				<b>JenderalCode</b> — agen AI yang menulis kode langsung di repo Anda, dan bertanya dulu sebelum bertindak.
				<span class="muted">Gratis, bisa memakai API key JenderalRouter Anda.</span>
			</div>
			<a class="s-cta" href={URL_JC} target="_blank" rel="noopener">Lihat <ArrowUpRight size={14} /></a>
			{#if dismissKey}<button class="x" onclick={dismiss} aria-label="Sembunyikan promosi JenderalCode"><X size={14} /></button>{/if}
		</aside>
	{:else}
		<aside class="card-jc">
			<div class="glow" aria-hidden="true"></div>
			{#if dismissKey}<button class="x" onclick={dismiss} aria-label="Sembunyikan promosi JenderalCode"><X size={15} /></button>{/if}
			<div class="head">
				{@render mark(44)}
				<div>
					<div class="eyebrow">Baru dari Jenderal · gratis &amp; open source</div>
					<h3>JenderalCode</h3>
				</div>
			</div>
			<p class="tag">Agen AI yang menulis kode langsung di repo Anda — <em>dan bertanya dulu sebelum bertindak.</em></p>
			<ul class="feats">
				<li><Plug size={15} /><span><b>Pakai API key JenderalRouter</b> — model yang tersedia untuk API di sini langsung bisa dipakai untuk coding.</span></li>
				<li><ShieldCheck size={15} /><span><b>Mode izin</b> Safe, Balanced, Full — Anda yang menentukan seberapa bebas agen bertindak.</span></li>
				<li><History size={15} /><span><b>Checkpoint</b> sebelum file diubah, sehingga perubahan bisa dikembalikan.</span></li>
			</ul>
			<div class="ctas">
				<a class="btn-jc" href={URL_JC} target="_blank" rel="noopener"><Download size={15} /> Unduh gratis</a>
				{#if guideHref}<a class="link" href={guideHref}>Cara menyambungkan ke JenderalRouter <ArrowUpRight size={13} /></a>{/if}
			</div>
			<div class="os">macOS · Windows · Linux · lisensi MIT</div>
		</aside>
	{/if}
{/if}

<style>
	.mark {
		flex: 0 0 auto; display: grid; place-items: center; border-radius: 12px; color: #fff;
		background: linear-gradient(135deg, #10b981, #0ea5e9 60%, #6366f1);
		box-shadow: 0 6px 18px rgb(14 165 233 / .35);
	}

	/* ---------- kartu ---------- */
	.card-jc {
		position: relative; overflow: hidden; isolation: isolate; padding: 20px 22px; border-radius: 16px;
		background: var(--surface);
		border: 1px solid color-mix(in srgb, #0ea5e9 35%, var(--border));
	}
	.glow {
		position: absolute; z-index: -1; inset: -40% -10% auto auto; width: 360px; height: 260px; border-radius: 50%;
		background: radial-gradient(closest-side, rgb(16 185 129 / .22), transparent), radial-gradient(closest-side, rgb(99 102 241 / .18), transparent);
		filter: blur(10px); pointer-events: none;
	}
	.head { display: flex; align-items: center; gap: 13px; }
	.eyebrow { font-family: var(--mono); font-size: 10.5px; letter-spacing: .06em; text-transform: uppercase; color: #10b981; }
	h3 { margin: 2px 0 0; font-size: 22px; font-weight: 800; letter-spacing: -0.03em; }
	.tag { margin: 12px 0 0; font-size: 14.5px; line-height: 1.55; max-width: 62ch; }
	.tag em { font-family: var(--font-display); font-size: 1.12em; color: #0ea5e9; }
	.feats { list-style: none; padding: 0; margin: 14px 0 0; display: grid; gap: 8px; }
	.feats li { display: flex; gap: 9px; align-items: flex-start; font-size: 13px; color: var(--muted); line-height: 1.5; }
	.feats li b { color: var(--text); }
	.feats :global(svg) { flex: 0 0 auto; margin-top: 2px; color: #10b981; }
	.ctas { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 16px; margin-top: 16px; }
	.btn-jc {
		display: inline-flex; align-items: center; gap: 8px; height: 38px; padding: 0 16px; border-radius: 10px;
		color: #fff; font-weight: 750; font-size: 13.5px; text-decoration: none;
		background: linear-gradient(135deg, #10b981, #0ea5e9);
		box-shadow: 0 8px 20px rgb(14 165 233 / .3);
		transition: transform var(--speed), filter var(--speed);
	}
	.btn-jc:hover { transform: translateY(-1px); filter: brightness(1.07); }
	.link { display: inline-flex; align-items: center; gap: 4px; font-size: 13px; font-weight: 600; color: var(--accent-2); text-decoration: none; }
	.link:hover { text-decoration: underline; }
	.os { margin-top: 10px; font-size: 11.5px; color: var(--muted); }

	/* ---------- strip ---------- */
	.strip {
		display: flex; align-items: center; gap: 12px; padding: 10px 12px 10px 10px; border-radius: 14px;
		background: linear-gradient(90deg, color-mix(in srgb, #10b981 9%, var(--surface)), var(--surface));
		border: 1px solid color-mix(in srgb, #10b981 30%, var(--border));
	}
	.s-txt { flex: 1; min-width: 0; font-size: 13px; line-height: 1.5; }
	.s-txt .muted { display: block; font-size: 12px; }
	.s-cta {
		flex: 0 0 auto; display: inline-flex; align-items: center; gap: 4px; padding: 6px 12px; border-radius: 9px;
		font-size: 12.5px; font-weight: 700; text-decoration: none; color: #fff; background: linear-gradient(135deg, #10b981, #0ea5e9);
	}
	.x {
		all: unset; cursor: pointer; flex: 0 0 auto; width: 28px; height: 28px; border-radius: 8px; display: grid; place-items: center; color: var(--muted);
	}
	.card-jc .x { position: absolute; top: 12px; right: 12px; }
	.x:hover { background: var(--surface-2); color: var(--text); }
	.x:focus-visible, .btn-jc:focus-visible, .s-cta:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
	@media (max-width: 640px) {
		.card-jc { padding: 16px; }
		.strip { flex-wrap: wrap; }
		.s-txt { flex-basis: calc(100% - 60px); }
	}
</style>
