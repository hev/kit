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
	}),
});

export const collections = { docs };
