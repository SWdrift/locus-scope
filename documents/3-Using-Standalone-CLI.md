# Using the Standalone CLI

Standalone CLI mode runs `locus-scope` and `locus-pkg` directly, without depending on Node.js, npm, or pnpm. It is suitable for projects that only need to describe and query Entity graphs, non-JavaScript projects, and environments where Locus should manage Package locks and the offline cache itself.

## Installation

On Windows, run `locus-setup-windows-amd64.exe`:

- Install `locus-scope` when using only local Scopes.
- Also install `locus-pkg` when you need to install, update, or publish Packages through a Registry.
- If the installer modifies the current user's `PATH`, open a new terminal after installation.

Create a minimal Scope:

```yaml
# app/locus.yaml
id: app
```

Verify the installation and file loading:

```text
locus-scope --scope ./app validate
```

## Using Local Scopes

`locus-scope` loads, validates, and operates on the Workspace:

```text
locus-scope --scope ./app validate
locus-scope --scope ./app entity
locus-scope --scope ./app relation
```

After entering `app` or one of its subdirectories, you can omit `--scope`. See [Basic Usage](1-Basic-Usage.md) for the complete modeling and CLI workflow.

When using only local Scopes, you do not need `package.json`, `locus.lock`, or `.locus/`.

## Installing Packages

The standalone CLI uses an npm-compatible Registry but does not invoke npm or pnpm. A project that consumes Packages must place `package.json` and `locus.yaml` in the same directory:

```json
{
  "name": "example-app",
  "private": true,
  "dependencies": {
    "@example/infra": "^1.0.0"
  }
}
```

Run the following commands in that directory:

```text
locus-pkg install
locus-scope validate
```

`locus-pkg install` maintains the following project state:

- `package.json`: Direct dependencies and their version ranges.
- `locus.lock`: The complete, deterministic dependency-resolution result.
- `.locus/`: The download cache and extracted Packages.

You can also modify dependencies directly:

```text
locus-pkg install @example/infra@^1.0.0
locus-pkg update @example/infra
locus-pkg uninstall @example/infra
locus-pkg list
```

Do not use npm or pnpm to modify the dependency layout of the same project. The two toolchains do not share locks, caches, or installation directories.

## CI and Offline Operation

In CI, require declarations, the lock, and resolved dependencies to match exactly:

```text
locus-pkg install --frozen-lockfile
locus-scope --json validate
```

When a complete lock and `.locus/` state are already available, you can prohibit all Registry requests:

```text
locus-pkg install --offline --frozen-lockfile
locus-scope --json validate
```

If the offline state is incomplete, the command fails instead of implicitly connecting to the network to fill the gaps.

## Packaging and Publishing

A publishable Package still uses a standard `package.json`, with `locus.entry` pointing to the Package's `locus.yaml`:

```json
{
  "name": "@example/infra",
  "version": "1.0.0",
  "files": ["locus.yaml", "resources.locus.yaml"],
  "locus": {
    "entry": "locus.yaml"
  },
  "exports": {
    "./package.json": "./package.json"
  }
}
```

Run the following commands in the Package root:

```text
locus-pkg pack
locus-pkg publish --registry https://registry.example.com/
```

Registry selection follows npm's `.npmrc` rules. You can also use `--registry` or `NPM_CONFIG_REGISTRY`. For bearer tokens, use the target Registry's `_authToken` or `NPM_TOKEN`. Do not write credentials to the Package, lock, `.locus/`, or command output.


## Boundary with npm Mode

The standalone CLI and npm mode use the same Scope, Entity, Relation, Import, and Export semantics. They differ only in the Package environment:

- In standalone CLI mode, `locus-pkg` generates `locus.lock` and maintains `.locus/`.
- In npm mode, npm or pnpm generates its own lockfile and maintains `node_modules`.
- `locus-scope` does not read `node_modules`; `locus-scope-node` does not read `.locus/`.

Projects whose dependencies are already managed by npm or pnpm should use [npm Mode](2-Using-npm.md) instead.
