import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		sveltekit({
			preprocess: vitePreprocess(),
			adapter: adapter({
				pages: 'build',
				assets: 'build',
				fallback: 'index.html' /* SPA mode — auth-gated, tanpa SEO */,
				precompress: false,
				strict: true
			})
		})
	],
	server: {
		proxy: {
			'/api': 'http://127.0.0.1:20130',
			'/v1': 'http://127.0.0.1:20130'
		}
	}
});
