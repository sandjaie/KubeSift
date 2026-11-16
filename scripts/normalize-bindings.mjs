/*
Purpose: Relocate project-owned Wails bindings without changing runtime identifiers.
Used by: The binding generation pipeline and its fixture tests.
Responsibilities: Resolve generated relative module references before moving their source and target.
Important: Only literal TypeScript module specifiers are eligible for rewriting.
*/
import { mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { join, posix as path } from "node:path";
import ts from "typescript";

function specifiers(source) {
  const result = [];
  function visit(node) {
    let value;
    if (ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) value = node.moduleSpecifier;
    if (ts.isExternalModuleReference(node)) value = node.expression;
    if (ts.isImportTypeNode(node) && ts.isLiteralTypeNode(node.argument)) value = node.argument.literal;
    if (ts.isCallExpression(node) && node.expression.kind === ts.SyntaxKind.ImportKeyword) value = node.arguments[0];
    if (value && ts.isStringLiteralLike(value) && /^\.\.?\//.test(value.text)) result.push(value);
    ts.forEachChild(node, visit);
  }
  visit(source);
  return result;
}

function resolveModule(files, base) {
  const extension = path.extname(base);
  if (extension === ".js") {
    const stem = base.slice(0, -3);
    return [stem + ".ts", stem + ".tsx", stem + ".d.ts", base].find((name) => files.has(name));
  }
  return [base, base + ".ts", base + ".tsx", base + ".d.ts", path.join(base, "index.ts"), path.join(base, "index.d.ts")].find((name) =>
    files.has(name),
  );
}

export function normalizeBindings(inputRoot, outputRoot, modulePrefix) {
  const files = new Map();
  const destinations = new Set();
  function relocate(name) {
    return name.startsWith(modulePrefix + "/internal/") ? name.slice(modulePrefix.length + 1) : name;
  }
  function scan(dir = "") {
    for (const entry of readdirSync(join(inputRoot, dir), { withFileTypes: true })) {
      const name = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        scan(name);
      } else if (entry.isFile()) {
        const destination = relocate(name);
        if (destinations.has(destination)) throw Error(`Generated binding collision at ${destination}`);
        destinations.add(destination);
        files.set(name, destination);
      } else {
        throw Error(`Unexpected generated entry ${name}`);
      }
    }
  }
  scan();
  if (!files.size) throw Error("No generated bindings found");
  for (const [name, destination] of files) {
    const input = readFileSync(join(inputRoot, name));
    let output = input;
    if (/\.[jt]sx?$/.test(name)) {
      const source = ts.createSourceFile(name, input.toString("utf8"), ts.ScriptTarget.Latest, true);
      let text = source.text;
      const edits = [];
      for (const specifier of specifiers(source)) {
        const oldBase = path.join(path.dirname(name), specifier.text);
        const target = resolveModule(files, oldBase);
        if (!target) throw Error(`Unresolved generated reference ${specifier.text} in ${name}`);
        const newBase = relocate(oldBase.replace(/\/$/, ""));
        let relative = path.relative(path.dirname(destination), newBase);
        if (!relative.startsWith("../") && relative !== "..") relative = "./" + relative;
        if (resolveModule(destinations, path.join(path.dirname(destination), relative)) !== files.get(target)) {
          throw Error(`Unresolved relocated reference ${specifier.text} in ${destination}`);
        }
        if (path.join(path.dirname(destination), specifier.text) !== newBase) {
          edits.push([specifier.getStart(source), specifier.getEnd(), JSON.stringify(relative)]);
        }
      }
      for (const [start, end, value] of edits.sort((a, b) => b[0] - a[0])) {
        text = text.slice(0, start) + value + text.slice(end);
      }
      output = text;
    }
    mkdirSync(join(outputRoot, destination, ".."), { recursive: true });
    writeFileSync(join(outputRoot, destination), output);
  }
}
