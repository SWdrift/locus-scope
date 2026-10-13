import assert from "node:assert/strict";
import { test } from "node:test";
import { ApplicationService } from "../../src/application/index.js";
import { Scope, Workspace, parseFilters, type Relation } from "../../src/domain/index.js";

function workspace(): Workspace {
  const scope = new Scope(
    { key: "file:///root", localPath: "temp/e2e-run/service-parity" },
    { id: "app", imports: {}, exports: ["api", "db"] },
    "locus.yaml"
  );
  for (const [id, type] of [
    ["api", "service"],
    ["db", "database"]
  ]) {
    assert.ok(id && type);
    scope.entities.set(id, {
      id,
      localId: id,
      properties: { type, ...(id === "api" ? { network: { region: "tokyo" } } : {}) },
      source: {
        scope: scope.key,
        file: id === "api" ? "services.locus.yaml" : "resources.locus.yaml",
        path: `temp/e2e-run/service-parity/${id}.locus.yaml`,
        line: id === "api" ? 3 : 0,
        index: 1
      }
    });
  }
  const result = new Workspace(scope.key, new Map([[scope.key, scope]]));
  result.relations = [
    {
      from: { scope: scope.key, id: "api" },
      to: { scope: scope.key, id: "db" },
      fromRef: "api",
      toRef: "db",
      type: "depends_on",
      properties: { critical: true },
      source: {
        scope: scope.key,
        path: "temp/e2e-run/service-parity/services.locus.yaml",
        file: "services.locus.yaml",
        index: 1
      }
    }
  ];
  return result;
}

test("3.1 TestUnifiedQueriesAndProvenance retains envelopes, ownership and open properties", () => {
  const app = new ApplicationService(workspace());
  const entities = app.queryEntities(parseFilters(["network.region=tokyo", "@scope=."]), true);
  assert.deepEqual(entities, [
    {
      key: { scope: "file:///root", id: "api" },
      ref: "api",
      object: { id: "api", type: "service", network: { region: "tokyo" } },
      source: { scope: "file:///root", scopeId: "app", file: "services.locus.yaml", line: 3, index: 1 }
    }
  ]);
  assert.deepEqual(app.queryRelations([], true)[0], {
    fromKey: { scope: "file:///root", id: "api" },
    toKey: { scope: "file:///root", id: "db" },
    object: { critical: true, from: "api", type: "depends_on", to: "db" },
    source: { scope: "file:///root", scopeId: "app", file: "services.locus.yaml", index: 1 }
  });
  assert.deepEqual(app.getEntity("db").key, { scope: "file:///root", id: "db" });
});

test("3.1 TestGraphPathAndImpact retains deterministic traversal and filters", () => {
  const app = new ApplicationService(workspace());
  const api = app.getEntity("api").key;
  const db = app.getEntity("db").key;
  const via = parseFilters(["type=depends_on"]);
  const graph = app.graph([api, api], 1, via);
  assert.deepEqual(
    graph.seeds.map(item => item.key),
    [api]
  );
  assert.deepEqual(
    graph.nodes.map(item => item.key),
    [api, db]
  );
  assert.deepEqual(graph.relations, app.queryRelations());
  assert.deepEqual(
    app.graph([api, api], 0).nodes.map(item => item.key),
    [api]
  );
  assert.deepEqual(app.graph([api], 0).relations, []);
  const path = app.path(api, db, via);
  assert.deepEqual(
    path.nodes.map(item => item.key),
    [api, db]
  );
  assert.deepEqual(path.relations, app.queryRelations());
  assert.deepEqual(
    app.path(api, api).nodes.map(item => item.key),
    [api]
  );
  const impact = app.impact([db, db], via);
  assert.deepEqual(
    impact.seeds.map(item => item.key),
    [db]
  );
  assert.deepEqual(
    impact.affected.map(item => item.key),
    [api]
  );
});

test("3.1 relation order breaks equal-length path and parallel-edge ties", () => {
  const w = workspace();
  const root = w.scopes.get(w.root)!;
  for (const id of ["mid", "target"])
    root.entities.set(id, {
      id,
      localId: id,
      properties: {},
      source: { scope: w.root, file: "graph.locus.yaml", path: "temp/e2e-run/service-parity/graph.locus.yaml" }
    });
  function edge(from: string, type: string, to: string): Relation {
    return {
      from: { scope: w.root, id: from },
      to: { scope: w.root, id: to },
      type,
      fromRef: from,
      toRef: to,
      properties: {},
      source: { scope: w.root, file: "graph.locus.yaml", path: "temp/e2e-run/service-parity/graph.locus.yaml" }
    };
  }
  w.relations = [
    edge("api", "a", "db"),
    edge("api", "b", "db"),
    edge("api", "c", "mid"),
    edge("db", "next", "target"),
    edge("mid", "next", "target")
  ];
  const app = new ApplicationService(w);
  assert.deepEqual(
    app.path({ scope: w.root, id: "api" }, { scope: w.root, id: "target" }).relations.map(item => item.object.type),
    ["a", "next"]
  );
});

test("scope query retains declared import targets rather than resolved owner identities", () => {
  const w = workspace();
  const root = w.scopes.get(w.root)!;
  root.manifest.imports.infra = "../infra";
  root.imports.set("infra", w.root);
  assert.deepEqual(new ApplicationService(w).getScope(".").object.imports, { infra: "../infra" });
});
