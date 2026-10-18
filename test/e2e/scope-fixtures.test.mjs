import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { cp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
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

test("built CLI completes Entity and Relation mutation lifecycle", async () => {
  const mutationRoot = path.resolve("temp/e2e-run/mutation-lifecycle");
  const mutationFixture = path.resolve("test/e2e/case/mutation-lifecycle");
  await rm(mutationRoot, { recursive: true, force: true });
  await cp(mutationFixture, mutationRoot, { recursive: true });
  await assert.rejects(
    execFileAsync(
      process.execPath,
      [
        executable,
        "--scope",
        mutationRoot,
        "relation",
        "add",
        "api",
        "uses",
        "api",
        "from=other",
        "--file",
        "relations.locus.yaml",
        "--json"
      ],
      { cwd: process.cwd() }
    ),
    error => {
      if (!error || typeof error !== "object" || !("stderr" in error)) return false;
      return String(error.stderr).includes("scope.field_invalid");
    }
  );
  await assert.rejects(readFile(path.join(mutationRoot, "relations.locus.yaml")));
  for (const args of [
    ["entity", "add", "db", "--file", "entities.locus.yaml", "--json"],
    ["entity", "set", "db", "value=2", "--json"],
    ["entity", "unset", "db", "value", "--json"],
    ["entity", "set", "yml", "value=3", "--json"],
    ["relation", "add", "api", "uses", "db", "--file", "relations.locus.yaml", "--json"],
    ["relation", "set", "api", "uses", "db", "critical=true", "--json"],
    ["relation", "unset", "api", "uses", "db", "critical", "--json"],
    ["relation", "remove", "api", "uses", "db", "--json"],
    ["entity", "remove", "db", "--json"]
  ]) {
    const response = await execFileAsync(process.execPath, [executable, "--scope", mutationRoot, ...args], {
      cwd: process.cwd()
    });
    assert.equal(response.stderr, "");
  }
  const yml = await execFileAsync(process.execPath, [executable, "--scope", mutationRoot, "entity", "yml", "--json"], {
    cwd: process.cwd()
  });
  assert.equal(JSON.parse(yml.stdout).object.value, 3);
  const validation = await execFileAsync(
    process.execPath,
    [executable, "--scope", mutationRoot, "validate", "--json"],
    { cwd: process.cwd() }
  );
  assert.deepEqual(JSON.parse(validation.stdout), {
    valid: true,
    root: `file:///${mutationRoot.replaceAll("\\", "/")}`,
    scopes: 1,
    entities: 2,
    relations: 0
  });
});

test("built CLI preserves graph command traversal semantics", async () => {
  const graphRoot = path.resolve("test/e2e/case/graph-commands");
  const run = (args) => execFileAsync(process.execPath, [executable, "--scope", graphRoot, ...args, "--json"], { cwd: process.cwd() });
  const graph = JSON.parse((await run(["graph", "a"])).stdout);
  assert.equal("depth" in graph, false);
  assert.deepEqual(graph.nodes.map(item => item.object.id), ["a", "b", "c"]);
  const depthZero = JSON.parse((await run(["graph", "a", "--depth", "0"])).stdout);
  assert.deepEqual(depthZero.nodes.map(item => item.object.id), ["a"]);
  assert.equal(JSON.parse((await run(["graph", "a", "--depth", "1"])).stdout).nodes.length, 2);
  const filteredGraph = JSON.parse((await run(["graph", "a", "--via", "type=next"])).stdout);
  assert.equal(filteredGraph.relations.length, 2);
  assert.equal(JSON.parse((await run(["adjacent", "b", "--direction", "incoming"])).stdout).relations.length, 1);
  assert.equal(JSON.parse((await run(["adjacent", "b", "--direction", "outgoing"])).stdout).relations.length, 2);
  assert.equal(JSON.parse((await run(["adjacent", "b", "--direction", "both"])).stdout).relations.length, 3);
  assert.equal(JSON.parse((await run(["path", "a", "c"])).stdout).relations.length, 2);
  assert.equal(JSON.parse((await run(["impact", "c"])).stdout).affected.length, 2);
});

test("built CLI preserves JSON errors and opaque mutation fields", async () => {
  const errorRoot = path.resolve("temp/e2e-run/cli-errors");
  await rm(errorRoot, { recursive: true, force: true });
  await mkdir(errorRoot, { recursive: true });
  await writeFile(path.join(errorRoot, "locus.yaml"), "id: cli-errors\n");
  await writeFile(path.join(errorRoot, "entities.locus.json"), '{"entities":[]}\n');
  await assert.rejects(
    execFileAsync(process.execPath, [executable, "--scope", errorRoot, "entity", "missing", "--json"], {
      cwd: process.cwd()
    }),
    error => {
      if (!error || typeof error !== "object" || !("stderr" in error)) return false;
      const body = JSON.parse(String(error.stderr));
      return body.code === "not_found" && body.reason === "scope.object_not_found" && !String(error.stderr).includes(errorRoot);
    }
  );
  await assert.rejects(
    execFileAsync(process.execPath, [executable, "--scope", errorRoot, "validate", "--unknown", "--json"], {
      cwd: process.cwd()
    }),
    error => {
      if (!error || typeof error !== "object" || !("stderr" in error)) return false;
      const body = JSON.parse(String(error.stderr));
      return body.code === "invalid_argument" && body.reason === "scope.selector_invalid";
    }
  );
  const payload = '{"__proto__":{"polluted":true},"constructor":{"prototype":{"marker":1}}}';
  await execFileAsync(
    process.execPath,
    [executable, "--scope", errorRoot, "entity", "add", "api", payload, "--file", "entities.locus.json", "--json"],
    { cwd: process.cwd() }
  );
  const persisted = JSON.parse(await readFile(path.join(errorRoot, "entities.locus.json"), "utf8"));
  assert.equal(persisted.entities[0]["__proto__"].polluted, true);
  assert.equal(persisted.entities[0].constructor.prototype.marker, 1);

  const malformedRoot = path.resolve("temp/e2e-run/malformed-manifest");
  await rm(malformedRoot, { recursive: true, force: true });
  await mkdir(malformedRoot, { recursive: true });
  await writeFile(path.join(malformedRoot, "locus.yaml"), "id: [\n");
  await assert.rejects(
    execFileAsync(process.execPath, [executable, "--scope", malformedRoot, "validate", "--json"], {
      cwd: process.cwd()
    }),
    error => {
      if (!error || typeof error !== "object" || !("stderr" in error)) return false;
      return String(error.stderr).includes("scope.manifest_invalid") && !String(error.stderr).includes(malformedRoot);
    }
  );

  const missingDependencyRoot = path.resolve("temp/e2e-run/missing-dependency");
  await rm(missingDependencyRoot, { recursive: true, force: true });
  await mkdir(missingDependencyRoot, { recursive: true });
  await writeFile(
    path.join(missingDependencyRoot, "package.json"),
    `${JSON.stringify({ name: "missing-dependency", version: "1.0.0", dependencies: { "@example/missing": "1.0.0" } })}\n`
  );
  await writeFile(
    path.join(missingDependencyRoot, "locus.yaml"),
    'id: missing-dependency\nimports:\n  missing: "@example/missing"\n'
  );
  await assert.rejects(
    execFileAsync(process.execPath, [executable, "--scope", missingDependencyRoot, "validate", "--json"], {
      cwd: process.cwd()
    }),
    error => {
      if (!error || typeof error !== "object" || !("stderr" in error)) return false;
      return String(error.stderr).includes("scope.dependency_unresolved");
    }
  );
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
