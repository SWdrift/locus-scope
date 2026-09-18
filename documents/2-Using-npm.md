# Using npm

In npm mode, npm or pnpm manages Package versions, the lockfile, the download cache, and `node_modules`, while `@sundw/locus-scope` provides the Workspace command entry point. This mode is suitable for existing Node.js projects, projects that need to reuse the npm toolchain, and dependency graphs containing both JavaScript Packages and Locus Packages.

## Installation

Node.js 20.6 or later is required. Install the Node adapter in your project:

```text
pnpm add @sundw/locus-scope
# or
npm install @sundw/locus-scope
```

`@sundw/locus-scope` provides `locus-scope`. First, create a minimal Scope:

```yaml
# locus.yaml
id: app
```

Verify the installation and file loading:

```text
pnpm exec locus-scope validate
# or
npx locus-scope validate
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
pnpm exec locus-scope validate
pnpm exec locus-scope entity infra:database
```

With npm:

```text
npm install @example/infra
npx locus-scope validate
npx locus-scope entity infra:database
```

## Running Commands

In pnpm projects, consistently run commands through `pnpm exec locus-scope`:

```text
pnpm exec locus-scope validate
pnpm exec locus-scope entity
pnpm exec locus-scope entity database
pnpm exec locus-scope relation
pnpm exec locus-scope graph backend --depth 1
```

In npm projects, replace the command prefix with `npx locus-scope`. When running outside the Scope directory, specify its location with `--scope <dir>`:

```text
pnpm exec locus-scope --scope ./app validate
```

The default output is formatted JSON. Agents, scripts, and CI can append `--json` to receive stable single-line JSON.

## Dependency and Runtime Boundaries

npm mode follows the package manager's normal workflow:

- Use `pnpm add`, `pnpm update`, and `pnpm remove`, or the corresponding npm commands, to modify dependencies.
- Commit `package.json` and the package manager lockfile.
- In CI, use the project's existing frozen-lockfile installation method, then run `locus-scope validate`.
- Do not create a second Package lock, cache, or installation state for Locus.

A Scope Import names only its dependency target, not a version or installation path. npm or pnpm owns version resolution and physical installation; the Node adapter resolves an importer-relative edge to an exact identity in each Package's own dependency context. A Package Scope resolves an Import only through its own edge, never by searching root dependencies or a global Package set. Go only consumes that resolved Environment and loads and validates the Scope files.

## Publishing a Package

For Agent-driven publication, use `$locus-publish-node`. A publishable Locus Package uses standard npm metadata: run the Skill's lightweight validator, validate the Workspace, then pack and publish the same npm tarball:

```text
node <skill-dir>/scripts/validate-package.mjs .
pnpm exec locus-scope validate
npm pack --json --ignore-scripts --pack-destination <workspace>/temp/locus-publish
npm publish <workspace>/temp/locus-publish/<filename> --ignore-scripts --registry https://registry.example.com/
```

Before publishing, inspect the files and SHA-512 integrity returned by `npm pack --json`; afterwards, confirm that the exact `name@version` has the same `dist.integrity` in the Registry. The Skill is an Agent workflow, not a Registry-enforced gate.

## Runtime Boundary

There is one Package environment:

- npm or pnpm owns version resolution, installation, the lockfile, and cache.
- The Node adapter discovers the importer-relative installed Package graph.
- The private Go host validates the resolved Environment and executes the shared Scope application.
- Local-only Scopes use the same `locus-scope` command and do not require Package Imports.
