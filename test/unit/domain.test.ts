import assert from "node:assert/strict";
import { cp, mkdir, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { test } from "node:test";
import {
  FindScope,
  Load,
  LoadDefinitionFile,
  LocalResolver,
  Match,
  NewLocalSource,
  ParseFilters,
  type EntityKey,
  type Resolver,
  type Source,
  type Workspace
} from "@sundw/locus-scope-core";
import { ApplicationError } from "@sundw/locus-scope-core";

const fixtureRoot = path.resolve("test/e2e/case");
const stateRoot = path.resolve("temp/e2e-run/domain");

async function materializeCase(name: string, ...rootParts: string[]): Promise<string> {
  const source = path.join(fixtureRoot, name);
  const destination = path.join(stateRoot, name);
  await rm(destination, { recursive: true, force: true });
  await mkdir(path.dirname(destination), { recursive: true });
  await cp(source, destination, { recursive: true });
  return path.join(destination, ...rootParts);
}

async function loadCase(name: string, ...rootParts: string[]) {
  const directory = await materializeCase(name, ...rootParts);
  const source = await NewLocalSource(directory);
  return { directory, source, workspace: await Load(source, new LocalResolver()) };
}

function relation(workspace: Workspace, from: EntityKey, type: string, to: EntityKey) {
  return workspace.relations.find(
    candidate =>
      candidate.from.scope === from.scope &&
      candidate.from.id === from.id &&
      candidate.type === type &&
      candidate.to.scope === to.scope &&
      candidate.to.id === to.id
  );
}

test("TestLoadScopeComposition: composition keeps opaque entity and relation properties", async () => {
  const { source, workspace } = await loadCase("scope-composition", "app");
  assert.equal(workspace.scopes.size, 2);
  assert.match(source.key, /^file:\/\//u);
  assert.equal(workspace.relations.length, 4);

  const root = workspace.scopes.get(workspace.root);
  assert.ok(root);
  const infraKey = root.imports.get("infra");
  assert.ok(infraKey);

  const database = workspace.resolve(workspace.root, "infra:database");
  assert.deepEqual(database, { scope: infraKey, id: "database" });
  const worker = workspace.resolve(workspace.root, "infra:backend/worker");
  assert.deepEqual(worker, { scope: infraKey, id: "backend/worker" });

  assert.equal(workspace.scopes.get(database.scope)?.entities.get(database.id)?.properties.engine, "postgres");
  const monitor = workspace.scopes.get(infraKey)?.entities.get("backend/monitor");
  const nested = (monitor?.properties.anything as { nested?: { values?: unknown } } | undefined)?.nested;
  assert.deepEqual(nested?.values, ["are", "allowed"]);

  const workerRelation = relation(workspace, worker, "reports_to", { scope: infraKey, id: "backend/monitor" });
  assert.ok(workerRelation);
  const api = { scope: workspace.root, id: "api" };
  const apiRelation = relation(workspace, api, "uses", database);
  assert.ok(apiRelation);
  assert.equal(apiRelation.properties.critical, true);
  assert.deepEqual(apiRelation.properties.policy, { retries: 3 });
});

test("TestDefinitionDiscoveryRecursesHonorsIgnoreAndStopsAtNestedScope", async () => {
  const directory = await materializeCase("discovery");
  await mkdir(path.join(directory, ".git"), { recursive: true });
  await writeFile(path.join(directory, ".git", "ignored.locus.yaml"), "invalid: git state\n");
  await mkdir(path.join(directory, ".locus"), { recursive: true });
  await writeFile(path.join(directory, ".locus", "generated.locus.yaml"), "invalid: generated state\n");
  const source = await NewLocalSource(directory);
  const workspace = await Load(source, new LocalResolver());
  const root = workspace.scopes.get(workspace.root);
  assert.ok(root);
  assert.deepEqual([...root.entities.keys()].sort(), ["child", "root"]);
});

test("TestGroupedRelationKeepsExternalRootReference", async () => {
  const { workspace } = await loadCase("group-fallback");
  const worker = { scope: workspace.root, id: "backend/worker" };
  const database = { scope: workspace.root, id: "database" };
  assert.ok(relation(workspace, worker, "uses", database));
});

test("TestMultilevelReexportKeepsOriginalOwnership", async () => {
  const { workspace } = await loadCase("reexport", "product");
  const resolved = workspace.resolve(workspace.root, "app:infra:database");
  const owner = workspace.scopes.get(resolved.scope);
  assert.ok(owner);
  assert.equal(owner.manifest.id, "infra");
  assert.equal(resolved.id, "database");
  assert.notEqual(resolved.scope, workspace.root);
  assert.throws(
    () => workspace.resolve(workspace.root, "app:infra:secret"),
    error => error instanceof ApplicationError && error.reason === "scope.object_not_found"
  );
});

test("TestCyclicImportsLoadAndResolve", async () => {
  const { workspace } = await loadCase("cycle", "a");
  assert.equal(workspace.scopes.size, 2);
  const beta = workspace.resolve(workspace.root, "b:beta");
  assert.equal(workspace.scopes.get(beta.scope)?.manifest.id, "b");
  assert.ok(relation(workspace, { scope: workspace.root, id: "alpha" }, "reaches", beta));
});

test("TestDistinctSourcesMayShareManifestID", async () => {
  const { workspace } = await loadCase("same-manifest-id", "root");
  const east = workspace.resolve(workspace.root, "east:item");
  const west = workspace.resolve(workspace.root, "west:item");
  assert.notEqual(east.scope, west.scope);
  assert.equal(workspace.scopes.get(east.scope)?.manifest.id, "shared");
  assert.equal(workspace.scopes.get(west.scope)?.manifest.id, "shared");
  assert.equal(workspace.scopes.get(east.scope)?.entities.get(east.id)?.properties.side, "east");
  assert.equal(workspace.scopes.get(west.scope)?.entities.get(west.id)?.properties.side, "west");
});

class FixtureResolver implements Resolver {
  constructor(private readonly mappings: Map<string, Map<string, Source>>) {}

  resolve(from: Source, reference: string): Source {
    const source = this.mappings.get(from.key)?.get(reference);
    if (!source) throw new Error(`missing fixture resolution for ${from.key}:${reference}`);
    return source;
  }
}

test("TestResolverSourceKeyDefinesIdentityAcrossMaterializations", async () => {
  const packageKey = "npm:@example/package@1.0.0";
  const resolved: EntityKey[] = [];
  for (const name of ["first", "second"]) {
    const base = path.join(stateRoot, "source-key", name);
    const rootDirectory = path.join(base, "root");
    const packageDirectory = path.join(base, "materialized-package");
    await rm(base, { recursive: true, force: true });
    await mkdir(rootDirectory, { recursive: true });
    await mkdir(packageDirectory, { recursive: true });
    await writeFile(
      path.join(rootDirectory, "locus.yaml"),
      "id: root\nimports:\n  pkg: package\nexports:\n  - pkg:item\n"
    );
    await writeFile(
      path.join(packageDirectory, "locus.yaml"),
      "id: package\nimports:\n  root: root\nexports:\n  - item\n"
    );
    await writeFile(path.join(packageDirectory, "entities.locus.yaml"), "entities:\n  - id: item\n");

    const rootSource = await NewLocalSource(rootDirectory);
    const packageSource: Source = { key: packageKey, localPath: packageDirectory };
    const resolver = new FixtureResolver(
      new Map([
        [rootSource.key, new Map([["package", packageSource]])],
        [packageKey, new Map([["root", rootSource]])]
      ])
    );
    const workspace = await Load(rootSource, resolver);
    resolved.push(workspace.resolve(workspace.root, "pkg:item"));
  }
  assert.deepEqual(resolved[0], resolved[1]);
  assert.deepEqual(resolved[0], { scope: packageKey, id: "item" });
});

test("TestValidationDiagnostics: diagnostics retain structured reasons and safe details", async () => {
  const cases: Array<{ name: string; reason: ApplicationError["reason"] }> = [
    { name: "duplicate-expanded", reason: "scope.entity_duplicate" },
    { name: "missing-relation", reason: "scope.relation_end_unresolved" },
    { name: "validation/missing-manifest", reason: "scope.manifest_missing" },
    { name: "validation/duplicate-manifest", reason: "scope.manifest_ambiguous" },
    { name: "validation/missing-scope-id", reason: "scope.manifest_invalid" },
    { name: "validation/missing-entity-id", reason: "scope.entity_id_invalid" },
    { name: "validation/missing-import", reason: "scope.source_not_found" },
    { name: "validation/missing-projection", reason: "scope.export_unresolved" },
    { name: "validation/malformed-relation", reason: "scope.relation_invalid" },
    { name: "validation/relation-tuple", reason: "scope.relation_invalid" },
    { name: "validation/invalid-locusignore", reason: "scope.definition_invalid" }
  ];

  for (const item of cases) {
    const directory = await materializeCase(item.name);
    const error = await (async () => {
      try {
        const source = await NewLocalSource(directory);
        await Load(source, new LocalResolver());
        return undefined;
      } catch (caught) {
        return caught;
      }
    })();
    assert.ok(error instanceof ApplicationError, `${item.name} unexpectedly loaded`);
    assert.equal(error.reason, item.reason, item.name);
    assert.equal(
      Object.values(error.details ?? {}).some(value => value.includes("file://") || /[A-Za-z]:[\\/]/u.test(value)),
      false,
      item.name
    );
  }
});

test("TestFindScopeWalksToNearestAncestor", async () => {
  const directory = await materializeCase("cycle", "a");
  const nested = path.join(directory, "nested", "deeper");
  await mkdir(nested, { recursive: true });
  const found = await FindScope(nested);
  const source = await NewLocalSource(found);
  const workspace = await Load(source, new LocalResolver());
  assert.equal(workspace.scopes.get(workspace.root)?.manifest.id, "a");
});

test("TestFilterStrictNestedAndMetadataSemantics", () => {
  const object = { id: "api", network: { region: "tokyo" }, replicas: 3, labels: ["prod"] };
  const metadata = { scope: ".", group: "backend" };
  for (const filters of [
    ["network.region=tokyo"],
    ["id~=^a.*"],
    ["id^=ap"],
    ["id*=pi"],
    ["@scope=.", "@group=backend"],
    ["replicas=3"]
  ]) {
    assert.equal(Match(object, metadata, ParseFilters(filters)), true, filters.join(","));
  }
  for (const filters of [["missing!=value"], ["replicas*=three"], ["labels*=prod"], ["@scope=dependency"]]) {
    assert.equal(Match(object, metadata, ParseFilters(filters)), false, filters.join(","));
  }
  assert.throws(
    () => ParseFilters(["id~=["]),
    error => error instanceof ApplicationError && error.reason === "scope.filter_invalid"
  );
});

test("synthetic definitions preserve declaration line provenance", async () => {
  const filename = path.join(stateRoot, "synthetic", "entities.locus.yaml");
  await rm(path.dirname(filename), { recursive: true, force: true });
  await mkdir(path.dirname(filename), { recursive: true });
  await writeFile(filename, "entities:\n  - id: api\nrelations:\n  - from: api\n    type: calls\n    to: api\n");
  const workspace = await LoadDefinitionFile(filename);
  const entity = [...workspace.scopes.values()][0]?.entities.get("api");
  assert.equal(entity?.source.line, 2);
  assert.equal([...workspace.relations][0]?.source.line, 4);
});

test("synthetic definitions reject strict fields and multiple documents", async () => {
  const filename = path.join(stateRoot, "synthetic-invalid", "entities.locus.yaml");
  await rm(path.dirname(filename), { recursive: true, force: true });
  await mkdir(path.dirname(filename), { recursive: true });
  await writeFile(filename, "unknown: true\nentities:\n  - id: api\n");
  await assert.rejects(
    () => LoadDefinitionFile(filename),
    error => error instanceof ApplicationError && error.reason === "scope.definition_invalid"
  );
  await writeFile(filename, "entities:\n  - id: api\n---\nentities:\n  - id: other\n");
  await assert.rejects(
    () => LoadDefinitionFile(filename),
    error => error instanceof ApplicationError && error.reason === "scope.definition_invalid"
  );
});
