#!/usr/bin/env node
// Runs the @hevmind/ask CLI from the site root, as hevlayer.com's
// scripts/ask.mjs does. The digest is built in three steps so distillation
// can run in your agent's own context instead of on a provider API key:
//
//   pnpm ask:digest:corpus     # shard the docs into .hev-ask/shards
//   (distil each pending shard; see the build-digest skill in @hevmind/ask)
//   pnpm ask:digest:assemble   # write the committed .hev-ask/ tree
//   pnpm ask:digest:verify     # build the site and check every anchor
//
// A full `digest build` calls the provider for every shard, so it needs
// --full to run on purpose.
import { spawn } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const siteRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const askBin = process.env.HEVKIT_ASK_BIN ?? path.join(siteRoot, "node_modules", "@hevmind", "ask", "bin", "ask.mjs");

const args = process.argv.slice(2);
const full = args.includes("--full");
const rest = args.filter((arg) => arg !== "--full");
if (rest[0] === "digest" && rest[1] === "build" && !full) {
	console.error("[hevkit-ask] Full-corpus distillation is off by default. Use the corpus/status/assemble flow, or rerun `digest build --full` on purpose.");
	process.exit(2);
}

const child = spawn(process.execPath, [askBin, ...rest], { cwd: siteRoot, env: process.env, stdio: "inherit" });
child.on("error", (error) => {
	console.error(`[hevkit-ask] Could not start ask: ${error.message}`);
	process.exit(1);
});
child.on("exit", (code, signal) => process.exit(signal ? 1 : (code ?? 1)));
