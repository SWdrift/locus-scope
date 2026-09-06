# Basic Usage

This guide walks through the basic workflow, from creating a Scope to composing Packages. The examples primarily use npm mode with pnpm managing dependencies; differences for standalone CLI mode are listed at the end.

## Installation

Install Node.js 20.6 or later, then install the Node adapter in your project:

```text
pnpm add @sundw/locus-scope
```

## Creating Your First Scope

Create `app/locus.yaml`:

```yaml
id: app

exports:
    - backend
```

`locus.yaml` defines a Scope. `id` is its name within its content; `exports` lists the Entities that other Scopes are allowed to access.

Define Entities and Relations in `app/model/services.locus.yaml`:

```yaml
entities:
    - id: database
      type: postgres
      host: db.internal
      port: 5432

    - id: backend
      type: service

relations:
    - from: backend
      type: uses
      to: database
```

An Entity is an open object with the required identity field `id`. A Relation is an open object with the three required locator fields `from`, `type`, and `to`. Both may declare any additional nested properties directly.

Definition documents use the `.locus.yaml`, `.locus.yml`, or `.locus.json` suffix. Locus recursively reads files with these suffixes from ordinary subdirectories of the current Scope. If a subdirectory has its own `locus.yaml`, it is treated as a separate Scope and is not automatically included in the current Scope.

The completed directory structure is:

```text
app/
├── locus.yaml
└── model/
    └── services.locus.yaml
```

## Working with Your First Scope

The following minimal set of operations validates the Workspace, queries objects, and traverses the relationship graph. `file:///path/to/project/app` represents the actual absolute file URI of the `app` directory.

### Validating the Workspace

```shell
pnpm exec locus-scope-node --scope ./app validate
```

Output:

```json
{
  "valid": true,
  "root": "file:///path/to/project/app",
  "scopes": 1,
  "entities": 2,
  "relations": 1
}
```

`validate` loads and validates all reachable Scopes.

### Querying Entities

First, retrieve `database` by reference:

```shell
pnpm exec locus-scope-node --scope ./app entity database
```

Output:

```json
{
  "key": {
    "scope": "file:///path/to/project/app",
    "id": "database"
  },
  "ref": "database",
  "object": {
    "host": "db.internal",
    "id": "database",
    "port": 5432,
    "type": "postgres"
  }
}
```

You can also filter by open properties:

```shell
pnpm exec locus-scope-node --scope ./app entity type=postgres
```

Output:

```json
[
  {
    "key": {
      "scope": "file:///path/to/project/app",
      "id": "database"
    },
    "ref": "database",
    "object": {
      "host": "db.internal",
      "id": "database",
      "port": 5432,
      "type": "postgres"
    }
  }
]
```

Multiple filters use AND semantics; for example, `entity type=postgres port=5432`.

### Querying Relations

```shell
pnpm exec locus-scope-node --scope ./app relation type=uses
```

Output:

```json
[
  {
    "fromKey": {
      "scope": "file:///path/to/project/app",
      "id": "backend"
    },
    "toKey": {
      "scope": "file:///path/to/project/app",
      "id": "database"
    },
    "object": {
      "from": "backend",
      "to": "database",
      "type": "uses"
    }
  }
]
```

`fromKey` and `toKey` are the resolved, stable owner keys; `object` preserves the references and other open properties from the declaration.

### Traversing the Relationship Graph

Query one level of outgoing edges from `backend`:

```shell
pnpm exec locus-scope-node --scope ./app graph backend --depth 1
```

The output contains the seed, both endpoint Entities, and the `backend → database` Relation:

