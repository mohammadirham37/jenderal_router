// i18n.js — dua bahasa: Indonesia & Inggris (NFR-16).
(function () {
  const dict = {
    id: {
      dashboard: 'Ringkasan', providers: 'Provider', models: 'Model & Harga', combos: 'Combo',
      users: 'User & Peran', logs: 'Log', analytics: 'Analitik', settings: 'Pengaturan',
      local: 'Status LlamaStash', chat: 'Chat', logout: 'Keluar',
      login: 'Masuk', setup_title: 'Pengaturan Awal', setup_desc: 'Buat akun administrator pertama. Tidak ada password bawaan — pilih yang kuat.',
      email: 'Email', password: 'Password', login_btn: 'Masuk', setup_btn: 'Buat Admin',
      requests_today: 'Request hari ini', tokens_today: 'Token hari ini', cost_7d: 'Biaya 7 hari (USD)',
      error_rate: 'Error rate', recent_logs: 'Log terbaru', no_data: 'Belum ada data',
      add_provider: 'Tambah Provider', seed_template: 'Dari Template', custom: 'Kustom',
      name: 'Nama', type: 'Tipe', prefix: 'Prefix', base_url: 'Base URL', enabled: 'Aktif',
      test: 'Tes koneksi', sync: 'Sinkron model', credentials: 'Kredensial', add_key: 'Tambah API key',
      api_key: 'API key', label: 'Label', delete: 'Hapus', edit: 'Ubah', save: 'Simpan', cancel: 'Batal',
      alias: 'Alias', price_in: 'Harga input/1M', price_out: 'Harga output/1M', context: 'Context window',
      combo_name: 'Nama combo', combo_desc: 'Deskripsi', combo_steps: 'Langkah (urut)', create: 'Buat',
      role: 'Peran', status: 'Status', quota: 'Kuota', keys: 'API Keys', create_user: 'Buat User',
      allowed_models: 'Model diizinkan (* = semua)', rpm: 'RPM', tpm: 'TPM', expires: 'Kedaluwarsa',
      key_created_note: 'Salin sekarang — key hanya ditampilkan sekali!',
      period: 'Periode', token_limit: 'Batas token', request_limit: 'Batas request', cost_limit: 'Batas biaya USD',
      day: 'Harian', month: 'Bulanan', export_csv: 'Ekspor CSV', from: 'Dari', to: 'Sampai',
      filter_user: 'User', filter_provider: 'Provider', filter_status: 'Status',
      backup_now: 'Backup sekarang', audit_log: 'Audit log', daemon: 'Daemon', alive: 'hidup', down: 'mati',
      start: 'Nyalakan', stop: 'Matikan', new_chat: 'Chat baru', send: 'Kirim', quota_left: 'Sisa kuota',
      system_prompt: 'System prompt', temperature: 'Temperature', max_tokens: 'Max tokens',
      answered_by: 'Dijawab oleh', loading: 'Memuat…', none: 'Tidak ada', model: 'Model',
      latency: 'Latensi', ttft: 'TTFT', tokens: 'Token', cost: 'Biaya', status_word: 'Status',
      attempts: 'Percobaan', requested: 'Diminta', time: 'Waktu', user: 'User', provider: 'Provider',
      per_day: 'Per hari', per_provider: 'Per provider', per_user: 'Per user', requests: 'Request',
      errors: 'Error', save_ok: 'Tersimpan', confirm_delete: 'Yakin hapus?',
    },
    en: {
      dashboard: 'Dashboard', providers: 'Providers', models: 'Models & Prices', combos: 'Combos',
      users: 'Users & Roles', logs: 'Logs', analytics: 'Analytics', settings: 'Settings',
      local: 'LlamaStash Status', chat: 'Chat', logout: 'Log out',
      login: 'Sign in', setup_title: 'First-run Setup', setup_desc: 'Create the first administrator account. There is no default password — choose a strong one.',
      email: 'Email', password: 'Password', login_btn: 'Sign in', setup_btn: 'Create admin',
      requests_today: 'Requests today', tokens_today: 'Tokens today', cost_7d: 'Cost 7d (USD)',
      error_rate: 'Error rate', recent_logs: 'Recent logs', no_data: 'No data yet',
      add_provider: 'Add provider', seed_template: 'From template', custom: 'Custom',
      name: 'Name', type: 'Type', prefix: 'Prefix', base_url: 'Base URL', enabled: 'Enabled',
      test: 'Test connection', sync: 'Sync models', credentials: 'Credentials', add_key: 'Add API key',
      api_key: 'API key', label: 'Label', delete: 'Delete', edit: 'Edit', save: 'Save', cancel: 'Cancel',
      alias: 'Alias', price_in: 'Input price/1M', price_out: 'Output price/1M', context: 'Context window',
      combo_name: 'Combo name', combo_desc: 'Description', combo_steps: 'Steps (ordered)', create: 'Create',
      role: 'Role', status: 'Status', quota: 'Quota', keys: 'API Keys', create_user: 'Create user',
      allowed_models: 'Allowed models (* = all)', rpm: 'RPM', tpm: 'TPM', expires: 'Expires',
      key_created_note: 'Copy it now — the key is shown only once!',
      period: 'Period', token_limit: 'Token limit', request_limit: 'Request limit', cost_limit: 'Cost limit USD',
      day: 'Daily', month: 'Monthly', export_csv: 'Export CSV', from: 'From', to: 'To',
      filter_user: 'User', filter_provider: 'Provider', filter_status: 'Status',
      backup_now: 'Back up now', audit_log: 'Audit log', daemon: 'Daemon', alive: 'alive', down: 'down',
      start: 'Start', stop: 'Stop', new_chat: 'New chat', send: 'Send', quota_left: 'Quota left',
      system_prompt: 'System prompt', temperature: 'Temperature', max_tokens: 'Max tokens',
      answered_by: 'Answered by', loading: 'Loading…', none: 'None', model: 'Model',
      latency: 'Latency', ttft: 'TTFT', tokens: 'Tokens', cost: 'Cost', status_word: 'Status',
      attempts: 'Attempts', requested: 'Requested', time: 'Time', user: 'User', provider: 'Provider',
      per_day: 'Per day', per_provider: 'Per provider', per_user: 'Per user', requests: 'Requests',
      errors: 'Errors', save_ok: 'Saved', confirm_delete: 'Delete?',
    },
  };

  let lang = localStorage.getItem('jr_lang') || 'id';

  window.I18N = {
    t(key) { return (dict[lang] && dict[lang][key]) || dict.id[key] || key; },
    lang: () => lang,
    setLang(l) {
      lang = l;
      localStorage.setItem('jr_lang', l);
      document.documentElement.lang = l;
      const btn = document.getElementById('langToggle');
      if (btn) btn.textContent = l.toUpperCase();
      document.querySelectorAll('[data-i18n]').forEach((el) => {
        el.textContent = I18N.t(el.getAttribute('data-i18n'));
      });
    },
    toggle() { I18N.setLang(lang === 'id' ? 'en' : 'id'); if (window.App) App.rerender(); },
  };
})();
