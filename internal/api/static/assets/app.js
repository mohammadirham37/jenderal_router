// app.js — router hash, alur auth, tema, dan bootstrap aplikasi.
(function () {
  const { el, toast } = UI;
  const t = (k) => I18N.t(k);

  const NAV = [
    { route: 'dashboard', icon: '📊', label: 'dashboard', minRole: 'viewer' },
    { route: 'chat', icon: '💬', label: 'chat', minRole: 'member' },
    { route: 'providers', icon: '🔌', label: 'providers', minRole: 'viewer' },
    { route: 'models', icon: '🧠', label: 'models', minRole: 'viewer' },
    { route: 'combos', icon: '🔗', label: 'combos', minRole: 'viewer' },
    { route: 'users', icon: '👥', label: 'users', minRole: 'admin' },
    { route: 'logs', icon: '📜', label: 'logs', minRole: 'viewer' },
    { route: 'analytics', icon: '📈', label: 'analytics', minRole: 'viewer' },
    { route: 'local', icon: '🖥️', label: 'local', minRole: 'viewer' },
    { route: 'settings', icon: '⚙️', label: 'settings', minRole: 'admin' },
  ];
  const roleRank = { viewer: 1, member: 2, admin: 3, super_admin: 4 };

  window.App = {
    me: null,
    onUnauthorized() {
      API.reset();
      App.me = null;
      showAuth();
    },
    rerender() { render(); },
  };

  function currentRoute() {
    const h = location.hash.replace(/^#\/?/, '') || 'dashboard';
    return h.split('?')[0];
  }

  function buildNav() {
    const nav = document.getElementById('nav');
    nav.innerHTML = '';
    for (const item of NAV) {
      if (App.me && roleRank[App.me.role] < roleRank[item.minRole]) continue;
      nav.appendChild(el('a', {
        href: '#/' + item.route,
        class: currentRoute() === item.route ? 'active' : '',
      }, [item.icon + ' ', I18N.t(item.label)]));
    }
  }

  async function render() {
    buildNav();
    const main = document.getElementById('main');
    main.innerHTML = '';
    const route = currentRoute();
    const fn = window.Pages && window.Pages[route];
    if (!fn) { main.appendChild(el('p', { class: 'muted' }, ['404'])); return; }
    try { await fn(main); }
    catch (e) { toast(e.message, 'err'); }
  }

  function showApp() {
    document.getElementById('auth-screen').classList.add('hidden');
    document.getElementById('app').classList.remove('hidden');
    document.getElementById('me-email').textContent = App.me ? App.me.email : '';
    render();
  }

  function showAuth() {
    document.getElementById('app').classList.add('hidden');
    const screen = document.getElementById('auth-screen');
    const card = document.getElementById('auth-card');
    screen.classList.remove('hidden');
    card.innerHTML = '';

    API.get('/api/setup/status', { noRedirect: true }).then((s) => {
      if (s.needs_setup) return setupForm(card);
      loginForm(card);
    }).catch(() => loginForm(card));
  }

  function loginForm(card) {
    card.innerHTML = '';
    const email = el('input', { type: 'email', placeholder: t('email'), autocomplete: 'username' });
    const pass = el('input', { type: 'password', placeholder: t('password'), autocomplete: 'current-password' });
    const err = el('div', { class: 'form-error' });
    const btn = el('button', { class: 'btn', style: 'width:100%;justify-content:center;margin-top:6px' }, [t('login_btn')]);
    btn.addEventListener('click', async () => {
      err.textContent = '';
      try {
        const res = await API.post('/api/login', { email: email.value, password: pass.value }, { noRedirect: true });
        API.setCSRF(res.csrf);
        App.me = res.user;
        showApp();
      } catch (e) { err.textContent = e.message; }
    });
    pass.addEventListener('keydown', (e) => { if (e.key === 'Enter') btn.click(); });
    card.appendChild(el('div', { class: 'logo-big' }, ['🛰️']));
    card.appendChild(el('h1', { style: 'text-align:center' }, ['JenderalRouter']));
    card.appendChild(el('label', {}, [t('email')])); card.appendChild(email);
    card.appendChild(el('label', {}, [t('password')])); card.appendChild(pass);
    card.appendChild(err); card.appendChild(btn);
  }

  function setupForm(card) {
    card.innerHTML = '';
    const email = el('input', { type: 'email', placeholder: 'admin@domainanda.com' });
    const pass = el('input', { type: 'password', placeholder: '≥12 karakter, huruf besar+kecil+angka' });
    const err = el('div', { class: 'form-error' });
    const btn = el('button', { class: 'btn', style: 'width:100%;justify-content:center;margin-top:6px' }, [t('setup_btn')]);
    btn.addEventListener('click', async () => {
      err.textContent = '';
      try {
        const res = await API.post('/api/setup', { email: email.value, password: pass.value }, { noRedirect: true });
        API.setCSRF(res.csrf);
        App.me = res.user;
        showApp();
        toast('Admin dibuat. Selamat datang! 🎉', 'ok');
      } catch (e) { err.textContent = e.message; }
    });
    card.appendChild(el('div', { class: 'logo-big' }, ['🛰️']));
    card.appendChild(el('h1', { style: 'text-align:center' }, [t('setup_title')]));
    card.appendChild(el('p', { class: 'muted', style: 'text-align:center' }, [t('setup_desc')]));
    card.appendChild(el('label', {}, [t('email')])); card.appendChild(email);
    card.appendChild(el('label', {}, [t('password')])); card.appendChild(pass);
    card.appendChild(err); card.appendChild(btn);
  }

  // tema
  function applyTheme() {
    const saved = localStorage.getItem('jr_theme') || 'dark';
    document.documentElement.setAttribute('data-theme', saved);
  }
  document.getElementById('themeToggle').addEventListener('click', () => {
    const cur = document.documentElement.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
    localStorage.setItem('jr_theme', cur);
    document.documentElement.setAttribute('data-theme', cur);
  });
  document.getElementById('langToggle').addEventListener('click', () => I18N.toggle());
  document.getElementById('logoutBtn').addEventListener('click', async () => {
    try { await API.post('/api/logout', {}); } catch (e) {}
    App.onUnauthorized();
  });
  window.addEventListener('hashchange', () => { if (App.me) render(); });

  // bootstrap: cek sesi
  applyTheme();
  I18N.setLang(localStorage.getItem('jr_lang') || 'id');
  (async function boot() {
    try {
      const res = await API.get('/api/me', { noRedirect: true });
      App.me = res.user;
      API.setCSRF(res.csrf);
      showApp();
    } catch (e) {
      showAuth();
    }
  })();
})();
