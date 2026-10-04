// pages-admin.js — halaman dashboard admin (F-11).
(function () {
  const { el, toast, modal, fmtNum, fmtCost, fmtMs, statCard, badge } = UI;
  const t = (k) => I18N.t(k);

  const Pages = {};

  // ---- Ringkasan ----
  Pages.dashboard = async function (main) {
    main.appendChild(el('h1', {}, [t('dashboard')]));
    let res;
    try { res = await API.get('/api/admin/analytics/summary?days=7'); }
    catch (e) { toast(e.message, 'err'); return; }
    const today = (res.per_day && res.per_day[res.per_day.length - 1]) || {};
    const cards = el('div', { class: 'grid cols-4' }, [
      statCard(t('requests_today'), fmtNum(today.requests || 0)),
      statCard(t('tokens_today'), fmtNum((today.tokens_in || 0) + (today.tokens_out || 0))),
      statCard(t('cost_7d'), fmtCost(res.totals.cost_usd)),
      statCard(t('error_rate'), ((res.error_rate || 0) * 100).toFixed(1) + '%'),
    ]);
    main.appendChild(cards);

    // grafik request/hari
    const chart = el('div', { class: 'card', style: 'margin-top:14px' });
    chart.appendChild(el('h2', {}, [t('per_day')]));
    const bars = el('div', { class: 'bar-chart' });
    const max = Math.max(1, ...(res.per_day || []).map((d) => d.requests));
    for (const d of res.per_day || []) {
      bars.appendChild(el('div', {
        class: 'bar', style: `height:${Math.max(4, (d.requests / max) * 100)}%`,
        'data-tip': `${d.day}: ${d.requests}`,
      }));
    }
    if (!res.per_day || !res.per_day.length) bars.appendChild(el('span', { class: 'muted' }, [t('no_data')]));
    chart.appendChild(bars);
    main.appendChild(chart);

    // log terbaru
    const card = el('div', { class: 'card', style: 'margin-top:14px' });
    card.appendChild(el('h2', {}, [t('recent_logs')]));
    card.appendChild(await logsTable('/api/admin/logs?limit=10'));
    main.appendChild(card);
  };

  async function logsTable(url) {
    try {
      const res = await API.get(url);
      const logs = res.logs || [];
      if (!logs.length) return el('p', { class: 'muted' }, [t('no_data')]);
      return el('div', { class: 'table-wrap' }, [el('table', {}, [
        el('thead', {}, [el('tr', {}, [t('time'), t('requested'), t('provider'), t('status_word'), t('latency'), t('tokens'), t('cost')].map((h) => el('th', {}, [h])))]),
        el('tbody', {}, logs.map((l) => el('tr', {}, [
          el('td', {}, [(l.ts || '').replace('T', ' ').slice(0, 19)]),
          el('td', { class: 'mono' }, [l.requested_model]),
          el('td', {}, [l.provider_name]),
          el('td', {}, [statusBadge(l.status)]),
          el('td', {}, [l.latency_ms + ' ms']),
          el('td', {}, [l.tokens_in + '/' + l.tokens_out]),
          el('td', {}, [fmtCost(l.cost_usd)]),
        ]))),
      ])]);
    } catch (e) { return el('p', { class: 'muted' }, [e.message]); }
  }
  function statusBadge(s) {
    const code = parseInt(s, 10);
    if (code >= 200 && code < 400) return badge(s, 'ok');
    if (code >= 500 || code === 0) return badge(s, 'err');
    return badge(s, 'warn');
  }

  // ---- Provider ----
  Pages.providers = async function (main) {
    const head = el('div', { class: 'page-head' }, [el('h1', {}, [t('providers')])]);
    const addBtn = el('button', { class: 'btn', onclick: () => addProviderModal() }, ['+ ' + t('add_provider')]);
    head.appendChild(addBtn);
    main.appendChild(head);

    const wrap = el('div', {}, [el('p', { class: 'muted' }, [t('loading')])]);
    main.appendChild(wrap);
    await reload();

    async function reload() {
      wrap.innerHTML = '';
      const res = await API.get('/api/admin/providers');
      const providers = res.providers || [];
      if (!providers.length) { wrap.appendChild(el('p', { class: 'muted' }, [t('no_data')])); }
      for (const p of providers) wrap.appendChild(providerCard(p, reload));
    }

    function providerCard(p, refresh) {
      const c = el('div', { class: 'card', style: 'margin-bottom:14px' });
      const headRow = el('div', { class: 'page-head' }, [
        el('div', {}, [
          el('strong', {}, [p.name + ' ']),
          badge(p.prefix, 'info'), ' ',
          p.enabled ? badge('ON', 'ok') : badge('OFF', 'err'),
          el('div', { class: 'muted small mono' }, [p.base_url]),
        ]),
        el('div', { style: 'display:flex;gap:6px;flex-wrap:wrap' }, [
          el('button', { class: 'btn ghost small', onclick: async () => {
            try { const r = await API.post(`/api/admin/providers/${p.id}/test`); toast(r.ok ? `OK ${r.latency_ms} ms` : ('FAIL: ' + r.error), r.ok ? 'ok' : 'err'); }
            catch (e) { toast(e.message, 'err'); }
          } }, [t('test')]),
          el('button', { class: 'btn ghost small', onclick: async () => {
            try { const r = await API.post(`/api/admin/providers/${p.id}/sync-models`); toast(r.ok ? `${r.count} model (${r.source})` : r.error, r.ok ? 'ok' : 'err'); }
            catch (e) { toast(e.message, 'err'); }
          } }, [t('sync')]),
          el('button', { class: 'btn ghost small', onclick: async () => {
            await API.patch(`/api/admin/providers/${p.id}`, { enabled: !p.enabled });
            refresh();
          } }, [p.enabled ? '⏸' : '▶']),
          el('button', { class: 'btn danger small', onclick: async () => {
            if (!confirm(t('confirm_delete'))) return;
            try { await API.del(`/api/admin/providers/${p.id}`); refresh(); } catch (e) { toast(e.message, 'err'); }
          } }, [t('delete')]),
        ]),
      ]);
      c.appendChild(headRow);

      // kredensial
      const credWrap = el('div', {}, [
        el('label', {}, [t('credentials') + ' (' + (p.credentials || []).length + ')']),
      ]);
      for (const cr of p.credentials || []) {
        credWrap.appendChild(el('div', { class: 'kv', style: 'align-items:center;margin-bottom:4px' }, [
          badge('#' + cr.id, 'info'), el('span', {}, [cr.label || '—']),
          cr.status === 'active' ? badge('active', 'ok') : badge(cr.status, 'err'),
          cr.cooldown ? badge('cooldown', 'warn') : null,
          el('span', { class: 'muted small' }, [`${cr.use_count}x · ${cr.fail_count} err`]),
          el('button', { class: 'btn ghost small', onclick: async () => {
            await API.del('/api/admin/credentials/' + cr.id); refresh();
          } }, ['×']),
        ]));
      }
      credWrap.appendChild(el('button', { class: 'btn small', style: 'margin-top:6px', onclick: () => addCredModal(p, refresh) }, ['+ ' + t('add_key')]));
      c.appendChild(credWrap);
      return c;
    }

    function addCredModal(p, refresh) {
      const label = el('input', { placeholder: t('label') });
      const key = el('input', { placeholder: t('api_key') });
      const close = modal(el('div', {}, [
        el('h2', {}, [t('add_key') + ' — ' + p.name]),
        el('label', {}, [t('label')]), label,
        el('label', {}, [t('api_key')]), key,
        el('div', { class: 'modal-actions' }, [
          el('button', { class: 'btn ghost', onclick: close }, [t('cancel')]),
          el('button', { class: 'btn', onclick: async () => {
            try {
              await API.post(`/api/admin/providers/${p.id}/credentials`, { label: label.value, api_key: key.value, weight: 1 });
              close(); refresh(); toast(t('save_ok'), 'ok');
            } catch (e) { toast(e.message, 'err'); }
          } }, [t('save')]),
        ]),
      ]));
    }

    function addProviderModal() {
      API.get('/api/admin/templates').then((res) => {
        const tplSel = el('select', {}, (res.templates || []).map((tp) =>
          el('option', { value: tp.prefix }, [`${tp.name} (${tp.prefix}/…)`])));
        const keyIn = el('input', { placeholder: t('api_key') + ' (opsional)' });
        // kustom
        const custName = el('input', { placeholder: t('name') });
        const custPrefix = el('input', { placeholder: t('prefix') });
        const custURL = el('input', { placeholder: t('base_url') });
        const custType = el('select', {}, ['openai-compatible', 'anthropic-compatible', 'gemini', 'llamastash'].map((x) => el('option', { value: x === 'anthropic-compatible' ? 'anthropic' : x }, [x])));
        const modeTpl = el('div', {}, [el('label', {}, [t('seed_template')]), tplSel, el('label', {}, [t('api_key')]), keyIn]);
        const modeCust = el('div', { class: 'hidden' }, [el('label', {}, [t('name')]), custName, el('label', {}, [t('prefix')]), custPrefix, el('label', {}, [t('type')]), custType, el('label', {}, [t('base_url')]), custURL]);
        const isTpl = el('input', { type: 'checkbox', checked: '', onchange: (e) => {
          modeTpl.classList.toggle('hidden', !e.target.checked);
          modeCust.classList.toggle('hidden', e.target.checked);
        } });
        const close = modal(el('div', {}, [
          el('h2', {}, [t('add_provider')]),
          el('label', { style: 'display:flex;gap:6px;align-items:center' }, [isTpl, t('seed_template')]),
          modeTpl, modeCust,
          el('div', { class: 'modal-actions' }, [
            el('button', { class: 'btn ghost', onclick: close }, [t('cancel')]),
            el('button', { class: 'btn', onclick: async () => {
              try {
                if (isTpl.checked) {
                  await API.post('/api/admin/providers/seed', { prefix: tplSel.value, api_key: keyIn.value });
                } else {
                  await API.post('/api/admin/providers', {
                    type: custType.value, name: custName.value, prefix: custPrefix.value, base_url: custURL.value,
                    settings: { strategy: 'round_robin' },
                  });
                }
                close(); reload(); toast(t('save_ok'), 'ok');
              } catch (e) { toast(e.message, 'err'); }
            } }, [t('save')]),
          ]),
        ]));
      });
    }
  };

  // ---- Model & harga ----
  Pages.models = async function (main) {
    main.appendChild(el('h1', {}, [t('models')]));
    const wrap = el('div', { class: 'card table-wrap' }, [el('p', { class: 'muted' }, [t('loading')])]);
    main.appendChild(wrap);
    const res = await API.get('/api/admin/models');
    const models = res.models || [];
    wrap.innerHTML = '';
    wrap.appendChild(el('table', {}, [
      el('thead', {}, [el('tr', {}, [t('model'), t('alias'), t('price_in'), t('price_out'), t('context'), 'Tools/Vision', t('enabled'), ''].map((h) => el('th', {}, [h])))]),
      el('tbody', {}, models.map((m) => {
        const pin = el('input', { value: m.price_in_per_1m, style: 'width:90px', onchange: () => save(m, { price_in_per_1m: parseFloat(pin.value) }) });
        const pout = el('input', { value: m.price_out_per_1m, style: 'width:90px', onchange: () => save(m, { price_out_per_1m: parseFloat(pout.value) }) });
        const alias = el('input', { value: m.alias, placeholder: 'alias', style: 'width:130px', onchange: () => save(m, { alias: alias.value }) });
        return el('tr', {}, [
          el('td', { class: 'mono' }, [m.public_id]),
          el('td', {}, [alias]),
          el('td', {}, [pin]), el('td', {}, [pout]),
          el('td', {}, [fmtNum(m.context_window)]),
          el('td', {}, [(m.capabilities.tools ? '🔧' : '–') + ' ' + (m.capabilities.vision ? '👁' : '–')]),
          el('td', {}, [el('input', { type: 'checkbox', checked: m.enabled ? '' : null, onchange: (e) => save(m, { enabled: e.target.checked }) })]),
        ]);
      })),
    ]));
    if (!models.length) wrap.appendChild(el('p', { class: 'muted' }, [t('no_data')]));

    async function save(m, patch) {
      try { await API.patch('/api/admin/models/' + m.id, patch); toast(t('save_ok'), 'ok'); }
      catch (e) { toast(e.message, 'err'); }
    }
  };

  // ---- Combo ----
  Pages.combos = async function (main) {
    const head = el('div', { class: 'page-head' }, [el('h1', {}, [t('combos')])]);
    head.appendChild(el('button', { class: 'btn', onclick: () => createCombo() }, ['+ ' + t('create')]));
    main.appendChild(head);
    const wrap = el('div', {});
    main.appendChild(wrap);

    async function reload() {
      wrap.innerHTML = '';
      const [comboRes, modelRes] = await Promise.all([API.get('/api/admin/combos'), API.get('/api/admin/models')]);
      for (const c of comboRes.combos || []) {
        const card = el('div', { class: 'card', style: 'margin-bottom:10px' }, [
          el('div', { class: 'page-head' }, [
            el('div', {}, [
              el('strong', { class: 'mono' }, [c.name]),
              el('div', { class: 'muted small' }, [c.description]),
            ]),
            el('button', { class: 'btn danger small', onclick: async () => {
              if (!confirm(t('confirm_delete'))) return;
              await API.del('/api/admin/combos/' + c.id); reload();
            } }, [t('delete')]),
          ]),
          el('div', { class: 'muted' }, (c.steps || []).map((s, i) => el('div', {}, [`${i + 1}. ${s.public_id || s.model_id}`]))),
        ]);
        wrap.appendChild(card);
      }
      return modelRes.models || [];
    }
    const models = await reload();

    function createCombo() {
      const name = el('input', { placeholder: t('combo_name') });
      const desc = el('input', { placeholder: t('combo_desc') });
      const steps = [];
      const listEl = el('div', {});
      const sel = el('select', {}, models.map((m) => el('option', { value: m.id }, [m.public_id])));
      function renderSteps() {
        listEl.innerHTML = '';
        steps.forEach((id, i) => {
          const m = models.find((x) => String(x.id) === String(id));
          listEl.appendChild(el('div', { class: 'kv', style: 'align-items:center' }, [
            `${i + 1}.`, el('span', { class: 'mono' }, [m ? m.public_id : id]),
            el('button', { class: 'btn ghost small', onclick: () => { steps.splice(i, 1); renderSteps(); } }, ['×']),
          ]));
        });
      }
      const close = modal(el('div', {}, [
        el('h2', {}, [t('combo_name')]),
        el('label', {}, [t('combo_name')]), name,
        el('label', {}, [t('combo_desc')]), desc,
        el('label', {}, [t('combo_steps')]), sel,
        el('button', { class: 'btn small', style: 'margin:6px 0', onclick: () => { steps.push(sel.value); renderSteps(); } }, ['+']),
        listEl,
        el('div', { class: 'modal-actions' }, [
          el('button', { class: 'btn ghost', onclick: close }, [t('cancel')]),
          el('button', { class: 'btn', onclick: async () => {
            try {
              await API.post('/api/admin/combos', { name: name.value, description: desc.value, model_ids: steps.map(Number) });
              close(); reload(); toast(t('save_ok'), 'ok');
            } catch (e) { toast(e.message, 'err'); }
          } }, [t('save')]),
        ]),
      ]));
    }
  };

  // ---- User & API key & kuota ----
  Pages.users = async function (main) {
    const head = el('div', { class: 'page-head' }, [el('h1', {}, [t('users')])]);
    head.appendChild(el('button', { class: 'btn', onclick: createUser }, ['+ ' + t('create_user')]));
    main.appendChild(head);
    const wrap = el('div', {});
    main.appendChild(wrap);

    async function reload() {
      wrap.innerHTML = '';
      const res = await API.get('/api/admin/users');
      for (const item of res.users || []) {
        const u = item.user;
        const card = el('div', { class: 'card', style: 'margin-bottom:12px' }, [
          el('div', { class: 'page-head' }, [
            el('div', {}, [
              el('strong', {}, [u.email]), ' ', badge(u.role, u.role === 'super_admin' ? 'err' : 'info'), ' ',
              u.status === 'active' ? badge(u.status, 'ok') : badge(u.status, 'warn'),
            ]),
            el('div', { style: 'display:flex;gap:6px' }, [
              el('button', { class: 'btn ghost small', onclick: () => quotaModal(u) }, [t('quota')]),
              el('button', { class: 'btn ghost small', onclick: () => newKeyModal(u) }, ['+ ' + t('api_key')]),
              el('button', { class: 'btn danger small', onclick: async () => {
                if (!confirm(t('confirm_delete'))) return;
                try { await API.del('/api/admin/users/' + u.id); reload(); } catch (e) { toast(e.message, 'err'); }
              } }, [t('delete')]),
            ]),
          ]),
          keysList(u),
        ]);
        wrap.appendChild(card);
      }
    }

    function keysList(u) {
      const holder = el('div', {}, [el('label', {}, [t('keys')])]);
      API.get(`/api/admin/users/${u.id}/keys`).then((res) => {
        for (const k of res.keys || []) {
          holder.appendChild(el('div', { class: 'kv', style: 'align-items:center;margin-bottom:2px' }, [
            el('span', { class: 'mono small' }, [k.prefix + '…']),
            el('span', {}, [k.name]),
            k.revoked_at ? badge('revoked', 'err') : badge('active', 'ok'),
            el('span', { class: 'muted small' }, [`rpm=${k.rpm} tpm=${k.tpm}`]),
            el('button', { class: 'btn ghost small', onclick: async () => {
              await API.del('/api/admin/keys/' + k.id); reload();
            } }, ['×']),
          ]));
        }
      });
      return holder;
    }

    function createUser() {
      const email = el('input', { placeholder: t('email') });
      const pass = el('input', { type: 'password', placeholder: t('password') });
      const role = el('select', {}, ['member', 'viewer', 'admin'].map((r) => el('option', { value: r }, [r])));
      const close = modal(el('div', {}, [
        el('h2', {}, [t('create_user')]),
        el('label', {}, [t('email')]), email,
        el('label', {}, [t('password')]), pass,
        el('label', {}, [t('role')]), role,
        el('div', { class: 'modal-actions' }, [
          el('button', { class: 'btn ghost', onclick: close }, [t('cancel')]),
          el('button', { class: 'btn', onclick: async () => {
            try { await API.post('/api/admin/users', { email: email.value, password: pass.value, role: role.value }); close(); reload(); toast(t('save_ok'), 'ok'); }
            catch (e) { toast(e.message, 'err'); }
          } }, [t('save')]),
        ]),
      ]));
    }

    function newKeyModal(u) {
      const name = el('input', { placeholder: t('name') });
      const allowed = el('input', { value: '*', placeholder: t('allowed_models') });
      const rpm = el('input', { value: '0', placeholder: t('rpm') });
      const tpm = el('input', { value: '0', placeholder: t('tpm') });
      const out = el('div', { class: 'hidden' }, [
        el('p', { class: 'form-error', style: 'color:var(--ok)' }, [t('key_created_note')]),
        el('p', { class: 'mono', style: 'word-break:break-all;background:var(--bg);padding:10px;border-radius:8px' }),
      ]);
      const close = modal(el('div', {}, [
        el('h2', {}, [t('api_key') + ' — ' + u.email]),
        el('label', {}, [t('name')]), name,
        el('label', {}, [t('allowed_models')]), allowed,
        el('label', {}, [t('rpm')]), rpm,
        el('label', {}, [t('tpm')]), tpm,
        out,
        el('div', { class: 'modal-actions' }, [
          el('button', { class: 'btn ghost', onclick: close }, [t('cancel')]),
          el('button', { class: 'btn', onclick: async () => {
            try {
              const r = await API.post(`/api/admin/users/${u.id}/keys`, {
                name: name.value, allowed_models: allowed.value, rpm: parseInt(rpm.value) || 0, tpm: parseInt(tpm.value) || 0,
              });
              out.classList.remove('hidden');
              out.querySelector('p.mono').textContent = r.plaintext;
              toast(t('save_ok'), 'ok');
            } catch (e) { toast(e.message, 'err'); }
          } }, [t('save')]),
        ]),
      ]));
    }

    function quotaModal(u) {
      const dayTok = el('input', { value: '0' });
      const dayReq = el('input', { value: '0' });
      const dayCost = el('input', { value: '0' });
      const monTok = el('input', { value: '0' });
      const monCost = el('input', { value: '0' });
      API.get(`/api/admin/users/${u.id}/keys`).catch(() => {});
      const close = modal(el('div', {}, [
        el('h2', {}, [t('quota') + ' — ' + u.email]),
        el('h2', {}, [t('day')]),
        el('label', {}, [t('token_limit')]), dayTok,
        el('label', {}, [t('request_limit')]), dayReq,
        el('label', {}, [t('cost_limit')]), dayCost,
        el('h2', {}, [t('month')]),
        el('label', {}, [t('token_limit')]), monTok,
        el('label', {}, [t('cost_limit')]), monCost,
        el('div', { class: 'modal-actions' }, [
          el('button', { class: 'btn ghost', onclick: close }, [t('cancel')]),
          el('button', { class: 'btn', onclick: async () => {
            try {
              await API.put(`/api/admin/users/${u.id}/quota`, { period: 'day', token_limit: +dayTok.value || 0, request_limit: +dayReq.value || 0, cost_limit_usd: +dayCost.value || 0 });
              await API.put(`/api/admin/users/${u.id}/quota`, { period: 'month', token_limit: +monTok.value || 0, cost_limit_usd: +monCost.value || 0 });
              close(); toast(t('save_ok'), 'ok');
            } catch (e) { toast(e.message, 'err'); }
          } }, [t('save')]),
        ]),
      ]));
    }
    reload();
  };

  // ---- Log ----
  Pages.logs = async function (main) {
    main.appendChild(el('h1', {}, [t('logs')]));
    const user = el('input', { placeholder: t('filter_user'), style: 'width:120px' });
    const prov = el('input', { placeholder: t('filter_provider'), style: 'width:120px' });
    const status = el('select', {}, ['', '2xx', '4xx', '5xx'].map((s) => el('option', { value: s }, [s || t('filter_status')])));
    const apply = el('button', { class: 'btn', onclick: load }, ['↻']);
    const exportBtn = el('a', { class: 'btn ghost', href: '/api/admin/logs/export.csv', download: '' }, [t('export_csv')]);
    main.appendChild(el('div', { class: 'page-head' }, [el('div', { class: 'kv' }, [user, prov, status, apply, exportBtn])]));
    const wrap = el('div', { class: 'card table-wrap' });
    main.appendChild(wrap);

    async function load() {
      const q = new URLSearchParams({ limit: '200' });
      if (user.value) q.set('user', user.value);
      if (prov.value) q.set('provider', prov.value);
      if (status.value) q.set('status', status.value);
      wrap.innerHTML = '';
      wrap.appendChild(await logsTable('/api/admin/logs?' + q.toString()));
    }
    await load();
  };

  // ---- Analitik ----
  Pages.analytics = async function (main) {
    main.appendChild(el('h1', {}, [t('analytics')]));
    const res = await API.get('/api/admin/analytics/summary?days=7');
    main.appendChild(el('div', { class: 'grid cols-4' }, [
      statCard(t('requests'), fmtNum(res.totals.requests)),
      statCard(t('tokens'), fmtNum(res.totals.tokens_in + res.totals.tokens_out)),
      statCard(t('cost'), fmtCost(res.totals.cost_usd)),
      statCard('p95 ' + t('latency'), fmtMs(res.totals.p95_latency_ms)),
    ]));

    const tbl = (title, head, rows) => {
      const c = el('div', { class: 'card', style: 'margin-top:14px' }, [el('h2', {}, [title])]);
      c.appendChild(el('div', { class: 'table-wrap' }, [el('table', {}, [
        el('thead', {}, [el('tr', {}, head.map((h) => el('th', {}, [h])))]),
        el('tbody', {}, rows),
      ])]));
      return c;
    };
    main.appendChild(tbl(t('per_day'), [t('time'), t('requests'), t('errors'), t('tokens'), t('cost')],
      (res.per_day || []).map((d) => el('tr', {}, [d.day, d.requests, d.errors, d.tokens_in + d.tokens_out, fmtCost(d.cost_usd)]))));
    main.appendChild(tbl(t('per_provider'), [t('provider'), t('requests'), t('errors'), t('tokens'), t('cost')],
      (res.per_provider || []).map((p) => el('tr', {}, [p.provider_name, p.requests, p.errors, p.tokens_in + p.tokens_out, fmtCost(p.cost_usd)]))));
    main.appendChild(tbl(t('per_user'), [t('user'), t('requests'), t('tokens'), t('cost')],
      (res.per_user || []).map((u) => el('tr', {}, [u.email, u.requests, u.tokens_in + u.tokens_out, fmtCost(u.cost_usd)]))));
  };

  // ---- Pengaturan ----
  Pages.settings = async function (main) {
    main.appendChild(el('h1', {}, [t('settings')]));
    const res = await API.get('/api/admin/settings');
    const card = el('div', { class: 'card' }, [el('h2', {}, [t('settings')])]);
    const tgToken = el('input', { value: res.settings.notification_telegram_token || '', type: 'password' });
    const tgChat = el('input', { value: res.settings.notification_chat_id || '' });
    card.appendChild(el('label', {}, ['Telegram bot token'])); card.appendChild(tgToken);
    card.appendChild(el('label', {}, ['Telegram chat id'])); card.appendChild(tgChat);
    card.appendChild(el('div', { class: 'muted small', style: 'margin-top:6px' }, ['v1.1: notifikasi via Telegram/Email (F-21)']));
    card.appendChild(el('div', { class: 'modal-actions' }, [
      el('button', { class: 'btn', onclick: async () => {
        await API.put('/api/admin/settings', { notification_telegram_token: tgToken.value, notification_chat_id: tgChat.value });
        toast(t('save_ok'), 'ok');
      } }, [t('save')]),
    ]));
    main.appendChild(card);

    const backupCard = el('div', { class: 'card', style: 'margin-top:14px' }, [
      el('h2', {}, ['Backup']),
      el('button', { class: 'btn', onclick: async () => {
        try { await API.post('/api/admin/system/backup'); toast(t('save_ok'), 'ok'); } catch (e) { toast(e.message, 'err'); }
      } }, [t('backup_now')]),
    ]);
    main.appendChild(backupCard);

    // ---- Pembaruan Aplikasi (git pull + rebuild + restart dari dashboard) ----
    const updCard = el('div', { class: 'card', style: 'margin-top:14px' }, [el('h2', {}, ['Pembaruan Aplikasi'])]);
    main.appendChild(updCard);
    const updInfo = el('div', { class: 'small' });
    const updLog = el('pre', { class: 'mono small hidden', style: 'background:var(--bg);padding:10px;border-radius:8px;white-space:pre-wrap;max-height:260px;overflow-y:auto' });
    const btnCheck = el('button', { class: 'btn ghost' }, ['Cek pembaruan']);
    const btnUpdate = el('button', { class: 'btn' }, ['Perbarui sekarang']);
    updCard.appendChild(el('div', { class: 'kv', style: 'margin:8px 0' }, [btnCheck, btnUpdate]));
    updCard.appendChild(updInfo);
    updCard.appendChild(updLog);

    async function loadUpdateStatus(fetchNew) {
      try {
        const st = await API.get('/api/admin/system/update/status' + (fetchNew ? '?fetch=1' : ''));
        updInfo.innerHTML = '';
        const rows = [
          ['Versi terpasang', st.version],
          ['Mode', st.mode],
        ];
        if (st.hint) rows.push(['Info', st.hint]);
        if (!st.in_container) {
          rows.push(['Repo (mirror)', st.repo_found ? (st.repo_dir + ' @ ' + (st.current_head || '') + ' [' + (st.branch || '') + ']') : 'belum di-clone (otomatis saat update)']);
          rows.push(['Ketinggalan commit', st.behind_commits !== undefined ? String(st.behind_commits) : '—']);
          rows.push(['git', st.git_available ? '✓' : '✗']);
          rows.push(['Go', st.go_available ? '✓' : '✗']);
          rows.push(['Restart otomatis (sudo helper)', st.sudo_apply_available ? '✓' : '✗ (binary akan distage; perintah pemasangan ditampilkan)']);
          if (st.staged_binary) rows.push(['Binary ter-stage', 'ada — siap dipasang']);
          if (st.last_update) rows.push(['Update terakhir', st.last_update.time + ' (' + st.last_update.commits + ' commit)']);
        }
        for (const [k, v] of rows) {
          updInfo.appendChild(el('div', { style: 'border-bottom:1px solid var(--border);padding:3px 0' }, [
            el('span', { class: 'muted', style: 'display:inline-block;min-width:220px' }, [k + ':']),
            el('span', { class: 'mono' }, [String(v === undefined ? '—' : v)]),
          ]));
        }
        return st;
      } catch (e) { toast(e.message, 'err'); }
    }
    btnCheck.addEventListener('click', async () => {
      btnCheck.disabled = true; btnCheck.textContent = 'Memeriksa…';
      await loadUpdateStatus(true);
      btnCheck.disabled = false; btnCheck.textContent = 'Cek pembaruan';
    });
    btnUpdate.addEventListener('click', async () => {
      if (!confirm('Perbarui aplikasi sekarang?\nBuild bisa 2–5 menit; bila helper sudo aktif, service akan di-restart otomatis.')) return;
      btnUpdate.disabled = true; btnUpdate.textContent = 'Memperbarui…';
      updLog.classList.remove('hidden');
      updLog.textContent = '⏳ fetch + build… (jangan tutup halaman ini)';
      let res = null;
      try {
        res = await API.post('/api/admin/system/update', {});
        updLog.textContent = res.log || '';
        if (res.error) toast(res.error, 'err');
      } catch (e) {
        // service mungkin sedang restart — jangan panik, poll status
        updLog.textContent += '\n(koneksi terputus — kemungkinan service sedang restart, memeriksa status…)';
      }
      // poll status hingga versi berubah / stabil
      const before = (await loadUpdateStatus(false)) || {};
      for (let i = 0; i < 12; i++) {
        await new Promise(r2 => setTimeout(r2, 3000));
        try {
          const st = await API.get('/api/admin/system/update/status');
          if (!st.busy) { updLog.textContent += '\nversi sekarang: ' + st.version; break; }
        } catch (e) { /* tunggu lagi */ }
      }
      toast('Proses pembaruan selesai', 'ok');
      btnUpdate.disabled = false; btnUpdate.textContent = 'Perbarui sekarang';
      loadUpdateStatus(false);
    });
    loadUpdateStatus(false);

    const auditCard = el('div', { class: 'card', style: 'margin-top:14px' }, [el('h2', {}, [t('audit_log')])]);
    const audit = await API.get('/api/admin/audit?limit=30');
    const list = el('div', {}, (audit.audit || []).map((a) => el('div', { class: 'kv small', style: 'border-bottom:1px solid var(--border);padding:4px 0' }, [
      el('span', { class: 'muted mono' }, [(a.ts || '').replace('T', ' ').slice(0, 19)]),
      el('span', { class: 'mono' }, [a.action]), el('span', {}, [a.target]),
    ])));
    auditCard.appendChild(list);
    main.appendChild(auditCard);
  };

  // ---- LlamaStash ----
  Pages.local = async function (main) {
    main.appendChild(el('h1', {}, [t('local')]));
    const card = el('div', { class: 'card' });
    main.appendChild(card);

    async function reload() {
      card.innerHTML = '';
      let s;
      try { s = await API.get('/api/admin/local/status'); } catch (e) { toast(e.message, 'err'); return; }

      // kartu instalasi (FR-6.8)
      const installCard = el('div', { class: 'card', style: 'margin-bottom:14px' });
      installCard.appendChild(el('h2', {}, ['Install LlamaStash']));
      if (s.installed) {
        installCard.appendChild(el('div', { class: 'kv', style: 'align-items:center' }, [
          badge('terpasang', 'ok'),
          s.version ? el('span', { class: 'muted small' }, [s.version]) : null,
          el('span', { class: 'muted small mono' }, [s.bin || '']),
        ]));
        if (s.version) card.appendChild(installCard);
      } else if (s.in_container) {
        installCard.appendChild(el('p', { class: 'muted small' }, [s.hint || '']));
        card.appendChild(installCard);
      } else {
        installCard.appendChild(el('p', { class: 'muted small' },
          ['Belum terpasang. Tombol ini akan mengunduh installer resmi, menjalankan ', 
           el('code', {}, ['llamastash init --recommended --json']),
           ', lalu mendaftarkan provider lokal + modelnya secara otomatis.']));
        const logPre = el('pre', { class: 'mono small hidden', style: 'background:var(--bg);padding:10px;border-radius:8px;white-space:pre-wrap;max-height:220px;overflow-y:auto' });
        const btn = el('button', { class: 'btn', onclick: async () => {
          if (!confirm('Install LlamaStash di server ini sekarang?')) return;
          btn.disabled = true; btn.textContent = 'Menginstall… (bisa beberapa menit)';
          logPre.classList.remove('hidden'); logPre.textContent = '⏳ sedang berjalan…';
          try {
            const res = await API.post('/api/admin/local/install', {});
            logPre.textContent = res.log || JSON.stringify(res, null, 2);
            toast(res.ok ? 'LlamaStash terpasang ✓' : (res.error || 'gagal'), res.ok ? 'ok' : 'err');
          } catch (e) {
            logPre.textContent = '✗ ' + e.message;
            toast(e.message, 'err');
          }
          btn.disabled = false; btn.textContent = 'Install LlamaStash';
          setTimeout(reload, 1500);
        } }, ['Install LlamaStash']);
        installCard.appendChild(el('div', { style: 'margin:8px 0' }, [btn]));
        installCard.appendChild(logPre);
        card.appendChild(installCard);
      }

      card.appendChild(el('div', { class: 'page-head' }, [
        el('div', {}, [
          el('strong', {}, ['LlamaStash']), ' ',
          s.daemon_alive ? badge(t('alive'), 'ok') : badge(t('down'), 'err'),
          s.latency_ms !== undefined ? el('span', { class: 'muted small' }, [' ' + s.latency_ms + ' ms']) : null,
          el('div', { class: 'muted small mono' }, [s.url]),
          el('div', { class: 'muted small' }, ['CLI: ' + (s.cli_available ? '✓' : '✗')]),
        ]),
      ]));
      const models = s.models || [];
      if (!models.length) { card.appendChild(el('p', { class: 'muted' }, [t('no_data')])); return; }
      const tbl = el('table', {}, [
        el('thead', {}, [el('tr', {}, ['Model', t('status_word'), ''] .map((h) => el('th', {}, [h])))]),
        el('tbody', {}, models.map((m) => el('tr', {}, [
          el('td', { class: 'mono' }, [m.name]),
          el('td', {}, [m.loaded ? badge('loaded', 'ok') : badge('idle', 'info')]),
          el('td', {}, [
            el('button', { class: 'btn small', onclick: async () => {
              try { await API.post(`/api/admin/local/models/${encodeURIComponent(m.name)}/start`); toast(t('save_ok'), 'ok'); reload(); }
              catch (e) { toast(e.message, 'err'); }
            } }, [t('start')]), ' ',
            el('button', { class: 'btn ghost small', onclick: async () => {
              try { await API.post(`/api/admin/local/models/${encodeURIComponent(m.name)}/stop`); toast(t('save_ok'), 'ok'); }
              catch (e) { toast(e.message, 'err'); }
            } }, [t('stop')]),
          ]),
        ]))),
      ]);
      card.appendChild(tbl);
    }
    await reload();
  };

  window.Pages = Pages;
})();
