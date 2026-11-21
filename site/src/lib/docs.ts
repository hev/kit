import { getCollection, type CollectionEntry } from "astro:content";

export type DocEntry = CollectionEntry<"docs">;

// The sidebar, in reading order. A page not listed here still builds but
// stays out of the nav. A group with edition "pro" shows only when the
// sidebar's Pro | OSS switch is on Pro.
type NavGroup = { label: string; items: readonly string[]; edition?: "pro" | "oss" };

export const docsNav: readonly NavGroup[] = [
	{ label: "Start", items: ["index", "quickstart", "install", "upgrade"] },
	{ label: "Use", items: ["concepts", "orders", "dispatch", "context", "peers", "board", "traces", "hooks", "notifications", "evidence", "review", "orchestrators", "orchestrator-examples", "steering"] },
	{ label: "Operate", items: ["clustering", "admission", "updating", "secrets", "subscriptions", "skills-shelf", "licensing", "exec-commands", "failure-modes"] },
	{ label: "Reference", items: ["cli", "order-reference", "config-reference", "windows", "changelog"] },
	{ label: "API", edition: "pro", items: ["api", "api/traces", "api/board", "api/orders", "api/peers", "api/hooks"] },
];

export function getDocHref(id: string): string {
	return id === "index" ? "/docs" : `/docs/${id}`;
}

export async function getAllDocs(): Promise<DocEntry[]> {
	return getCollection("docs");
}

export async function getDocNavGroups() {
	const byId = new Map((await getAllDocs()).map((entry) => [entry.id, entry]));
	return docsNav
		.map((group) => ({
			label: group.label,
			edition: group.edition,
			items: group.items.map((id) => byId.get(id)).filter((entry): entry is DocEntry => Boolean(entry)),
		}))
		.filter((group) => group.items.length);
}

// Previous and next pages in nav order, each with its group's edition so
// the layout can hide a pager link into a Pro-only group in OSS mode.
export async function getDocSiblings(id: string) {
	const ordered = (await getDocNavGroups()).flatMap((group) =>
		group.items.map((entry) => ({ entry, edition: group.edition })),
	);
	const index = ordered.findIndex((item) => item.entry.id === id);
	return {
		previous: index > 0 ? ordered[index - 1] : undefined,
		next: index >= 0 && index < ordered.length - 1 ? ordered[index + 1] : undefined,
	};
}
