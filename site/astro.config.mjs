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
	redirects: { "/docs/expediter": "/docs/orchestrators", "/docs/headless": "/docs/hosts#add-a-headless-machine", "/docs/join-a-factory": "/docs/join-a-host" },
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
