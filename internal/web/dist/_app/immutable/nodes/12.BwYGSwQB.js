import{B as e,D as t,F as n,H as r,J as i,K as a,L as o,P as s,Q as c,S as l,U as u,W as d,X as f,Y as p,Z as m,at as h,b as ee,g as te,h as g,j as _,k as v,l as y,n as b,nt as x,ot as S,q as C,r as w,rt as ne,s as T}from"../chunks/gcTiPw5J.js";import{g as E,i as D,m as O,p as k,t as A}from"../chunks/D0PlgAfW.js";import{n as re,t as j}from"../chunks/CKLoyOl6.js";import{t as M}from"../chunks/Cvq5qHL3.js";import{t as ie}from"../chunks/DM-o5-He.js";import{t as N}from"../chunks/Z8DzN-i5.js";import{t as P}from"../chunks/BvkObV8t.js";import{t as ae}from"../chunks/D7pkh_RY.js";import{t as oe}from"../chunks/Yo_rocZg.js";var F={name:`database`,size:24,node:[[`ellipse`,{cx:`12`,cy:`5`,rx:`9`,ry:`3`}],[`path`,{d:`M3 5V19A9 3 0 0 0 21 19V5`}],[`path`,{d:`M3 12A9 3 0 0 0 21 12`}]]};F.node;var I=new Set([`$$slots`,`$$events`,`$$legacy`]);function L(e,t){let n=b(t,I);E(e,w(()=>n,{get icon(){return F}}))}var se=_(`<span class="muted small"><span class="spinner dark"></span> memeriksa cloudflared di host…</span>`),ce=_(`<span class="badge ok"><span class="dot"></span>active</span>`),le=_(`<span class="badge warn"> </span>`),ue=_(`<span class="muted">belum ada</span>`),de=_(`<span class="badge ok"><span class="dot pulse"></span>berjalan</span>`),fe=_(`<span class="badge warn">tidak berjalan</span>`),pe=_(`<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:170px">Hostname:</span> <span class="mono"> </span></div> <div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:170px">Ingress ke:</span> <span class="mono"> </span></div>`,1),me=_(`<div class="kv small" style="margin-bottom:10px"><span class="muted" style="min-width:170px">Binary cloudflared:</span> <span> </span></div> <div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:170px">Service systemd:</span> <!></div> <div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:170px">Proses tunnel:</span> <!></div> <!> <div class="row" style="margin:10px 0"><button class="btn ghost sm"><!> Periksa ulang</button> <span class="grow"></span> <button>1. Daftar di Cloudflare</button> <button>2. Pemasangan di server</button> <button class="btn ghost sm"><!> Salin</button></div> <pre class="logbox" style="max-height:340px;overflow:auto"> </pre> <p class="muted small" style="margin:8px 0 0">Streaming chat & API aman lewat tunnel (heartbeat otomatis tiap 15 dtk);
			hindari mode non-streaming untuk jawaban &gt;100 dtk — itu batas platform Cloudflare.
			Tunnel juga otomatis membuat DNS (CNAME) + sertifikat SSL — tidak perlu membuka port.</p>`,1),he=_(`<button class="btn ghost sm">Periksa host</button>`),ge=_(`<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted mono" style="min-width:150px"> </span> <span class="mono"> </span> <span class="muted"> </span></div>`),_e=_(`<span class="muted small"> </span>`),ve=_(`<div class="page-head"><h1 class="page-title"><!> </h1></div> <div class="card"><h2>Notifikasi (v1.1)</h2> <label for="f-tg-token">Telegram bot token</label> <input id="f-tg-token" type="password" placeholder="123456:ABC…"/> <label for="f-tg-chat">Telegram chat id</label> <input id="f-tg-chat"/> <div class="modal-actions" style="justify-content:flex-start;margin-top:12px"><button class="btn"><!> </button></div></div> <div class="card"><h2><!> Backup</h2> <p class="muted small">Backup harian otomatis ke <code>data/backups/</code> (retensi 7 salinan).</p> <button class="btn"><!> </button></div> <div class="card"><h2><!> Pembaruan Aplikasi</h2> <p class="muted small" style="margin:0 0 10px">Tarik versi terbaru, build, pasang, dan rollback kini ada di menu tersendiri.</p> <a class="btn ghost sm" href="/update"><!> </a></div> <div class="card"><h2><!> Cloudflare Tunnel</h2> <p class="muted small" style="margin:0 0 10px">Akses web UI & API dari internet tanpa membuka port — lewat tunnel Cloudflare.
		Streaming chat sudah mengirim heartbeat berkala, jadi batas idle Cloudflare
		(~100 dtk) dan cloudflared (~90 dtk) tidak memutus jawaban panjang.</p> <!></div> <div class="card"><h2><!> </h2> <!></div>`,1);function R(s,_){ne(_,!0);let b=c(``),w=c(``);async function E(){try{await A.put(`/api/admin/settings`,{notification_telegram_token:o(b),notification_chat_id:o(w)}),O(k(`save`)+` ✓`,`ok`)}catch(e){O(e.message,`err`)}}let F=c(null),I=c(!1),R=c(`daftar`);async function z(){m(I,!0);try{m(F,await A.get(`/api/admin/system/cloudflare`),!0)}catch(e){O(e.message,`err`)}m(I,!1)}u(()=>{z()});function ye(){return o(R)===`daftar`?`YANG HARUS DIDAFTARKAN DI DASHBOARD CLOUDFLARE
=================================================

1) DOMAIN AKTIF DI CLOUDFLARE
   - Masuk dash.cloudflare.com → "Add a domain" → masukkan domain Anda
   - Pilih paket FREE (cukup)
   - Arahkan nameserver domain Anda ke 2 nameserver yang Cloudflare berikan
     (di registrar tempat Anda membeli domain)
   - Tunggu status domain: ACTIVE

2) BUAT TUNNEL (tanpa CLI)
   - Dashboard Cloudflare → menu "Zero Trust" (one-time setup singkat)
   - Networks → Tunnels → "Create a tunnel"
   - Pilih konektor "Cloudflared" → beri nama, mis. jenderal
   - Di langkah "Install and run a connector": pilih Debian / 64-bit
   - SALIN perintah yang ditampilkan — berisi token unik tunnel Anda:
       sudo cloudflared service install <TOKEN-PANJANG>
     (perintah ini sekaligus memasang service otomatis di server)

3) DAFTARKAN HOSTNAME PUBLIK (masih di wizard yang sama)
   - Subdomain : chat        Domain : domainanda.com
   - Path      : (kosongkan)
   - Service   : HTTP  →  URL: 127.0.0.1:20130
   - Klik "Save tunnel"
   → CNAME ke <ID>.cfargotunnel.com + sertifikat SSL dibuat OTOMATIS

4) CEK STATUS
   - Networks → Tunnels → tunnel "jenderal" harus berstatus HEALTHY
   - Buka https://chat.domainanda.com — selesai, tanpa buka port`:`PEMASANGAN DI SERVER (metode CLI / lokal-managed)
=================================================

# 1) pasang cloudflared
sudo curl -L --output /usr/local/bin/cloudflared \\
  https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64
sudo chmod +x /usr/local/bin/cloudflared

# 2) autentikasi & buat tunnel (butuh akun Cloudflare + domain)
cloudflared tunnel login
cloudflared tunnel create jenderal

# 3) /etc/cloudflared/config.yml
tunnel: <ID-tunnel>
credentials-file: /root/.cloudflared/<ID-tunnel>.json
ingress:
  - hostname: chat.domainanda.com
    service: http://${o(F)?.gateway_addr||`127.0.0.1:20130`}
  - service: http_status:404

# 4) daftarkan DNS & jalankan sebagai service
cloudflared tunnel route dns jenderal chat.domainanda.com
sudo cloudflared service install`}async function be(){await j(ye())?O(`panduan disalin ✓`,`ok`):O(`gagal menyalin — salin manual`,`err`)}async function xe(){try{await A.post(`/api/admin/system/backup`),O(k(`save`)+` ✓`,`ok`)}catch(e){O(e.message,`err`)}}u(()=>{(async()=>{try{let e=await A.get(`/api/admin/settings`);m(b,e.settings.notification_telegram_token||``,!0),m(w,e.settings.notification_chat_id||``,!0)}catch{}})()});let Se=c(f([]));u(()=>{A.get(`/api/admin/audit?limit=30`).then(e=>m(Se,e.audit||[],!0)).catch(()=>{})});var Ce=ve();te(`1i19ct2`,t=>{e(e=>{d.title=`${e??``} — JenderalRouter`},[()=>k(`settings`)])});var B=C(Ce),we=a(B),Te=a(we);oe(Te,{size:20});var Ee=p(Te);S(we),S(B);var V=p(B,2),H=p(a(V),4);y(H);var U=p(H,4);y(U);var W=p(U,2),G=a(W),De=a(G);P(De,{size:15});var Oe=p(De);S(G),S(W),S(V);var K=p(V,2),q=a(K);L(a(q),{size:15}),h(),S(q);var J=p(q,4),ke=a(J);L(ke,{size:15});var Ae=p(ke);S(J),S(K);var Y=p(K,2),X=a(Y),je=a(X);N(je,{size:15}),h(),S(X);var Me=p(X,4),Ne=a(Me);N(Ne,{size:13});var Pe=p(Ne);S(Me),S(Y);var Z=p(Y,2),Q=a(Z),Fe=a(Q);M(Fe,{size:15}),h(),S(Q);var Ie=p(Q,4),Le=e=>{var t=se();v(e,t)},Re=e=>{var s=me(),c=C(s),u=p(a(c),2),d=i(u,!0);S(c);var f=p(c,2),ee=p(a(f),2),te=e=>{var t=ce();v(e,t)},_=e=>{var n=le(),a=i(n,!0);r(()=>t(a,o(F).service_state)),v(e,n)},y=e=>{var t=ue();v(e,t)};l(ee,e=>{o(F).service_state===`active`?e(te):o(F).service_state?e(_,1):e(y,-1)}),S(f);var b=p(f,2),x=p(a(b),2),w=e=>{var t=de();v(e,t)},ne=e=>{var t=fe();v(e,t)};l(x,e=>{o(F).running?e(w):e(ne,-1)}),S(b);var T=p(b,2),E=e=>{var n=pe(),s=C(n),c=p(a(s),2),l=i(c,!0);S(s);var u=p(s,2),d=p(a(u),2),f=i(d,!0);S(u),r(()=>{t(l,o(F).hostname||`—`),t(f,o(F).ingress_service||`—`)}),v(e,n)};l(T,e=>{o(F).config_found&&e(E)});var D=p(T,2),O=a(D),k=a(O);ie(k,{size:13}),h(),S(O);var A=p(O,4);let j;var M=p(A,2);let N;var P=p(M,2),ae=a(P);re(ae,{size:13}),h(),S(P),S(D);var oe=p(D,2),I=i(oe,!0);h(2),r(e=>{t(d,o(F).installed?`✓ terpasang`:`✗ belum terpasang`),j=g(A,1,`btn sm`,null,j,{ghost:o(R)!==`daftar`}),N=g(M,1,`btn sm`,null,N,{ghost:o(R)!==`server`}),t(I,e)},[()=>ye()]),n(`click`,O,z),n(`click`,A,()=>m(R,`daftar`)),n(`click`,M,()=>m(R,`server`)),n(`click`,P,be),v(e,s)},ze=e=>{var t=he();n(`click`,t,z),v(e,t)};l(Ie,e=>{o(I)&&!o(F)?e(Le):o(F)?e(Re,1):e(ze,-1)}),S(Z);var Be=p(Z,2),$=a(Be),Ve=a($);ae(Ve,{size:15});var He=p(Ve);S($);var Ue=p($,2);ee(Ue,17,()=>o(Se),e=>e.id,(e,n)=>{var s=ge(),c=a(s),l=i(c,!0),u=p(c,2),d=i(u,!0),f=p(u,2),m=i(f,!0);S(s),r(e=>{t(l,e),t(d,o(n).action),t(m,o(n).target)},[()=>D(o(n).ts)]),v(e,s)},e=>{var n=_e(),a=i(n,!0);r(e=>t(a,e),[()=>k(`no_data`)]),v(e,n)}),S(Be),r((e,n,r,i,a)=>{t(Ee,` ${e??``}`),t(Oe,` ${n??``}`),t(Ae,` ${r??``}`),t(Pe,` Buka ${i??``}`),t(He,` ${a??``}`)},[()=>k(`settings`),()=>k(`save`),()=>k(`backup_now`),()=>k(`system_update`),()=>k(`audit_log`)]),T(H,()=>o(b),e=>m(b,e)),T(U,()=>o(w),e=>m(w,e)),n(`click`,G,E),n(`click`,J,xe),v(s,Ce),x()}s([`click`]);export{R as component};