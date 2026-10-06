// stores.svelte.ts — state global (Svelte 5 runes): saya, bahasa, tema, toast.
import type { Toast } from './components/Toasts.svelte';

export type Me = {
	id: number;
	email: string;
	role: string;
	status: string;
};

export const app = $state({
	me: null as Me | null,
	csrfReady: false,
	lang: (typeof localStorage !== 'undefined' && localStorage.getItem('jr_lang')) || 'id',
	toasts: [] as Toast[]
});

export const roleRank: Record<string, number> = {
	viewer: 1,
	member: 2,
	admin: 3,
	super_admin: 4
};

export function roleAtLeast(role: string, min: string): boolean {
	return (roleRank[role] ?? 0) >= (roleRank[min] ?? 0);
}

export function toast(msg: string, kind: 'info' | 'ok' | 'err' = 'info', ms = 4200) {
	const id = Math.random().toString(36).slice(2);
	app.toasts.push({ id, msg, kind });
	setTimeout(() => dismissToast(id), ms);
}
export function dismissToast(id: string) {
	const i = app.toasts.findIndex((t) => t.id === id);
	if (i >= 0) app.toasts.splice(i, 1);
}

// ---- i18n ----
const dict: Record<string, Record<string, string>> = {
	id: {
		dashboard: 'Ringkasan', providers: 'Provider', models: 'Model & Harga', combos: 'Combo',
		users: 'User & Peran', logs: 'Log', analytics: 'Analitik', settings: 'Pengaturan',
		local: 'LlamaStash', chat: 'Chat', logout: 'Keluar', login: 'Masuk',
		usage_api: 'Penggunaan API', my_usage: 'Pemakaian saya',
		history: 'Riwayat',
		setup_title: 'Pengaturan Awal',
		setup_desc: 'Buat akun administrator pertama. Tidak ada password bawaan — pilih yang kuat.',
		email: 'Email', password: 'Password', login_btn: 'Masuk', setup_btn: 'Buat Admin',
		auth_headline: 'Satu gerbang untuk semua model.',
		auth_sub: 'Gateway LLM multi-user yang berjalan sepenuhnya di server Anda.',
		login_continue: 'Masuk untuk melanjutkan.',
		requests_today: 'Request hari ini', tokens_today: 'Token hari ini', cost_7d: 'Biaya 7 hari (USD)',
		error_rate: 'Error rate', recent_logs: 'Log terbaru', no_data: 'Belum ada data',
		add_provider: 'Tambah Provider', name: 'Nama', prefix: 'Prefix', base_url: 'Base URL',
		test: 'Tes koneksi', sync: 'Sinkron model', credentials: 'Kredensial', add_key: 'Tambah API key',
		api_key: 'API key', delete: 'Hapus', save: 'Simpan', cancel: 'Batal', enabled: 'Aktif',
		alias: 'Alias', price_in: 'Harga in/1M', price_out: 'Harga out/1M', context: 'Context',
		combo_name: 'Nama combo', create: 'Buat', role: 'Peran', status: 'Status', quota: 'Kuota',
		keys: 'API Keys', create_user: 'Buat User', period: 'Periode', day: 'Harian', month: 'Bulanan',
		export_csv: 'Ekspor CSV', backup_now: 'Backup sekarang', audit_log: 'Audit log',
		alive: 'hidup', down: 'mati', start: 'Nyalakan', stop: 'Matikan', new_chat: 'Chat baru',
		send: 'Kirim', quota_left: 'Sisa kuota', answered_by: 'Dijawab oleh', loading: 'Memuat…',
		export_conv: 'Ekspor percakapan',
		time: 'Waktu', requested: 'Diminta', provider: 'Provider', latency: 'Latensi',
		tokens: 'Token', cost: 'Biaya', attempts: 'Percobaan', requests: 'Request', errors: 'Error',
		per_day: 'Per hari', per_provider: 'Per provider', per_user: 'Per user', user: 'User',
		model: 'Model', update_now: 'Perbarui sekarang', check_update: 'Cek pembaruan',
		install: 'Install', confirm_delete: 'Yakin hapus?'
	},
	en: {
		dashboard: 'Dashboard', providers: 'Providers', models: 'Models & Prices', combos: 'Combos',
		users: 'Users & Roles', logs: 'Logs', analytics: 'Analytics', settings: 'Settings',
		local: 'LlamaStash', chat: 'Chat', logout: 'Log out', login: 'Sign in',
		usage_api: 'API Usage', my_usage: 'My usage',
		history: 'History',
		setup_title: 'First-run Setup',
		setup_desc: 'Create the first administrator account. There is no default password — choose a strong one.',
		email: 'Email', password: 'Password', login_btn: 'Sign in', setup_btn: 'Create admin',
		auth_headline: 'One gateway for every model.',
		auth_sub: 'A multi-user LLM gateway that runs entirely on your own server.',
		login_continue: 'Sign in to continue.',
		requests_today: 'Requests today', tokens_today: 'Tokens today', cost_7d: 'Cost 7d (USD)',
		error_rate: 'Error rate', recent_logs: 'Recent logs', no_data: 'No data yet',
		add_provider: 'Add provider', name: 'Name', prefix: 'Prefix', base_url: 'Base URL',
		test: 'Test connection', sync: 'Sync models', credentials: 'Credentials', add_key: 'Add API key',
		api_key: 'API key', delete: 'Delete', save: 'Save', cancel: 'Cancel', enabled: 'Enabled',
		alias: 'Alias', price_in: 'In price/1M', price_out: 'Out price/1M', context: 'Context',
		combo_name: 'Combo name', create: 'Create', role: 'Role', status: 'Status', quota: 'Quota',
		keys: 'API Keys', create_user: 'Create user', period: 'Period', day: 'Daily', month: 'Monthly',
		export_csv: 'Export CSV', backup_now: 'Back up now', audit_log: 'Audit log',
		alive: 'alive', down: 'down', start: 'Start', stop: 'Stop', new_chat: 'New chat',
		send: 'Send', quota_left: 'Quota left', answered_by: 'Answered by', loading: 'Loading…',
		export_conv: 'Export conversation',
		time: 'Time', requested: 'Requested', provider: 'Provider', latency: 'Latency',
		tokens: 'Tokens', cost: 'Cost', attempts: 'Attempts', requests: 'Requests', errors: 'Errors',
		per_day: 'Per day', per_provider: 'Per provider', per_user: 'Per user', user: 'User',
		model: 'Model', update_now: 'Update now', check_update: 'Check for updates',
		install: 'Install', confirm_delete: 'Delete?'
	}
};

export function t(key: string): string {
	return dict[app.lang]?.[key] ?? dict.id[key] ?? key;
}

export function setLang(l: 'id' | 'en') {
	app.lang = l;
	localStorage.setItem('jr_lang', l);
	document.documentElement.lang = l;
}
export function toggleLang() {
	setLang(app.lang === 'id' ? 'en' : 'id');
}

export function applyTheme(theme: 'dark' | 'light') {
	localStorage.setItem('jr_theme', theme);
	document.documentElement.setAttribute('data-theme', theme);
}
export function toggleTheme() {
	const cur = document.documentElement.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
	applyTheme(cur as 'dark' | 'light');
}
