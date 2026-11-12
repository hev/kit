import { defineConfig } from "astro/config";
import mdx from "@astrojs/mdx";
import sitemap from "@astrojs/sitemap";

export default defineConfig({
	site: "https://hevkit.com",
	devToolbar: { enabled: false },
	// layer = 4331, mind = 4321, kit = 4341. strictPort fails loudly instead
	// of hopping into a neighbour's port.
	server: { port: 4341, strictPort: true },
	redirects: { "/docs/expediter": "/docs/orchestrators", "/docs/headless": "/docs/hosts#add-a-headless-machine", "/docs/join-a-factory": "/docs/join-a-host" },
	integrations: [mdx(), sitemap({ filter: (page) => !page.includes("/docs/invited") })],
	markdown: { syntaxHighlight: "shiki", shikiConfig: { theme: "vesper" } },
});
