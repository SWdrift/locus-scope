# locus-scope

[![npm version](https://img.shields.io/npm/v/%40sundw%2Flocus-scope?logo=npm)](https://www.npmjs.com/package/@sundw/locus-scope)

[中文](https://github.com/SWdrift/locus-scope/blob/master/README_CN.md) | [English](https://github.com/SWdrift/locus-scope/blob/master/README.md)


A Scope protocol and lightweight toolkit for composable Entity graphs, defining identity, relationships, scopes, and composition.

Locus Scope organizes identifiable things and their relationships into bounded graphs. An Entity can represent an environment, resource, capability, code, knowledge, logical structure, or another domain object. A Scope provides ownership, naming, visibility, and composition boundaries. Each distributable npm Package corresponds to one Scope.

## Usage

Two usage modes are available. Both use the same Scope files and Workspace validation semantics; they differ only in which tool manages Package dependencies.

| Mode | When to use | Command entry point |
| --- | --- | --- |
| [Using npm](https://github.com/SWdrift/locus-scope/blob/master/documents/2-Using-npm.md) | Existing Node.js projects; continues using npm or pnpm for installation, lockfiles, and caching. | `locus-scope-node` |
| [Using the standalone CLI](https://github.com/SWdrift/locus-scope/blob/master/documents/3-Using-Standalone-CLI.md) | Non-Node.js projects; uses only local Scopes, with Locus managing Package locks and the offline cache. | `locus-scope`, `locus-pkg` |

To create your first Scope, start with [Basic Usage](https://github.com/SWdrift/locus-scope/blob/master/documents/1-Basic-Usage.md).

Agents can select the matching Skill by environment and task:

| Environment | Use and consume Packages | Publish Packages |
| --- | --- | --- |
| npm / pnpm | `$locus-use-node` | `$locus-publish-node` |
| Standalone CLI / Pure Locus | `$locus-use` | `$locus-publish` |

<details>
<summary>Building from source</summary>

Requires Go 1.26+, Node.js 20.6+, and the pnpm version pinned in the root `package.json`:

```powershell
pnpm run build
pnpm run deploy
```

Build artifacts are written to `temp/local/bin/`. For a user-level deployment, run `pnpm run deploy:user`. See [`scripts/README.md`](https://github.com/SWdrift/locus-scope/blob/master/scripts/README.md) for complete script documentation.

</details>

## Documentation

- [Basic Usage](https://github.com/SWdrift/locus-scope/blob/master/documents/1-Basic-Usage.md): Create, validate, and compose Scopes in npm mode, with notes on differences in the standalone CLI.
- [Using npm](https://github.com/SWdrift/locus-scope/blob/master/documents/2-Using-npm.md): Install the Node adapter and let npm or pnpm manage Packages.
- [Using the standalone CLI](https://github.com/SWdrift/locus-scope/blob/master/documents/3-Using-Standalone-CLI.md): Install the standalone commands and manage locks, offline state, and Package publishing.
- [CLI Reference](https://github.com/SWdrift/locus-scope/blob/master/documents/4-CLI-Reference.md): All Workspace and Package commands, options, filters, mutations, output, and exit statuses.

## License

[MIT](https://github.com/SWdrift/locus-scope/blob/master/LICENSE)