```json
{
  "seeds": [
    {
      "key": {
        "scope": "file:///path/to/project/app",
        "id": "backend"
      },
      "ref": "backend",
      "object": {
        "id": "backend",
        "type": "service"
      }
    }
  ],
  "depth": 1,
  "nodes": [
    {
      "key": {
        "scope": "file:///path/to/project/app",
        "id": "backend"
      },
      "ref": "backend",
      "object": {
        "id": "backend",
        "type": "service"
      }
    },
    {
      "key": {
        "scope": "file:///path/to/project/app",
        "id": "database"
      },
      "ref": "database",
      "object": {
        "host": "db.internal",
        "id": "database",
        "port": 5432,
        "type": "postgres"
      }
    }
  ],
  "relations": [
    {
      "fromKey": {
        "scope": "file:///path/to/project/app",
        "id": "backend"
      },
      "toKey": {
        "scope": "file:///path/to/project/app",
        "id": "database"
      },
      "object": {
        "from": "backend",
        "to": "database",
        "type": "uses"
      }
    }
  ]
}
```

The same data can answer two other types of graph questions:

- `path backend database` returns the shortest directed path from `backend` to `database`.
- `impact database` traverses incoming edges in reverse and returns the `backend` affected by `database`.

### Other Key Operations

| Purpose | Command |
| --- | --- |
| View the root Scope | `scope .` |
| List or filter Groups | `group [<group-ref>\|<filter>...]` |
| List all Entities | `entity` |
| List all Relations | `relation` |
| Add, modify, or remove an Entity | `entity add\|set\|unset\|remove` |
| Add, modify, or remove a Relation | `relation add\|set\|unset\|remove` |
| Compare two semantic subgraphs | `diff <selector-a> <selector-b>` |
| Show declaration sources | Append `--source` to a query command |

After entering `app` or one of its subdirectories, you can omit `--scope ./app`. The default formatted JSON is suitable for interactive reading; agents, scripts, and CI can append `--json` to receive stable single-line JSON. See the [CLI Reference](4-CLI-Reference.md) for all commands, arguments, filters, mutations, and exit statuses.

## Composing Local Scopes

Suppose the sibling directory `infra/` is also a Scope and exports `database`. Add a relative Import to `app/locus.yaml`:

```yaml
id: app

imports:
    infra: ../infra

exports:
    - backend
```

`infra` is the alias assigned to this Import by the current Scope. Use the alias to inspect the exported Entity:

```text
pnpm exec locus-scope-node --scope ./app entity infra:database
```

## Composing Packages

Install a published Locus Package:

```text
pnpm add @example/infra
```

Then change the Import in `app/locus.yaml` to the Package name:

```yaml
id: app

imports:
    infra: "@example/infra"

exports:
    - backend
```

Validate and inspect it again:

```text
pnpm exec locus-scope-node --scope ./app validate
pnpm exec locus-scope-node --scope ./app entity infra:database
```

`locus.yaml` contains only the Package name, without a version or subpath. The version range is stored in `package.json.dependencies`, and the resolved version is pinned by `pnpm-lock.yaml`.

## npm and pnpm

npm uses the same files and semantics. Replace the installation command and execution prefix with:

```text
npm install @sundw/locus-scope @example/infra
npx locus-scope-node --scope ./app validate
```

See [npm Mode](2-Using-npm.md) for more dependency-management boundaries.

## Differences in Standalone CLI Mode

Standalone CLI mode does not change the syntax of Scope, Entity, Relation, Import, or Export. It only replaces the command entry point and Package-management method:

| npm mode | Standalone CLI mode |
| --- | --- |
| `pnpm exec locus-scope-node ...` | `locus-scope ...` |
| `pnpm add @example/infra` | `locus-pkg install @example/infra@^1.0.0` |
| `pnpm-lock.yaml` and `node_modules` | `locus.lock` and `.locus/` |
| Requires Node.js and npm/pnpm | Does not require Node.js, npm, or pnpm |

For example, the standalone CLI performs the same check with:

```text
locus-pkg install @example/infra@^1.0.0
locus-scope --scope ./app validate
locus-scope --scope ./app entity infra:database
```

See [Standalone CLI Mode](3-Using-Standalone-CLI.md) for installation, offline operation, and publishing with the standalone CLI.
