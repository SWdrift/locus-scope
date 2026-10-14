# Core API Reference

[中文](4-核心API参考.md) | [English](4-Core-API-Reference.md)

This guide covers the callable TypeScript API of `@sundw/locus-scope-core` for loading Workspaces and consuming in-memory graphs in Node.js applications. See [Basic Usage](1-Basic-Usage.md) for modeling, [Using npm](2-Using-npm.md) for dependencies, and [CLI Reference](3-CLI-Reference.md) for commands. The [Core API contract](design/protocol/CORE_API.md) defines stable capability boundaries; this guide does not introduce a new protocol.

## Installation and Loading

Requires Node.js 24 or later and ES modules:

```sh
pnpm add @sundw/locus-scope-core
# or
npm install @sundw/locus-scope-core
```

`@sundw/locus-scope` also re-exports the Core API. Depend directly on Core for library-only use.

Create these files in `./app`:

```yaml
# locus.yaml
id: app
exports: [api, db]
```

```yaml
# entities.locus.yaml
entities:
  - id: api
    type: service
  - id: db
    type: database
relations:
  - from: api
    type: depends_on
    to: db
```

Load and validate the local Scope and its Import closure:

```ts
import {
  ApplicationService,
  ApplicationError,
  buildPackageEnvironment,
  loadPackageEnvironment,
  parseFilters
} from "@sundw/locus-scope-core";

const loadWorkspace = async () => loadPackageEnvironment(await buildPackageEnvironment("./app"));

let application = new ApplicationService(await loadWorkspace());
const workspace = application.getWorkspace();
const api = workspace.resolve(workspace.root, "api");
const db = workspace.resolve(workspace.root, "db");
```

`buildPackageEnvironment` constructs the installed npm/pnpm dependency graph from an explicit Scope directory; it does not install dependencies. `loadPackageEnvironment` returns a loaded, validated Workspace. Local-only Scopes need no `package.json`. A failed load throws instead of returning a partial graph.

## Identity and References

An `EntityKey` is `{ scope: ScopeKey, id: string }`. A ScopeKey is a string, not the manifest's `id`. Entities with the same local ID in different Scopes are distinct nodes.

`workspace.resolve(fromScope, reference)` interprets a reference relative to a loaded Scope. For example, `infra:server` uses Projection resolution and Import/Export visibility. Different Projections targeting the same Entity yield the same EntityKey. Graph operations take EntityKeys, not Projection strings.

## Queries and DTOs

Current query entry points are these methods, not a `Query` function accepting command strings:

| Method                                     | Result                               |
| ------------------------------------------ | ------------------------------------ |
| `queryScopes(predicates?, withSource?)`    | Scope DTO array                      |
| `getScope(reference, withSource?)`         | One Scope DTO                        |
| `queryGroups(predicates?, withSource?)`    | Group DTO array                      |
| `getGroup(reference, withSource?)`         | One Group DTO                        |
| `queryEntities(predicates?, withSource?)`  | Entity DTO array                     |
| `getEntity(reference, withSource?)`        | Entity DTO resolved relative to root |
| `entityByKey(key, withSource?)`            | Entity DTO for a canonical identity  |
| `queryRelations(predicates?, withSource?)` | Relation DTO array                   |

```ts
const services = application.queryEntities(parseFilters(["type=service"]), true);
const entity = application.entityByKey(api);
const relations = application.queryRelations(parseFilters(["type=depends_on"]));
```

Entity DTOs use `{ key, ref?, object, source? }`; Relation DTOs use `{ fromKey, toKey, object, source? }`. Open properties remain in `object`, along with Entity `id` or declared Relation `from`, `type`, and `to`. Only Entities resolvable from root have a `ref`. `withSource` defaults to `false`; enabling it adds provenance without changing identity.

The Core root exports `Entity` and `Relation` as domain types. Use `EntityView`, `RelationView`, `ScopeView`, and `GroupView` for DTO types.

## Filters

`parseFilters(string[])` produces `Predicates`; conditions combine with AND. Operators are `=`, `!=`, `^=`, `*=`, and `~=`. Values are parsed as JSON when possible, otherwise as strings. Dot paths traverse objects, not arrays. Missing fields do not match.

