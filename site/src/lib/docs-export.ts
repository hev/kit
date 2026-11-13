// Plain-text exports of the docs for agents: /llms.txt (an index of every
// page in nav order), /llms-full.txt (every page's text) and /docs/ID.md
// (one page's Markdown). The MDX components are flattened to Markdown so an
// agent reading the raw text sees the same content a reader sees.
import { getAllDocs, getDocNavGroups, getDocHref, type DocEntry } from "./docs";

export const SITE = "https://hevkit.com";

export function pageUrl(id: string): string {
	return `${SITE}${getDocHref(id)}`;
}

export function markdownPath(id: string): string {
	return `/docs/${id}.md`;
}

export async function orderedDocs(): Promise<DocEntry[]> {
	return (await getDocNavGroups()).flatMap((group) => group.items);
}

// Reads the string literals of `key: "..."` pairs inside one JS object literal.
function objectFields(source: string): Record<string, string> {
	const fields: Record<string, string> = {};
	for (const match of source.matchAll(/(\w+):\s*"((?:[^"\\]|\\.)*)"/g)) {
		fields[match[1]] = JSON.parse(`"${match[2]}"`);
	}
	return fields;
}

function objects(source: string): Record<string, string>[] {
	return [...source.matchAll(/\{([^{}]*)\}/g)].map((match) => objectFields(match[1]));
}

export function toMarkdown(entry: DocEntry): string {
	let body = entry.body ?? "";
	body = body.replace(/^import .*;\n/gm, "");
	body = body.replace(/<LinkGrid links=\{\[([\s\S]*?)\]\} \/>/g, (_, list: string) =>
		objects(list)
			.map((link) => `- [${link.label}](${link.href.startsWith("/") ? SITE + link.href : link.href}): ${link.description}`)
			.join("\n"),
	);
	body = body.replace(/<Steps items=\{\[([\s\S]*?)\]\} \/>/g, (_, list: string) =>
		objects(list)
			.map((step, index) => `${index + 1}. **${step.title}.** ${step.body}`)
			.join("\n"),
	);
	body = body.replace(/<Callout label="([^"]*)">\n?([\s\S]*?)\n?<\/Callout>/g, (_, label: string, text: string) =>
		`> **${label}:** ${text.trim().split("\n").join("\n> ")}`,
	);
	body = body.replace(/<AgentPrompt>\s*\n/g, "Agent prompt:\n\n").replace(/\n?<\/AgentPrompt>/g, "");
	body = body.replace(/<CodeTabs[^>]*>\s*\n/g, "").replace(/\n?<\/CodeTabs>/g, "");
	body = body.replace(/<img [^>]*class="shot-dark"[^>]*\/>\n?/g, "");
	body = body.replace(/<img [^>]*src="([^"]*)"[^>]*alt="([^"]*)"[^>]*\/>/g, (_, src: string, alt: string) => `![${alt}](${SITE}${src})`);
	body = body.replace(/\]\(\/(?!\/)/g, `](${SITE}/`);
	return `# ${entry.data.title}\n\n> ${entry.data.description}\n\nSource: ${pageUrl(entry.id)}\n\n${body.trim().replace(/\n{3,}/g, "\n\n")}\n`;
}

function text(body: string): Response {
	return new Response(body, { headers: { "Content-Type": "text/plain; charset=utf-8" } });
}

export async function llmsIndex(): Promise<Response> {
	const groups = await getDocNavGroups();
	const parts = [
		"# hev kit",
		"",
		"> hev kit gives a team the tools to build its own software factory: a trace daemon, dispatch to shared hosts, and admission for repos, in one binary.",
		"",
		`Each page links to its Markdown. The full text of every page is at ${SITE}/llms-full.txt.`,
	];
	for (const group of groups) {
		parts.push("", `## ${group.label}`, "");
		for (const entry of group.items) {
			parts.push(`- [${entry.data.title}](${SITE}${markdownPath(entry.id)}): ${entry.data.description} Page: ${pageUrl(entry.id)}`);
		}
	}
	const listed = new Set(groups.flatMap((group) => group.items.map((entry) => entry.id)));
	const others = (await getAllDocs()).filter((entry) => !listed.has(entry.id));
	if (others.length) {
		parts.push("", "## Other pages", "");
		for (const entry of others) {
			parts.push(`- [${entry.data.title}](${SITE}${markdownPath(entry.id)}): ${entry.data.description} Page: ${pageUrl(entry.id)}`);
		}
	}
	return text(parts.join("\n") + "\n");
}

export async function llmsFull(): Promise<Response> {
	const ordered = await orderedDocs();
	const listed = new Set(ordered.map((entry) => entry.id));
	const docs = [...ordered, ...(await getAllDocs()).filter((entry) => !listed.has(entry.id))];
	return text(["# hev kit docs", "", ...docs.map(toMarkdown)].join("\n"));
}

export function markdownResponse(entry: DocEntry): Response {
	return new Response(toMarkdown(entry), { headers: { "Content-Type": "text/markdown; charset=utf-8" } });
}
