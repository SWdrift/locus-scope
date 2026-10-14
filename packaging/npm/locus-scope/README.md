# @sundw/locus-scope

[中文](README_CN.md) | [English](README.md)

The `locus-scope` command for querying, validating and modifying composed Scope Workspaces. It uses `@sundw/locus-scope-core` for the shared domain and application implementation and also re-exports the Core library API.

Requires Node.js 24 or later.

## Install

```sh
pnpm add @sundw/locus-scope
pnpm exec locus-scope --help
```

npm is also supported:

```sh
npm install @sundw/locus-scope
npx locus-scope --help
```

Use npm or pnpm to install, update, pack and publish Scope packages; those operations are not Locus CLI commands.

## Query a Workspace

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

Run:

```sh
pnpm exec locus-scope --scope ./workspace validate --json
pnpm exec locus-scope --scope ./workspace entity api --json
pnpm exec locus-scope --scope ./workspace graph api --depth 1 --json
pnpm exec locus-scope --scope ./workspace adjacent api --direction outgoing --via type=depends_on --json
pnpm exec locus-scope --scope ./workspace path api db --json
```

`--scope` selects the root directory explicitly. Without it, the CLI finds the nearest ancestor Scope manifest. `--json` selects machine-readable results and structured errors; `--source` adds declaration provenance.

`adjacent` defaults to `--direction both`. `graph -`, `impact -` and `adjacent -` accept an Entity query envelope or an envelope array from stdin, preserving `{ scope, id }` identity across Scopes.

Success exits with `0`; invalid arguments exit with `2`; other errors exit with `1`. Do not parse human-readable error messages; use the JSON error `code` and `reason`.

## Library consumers

For a library-only dependency, use `@sundw/locus-scope-core`. This package continues to re-export its API, so existing consumers can import `ApplicationService`, `buildPackageEnvironment` and `loadPackageEnvironment` from `@sundw/locus-scope`.

## Documentation

- [Basic Usage](https://github.com/SWdrift/locus-scope/blob/master/documents/1-Basic-Usage.md)
- [Using npm](https://github.com/SWdrift/locus-scope/blob/master/documents/2-Using-npm.md)
- [CLI Reference](https://github.com/SWdrift/locus-scope/blob/master/documents/3-CLI-Reference.md)
- [Core API Reference](https://github.com/SWdrift/locus-scope/blob/master/documents/4-Core-API-Reference.md)
- [Project overview](https://github.com/SWdrift/locus-scope/blob/master/README.md)

## License

[MIT](https://github.com/SWdrift/locus-scope/blob/master/LICENSE)
