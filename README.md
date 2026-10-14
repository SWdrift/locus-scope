# locus-scope

[![npm version](https://img.shields.io/npm/v/%40sundw%2Flocus-scope?logo=npm)](https://www.npmjs.com/package/@sundw/locus-scope)

[中文](https://github.com/SWdrift/locus-scope/blob/master/README_CN.md) | [English](https://github.com/SWdrift/locus-scope/blob/master/README.md)

A Scope protocol and lightweight toolkit for composable Entity graphs, defining identity, relationships, scopes, and composition.

Locus Scope organizes identifiable things and their relationships into bounded graphs. An Entity can represent an environment, resource, capability, code, knowledge, logical structure, or another domain object. A Scope provides ownership, naming, visibility, and composition boundaries. Each distributable npm Package corresponds to one Scope.

## Usage

The npm package provides the `locus-scope` command and library API; `@sundw/locus-scope-core` can also be used independently for the library API.

| Package | Use |
| --- | --- |
| [@sundw/locus-scope](packaging/npm/locus-scope/README.md) | CLI commands and the existing library entry point. |
| [@sundw/locus-scope-core](packaging/npm/locus-scope-core/README.md) | Node.js library API, without a CLI. |

To create your first Scope, start with [Basic Usage](https://github.com/SWdrift/locus-scope/blob/master/documents/1-Basic-Usage.md).

Agents can use the CLI workflow Skill:

| CLI workflow                                                                                                     |
| ---------------------------------------------------------------------------------------------------------------- |
| [`$use-locus-scope`](https://github.com/SWdrift/locus-scope/blob/master/.agents/skills/use-locus-scope/SKILL.md) |

<details>
<summary>Building from source</summary>
Requires the Node.js LTS version declared by the root `package.json` and the pnpm version pinned there:

```powershell
pnpm install --frozen-lockfile
pnpm run typecheck
pnpm run build
pnpm run release:npm
```

</details>

## Documentation

- [Basic Usage](https://github.com/SWdrift/locus-scope/blob/master/documents/1-Basic-Usage.md): Create, validate, compose, and query Scopes.
- [Using npm](https://github.com/SWdrift/locus-scope/blob/master/documents/2-Using-npm.md): Install the unified CLI and let npm or pnpm manage Packages.
- [CLI Reference](https://github.com/SWdrift/locus-scope/blob/master/documents/3-CLI-Reference.md): Workspace queries, graph operations, diff, mutation, output, and exit statuses.
- [Core API Reference](documents/4-Core-API-Reference.md): Load Workspaces and use queries, graph operations, diff, mutation and structured errors in-process.

## License

[MIT](https://github.com/SWdrift/locus-scope/blob/master/LICENSE)
