import assert from "node:assert/strict";
import { test } from "node:test";
import { ApplicationService } from "../../src/application/index.js";
import { run } from "../../src/cli/index.js";
import { Scope, Workspace } from "../../src/domain/index.js";

function application(): ApplicationService {
  const key = "file:///workspace";
  const scope = new Scope(
    { key, localPath: "temp/unit-ts/cli-parity" },
    { id: "app", imports: {}, exports: ["api", "db"] },
    "locus.yaml"
  );
  scope.entities.set("api", {
    id: "api",
    localId: "api",
    properties: { type: "service" },
    source: {
      scope: key,
      file: "scope.locus.yaml",
      line: 2,
      index: 1,
      path: "temp/unit-ts/cli-parity/scope.locus.yaml"
    }
  });
  scope.entities.set("db", {
    id: "db",
    localId: "db",
    properties: { type: "database" },
    source: {
      scope: key,
      file: "scope.locus.yaml",
      line: 4,
      index: 2,
      path: "temp/unit-ts/cli-parity/scope.locus.yaml"
    }
  });
  const workspace = new Workspace(key, new Map([[key, scope]]));
  workspace.relations = [
    {
      from: { scope: key, id: "api" },
      to: { scope: key, id: "db" },
      type: "depends_on",
      fromRef: "api",
      toRef: "db",
      properties: { critical: true },
      source: {
        scope: key,
        file: "scope.locus.yaml",
        line: 7,
        index: 1,
        path: "temp/unit-ts/cli-parity/scope.locus.yaml"
      }
    }
  ];
  return new ApplicationService(workspace);
}

function context(app = application()) {
  return {
    application: app,
    reloadWorkspace: async () => app.workspace,
    loadPath: async () => app.workspace,
    version: "4.0.0"
  };
}

test("3.1 CLI entity, graph, path, validate and relation JSON contracts", async () => {
  const entity = await run(["entity", "type=service", "--source", "--json"], context());
  assert.equal(entity.exitCode, 0);
  assert.deepEqual(JSON.parse(entity.stdout), [
    {
      key: { scope: "file:///workspace", id: "api" },
      ref: "api",
      object: { type: "service", id: "api" },
      source: { scope: "file:///workspace", scopeId: "app", file: "scope.locus.yaml", line: 2, index: 1 }
    }
  ]);
  const graph = await run(["graph", "api", "--depth", "1", "--via", "type=depends_on", "--json"], context());
  assert.equal(JSON.parse(graph.stdout).nodes.length, 2);
  const path = await run(["path", "api", "db", "--json"], context());
  assert.equal(JSON.parse(path.stdout).relations[0].object.type, "depends_on");
  const validate = await run(["validate", "--json"], context());
  assert.deepEqual(JSON.parse(validate.stdout), {
    valid: true,
    root: "file:///workspace",
    scopes: 1,
    entities: 2,
    relations: 1
  });
  const relation = await run(["relation", "type=depends_on", "--json"], context());
  assert.equal(JSON.parse(relation.stdout)[0].object.type, "depends_on");
});

test("3.1 CLI invalid arguments, operational errors, unknown command and version contracts", async () => {
  const invalid = await run(["graph", "api", "--depth", "bad", "--json"], context());
  assert.equal(invalid.exitCode, 2);
  assert.match(invalid.stderr, /graph depth/);
  const operational = await run(["path", "db", "api", "--json"], context());
  assert.equal(operational.exitCode, 1);
  assert.match(operational.stderr, /no directed path/);
  const unknown = await run(["--json", "definitely-not-a-command"], context());
  assert.equal(unknown.exitCode, 2);
  assert.match(unknown.stderr, /unknown command/);
  const version = await run(["--json", "--version"], context());
  assert.deepEqual(JSON.parse(version.stdout), { name: "locus-scope", version: "4.0.0" });
});

test("3.1 rootless help and version do not load application", async () => {
  let loaded = false;
  const contextValue = {
    version: "4.0.0",
    loadApplication: async () => {
      loaded = true;
      return application();
    }
  };
  const help = await run(["--help"], contextValue);
  const version = await run(["version"], contextValue);
  assert.equal(help.exitCode, 0);
  assert.equal(version.exitCode, 0);
  assert.equal(loaded, false);
});

test("diff rejects malformed selectors as argument errors on either side", async () => {
  for (const selector of ["invalid", ":app", "scope:", "group:app", "group:#team", "group:app#", "unknown:app"]) {
    for (const selectors of [
      [selector, "scope:."],
      ["scope:.", selector]
    ]) {
      const response = await run(["diff", ...selectors, "--json"], context());
      assert.equal(response.exitCode, 2, selector);
      assert.equal(response.stdout, "");
      assert.match(JSON.parse(response.stderr).error, /diff selector/, selector);
    }
  }
});
