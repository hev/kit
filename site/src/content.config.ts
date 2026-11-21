import { defineCollection, z } from "astro:content";
import { glob } from "astro/loaders";

const docs = defineCollection({
	loader: glob({ pattern: "**/*.{md,mdx}", base: "./src/content/docs" }),
	schema: z.object({
		title: z.string(),
		description: z.string(),
		group: z.string(),
		noindex: z.boolean().optional(),
		// Shown beside the page title: the page's primary CLI command and the
		// bundled skill (a directory under skills/) that teaches it to agents.
		command: z.string().optional(),
		skill: z.string().optional(),
		// The page's hosted endpoint (API pages), shown in the same box.
		endpoint: z.string().optional(),
		// "pro": the page documents a Pro-only feature. In OSS mode the
		// layout shows a switch-to-Pro banner above it.
		edition: z.enum(["oss", "pro"]).optional(),
		// Shorter label for the sidebar and pager when the title needs
		// context the nav group already gives ("Traces API" under API).
		sidebarLabel: z.string().optional(),
	}),
});

export const collections = { docs };
