# @sundw/locus-scope-core

[中文](README_CN.md) | [English](README.md)

In-process Workspace graphs for Node.js and TypeScript applications. Load and validate composed Scopes, then query entities and relations through the Core application API. No CLI dependency or command parsing is required.

Requires Node.js 24 or later. This package uses ES modules and includes TypeScript declarations.

## Install

```sh
pnpm add @sundw/locus-scope-core
```

npm is also supported. npm or pnpm owns dependency installation, lockfiles and caches; Core consumes the resolved package environment.

## Load a Workspace

Given a directory `./workspace` containing:

```yaml
# locus.yaml
id: app
exports: [api, db]
```

```yaml
# entities.locus.yaml
entities:
  - id: api
  - id: db
relations:
  - from: api
    type: depends_on
    to: db
```

Load the graph using an explicit directory:

```ts
import {
  ApplicationService,
  buildPackageEnvironment,
  loadPackageEnvironment,
  parseFilters
} from "@sundw/locus-scope-core";

const environment = await buildPackageEnvironment("./workspace");
const workspace = await loadPackageEnvironment(environment);
const application = new ApplicationService(workspace);
const api = workspace.resolve(workspace.root, "api");

// Omitted depth traverses all forward-reachable entities, including cycles.
const graph = application.graph([api]);

// Immediate incoming/outgoing neighbors and matching relations.
const adjacent = application.adjacent({
  entities: [api],
  direction: "both",
  via: parseFilters(["type=depends_on"])
});

console.log(graph.nodes, adjacent.relations);
```

Entity identity is `{ scope, id }`, not a globally unique local ID. Resolve Projection references relative to a loaded Scope before passing EntityKeys to graph operations.

## Application API

The application provides entity, relation, Scope and Group queries; `graph`, `path`, `impact`, `adjacent`, `diff`, `mutate` and `validate`. Results use Entity/Relation DTOs with optional provenance. `graph(seeds, 0)` returns seeds only; `adjacent` supports `incoming`, `outgoing` and `both`.

Treat a loaded Workspace as a read-only snapshot. Reload after a successful mutation and use a new application for the new Workspace. An old snapshot remains readable and does not automatically reflect later writes; direct mutation of internal collections is unsupported.

Use `@sundw/locus-scope` when you need the `locus-scope` command instead of a library-only dependency.

## Documentation

- [Basic Usage](https://github.com/SWdrift/locus-scope/blob/master/documents/1-Basic-Usage.md)
- [Using npm](https://github.com/SWdrift/locus-scope/blob/master/documents/2-Using-npm.md)
- [CLI Reference](https://github.com/SWdrift/locus-scope/blob/master/documents/3-CLI-Reference.md)
- [Core API Reference](https://github.com/SWdrift/locus-scope/blob/master/documents/4-Core-API-Reference.md)
- [Project overview](https://github.com/SWdrift/locus-scope/blob/master/README.md)

## License

[MIT](https://github.com/SWdrift/locus-scope/blob/master/LICENSE)
