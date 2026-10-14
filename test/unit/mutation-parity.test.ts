import assert from "node:assert/strict";
import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { test, type TestContext } from "node:test";
import { ApplicationService } from "@sundw/locus-scope-core";
import { ApplicationError } from "@sundw/locus-scope-core";
import { load, LocalResolver, newLocalSource, type Workspace } from "@sundw/locus-scope-core";
import type { MutationRequest, MutationResult } from "@sundw/locus-scope-core";
import { parseFilters } from "@sundw/locus-scope-core";
import { run } from "@sundw/locus-scope-cli";

interface MutationResponse {
  readonly result: MutationResult;
  readonly workspace: Workspace;
}
interface EntityEnvelope {
  readonly object: { readonly id: string };
  readonly source: { readonly group?: string; readonly file: string; readonly index?: number };
}
interface JsonDefinition {
  readonly entities: readonly { readonly id?: string; readonly value?: number }[];
}

async function loadAt(root: string) {
  return load(await newLocalSource(root), new LocalResolver());
}

async function setup(context: TestContext, name: string, files: Record<string, string>): Promise<string> {
  const root = path.resolve("temp/unit-ts", `mutation-parity-${name}`);
  await rm(root, { recursive: true, force: true });
  for (const [file, value] of Object.entries(files)) {
    const filename = path.join(root, file);
    await mkdir(path.dirname(filename), { recursive: true });
    await writeFile(filename, value);
  }
  context.after(async () => rm(root, { recursive: true, force: true }));
  return root;
}

async function mutate(root: string, request: MutationRequest, reloadRoot = root): Promise<MutationResponse> {
  const application = new ApplicationService(await loadAt(root));
  return application.mutate(request, () => loadAt(reloadRoot));
}

function typedError(code: ApplicationError["code"], reason: string): (error: unknown) => boolean {
  return error => error instanceof ApplicationError && error.code === code && error.reason === reason;
}

test("mutation covers entity/relation add set unset remove and dangling rollback", async t => {
  const root = await setup(t, "lifecycle", {
    "locus.yaml": "id: app\nexports:\n  - api\n",
    "entities.locus.yaml": "entities:\n  - id: api\n    type: service\n    network:\n      region: tokyo\n",
    "edges.locus.yml": "relations:\n  - from: api\n    type: depends_on\n    to: api\n    critical: false\n"
  });
  const added = await mutate(root, {
    kind: "entity",
    action: "add",
    file: "entities.locus.yaml",
    object: { id: "db", type: "database", network: { region: "london" } },
    fields: {}
  });
  const addedEntity = added.result.object as EntityEnvelope;
  assert.equal(addedEntity.object.id, "db");
  await mutate(root, {
    kind: "entity",
    action: "set",
    ref: "api",
    object: { network: { zone: "a", region: "osaka" }, nullable: null },
    fields: { "metadata.owner": "platform" }
  });
  const loadedAfterSet = await loadAt(root);
  const setEntity = loadedAfterSet.scopes.get(loadedAfterSet.root)?.entities.get("api");
  const network = setEntity?.properties.network;
  const region = network && typeof network === "object" && !Array.isArray(network) ? network : undefined;
  const zone = region && "zone" in region ? region.zone : undefined;
  assert.equal(zone, "a");
  await mutate(root, { kind: "entity", action: "unset", ref: "api", unset: ["metadata.owner"] });
  await mutate(root, {
    kind: "relation",
    action: "add",
    file: "edges.locus.yml",
    object: { from: "api", type: "uses", to: "db", policy: { retries: 3 } },
    fields: {}
  });
  await mutate(root, {
    kind: "relation",
    action: "set",
    from: "api",
    type: "uses",
    to: "db",
    object: { policy: { timeout: 10 } },
    fields: {}
  });
  const beforeDatabaseRemoval = await readFile(path.join(root, "entities.locus.yaml"));
  await assert.rejects(
    mutate(root, { kind: "entity", action: "remove", ref: "db" }),
    typedError("invalid_data", "scope.mutation_invalid")
  );
  assert.deepEqual(await readFile(path.join(root, "entities.locus.yaml")), beforeDatabaseRemoval);
  await mutate(root, { kind: "relation", action: "remove", from: "api", type: "uses", to: "db" });
  await mutate(root, { kind: "entity", action: "remove", ref: "db" });
  const final = await loadAt(root);
  assert.equal(final.scopes.get(final.root)?.entities.has("db"), false);
});

