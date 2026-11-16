/*
Purpose: Check deterministic Wails binding layout normalization.
Used by: Node's test runner.
Responsibilities: Exercise module relocation and reject invalid generated trees.
Important: Wails runtime identifiers and bare imports remain untouched.
*/
import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { normalizeBindings } from "./normalize-bindings.mjs";

const modulePrefix = "github.com/sandjaie/KubeSift";
function fixture(t, files) {
  const base = mkdtempSync(join(tmpdir(), "bindings-test-"));
  t.after(() => rmSync(base, { recursive: true, force: true }));
  const input = join(base, "input");
  for (const [file, contents] of Object.entries(files)) {
    mkdirSync(join(input, file, ".."), { recursive: true });
    writeFileSync(join(input, file), contents);
  }
  return [input, join(base, "output"), join(base, "second")];
}

test("relocates TypeScript specifiers and leaves runtime names alone", (t) => {
  const model = `${modulePrefix}/internal/model/models.ts`;
  const service = `${modulePrefix}/internal/desktop/service.ts`;
  const ids = `ByID(42); ByName("${modulePrefix}/internal/desktop.Service.Do");`;
  const [input, output, second] = fixture(t, {
    [model]: "export type Event = {}",
    [service]: 'import type { Event } from "../model/models.js";\n' + ids,
    "github.com/framework/event/types.d.ts":
      'import "@wailsio/runtime";\n' +
      `import "../../sandjaie/KubeSift/internal/model/models.js";\n` +
      `export type { Event } from "../../sandjaie/KubeSift/internal/model/models.js";\n` +
      `type E = import("../../sandjaie/KubeSift/internal/model/models.js").Event;\n`,
    "events/index.ts": 'import "../github.com/framework/event/types.js";',
  });
  normalizeBindings(input, output, modulePrefix);
  normalizeBindings(input, second, modulePrefix);
  const paths = ["events/index.ts", "internal/desktop/service.ts", "internal/model/models.ts", "github.com/framework/event/types.d.ts"];
  for (const path of paths) assert.deepEqual(readFileSync(join(output, path)), readFileSync(join(second, path)));
  assert.equal(readFileSync(join(output, paths[1]), "utf8"), 'import type { Event } from "../model/models.js";\n' + ids);
  const event = readFileSync(join(output, paths[3]), "utf8");
  assert.equal(event.match(/\.\.\/\.\.\/\.\.\/internal\/model\/models\.js/g)?.length, 3);
  assert.match(event, /import "@wailsio\/runtime"/);
  assert.deepEqual(readdirSync(join(output, "github.com")), ["framework"]);
});

test("rejects collisions and unresolved relative references", (t) => {
  const [input, output, second] = fixture(t, {
    [`${modulePrefix}/internal/model.ts`]: "",
    "internal/model.ts": "",
    "events/index.ts": 'import "./missing.js";',
  });
  assert.throws(() => normalizeBindings(input, output, modulePrefix), /collision/i);
  rmSync(join(input, "internal/model.ts"));
  assert.throws(() => normalizeBindings(input, second, modulePrefix), /unresolved/i);
});
