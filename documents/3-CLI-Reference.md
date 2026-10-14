# CLI Reference

Locus Scope provides one public command-line entry point:

```text
pnpm exec locus-scope [--scope <dir>] [--json] <command>
# or
npx locus-scope [--scope <dir>] [--json] <command>
```

The `@sundw/locus-scope` runtime resolves dependencies installed by npm or pnpm and executes the shared domain/application API in-process. Except for `help` and `version`, commands load and validate all reachable Scopes before executing. When `--scope` is omitted, the CLI searches from the current directory upward for the nearest Scope.

To call the same Core API in an application process, see the [Core API Reference](4-Core-API-Reference.md).

## Workspace Options

These options apply to `locus-scope`:

| Option | Description |
| --- | --- |
| `--scope <dir>` or `--scope=<dir>` | Use the specified directory as the root Scope. Otherwise, discover the nearest Scope from the current directory. |
| `--json` | Write stable, single-line JSON. Errors use `{"error":"<message>"}`. |
| `--source` | Add declaration provenance to query results. |
| `--version` | Print the CLI version without loading a Workspace. |
| `--help`, `-h` | Print command help without loading a Workspace. |

Options may appear before or after the command unless the command syntax states otherwise.

## References and Filters

### Scope, Group, and Entity References

| Reference        | Meaning                                                |
| ---------------- | ------------------------------------------------------ |
| `.`              | The root Scope.                                        |
| `infra`          | An imported Scope using the alias `infra`.             |
| `platform:infra` | A nested Import resolved through an alias chain.       |
| `.#services`     | The `services` Group in the root Scope.                |
| `infra#storage`  | The `storage` Group in the imported `infra` Scope.     |
| `database`       | An Entity in the current Scope context.                |
| `infra:database` | An exported Entity reached through the `infra` Import. |

### Filters

A filter has the form `field<operator>value`. Multiple filters use AND semantics.

| Operator | Meaning                               |
| -------- | ------------------------------------- |
| `=`      | Strict JSON equality.                 |
| `!=`     | Strict JSON inequality.               |
| `^=`     | String starts with.                   |
| `*=`     | String contains.                      |
| `~=`     | Regular-expression match on a string. |

Values are parsed as JSON when possible; otherwise they are strings. For example, `port=5432` compares a number, while `type=postgres` compares a string. Dot paths traverse JSON objects, such as `metadata.owner=platform`; they do not index arrays. Missing fields do not match any predicate.

Envelope metadata uses these reserved fields:

- `@scope` for the owning Scope.
- `@group` for the declaring Group.
- `@fromScope` for a Relation's resolved source Scope.
- `@toScope` for a Relation's resolved target Scope.

## Workspace Query Commands

### `scope`

List Scopes, retrieve one Scope, or filter Scopes:

```text
locus-scope scope
locus-scope scope .
locus-scope scope id=infra
locus-scope scope @scope^=file:
```

Syntax:

```text
scope [<scope-ref>] [<filter>...] [--source]
```

With no arguments, the command returns all reachable Scopes. A single non-filter argument is treated as a Scope reference.

### `group`

List Groups, retrieve one Group, or filter Groups:

```text
locus-scope group
locus-scope group .#services
locus-scope group type=layer
```

Syntax:

```text
group [<group-ref>] [<filter>...] [--source]
```

With no arguments, the command returns all reachable Groups. A single non-filter argument is treated as a Group reference.

### `entity`

List Entities, retrieve one Entity, or filter Entities:

```text
locus-scope entity
locus-scope entity database
locus-scope entity infra:database
locus-scope entity type=postgres port=5432
```

Syntax:

```text
entity [<entity-ref>] [<filter>...] [--source]
```

With no arguments, the command returns all reachable Entities. A single non-filter argument is treated as an Entity reference.

### `relation`

List or filter Relations:

```text
locus-scope relation
locus-scope relation type=uses
locus-scope relation @fromScope=. type=uses
```

Syntax:

```text
relation [<filter>...] [--source]
```

Relations are selected by filters rather than by a single reference.

## Graph Commands

### `graph`

Traverse outgoing Relations from one or more seed Entities:

```text
locus-scope graph backend
locus-scope graph backend --depth 2
locus-scope graph backend --depth 2 --via type=uses
locus-scope entity type=service --json | locus-scope graph - --depth 1
```

Syntax:

```text
graph [<entity-ref>|-] [--depth <n>] [--via <filter>...]
```

- The default depth is `1`.
- Depth `0` returns only the deduplicated seeds.
- With no seed argument, all Entities owned by the root Scope are seeds.
- `-` reads one Entity query envelope or an array of envelopes from stdin.
- `--via` restricts traversed Relations using Relation filters.

### `path`

Find one deterministic shortest directed path between two Entities:

```text
locus-scope path frontend database
locus-scope path frontend infra:database --via type=uses
```

Syntax:

```text
path <from> <to> [--via <filter>...]
```

If multiple shortest paths or parallel Relations exist, stable Relation ordering determines the result. The command fails when the destination is unreachable.

### `impact`

Traverse incoming Relations to find affected Entities:

```text
locus-scope impact database
locus-scope impact database --via type=uses
locus-scope entity type=postgres --json | locus-scope impact -
```

Syntax:

```text
impact <entity-ref|-> [--via <filter>...]
```

