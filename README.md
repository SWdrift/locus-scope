# locus-scope

[![npm version](https://img.shields.io/npm/v/%40sundw%2Flocus-scope?logo=npm)](https://www.npmjs.com/package/@sundw/locus-scope)

[中文](https://github.com/SWdrift/locus-scope/blob/master/README_CN.md) | [English](https://github.com/SWdrift/locus-scope/blob/master/README.md)

A Scope protocol and lightweight toolkit for composable Entity graphs, defining identity, relationships, scopes, and composition.

Locus Scope organizes identifiable things and their relationships into bounded graphs. An Entity can represent an environment, resource, capability, code, knowledge, logical structure, or another domain object. A Scope provides ownership, naming, visibility, and composition boundaries. Each distributable npm Package corresponds to one Scope.

## Usage

The npm Package provides one `locus-scope` command and a library API. npm or pnpm owns dependency resolution, lockfiles, caches, and installation. The runtime resolves the Package graph from the installed environment, then composes and validates the Workspace in-process.

| Usage | When to use | Command entry point |
| --- | --- | --- |
| [Using npm](https://github.com/SWdrift/locus-scope/blob/master/documents/2-Using-npm.md) | Manage Locus Packages with npm or pnpm, then query, validate, or mutate the Scope Workspace. | `locus-scope` |

To create your first Scope, start with [Basic Usage](https://github.com/SWdrift/locus-scope/blob/master/documents/1-Basic-Usage.md).

Agents can select the matching Skill by task:

| Node.js Package workflow                                                                                         |
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

TypeScript build output is written to `temp/ts-build/`; npm release tarballs are written to `temp/release/npm/`.

</details>

## Documentation

- [Basic Usage](https://github.com/SWdrift/locus-scope/blob/master/documents/1-Basic-Usage.md): Create, validate, compose, and query Scopes.
- [Using npm](https://github.com/SWdrift/locus-scope/blob/master/documents/2-Using-npm.md): Install the unified CLI and let npm or pnpm manage Packages.
- [CLI Reference](https://github.com/SWdrift/locus-scope/blob/master/documents/4-CLI-Reference.md): Workspace queries, graph operations, diff, mutation, output, and exit statuses.

## License

[MIT](https://github.com/SWdrift/locus-scope/blob/master/LICENSE)
