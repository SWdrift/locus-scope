# Using npm

In npm mode, npm or pnpm manages Package versions, the lockfile, the download cache, and `node_modules`, while `@sundw/locus-scope` provides the Workspace command entry point. This mode is suitable for existing Node.js projects, projects that need to reuse the npm toolchain, and dependency graphs containing both JavaScript Packages and Locus Packages.

## Installation

Node.js 20.6 or later is required. Install the Node adapter in your project:

```text
pnpm add @sundw/locus-scope
# or
npm install @sundw/locus-scope
```

`@sundw/locus-scope` provides `locus-scope-node`. First, create a minimal Scope:

```yaml
# locus.yaml
id: app
```

Verify the installation and file loading:

```text
pnpm exec locus-scope-node validate
# or
npx locus-scope-node validate
```

See [Basic Usage](1-Basic-Usage.md) for the complete modeling and CLI workflow.

## Installing a Locus Package

A Locus Package is a standard npm Package containing `locus.entry`. First, declare an Import by Package name in the consuming project's `locus.yaml`:

```yaml
id: app
imports:
    infra: "@example/infra"
```

The Package name does not include a version or subpath. The version range is declared in `package.json.dependencies` in the same directory, and the package manager lockfile pins the resolved result.

Install and query with pnpm:

```text
pnpm add @example/infra
pnpm exec locus-scope-node validate
pnpm exec locus-scope-node entity infra:database
```

With npm:

```text
npm install @example/infra
npx locus-scope-node validate
npx locus-scope-node entity infra:database
```

## Running Commands

In pnpm projects, consistently run commands through `pnpm exec locus-scope-node`:

```text
pnpm exec locus-scope-node validate
pnpm exec locus-scope-node entity
pnpm exec locus-scope-node entity database
pnpm exec locus-scope-node relation
pnpm exec locus-scope-node graph backend --depth 1
```

In npm projects, replace the command prefix with `npx locus-scope-node`. When running outside the Scope directory, specify its location with `--scope <dir>`:

```text
pnpm exec locus-scope-node --scope ./app validate
```

The default output is formatted JSON. Agents, scripts, and CI can append `--json` to receive stable single-line JSON.

## Dependency and Runtime Boundaries

npm mode follows the package manager's normal workflow:

- Use `pnpm add`, `pnpm update`, and `pnpm remove`, or the corresponding npm commands, to modify dependencies.
- Commit `package.json` and the package manager lockfile.
- In CI, use the project's existing frozen-lockfile installation method, then run `locus-scope-node validate`.
- Do not run `locus-pkg install`, and do not read Pure Locus's `locus.lock` or `.locus/`.

The Node adapter resolves direct dependencies from each Package's own dependency context, so npm hoisting and pnpm's symlink layout do not change Scope semantics. JavaScript only provides resolved Package information; Scope files are still loaded and validated by the same Go core used by the standalone CLI.

## Publishing a Package

There is currently no Node.js/npm ecosystem counterpart to `locus-pkg`; for now, the `$locus-publish-node` Skill provides the publication workflow.

A publishable Locus Package uses standard npm metadata. For Agent-driven publication, use `$locus-publish-node`: run its lightweight validator, validate the Workspace, then pack and publish the same npm tarball:

```text
node <skill-dir>/scripts/validate-package.mjs .
pnpm exec locus-scope-node validate
npm pack --json --ignore-scripts --pack-destination <workspace>/temp/locus-publish
npm publish <workspace>/temp/locus-publish/<filename> --ignore-scripts --registry https://registry.example.com/
```

Before publishing, inspect the files and SHA-512 integrity returned by `npm pack --json`; afterwards, confirm that the exact `name@version` has the same `dist.integrity` in the Registry. The Skill is an Agent workflow, not a Registry-enforced gate. Use `locus-pkg pack` and `locus-pkg publish` when a non-Node.js environment or tool-enforced validation is required.

## Boundary with Standalone CLI Mode

Both modes use the same Scope files and Workspace validation semantics, but their dependency state cannot be mixed:

- npm mode reads the package manager lockfile and `node_modules`; its command is `locus-scope-node`.
- Standalone CLI mode reads `locus.lock` and `.locus/`; its command is `locus-scope`.
- When using only local Scopes without Packages, either CLI can load the same files directly.

Non-Node.js projects, or projects that should remain independent of the npm toolchain, should use [Standalone CLI Mode](3-Using-Standalone-CLI.md) instead.
