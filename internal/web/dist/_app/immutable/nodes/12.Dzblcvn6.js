import{B as e,D as t,F as n,H as r,J as i,K as a,L as o,P as s,Q as c,S as l,U as u,W as d,X as f,Y as p,Z as m,at as h,b as g,g as ee,h as te,j as _,k as v,l as y,n as b,nt as ne,ot as x,q as S,r as C,rt as re,s as w,u as ie}from"../chunks/gcTiPw5J.js";import{f as T,h as E,i as ae,p as D,t as O}from"../chunks/C7m6Z89G.js";import{n as oe,t as k}from"../chunks/k08DAQiS.js";import{t as A}from"../chunks/BM3sIxZK.js";import{t as se}from"../chunks/q_oFvlEJ.js";import{t as j}from"../chunks/DPAJi4nx.js";import{t as ce}from"../chunks/72X52EMp.js";import{t as M}from"../chunks/5fwYRzzh.js";import{t as N}from"../chunks/zJL5DE5x.js";var P={name:`database`,size:24,node:[[`ellipse`,{cx:`12`,cy:`5`,rx:`9`,ry:`3`}],[`path`,{d:`M3 5V19A9 3 0 0 0 21 19V5`}],[`path`,{d:`M3 12A9 3 0 0 0 21 12`}]]};P.node;var F=new Set([`$$slots`,`$$events`,`$$legacy`]);function le(e,t){let n=b(t,F);E(e,C(()=>n,{get icon(){return P}}))}var ue=_(`<span class="muted small"><span class="spinner dark"></span> memeriksa cloudflared di host…</span>`),de=_(`<span class="badge ok"><span class="dot"></span>active</span>`),fe=_(`<span class="badge warn"> </span>`),pe=_(`<span class="muted">belum ada</span>`),me=_(`<span class="badge ok"><span class="dot pulse"></span>berjalan</span>`),he=_(`<span class="badge warn">tidak berjalan</span>`),ge=_(`<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:170px">Hostname:</span> <span class="mono"> </span></div> <div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:170px">Ingress ke:</span> <span class="mono"> </span></div>`,1),_e=_(`<div class="kv small" style="margin-bottom:10px"><span class="muted" style="min-width:170px">Binary cloudflared:</span> <span> </span></div> <div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:170px">Service systemd:</span> <!></div> <div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:170px">Proses tunnel:</span> <!></div> <!> <div class="row" style="margin:10px 0"><button class="btn ghost sm"><!> Periksa ulang</button> <span class="grow"></span> <button>1. Daftar di Cloudflare</button> <button>2. Pemasangan di server</button> <button class="btn ghost sm"><!> Salin</button></div> <pre class="logbox" style="max-height:340px;overflow:auto"> </pre> <p class="muted small" style="margin:8px 0 0">Streaming chat & API aman lewat tunnel (heartbeat otomatis tiap 15 dtk);
			hindari mode non-streaming untuk jawaban &gt;100 dtk — itu batas platform Cloudflare.
			Tunnel juga otomatis membuat DNS (CNAME) + sertifikat SSL — tidak perlu membuka port.</p>`,1),ve=_(`<button class="btn ghost sm">Periksa host</button>`),ye=_(`<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted mono" style="min-width:150px"> </span> <span class="mono"> </span> <span class="muted"> </span></div>`),be=_(`<span class="muted small"> </span>`),xe=_(`<div class="page-head"><h1 class="page-title"><!> </h1></div> <div class="card"><h2>Notifikasi (v1.1)</h2> <label for="f-tg-token">Telegram bot token</label> <input id="f-tg-token" type="password" autocomplete="off"/> <label for="f-tg-chat">Telegram chat id</label> <input id="f-tg-chat"/> <div class="modal-actions" style="justify-content:flex-start;margin-top:12px"><button class="btn"><!> </button></div></div> <div class="card"><h2><!> Backup</h2> <p class="muted small">Backup harian otomatis ke <code>data/backups/</code> (retensi 7 salinan).</p> <button class="btn"><!> </button></div> <div class="card"><h2><!> Pembaruan Aplikasi</h2> <p class="muted small" style="margin:0 0 10px">Tarik versi terbaru, build, pasang, dan rollback kini ada di menu tersendiri.</p> <a class="btn ghost sm" href="/update"><!> </a></div> <div class="card"><h2><!> Cloudflare Tunnel</h2> <p class="muted small" style="margin:0 0 10px">Akses web UI & API dari internet tanpa membuka port — lewat tunnel Cloudflare.
		Streaming chat sudah mengirim heartbeat berkala, jadi batas idle Cloudflare
		(~100 dtk) dan cloudflared (~90 dtk) tidak memutus jawaban panjang.</p> <!></div> <div class="card"><h2><!> </h2> <!></div>`,1);function I(s,_){re(_,!0);let b=c(``),C=c(``),E=c(!1);async function P(){try{await O.put(`/api/admin/settings`,{notification_telegram_token:o(b),notification_chat_id:o(C)}),D(T(`save`)+` ✓`,`ok`)}catch(e){D(e.message,`err`)}}let F=c(null),I=c(!1),L=c(`daftar`);async function R(){m(I,!0);try{m(F,await O.get(`/api/admin/system/cloudflare`),!0)}catch(e){D(e.message,`err`)}m(I,!1)}u(()=>{R()});function Se(){return o(L)===`daftar`?`YANG HARUS DIDAFTARKAN DI DASHBOARD CLOUDFLARE
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
sudo cloudflared service install`}async function Ce(){await k(Se())?D(`panduan disalin ✓`,`ok`):D(`gagal menyalin — salin manual`,`err`)}async function we(){try{await O.post(`/api/admin/system/backup`),D(T(`save`)+` ✓`,`ok`)}catch(e){D(e.message,`err`)}}u(()=>{(async()=>{try{let e=await O.get(`/api/admin/settings`);m(E,e.settings.notification_telegram_token_set===`true`),m(C,e.settings.notification_chat_id||``,!0)}catch{}})()});let Te=c(f([]));u(()=>{O.get(`/api/admin/audit?limit=30`).then(e=>m(Te,e.audit||[],!0)).catch(()=>{})});var z=xe();ee(`1i19ct2`,t=>{e(e=>{d.title=`${e??``} — JenderalRouter`},[()=>T(`settings`)])});var B=S(z),Ee=a(B),V=a(Ee);N(V,{size:20});var De=p(V);x(Ee),x(B);var H=p(B,2),U=p(a(H),4);y(U);var W=p(U,4);y(W);var Oe=p(W,2),G=a(Oe),ke=a(G);ce(ke,{size:15});var Ae=p(ke);x(G),x(Oe),x(H);var K=p(H,2),q=a(K);le(a(q),{size:15}),h(),x(q);var J=p(q,4),je=a(J);le(je,{size:15});var Me=p(je);x(J),x(K);var Y=p(K,2),X=a(Y),Ne=a(X);j(Ne,{size:15}),h(),x(X);var Pe=p(X,4),Fe=a(Pe);j(Fe,{size:13});var Ie=p(Fe);x(Pe),x(Y);var Z=p(Y,2),Q=a(Z),Le=a(Q);A(Le,{size:15}),h(),x(Q);var Re=p(Q,4),ze=e=>{var t=ue();v(e,t)},Be=e=>{var s=_e(),c=S(s),u=p(a(c),2),d=i(u,!0);x(c);var f=p(c,2),g=p(a(f),2),ee=e=>{var t=de();v(e,t)},_=e=>{var n=fe(),a=i(n,!0);r(()=>t(a,o(F).service_state)),v(e,n)},y=e=>{var t=pe();v(e,t)};l(g,e=>{o(F).service_state===`active`?e(ee):o(F).service_state?e(_,1):e(y,-1)}),x(f);var b=p(f,2),ne=p(a(b),2),C=e=>{var t=me();v(e,t)},re=e=>{var t=he();v(e,t)};l(ne,e=>{o(F).running?e(C):e(re,-1)}),x(b);var w=p(b,2),ie=e=>{var n=ge(),s=S(n),c=p(a(s),2),l=i(c,!0);x(s);var u=p(s,2),d=p(a(u),2),f=i(d,!0);x(u),r(()=>{t(l,o(F).hostname||`—`),t(f,o(F).ingress_service||`—`)}),v(e,n)};l(w,e=>{o(F).config_found&&e(ie)});var T=p(w,2),E=a(T),ae=a(E);se(ae,{size:13}),h(),x(E);var D=p(E,4);let O;var k=p(D,2);let A;var j=p(k,2),ce=a(j);oe(ce,{size:13}),h(),x(j),x(T);var M=p(T,2),N=i(M,!0);h(2),r(e=>{t(d,o(F).installed?`✓ terpasang`:`✗ belum terpasang`),O=te(D,1,`btn sm`,null,O,{ghost:o(L)!==`daftar`}),A=te(k,1,`btn sm`,null,A,{ghost:o(L)!==`server`}),t(N,e)},[()=>Se()]),n(`click`,E,R),n(`click`,D,()=>m(L,`daftar`)),n(`click`,k,()=>m(L,`server`)),n(`click`,j,Ce),v(e,s)},Ve=e=>{var t=ve();n(`click`,t,R),v(e,t)};l(Re,e=>{o(I)&&!o(F)?e(ze):o(F)?e(Be,1):e(Ve,-1)}),x(Z);var He=p(Z,2),$=a(He),Ue=a($);M(Ue,{size:15});var We=p(Ue);x($);var Ge=p($,2);g(Ge,17,()=>o(Te),e=>e.id,(e,n)=>{var s=ye(),c=a(s),l=i(c,!0),u=p(c,2),d=i(u,!0),f=p(u,2),m=i(f,!0);x(s),r(e=>{t(l,e),t(d,o(n).action),t(m,o(n).target)},[()=>ae(o(n).ts)]),v(e,s)},e=>{var n=be(),a=i(n,!0);r(e=>t(a,e),[()=>T(`no_data`)]),v(e,n)}),x(He),r((e,n,r,i,a)=>{t(De,` ${e??``}`),ie(U,`placeholder`,o(E)?`tersimpan — isi untuk mengganti`:`123456:ABC…`),t(Ae,` ${n??``}`),t(Me,` ${r??``}`),t(Ie,` Buka ${i??``}`),t(We,` ${a??``}`)},[()=>T(`settings`),()=>T(`save`),()=>T(`backup_now`),()=>T(`system_update`),()=>T(`audit_log`)]),w(U,()=>o(b),e=>m(b,e)),w(W,()=>o(C),e=>m(C,e)),n(`click`,G,P),n(`click`,J,we),v(s,z),ne()}s([`click`]);export{I as component};