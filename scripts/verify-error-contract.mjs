import { readdir, readFile } from "node:fs/promises";
import { join, relative } from "node:path";

const root = process.cwd();
const sourceRoots = ["src", "test"];
const ignoredNonReasonStrings = new Set(["scope.mutate", "scope.cli"]);
const reasonPattern = /['"`]((?:scope\.)[a-z0-9_]+)['"`]/gu;
const registryPattern = /^\s*['"]((?:scope\.)[a-z0-9_]+)['"]:\s*['"]([a-z_]+)['"],?$/gmu;
const documentedPattern = /\| `((?:scope\.)[a-z0-9_]+)`\s*\|/gu;

async function filesUnder(directory) {
  const entries = await readdir(join(root, directory), { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) files.push(...(await filesUnder(path)));
    else if (/\.(?:[cm]?js|ts)$/u.test(entry.name)) files.push(path);
  }
  return files;
}

const errorsSource = await readFile(join(root, "src", "domain", "errors.ts"), "utf8");
const protocol = await readFile(join(root, "documents", "design", "protocol", "ERRORS.md"), "utf8");
const registry = new Map();
for (const match of errorsSource.matchAll(registryPattern)) registry.set(match[1], match[2]);
const documented = new Map();
for (const match of protocol.matchAll(documentedPattern)) documented.set(match[1], match[2] ?? registry.get(match[1]));
const referenced = new Map();
for (const directory of sourceRoots) {
  for (const filename of await filesUnder(directory)) {
    const source = await readFile(join(root, filename), "utf8");
    for (const match of source.matchAll(reasonPattern)) {
      const reason = match[1];
      if (ignoredNonReasonStrings.has(reason)) continue;
      const files = referenced.get(reason) ?? [];
      files.push(relative(root, filename));
      referenced.set(reason, files);
    }
  }
}

const problems = [];
for (const [reason, code] of registry) {
  if (!documented.has(reason)) problems.push(`registry reason is undocumented: ${reason}`);
  else if (documented.get(reason) !== code)
    problems.push(`code mismatch for ${reason}: registry=${code}, document=${documented.get(reason)}`);
  if (!referenced.has(reason)) problems.push(`registry reason is unreferenced: ${reason}`);
}
for (const [reason, files] of referenced) {
  if (!registry.has(reason))
    problems.push(`source reason is undeclared: ${reason} (${[...new Set(files)].join(", ")})`);
}
for (const reason of documented.keys()) {
  if (!registry.has(reason)) problems.push(`document reason is absent from registry: ${reason}`);
}
for (const filename of await filesUnder("src")) {
  const source = await readFile(join(root, filename), "utf8");
  if (filename.replaceAll("\\", "/") !== "src/domain/errors.ts" && /new ApplicationError\s*\(/u.test(source))
    problems.push(`direct ApplicationError construction outside errors.ts: ${filename}`);
}

if (problems.length > 0) {
  console.error(problems.map(problem => `error-contract: ${problem}`).join("\n"));
  process.exitCode = 1;
} else {
  console.log(`error-contract: ${registry.size} reasons declared, documented, and referenced`);
}
