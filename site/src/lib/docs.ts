import { getCollection, type CollectionEntry } from "astro:content";

export type DocEntry = CollectionEntry<"docs">;

// The sidebar, in reading order. A page not listed here still builds but
// stays out of the nav.
export const docsNav = [
	{ label: "Start", items: ["index", "quickstart", "install", "join-a-factory"] },
	{ label: "Use", items: ["concepts", "orders", "dispatch", "peers", "board", "traces", "orchestrators"] },
	{ label: "Operate", items: ["hosts", "admission", "headless", "share-a-host", "windows", "plugins", "failure-modes"] },
	{ label: "Reference", items: ["cli", "order-reference", "config-reference", "tradeoffs", "changelog"] },
] as const;

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
			items: group.items.map((id) => byId.get(id)).filter((entry): entry is DocEntry => Boolean(entry)),
		}))
		.filter((group) => group.items.length);
}

export async function getDocSiblings(id: string) {
	const ordered = (await getDocNavGroups()).flatMap((group) => group.items);
	const index = ordered.findIndex((entry) => entry.id === id);
	return {
		previous: index > 0 ? ordered[index - 1] : undefined,
		next: index >= 0 && index < ordered.length - 1 ? ordered[index + 1] : undefined,
	};
}