`-` reads one Entity query envelope or an array of envelopes from stdin. `--via` restricts traversed Relations using Relation filters.

## Entity Mutation Commands

Mutations can receive fields as `field=value` assignments, one complete JSON object, or JSON from stdin using `-`. Values in field assignments are parsed as JSON when possible. Dot paths update nested objects.

### `entity add`

Add an Entity to the writable root Scope:

```text
locus-scope entity add backend type=service replicas=2
locus-scope entity add '{"id":"backend","type":"service"}'
locus-scope entity add backend type=service --file model/services.locus.yaml
```

Syntax:

```text
entity add [<id>] [<mutation-input>] [--file <path>]
```

The Entity must have an `id`. Without `--file`, the command writes to `scope.locus.yaml`. The target must be a `*.locus.yaml` file inside the root Scope and outside nested Scopes.

### `entity set`

Update an Entity's non-identity fields in its original declaration file:

```text
locus-scope entity set backend replicas=3 metadata.owner=platform
locus-scope entity set backend '{"id":"backend","type":"service","replicas":3}'
```

Syntax:

```text
entity set [<ref>] [<mutation-input>]
```

A JSON object may supply the `id` when the positional reference is omitted. If both are present, they must match. Object fields are recursively merged; `null` is stored as a value rather than deleting a field.

### `entity unset`

Remove one or more non-identity fields:

```text
locus-scope entity unset backend replicas metadata.owner
```

Syntax:

```text
entity unset <ref> <field>...
```

Use `unset` rather than `set field=null` to delete fields. The `id` field cannot be removed.

### `entity remove`

Remove an Entity from its original declaration file:

```text
locus-scope entity remove backend
```

Syntax:

```text
entity remove <ref>
```

The command fails without writing when removing the Entity would leave a dangling Relation.

## Relation Mutation Commands

A Relation is identified by its resolved `from`, `type`, and `to` triple. Mutation input follows the same assignment, JSON object, and stdin rules as Entity mutations.

### `relation add`

Add a Relation to the writable root Scope:

```text
locus-scope relation add backend uses database weight=1
locus-scope relation add '{"from":"backend","type":"uses","to":"database"}'
locus-scope relation add backend uses database --file model/relations.locus.yaml
```

Syntax:

```text
relation add [<from> <type> <to>] [<mutation-input>] [--file <path>]
```

The three locator fields may be positional or supplied by a complete JSON object. Without `--file`, the command writes to `scope.locus.yaml`.

### `relation set`

Update a Relation's non-locator fields in its original declaration file:

```text
locus-scope relation set backend uses database weight=2
locus-scope relation set backend uses database '{"from":"backend","type":"uses","to":"database","weight":2}'
```

Syntax:

```text
relation set <from> <type> <to> [<mutation-input>]
```

The `from`, `type`, and `to` fields cannot be changed. Remove and re-add the Relation to change its identity.

### `relation unset`

Remove one or more non-locator fields:

```text
locus-scope relation unset backend uses database weight metadata.note
```

Syntax:

```text
relation unset <from> <type> <to> <field>...
```

The locator fields `from`, `type`, and `to` cannot be removed.

### `relation remove`

Remove a Relation by its resolved triple:

```text
locus-scope relation remove backend uses database
```

Syntax:

```text
relation remove <from> <type> <to>
```

All mutation commands validate the changed Workspace before atomically writing the declaration. Dependency Scopes are read-only. A failed mutation reports an error; if rollback state cannot be confirmed, inspect the target file and run `validate` before retrying.

## Comparison and Validation Commands

### `diff`

Compare two semantic subgraphs:

```text
locus-scope diff scope:. scope:infra
locus-scope diff group:.#services group:infra#services
locus-scope diff path:./before path:./after
locus-scope diff path:./before.locus.yaml path:./after.locus.yaml
```

Syntax:

```text
diff <selector-a> <selector-b>
```

Selectors are:

- `scope:<scope-ref>`: all Entities owned by the selected Scope and Relations whose endpoints are both selected.
- `group:<scope-ref>#<group-path>`: the selected Group's Entities and Relations whose endpoints are both selected.
- `path:<path>`: a Scope directory or one Definition document loaded as a comparison input.

The result groups Entity and Relation changes under `added`, `removed`, and `changed`. Changed items contain `before` and `after` values.

### `validate`

Load and validate every reachable Scope:

```text
locus-scope validate
locus-scope --scope ./app --json validate
```

Syntax:

```text
validate
```

The result contains `valid`, the root Scope URI, and counts for Scopes, Entities, and Relations.

### `version`

Print the CLI version without loading a Workspace:

```text
locus-scope version
locus-scope --json version
pnpm exec locus-scope version
```

### `help`

Print command usage without loading a Workspace:

```text
locus-scope help
locus-scope --help
locus-scope -h
```

## Output and Exit Status

| Result                       | stdout         | stderr        | Exit status |
| ---------------------------- | -------------- | ------------- | ----------- |
| Success                      | Command result | Empty         | `0`         |
| Invalid command or arguments | Empty          | Error message | `2`         |
| Other error                  | Empty          | Error message | `1`         |

Unknown commands or options, missing arguments, unsupported option combinations, and invalid filters are argument errors. With `--json`, both success and failure output use stable JSON; failures use `{"error":"<message>"}`.
