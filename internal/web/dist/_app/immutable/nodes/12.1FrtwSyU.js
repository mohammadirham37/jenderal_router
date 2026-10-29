import{B as e,D as t,F as n,H as r,J as i,K as a,L as o,P as s,Q as c,S as l,U as u,W as d,X as f,Y as p,Z as m,at as h,b as ee,g,h as te,j as _,k as v,l as y,n as b,nt as ne,ot as x,q as S,r as C,rt as re,s as w,u as ie}from"../chunks/gcTiPw5J.js";import{g as T,i as E,m as D,p as O,t as k}from"../chunks/BruuJ9_u.js";import{n as ae,t as A}from"../chunks/dMKvEdSy.js";import{t as j}from"../chunks/CpD5mfgP.js";import{t as oe}from"../chunks/6f3OkqHk.js";import{t as M}from"../chunks/DJdSJhP1.js";import{t as se}from"../chunks/B1pEx-Q7.js";import{t as ce}from"../chunks/BxCNd4jm.js";import{t as N}from"../chunks/rXnkAtuh.js";var P={name:`database`,size:24,node:[[`ellipse`,{cx:`12`,cy:`5`,rx:`9`,ry:`3`}],[`path`,{d:`M3 5V19A9 3 0 0 0 21 19V5`}],[`path`,{d:`M3 12A9 3 0 0 0 21 12`}]]};P.node;var F=new Set([`$$slots`,`$$events`,`$$legacy`]);function le(e,t){let n=b(t,F);T(e,C(()=>n,{get icon(){return P}}))}var ue=_(`<span class="muted small"><span class="spinner dark"></span> memeriksa cloudflared di host…</span>`),de=_(`<span class="badge ok"><span class="dot"></span>active</span>`),fe=_(`<span class="badge warn"> </span>`),pe=_(`<span class="muted">belum ada</span>`),me=_(`<span class="badge ok"><span class="dot pulse"></span>berjalan</span>`),he=_(`<span class="badge warn">tidak berjalan</span>`),ge=_(`<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:170px">Hostname:</span> <span class="mono"> </span></div> <div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:170px">Ingress ke:</span> <span class="mono"> </span></div>`,1),_e=_(`<div class="kv small" style="margin-bottom:10px"><span class="muted" style="min-width:170px">Binary cloudflared:</span> <span> </span></div> <div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:170px">Service systemd:</span> <!></div> <div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted" style="min-width:170px">Proses tunnel:</span> <!></div> <!> <div class="row" style="margin:10px 0"><button class="btn ghost sm"><!> Periksa ulang</button> <span class="grow"></span> <button>1. Daftar di Cloudflare</button> <button>2. Pemasangan di server</button> <button class="btn ghost sm"><!> Salin</button></div> <pre class="logbox" style="max-height:340px;overflow:auto"> </pre> <p class="muted small" style="margin:8px 0 0">Streaming chat & API aman lewat tunnel (heartbeat otomatis tiap 15 dtk);
			hindari mode non-streaming untuk jawaban &gt;100 dtk — itu batas platform Cloudflare.
			Tunnel juga otomatis membuat DNS (CNAME) + sertifikat SSL — tidak perlu membuka port.</p>`,1),ve=_(`<button class="btn ghost sm">Periksa host</button>`),ye=_(`<div class="kv small" style="border-bottom:1px solid var(--border);padding:4px 0"><span class="muted mono" style="min-width:150px"> </span> <span class="mono"> </span> <span class="muted"> </span></div>`),be=_(`<span class="muted small"> </span>`),xe=_(`<div class="page-head"><h1 class="page-title"><!> </h1></div> <div class="card"><h2>Notifikasi (v1.1)</h2> <label for="f-tg-token">Telegram bot token</label> <input id="f-tg-token" type="password" autocomplete="off"/> <label for="f-tg-chat">Telegram chat id</label> <input id="f-tg-chat"/> <div class="modal-actions" style="justify-content:flex-start;margin-top:12px"><button class="btn"><!> </button></div></div> <div class="card"><h2><!> Backup</h2> <p class="muted small">Backup harian otomatis ke <code>data/backups/</code> (retensi 7 salinan).</p> <button class="btn"><!> </button></div> <div class="card"><h2><!> Pembaruan Aplikasi</h2> <p class="muted small" style="margin:0 0 10px">Tarik versi terbaru, build, pasang, dan rollback kini ada di menu tersendiri.</p> <a class="btn ghost sm" href="/update"><!> </a></div> <div class="card"><h2><!> Cloudflare Tunnel</h2> <p class="muted small" style="margin:0 0 10px">Akses web UI & API dari internet tanpa membuka port — lewat tunnel Cloudflare.
		Streaming chat sudah mengirim heartbeat berkala, jadi batas idle Cloudflare
		(~100 dtk) dan cloudflared (~90 dtk) tidak memutus jawaban panjang.</p> <!></div> <div class="card"><h2><!> </h2> <!></div>`,1);function I(s,_){re(_,!0);let b=c(``),C=c(``),T=c(!1);async function P(){try{await k.put(`/api/admin/settings`,{notification_telegram_token:o(b),notification_chat_id:o(C)}),D(O(`save`)+` ✓`,`ok`)}catch(e){D(e.message,`err`)}}let F=c(null),I=c(!1),L=c(`daftar`);async function R(){m(I,!0);try{m(F,await k.get(`/api/admin/system/cloudflare`),!0)}catch(e){D(e.message,`err`)}m(I,!1)}u(()=>{R()});function Se(){return o(L)===`daftar`?`YANG HARUS DIDAFTARKAN DI DASHBOARD CLOUDFLARE
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
sudo cloudflared service install`}async function Ce(){await A(Se())?D(`panduan disalin ✓`,`ok`):D(`gagal menyalin — salin manual`,`err`)}async function we(){try{await k.post(`/api/admin/system/backup`),D(O(`save`)+` ✓`,`ok`)}catch(e){D(e.message,`err`)}}u(()=>{(async()=>{try{let e=await k.get(`/api/admin/settings`);m(T,e.settings.notification_telegram_token_set===`true`),m(C,e.settings.notification_chat_id||``,!0)}catch{}})()});let Te=c(f([]));u(()=>{k.get(`/api/admin/audit?limit=30`).then(e=>m(Te,e.audit||[],!0)).catch(()=>{})});var Ee=xe();g(`1i19ct2`,t=>{e(e=>{d.title=`${e??``} — JenderalRouter`},[()=>O(`settings`)])});var z=S(Ee),De=a(z),B=a(De);N(B,{size:20});var Oe=p(B);x(De),x(z);var V=p(z,2),H=p(a(V),4);y(H);var U=p(H,4);y(U);var ke=p(U,2),W=a(ke),Ae=a(W);se(Ae,{size:15});var je=p(Ae);x(W),x(ke),x(V);var G=p(V,2),K=a(G);le(a(K),{size:15}),h(),x(K);var q=p(K,4),Me=a(q);le(Me,{size:15});var Ne=p(Me);x(q),x(G);var J=p(G,2),Y=a(J),Pe=a(Y);M(Pe,{size:15}),h(),x(Y);var X=p(Y,4),Fe=a(X);M(Fe,{size:13});var Ie=p(Fe);x(X),x(J);var Z=p(J,2),Q=a(Z),Le=a(Q);j(Le,{size:15}),h(),x(Q);var Re=p(Q,4),ze=e=>{var t=ue();v(e,t)},Be=e=>{var s=_e(),c=S(s),u=p(a(c),2),d=i(u,!0);x(c);var f=p(c,2),ee=p(a(f),2),g=e=>{var t=de();v(e,t)},_=e=>{var n=fe(),a=i(n,!0);r(()=>t(a,o(F).service_state)),v(e,n)},y=e=>{var t=pe();v(e,t)};l(ee,e=>{o(F).service_state===`active`?e(g):o(F).service_state?e(_,1):e(y,-1)}),x(f);var b=p(f,2),ne=p(a(b),2),C=e=>{var t=me();v(e,t)},re=e=>{var t=he();v(e,t)};l(ne,e=>{o(F).running?e(C):e(re,-1)}),x(b);var w=p(b,2),ie=e=>{var n=ge(),s=S(n),c=p(a(s),2),l=i(c,!0);x(s);var u=p(s,2),d=p(a(u),2),f=i(d,!0);x(u),r(()=>{t(l,o(F).hostname||`—`),t(f,o(F).ingress_service||`—`)}),v(e,n)};l(w,e=>{o(F).config_found&&e(ie)});var T=p(w,2),E=a(T),D=a(E);oe(D,{size:13}),h(),x(E);var O=p(E,4);let k;var A=p(O,2);let j;var M=p(A,2),se=a(M);ae(se,{size:13}),h(),x(M),x(T);var ce=p(T,2),N=i(ce,!0);h(2),r(e=>{t(d,o(F).installed?`✓ terpasang`:`✗ belum terpasang`),k=te(O,1,`btn sm`,null,k,{ghost:o(L)!==`daftar`}),j=te(A,1,`btn sm`,null,j,{ghost:o(L)!==`server`}),t(N,e)},[()=>Se()]),n(`click`,E,R),n(`click`,O,()=>m(L,`daftar`)),n(`click`,A,()=>m(L,`server`)),n(`click`,M,Ce),v(e,s)},Ve=e=>{var t=ve();n(`click`,t,R),v(e,t)};l(Re,e=>{o(I)&&!o(F)?e(ze):o(F)?e(Be,1):e(Ve,-1)}),x(Z);var He=p(Z,2),$=a(He),Ue=a($);ce(Ue,{size:15});var We=p(Ue);x($);var Ge=p($,2);ee(Ge,17,()=>o(Te),e=>e.id,(e,n)=>{var s=ye(),c=a(s),l=i(c,!0),u=p(c,2),d=i(u,!0),f=p(u,2),m=i(f,!0);x(s),r(e=>{t(l,e),t(d,o(n).action),t(m,o(n).target)},[()=>E(o(n).ts)]),v(e,s)},e=>{var n=be(),a=i(n,!0);r(e=>t(a,e),[()=>O(`no_data`)]),v(e,n)}),x(He),r((e,n,r,i,a)=>{t(Oe,` ${e??``}`),ie(H,`placeholder`,o(T)?`tersimpan — isi untuk mengganti`:`123456:ABC…`),t(je,` ${n??``}`),t(Ne,` ${r??``}`),t(Ie,` Buka ${i??``}`),t(We,` ${a??``}`)},[()=>O(`settings`),()=>O(`save`),()=>O(`backup_now`),()=>O(`system_update`),()=>O(`audit_log`)]),w(H,()=>o(b),e=>m(b,e)),w(U,()=>o(C),e=>m(C,e)),n(`click`,W,P),n(`click`,q,we),v(s,Ee),ne()}s([`click`]);export{I as component};