test("mutation preserves YAML, YML, and JSON definition codecs", async t => {
  const root = await setup(t, "codecs", {
    "locus.yaml": "id: app\n",
    "yaml.locus.yaml": "entities:\n  - id: yaml\n    value: 1\n",
    "yml.locus.yml": "entities:\n  - id: yml\n    value: 2\n",
    "json.locus.json": '{\n  "entities": [\n    { "id": "json", "value": 3 }\n  ]\n}\n'
  });
  await mutate(root, { kind: "entity", action: "set", ref: "yaml", object: {}, fields: { value: 11 } });
  await mutate(root, { kind: "entity", action: "set", ref: "yml", object: {}, fields: { value: 22 } });
  await mutate(root, { kind: "entity", action: "set", ref: "json", object: {}, fields: { value: 33 } });
  await mutate(root, {
    kind: "entity",
    action: "add",
    file: "new.locus.json",
    object: { id: "new", value: 4 },
    fields: {}
  });
  const yamlText = await readFile(path.join(root, "yaml.locus.yaml"), "utf8");
  const ymlText = await readFile(path.join(root, "yml.locus.yml"), "utf8");
  const jsonText = await readFile(path.join(root, "json.locus.json"), "utf8");
  assert.match(yamlText, /entities:/);
  assert.match(ymlText, /entities:/);
  assert.doesNotMatch(ymlText, /^\{/u);
  const jsonDefinition = JSON.parse(jsonText) as JsonDefinition;
  assert.equal(jsonDefinition.entities[0]?.value, 33);
  const newDefinitionText = await readFile(path.join(root, "new.locus.json"), "utf8");
  const newDefinition = JSON.parse(newDefinitionText) as JsonDefinition;
  assert.equal(newDefinition.entities[0]?.id, "new");
});

test("mutation uses grouped local IDs and preserves provenance in results", async t => {
  const root = await setup(t, "grouped", {
    "locus.yaml": "id: app\n",
    "team.locus.yaml": "group: team\nentities:\n  - id: api\n    type: service\n"
  });
  const added = await mutate(root, {
    kind: "entity",
    action: "add",
    file: "team.locus.yaml",
    object: { id: "team/db", type: "database" },
    fields: {}
  });
  const addedEntity = added.result.object as EntityEnvelope;
  assert.equal(addedEntity.object.id, "team/db");
  assert.equal(addedEntity.source.group, "team");
  assert.equal(addedEntity.source.file, "team.locus.yaml");
  assert.equal(addedEntity.source.index, 2);
  const changed = await mutate(root, {
    kind: "entity",
    action: "set",
    ref: "team/api",
    object: {},
    fields: { "metadata.owner": "platform" }
  });
  const changedEntity = changed.result.object as EntityEnvelope;
  assert.equal(changedEntity.source.group, "team");
});

test("mutation recursively merges values, preserves null, and rejects invalid point paths", async t => {
  const root = await setup(t, "paths", {
    "locus.yaml": "id: app\n",
    "entities.locus.yaml": "entities:\n  - id: api\n    nested:\n      keep: true\n      child:\n        old: 1\n"
  });
  await mutate(root, {
    kind: "entity",
    action: "set",
    ref: "api",
    object: { nested: { child: { next: 2 }, nullable: null } },
    fields: { "nested.deep.value": 3 }
  });
  const entity = (await loadAt(root)).scopes.values().next().value?.entities.get("api");
  assert.deepEqual(entity?.properties.nested, {
    keep: true,
    child: { old: 1, next: 2 },
    nullable: null,
    deep: { value: 3 }
  });
  const original = await readFile(path.join(root, "entities.locus.yaml"));
  await assert.rejects(
    mutate(root, { kind: "entity", action: "set", ref: "api", object: {}, fields: { "nested.nullable.value": 4 } }),
    typedError("invalid_argument", "scope.field_invalid")
  );
  await assert.rejects(
    mutate(root, { kind: "entity", action: "unset", ref: "api", unset: ["missing"] }),
    typedError("invalid_argument", "scope.field_invalid")
  );
  assert.deepEqual(await readFile(path.join(root, "entities.locus.yaml")), original);
});

test("mutation rolls back export validation and removes newly-created files on reload failure", async t => {
  const root = await setup(t, "rollback", {
    "locus.yaml": "id: app\nexports:\n  - api\n",
    "entities.locus.yaml": "entities:\n  - id: api\n"
  });
  const original = await readFile(path.join(root, "entities.locus.yaml"));
  await assert.rejects(
    mutate(root, { kind: "entity", action: "remove", ref: "api" }),
    typedError("invalid_data", "scope.mutation_invalid")
  );
  assert.deepEqual(await readFile(path.join(root, "entities.locus.yaml")), original);
  const application = new ApplicationService(await loadAt(root));
  const generated = path.join(root, "generated.locus.json");
  let reloads = 0;
  await assert.rejects(
    application.mutate(
      { kind: "entity", action: "add", file: "generated.locus.json", object: { id: "new" }, fields: {} },
      async () => {
        reloads += 1;
        if (reloads === 1) throw new Error("synthetic reload failure");
        return loadAt(root);
      }
    ),
    typedError("internal", "scope.mutation_write_failed")
  );
  await assert.rejects(readFile(generated));
});

test("mutation reports rollback cleanup and reload failures", async t => {
  const root = await setup(t, "rollback-failure", {
    "locus.yaml": "id: app\n",
    "entities.locus.yaml": "entities:\n  - id: api\n"
  });
  const file = path.join(root, "entities.locus.yaml");
  const application = new ApplicationService(await loadAt(root));
  let reloads = 0;
  await assert.rejects(
    application.mutate({ kind: "entity", action: "set", ref: "api", object: {}, fields: { value: 1 } }, async () => {
      reloads += 1;
      if (reloads === 1) {
        await rm(file);
        await mkdir(file);
        throw new Error("synthetic reload failure");
      }
      throw new Error("synthetic rollback reload failure");
    }),
    error => {
      assert.ok(error instanceof ApplicationError);
      assert.equal(error.code, "internal");
      assert.equal(error.reason, "scope.mutation_write_failed");
      assert.ok(error.cause instanceof AggregateError);
      assert.equal(error.cause.errors.length, 3);
      return true;
    }
  );
});

test("mutation refuses dependency writes and nested or escaping targets", async t => {
  const compositionRoot = path.resolve("test/e2e/case/scope-composition/app");
  const dependencyApp = new ApplicationService(await loadAt(compositionRoot));
  await assert.rejects(
    dependencyApp.mutate(
      { kind: "entity", action: "set", ref: "infra:database", object: {}, fields: { engine: "sqlite" } },
      () => loadAt(compositionRoot)
    ),
    typedError("permission_denied", "scope.object_read_only")
  );
  const root = await setup(t, "containment", {
    "locus.yaml": "id: app\n",
    "nested/locus.yaml": "id: nested\n"
  });
  await assert.rejects(
    mutate(root, { kind: "entity", action: "add", file: "nested/new.locus.yaml", object: { id: "new" }, fields: {} }),
    typedError("invalid_argument", "scope.mutation_input_invalid")
  );
  await assert.rejects(
    mutate(root, { kind: "entity", action: "add", file: "../escape.locus.yaml", object: { id: "new" }, fields: {} }),
    typedError("invalid_argument", "scope.mutation_input_invalid")
  );
});

test("mutation treats prototype names as ordinary data in JSON and YAML", async t => {
  for (const extension of ["json", "yaml"]) {
    await t.test(extension, async t => {
      const file = `entities.locus.${extension}`;
      const root = await setup(t, `own-fields-${extension}`, { "locus.yaml": "id: app\n" });
      const prototype = Object.getOwnPropertyDescriptors(Object.prototype);
      const payload =
        '{"__proto__":{"owner":"before"},"constructor":{"prototype":{"value":1}},"toString":"label","hasOwnProperty":true,"items":[{"__proto__":{"value":2}}]}';
      const response = await run(["entity", "add", "api", payload, "--file", file, "--json"], {
        application: new ApplicationService(await loadAt(root)),
        reloadWorkspace: () => loadAt(root)
      });
      assert.equal(response.exitCode, 0, response.stderr);
      await mutate(root, {
        kind: "relation",
        action: "add",
        file,
        object: { from: "api", type: "uses", to: "api", ...JSON.parse(payload) }
      });
      const before = await loadAt(root);
      const expected: Record<string, unknown> = JSON.parse(payload);
      const initial = new ApplicationService(before);
      assert.deepEqual(initial.getEntity("api").object, { ...expected, id: "api" });
      assert.deepEqual(initial.queryRelations()[0]?.object, { ...expected, from: "api", type: "uses", to: "api" });
      assert.equal(initial.queryEntities(parseFilters(["__proto__.owner=before"]))[0]?.key.id, "api");
      assert.equal(Object.getPrototypeOf(initial.getEntity("api").object), Object.prototype);
      for (const kind of ["entity", "relation"] as const) {
        await mutate(root, {
          kind,
          action: "set",
          ref: "api",
          from: "api",
          type: "uses",
          to: "api",
          object: JSON.parse('{"__proto__":{"next":3},"constructor":{"prototype":{"next":4}}}'),
          fields: { "__proto__.owner": "after", toString: "updated", hasOwnProperty: false }
        });
      }
      const after = await loadAt(root);
      const service = new ApplicationService(after);
      const updated = {
        ...expected,
        ["__proto__"]: { owner: "after", next: 3 },
        constructor: { prototype: { value: 1, next: 4 } },
        toString: "updated",
        hasOwnProperty: false
      };
      assert.deepEqual(service.getEntity("api").object, { ...updated, id: "api" });
      assert.deepEqual(service.queryRelations()[0]?.object, { ...updated, from: "api", type: "uses", to: "api" });
      const diff = await service.diff("path:before", "path:after", async value =>
        value === "before" ? before : after
      );
      assert.equal(diff.changed.entities[0]?.after.key.id, "api");
      assert.equal(diff.changed.relations[0]?.after.object.type, "uses");
      for (const kind of ["entity", "relation"] as const) {
        await mutate(root, {
          kind,
          action: "unset",
          ref: "api",
          from: "api",
          type: "uses",
          to: "api",
          unset: ["__proto__.next", "constructor", "toString", "hasOwnProperty"]
        });
      }
      const final = new ApplicationService(await loadAt(root));
      const remaining = { ["__proto__"]: { owner: "after" }, items: expected.items };
      assert.deepEqual(final.getEntity("api").object, { ...remaining, id: "api" });
      assert.deepEqual(final.queryRelations()[0]?.object, { ...remaining, from: "api", type: "uses", to: "api" });
      const assigned = await run(
        [
          "entity",
          "set",
          "api",
          '__proto__={"owner":"cli"}',
          'constructor={"prototype":{"value":5}}',
          "toString=cli",
          "--json"
        ],
        { application: final, reloadWorkspace: () => loadAt(root) }
      );
      assert.equal(assigned.exitCode, 0, assigned.stderr);
      const assignedService = new ApplicationService(await loadAt(root));
      assert.deepEqual(assignedService.getEntity("api").object, {
        ...remaining,
        id: "api",
        ["__proto__"]: { owner: "cli" },
        constructor: { prototype: { value: 5 } },
        toString: "cli"
      });
      await mutate(root, { kind: "entity", action: "unset", ref: "api", unset: ["__proto__"] });
      assert.equal(
        Object.hasOwn(new ApplicationService(await loadAt(root)).getEntity("api").object, "__proto__"),
        false
      );
      assert.deepEqual(Object.getOwnPropertyDescriptors(Object.prototype), prototype);
    });
  }
});

test("field paths cannot mutate inherited objects, including on rollback", async t => {
  const root = await setup(t, "prototype-paths", {
    "locus.yaml": "id: app\n",
    "entities.locus.json": '{"entities":[{"id":"api"},{"id":"merge"}]}'
  });
  const prototype = Object.getOwnPropertyDescriptors(Object.prototype);
  t.after(() => Reflect.deleteProperty(Object.prototype, "locusScopePollution"));
  await assert.rejects(
    mutate(root, { kind: "entity", action: "unset", ref: "api", unset: ["__proto__.locusScopePollution"] }),
    typedError("invalid_argument", "scope.field_invalid")
  );
  await mutate(root, {
    kind: "entity",
    action: "set",
    ref: "merge",
    object: JSON.parse('{"__proto__":{"locusScopePollution":"merged"}}')
  });
  assert.deepEqual(Object.getOwnPropertyDescriptors(Object.prototype), prototype);
  assert.deepEqual(new ApplicationService(await loadAt(root)).getEntity("merge").object, {
    id: "merge",
    ["__proto__"]: { locusScopePollution: "merged" }
  });
  await mutate(root, {
    kind: "entity",
    action: "set",
    ref: "api",
    fields: { "__proto__.locusScopePollution": "local", "constructor.prototype.value": 1 }
  });
  assert.deepEqual(Object.getOwnPropertyDescriptors(Object.prototype), prototype);
  assert.deepEqual(new ApplicationService(await loadAt(root)).getEntity("api").object, {
    id: "api",
    ["__proto__"]: { locusScopePollution: "local" },
    constructor: { prototype: { value: 1 } }
  });
  await assert.rejects(
    mutate(root, { kind: "entity", action: "unset", ref: "api", unset: ["__proto__.toString"] }),
    typedError("invalid_argument", "scope.field_invalid")
  );
  const file = path.join(root, "entities.locus.json");
  const original = await readFile(file);
  const application = new ApplicationService(await loadAt(root));
  let reloads = 0;
  await assert.rejects(
    application.mutate(
      { kind: "entity", action: "set", ref: "api", fields: { "__proto__.locusScopePollution": "rollback" } },
      async () => {
        if (++reloads === 1) throw new Error("reload failed");
        return loadAt(root);
      }
    ),
    typedError("internal", "scope.mutation_write_failed")
  );
  assert.deepEqual(await readFile(file), original);
  assert.deepEqual(Object.getOwnPropertyDescriptors(Object.prototype), prototype);
});
