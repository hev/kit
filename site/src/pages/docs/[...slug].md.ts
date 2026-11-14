// Each docs page's Markdown at /docs/ID.md, linked from /llms.txt.
import type { APIRoute } from "astro";
import { getAllDocs, type DocEntry } from "../../lib/docs";
import { markdownResponse } from "../../lib/docs-export";

export const prerender = true;

export async function getStaticPaths() {
	return (await getAllDocs()).map((entry) => ({ params: { slug: entry.id }, props: { entry } }));
}

export const GET: APIRoute = ({ props }) => markdownResponse((props as { entry: DocEntry }).entry);
