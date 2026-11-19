import { defineConfig } from "astro/config";
import mdx from "@astrojs/mdx";
import sitemap from "@astrojs/sitemap";
import cloudflare from "@astrojs/cloudflare";
import hevAsk from "@hevmind/ask";

export default defineConfig({
	site: "https://hevkit.com",
	devToolbar: { enabled: false },
	// layer = 4331, mind = 4321, kit = 4341. strictPort fails loudly instead
	// of hopping into a neighbour's port.
	server: { port: 4341, strictPort: true },
	// Pages folded into others keep their old URLs working.
	redirects: {
		"/docs/expediter": "/docs/orchestrators",
		"/docs/headless": "/docs/clustering#add-a-headless-machine",
		"/docs/join-a-factory": "/docs/clustering#join-a-cluster",
		"/docs/kit-and-layer": "/docs/upgrade",
		"/docs/hosts": "/docs/clustering",
		"/docs/share-a-host": "/docs/clustering#share-a-node-with-a-team",
		"/docs/join-a-host": "/docs/clustering#join-a-cluster",
		"/docs/api": "/docs/upgrade",
		"/docs/api/dashboard": "/docs/upgrade",
		"/docs/api/board": "/docs/upgrade",
		"/docs/api/field-kit": "/docs/upgrade",
		"/docs/mobile-app-config": "/docs/config-reference",
		"/docs/tradeoffs": "/docs/concepts",
	},
	// Static-first, as on hevlayer.com: every page prerenders; only the
	// on-demand /api/ask route that hev ask injects runs as a Cloudflare
	// Pages Function. The overlay's answers need ANTHROPIC_API_KEY there.
	adapter: cloudflare({ platformProxy: { enabled: true } }),
	integrations: [
		mdx(),
		sitemap({ filter: (page) => !page.includes("/docs/invited") }),
		hevAsk({ collections: ["docs"], basePath: "/docs/", model: "claude-sonnet-4-6" }),
	],
	markdown: { syntaxHighlight: "shiki", shikiConfig: { theme: "vesper" } },
});
