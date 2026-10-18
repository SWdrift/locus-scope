import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { test } from "node:test";
import path from "node:path";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const root = path.resolve("test/e2e/case/scope-composition/app");
const executable = path.resolve("temp/release-validation/npm/node_modules/@sundw/locus-scope/bin/locus-scope.mjs");

test("built CLI preserves composed Scope fixture behavior", async () => {
  const { stdout } = await execFileAsync(process.execPath, [executable, "--scope", root, "validate", "--json"], {
    cwd: process.cwd()
  });
  const summary = JSON.parse(stdout);
  assert.deepEqual(
    { valid: summary.valid, scopes: summary.scopes, entities: summary.entities, relations: summary.relations },
    { valid: true, scopes: 2, entities: 5, relations: 4 }
  );

  const graph = await execFileAsync(
    process.execPath,
    [executable, "--scope", root, "graph", "api", "--depth", "1", "--json"],
    { cwd: process.cwd() }
  );
  const graphResult = JSON.parse(graph.stdout);
  assert.equal(graphResult.seeds[0].key.id, "api");
  assert.equal(graphResult.relations.length, 2);
});

test("built CLI rejects mutation targets excluded by Scope document policy", async () => {
  const mutationRoot = path.resolve("test/e2e/case/mutation-discovery");
  for (const file of ["ignored/new.locus.yaml", ".locus/hidden.locus.yaml"]) {
    await assert.rejects(
      execFileAsync(
        process.execPath,
        [executable, "--scope", mutationRoot, "entity", "add", "new", "--file", file, "--json"],
        { cwd: process.cwd() }
      ),
      error => {
        if (!error || typeof error !== "object" || !("stderr" in error)) return false;
        return String(error.stderr).includes("definition path is ignored");
      }
    );
    await assert.rejects(readFile(path.join(mutationRoot, file)));
  }
});

test("built CLI resolves Entity IDs containing filter operators", async () => {
  const queryRoot = path.resolve("test/e2e/case/query-edge");
  const { stdout } = await execFileAsync(
    process.execPath,
    [executable, "--scope", queryRoot, "entity", "a=b", "--json"],
    { cwd: process.cwd() }
  );
  assert.equal(JSON.parse(stdout).object.id, "a=b");
});

test("built CLI mutates JSON definitions with a UTF-8 BOM", async () => {
  const mutationRoot = path.resolve("temp/e2e-run/mutation-bom");
  await rm(mutationRoot, { recursive: true, force: true });
  await mkdir(mutationRoot, { recursive: true });
  await writeFile(path.join(mutationRoot, "locus.yaml"), "id: mutation-bom\n");
  await writeFile(
    path.join(mutationRoot, "entities.locus.json"),
    '\uFEFF{\n  "entities": [\n    { "id": "bom", "value": 1 }\n  ]\n}\n'
  );
  const response = await execFileAsync(
    process.execPath,
    [executable, "--scope", mutationRoot, "entity", "set", "bom", "value=2", "--json"],
    { cwd: process.cwd() }
  );
  assert.equal(JSON.parse(response.stdout).object.object.id, "bom");
  const persisted = JSON.parse((await readFile(path.join(mutationRoot, "entities.locus.json"), "utf8")).replace(/^\uFEFF/u, ""));
  assert.equal(persisted.entities[0].value, 2);
});

test("built CLI rejects a Locus package that blocks package.json export", async () => {
  const packageRoot = path.resolve("temp/e2e-run/blocked-package-export");
  const packageDirectory = path.join(packageRoot, "node_modules", "@example", "blocked");
  await rm(packageRoot, { recursive: true, force: true });
  await mkdir(packageDirectory, { recursive: true });
  await writeFile(
    path.join(packageRoot, "package.json"),
    `${JSON.stringify({ name: "blocked-consumer", version: "1.0.0", dependencies: { "@example/blocked": "1.0.0" } }, null, 2)}\n`
  );
  await writeFile(path.join(packageRoot, "locus.yaml"), 'id: blocked-consumer\nimports:\n  blocked: "@example/blocked"\n');
  await writeFile(
    path.join(packageDirectory, "package.json"),
    `${JSON.stringify(
      {
        name: "@example/blocked",
        version: "1.0.0",
        locus: { entry: "locus.yaml" },
        exports: { ".": { import: "./index.js" } }
      },
      null,
      2
    )}\n`
  );
  await writeFile(path.join(packageDirectory, "index.js"), "export {};\n");
  await writeFile(path.join(packageDirectory, "locus.yaml"), "id: blocked\n");
  await assert.rejects(
    execFileAsync(process.execPath, [executable, "--scope", packageRoot, "validate", "--json"], {
      cwd: process.cwd()
    }),
    error => {
      if (!error || typeof error !== "object" || !("stderr" in error)) return false;
      return String(error.stderr).includes("scope.package_invalid");
    }
  );
});
