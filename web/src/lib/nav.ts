// nav.ts — definisi menu aplikasi, dipakai sidebar dan palet perintah (⌘K).
import {
	LayoutDashboard, MessageSquare, Cable, BrainCircuit, Link2, Users,
	ScrollText, ChartLine, Server, Settings, Gauge, Rocket
} from '@lucide/svelte';
import { roleAtLeast } from '#lib/stores.svelte.js';

export type NavItem = {
	route: string;
	icon: any;
	label: string; // kunci i18n
	min: string; // role minimal
	max?: string; // disembunyikan untuk role ini ke atas
	group: string;
	keywords?: string; // kata kunci tambahan untuk pencarian
	badge?: 'providers' | 'errors' | 'quota'; // lencana status (admin)
};

export const NAV: NavItem[] = [
	{ route: '/dashboard', icon: LayoutDashboard, label: 'dashboard', min: 'viewer', group: 'main', keywords: 'ringkasan beranda home statistik' },
	{ route: '/chat', icon: MessageSquare, label: 'chat', min: 'member', group: 'main', keywords: 'ai tanya percakapan' },
	// halaman Penggunaan API: untuk member saja (admin punya Log/Analitik lengkap)
	{ route: '/usage', icon: Gauge, label: 'usage_api', min: 'member', max: 'admin', group: 'main', keywords: 'api key kuota panduan base url' },
	{ route: '/providers', icon: Cable, label: 'providers', min: 'admin', group: 'gateway', keywords: 'openai anthropic gemini kredensial', badge: 'providers' },
	{ route: '/models', icon: BrainCircuit, label: 'models', min: 'admin', group: 'gateway', keywords: 'harga alias context vision' },
	{ route: '/combos', icon: Link2, label: 'combos', min: 'admin', group: 'gateway', keywords: 'fallback rute' },
	{ route: '/users', icon: Users, label: 'users', min: 'admin', group: 'kelola', keywords: 'peran role kuota api key', badge: 'quota' },
	{ route: '/logs', icon: ScrollText, label: 'logs', min: 'admin', group: 'observe', keywords: 'request error isi', badge: 'errors' },
	{ route: '/analytics', icon: ChartLine, label: 'analytics', min: 'admin', group: 'observe', keywords: 'grafik biaya token' },
	{ route: '/local', icon: Server, label: 'local', min: 'admin', group: 'system', keywords: 'llamastash llama.cpp model lokal unduh' },
	{ route: '/update', icon: Rocket, label: 'system_update', min: 'admin', group: 'system', keywords: 'versi rilis upgrade' },
	{ route: '/settings', icon: Settings, label: 'settings', min: 'admin', group: 'system', keywords: 'pengaturan backup audit cloudflare' }
];

export const GROUPS: { key: string; label: string }[] = [
	{ key: 'main', label: '' },
	{ key: 'gateway', label: 'Gateway' },
	{ key: 'kelola', label: 'Kelola' },
	{ key: 'observe', label: 'Observabilitas' },
	{ key: 'system', label: 'Sistem' }
];

export function visibleFor(role: string): NavItem[] {
	return NAV.filter((n) => roleAtLeast(role, n.min) && (!n.max || !roleAtLeast(role, n.max)));
}

export function isActive(path: string, route: string): boolean {
	return path === route || path.startsWith(route + '/');
}