`@scope`, `@group`, `@fromScope`, and `@toScope` select applicable envelope metadata using application Scope references, such as `.` or `infra`, rather than comparing ScopeKeys directly. See [Filters](3-CLI-Reference.md#filters) for details. Graph `via` uses the same Relation predicates.

## Graph Operations

Graph operations synchronously return DTOs without changing the Workspace. `via` defaults to `[]`; `withSource` defaults to `false`.

| Method                                    | Result fields                           |
| ----------------------------------------- | --------------------------------------- |
| `graph(seeds, depth?, via?, withSource?)` | `seeds`, `depth?`, `nodes`, `relations` |
| `path(from, to, via?, withSource?)`       | `from`, `to`, `nodes`, `relations`      |
| `impact(seeds, via?, withSource?)`        | `seeds`, `affected`, `relations`        |
| `adjacent(request, withSource?)`          | `seeds`, `nodes`, `relations`           |

### Graph

```ts
const allReachable = application.graph([api]);
const seedsOnly = application.graph([api], 0);
const oneHop = application.graph([api], 1, parseFilters(["type=depends_on"]));
```

Omitted depth traverses all forward-reachable nodes, using a visited set for cycles. Depth `0` returns deduplicated seeds and no relations; negative depth is an argument error. Unbounded results omit the `depth` field. CLI defaults do not determine Core API defaults.

### Path and Impact

```ts
const shortest = application.path(api, db);
const reverseReachable = application.impact([db]);
```

Path returns a deterministic shortest directed path, with nodes and relations in path order. An unreachable target throws `scope.path_unreachable`; identical endpoints return that node. Impact traverses incoming edges in reverse, excluding seeds from `affected`.

### Adjacent

```ts
const neighbors = application.adjacent(
  {
    entities: [api],
    direction: "both",
    via: parseFilters(["type=depends_on"])
  },
  true
);
```

`direction` is required and must be `incoming`, `outgoing`, or `both`. `seeds` contains deduplicated input; `nodes` includes input and immediate neighbors; `relations` contains matching edges. Empty entities produce empty arrays. Self-loops and relations matched by multiple seeds appear once. Parallel edges with different types remain distinct. Entities sort by canonical identity; relations retain stable order.

## Validate

```ts
const summary = application.validate();
```

Returns `{ valid, root, scopes, entities, relations }`. The application uses an already validated Workspace; this method reports its statistics. Reload to validate file changes instead of calling `validate()`.

## Diff

```ts
const difference = await application.diff("scope:.", "path:./other-app", async directory =>
  loadPackageEnvironment(await buildPackageEnvironment(directory))
);
```

`diff(left, right, loadPath)` accepts `scope:`, `group:`, and `path:` selectors. `scope:.` and `group:.#backend` select from the current Workspace; `path:` calls the host-supplied loader. The loader above loads directories; a host can use `loadDefinitionFile` for individual definition documents.

Results use `{ added, removed, changed }`, each covering entities and relations. Changed items use `{ before, after }`. Comparison uses canonical EntityKeys, Relation triples, and open properties rather than file formatting.

## Mutate and Snapshots

```ts
const mutation = await application.mutate(
  {
    kind: "entity",
    action: "set",
    ref: "api",
    fields: { owner: "platform" }
  },
  loadWorkspace
);

application = new ApplicationService(mutation.workspace);
```

`mutate(request, reload)` returns a Promise resolving to `{ result, workspace }`. Requests use `kind: "entity" | "relation"` and `action: "add" | "set" | "unset" | "remove"`. Entities use `ref`; relations use `from`, `type`, and `to` for identity.

- add uses `object`, with an optional root-relative `file`.
- set uses `fields` for non-identity properties.
- unset uses an `unset` array of field names.
- remove deletes the selected declaration.

Dependency Scopes are read-only. set/unset/remove write back according to provenance; identity fields cannot change. The host reload must reload and validate; failures follow the rollback contract. See [Mutation Rules](design/protocol/CLI.md#写入规则).

Treat loaded Workspaces as read-only snapshots. Successful mutation returns a new Workspace without switching the original application. Old Workspaces and their indexes remain readable but do not reflect subsequent writes. Direct collection mutation is unsupported.

## Error Handling

```ts
try {
  application.path(db, api);
} catch (error) {
  if (error instanceof ApplicationError && error.reason === "scope.path_unreachable") {
    console.log("No directed path");
  } else {
    throw error;
  }
}
```

ApplicationError exposes `code`, `reason`, optional `details`, and an internal `cause`. Branch on structured fields, not messages, and do not expose causes or stacks. See the [Error Contract](design/protocol/ERRORS.md) for classifications.
