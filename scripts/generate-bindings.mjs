/*
Purpose: Generate and publish one complete set of normalized Wails bindings.
Used by: The shared bindings task for npm, development and packaging.
Responsibilities: Isolate generation, normalize and format the artifacts, and recover failed publication.
Important: The published tree remains intact if staging fails.
*/
import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, readdirSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { format, resolveConfig } from "prettier";
import { normalizeBindings } from "./normalize-bindings.mjs";

const root = fileURLToPath(new URL("../", import.meta.url));
const frontend = join(root, "frontend");
const published = join(frontend, "bindings");
const modulePrefix = /^module\s+([^\s]+)\s*$/m.exec(readFileSync(join(root, "go.mod"), "utf8"))?.[1];
if (!modulePrefix) throw Error("Missing Go module declaration");
const stage = mkdtempSync(join(frontend, ".bindings-"));
const raw = join(stage, "raw");
const normalized = join(stage, "normalized");
const previous = join(stage, "previous");
const run = (file, args) => execFileSync(file, args, { cwd: root, stdio: "inherit" });
async function formatTree(dir) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) await formatTree(path);
    else if (path.endsWith(".ts"))
      writeFileSync(path, await format(readFileSync(path, "utf8"), { ...(await resolveConfig(path)), filepath: path }));
  }
}

try {
  run("wails3", ["generate", "bindings", "-ts", "-i", "-d", raw, "./cmd/kubesift"]);
  normalizeBindings(raw, normalized, modulePrefix);
  run("python3", ["scripts/source-descriptions.py", "bindings", normalized]);
  await formatTree(normalized);
  if (existsSync(published)) renameSync(published, previous);
  try {
    renameSync(normalized, published);
  } catch (error) {
    if (existsSync(previous) && !existsSync(published)) renameSync(previous, published);
    throw error;
  }
} finally {
  if (!existsSync(previous) || existsSync(published)) rmSync(stage, { recursive: true, force: true });
}
