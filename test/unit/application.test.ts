import assert from 'node:assert/strict';
import { mkdir, readFile, rm, symlink, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { test } from 'node:test';
import { ApplicationService } from '../../src/application/index.js';
import { ApplicationError } from '../../src/domain/errors.js';
import { run } from '../../src/cli/index.js';
import { buildPackageEnvironment } from '../../src/package-environment.js';
import { load, type Environment } from '../../src/packageenv.js';
import { Scope, Workspace, type Entity, type Relation } from '../../src/domain/index.js';

type GraphBody = { nodes: unknown[] };

function fixtureEntity(scope: string, value: string): Entity {
  return {
    id: 'same',
    localId: 'same',
    properties: { value },
    source: { scope, file: 'entities.locus.yaml', path: path.join('temp', 'unit-ts', 'diff', 'entities.locus.yaml') },
  };
}

function fixtureWorkspace(value: string): Workspace {
  const first = 'file:///first';
  const second = 'file:///second';
  const firstScope = new Scope(
    { key: first, localPath: path.resolve('temp/unit-ts/diff/first') },
    { id: 'first', imports: {}, exports: [] },
    'locus.yaml',
  );
  const secondScope = new Scope(
    { key: second, localPath: path.resolve('temp/unit-ts/diff/second') },
    { id: 'second', imports: {}, exports: [] },
    'locus.yaml',
  );
  firstScope.entities.set('same', fixtureEntity(first, 'before'));
  secondScope.entities.set('same', fixtureEntity(second, value));
  const workspace = new Workspace(first, new Map([[first, firstScope], [second, secondScope]]));
  const relation: Relation = {
    from: { scope: first, id: 'same' },
    type: 'links',
    to: { scope: second, id: 'same' },
    fromRef: 'same',
    toRef: 'same',
    properties: { value },
    source: { scope: first, file: 'entities.locus.yaml', path: path.join('temp', 'unit-ts', 'diff', 'entities.locus.yaml') },
  };
  workspace.relations = [relation];
  return workspace;
}

test('diff preserves same IDs owned by different Scopes', async () => {
  const left = fixtureWorkspace('before');
  const right = fixtureWorkspace('after');
  const result = await new ApplicationService(left).diff('path:left', 'path:right', async selector => selector === 'left' ? left : right);
  assert.equal(result.changed.entities.length, 1);
  assert.equal(result.changed.relations.length, 1);
  assert.equal(result.added.entities.length, 0);
  assert.equal(result.removed.entities.length, 0);
});

test('graph rejects negative depth as an invalid argument', () => {
  assert.throws(
    () => new ApplicationService(fixtureWorkspace('before')).graph([], -1),
    error => error instanceof ApplicationError && error.code === 'invalid_argument' && error.reason === 'scope.selector_invalid',
  );
});

test('built CLI loads and validates a local Scope', async () => {
  const root = path.resolve('test/e2e/case/scope-composition/app');
  const response = await run(['validate', '--json'], {
    workingDirectory: root,
    discoverRoot: async () => root,
    buildEnvironment: buildPackageEnvironment,
    loadApplication: async environment => new ApplicationService(await load(environment)),
  });
  assert.equal(response.exitCode, 0);
  assert.equal(response.stderr, '');
  assert.match(response.stdout, /"valid":true/);
});

test('CLI preserves default graph seeds, help alias, version text, and path errors', async () => {
  const root = path.resolve('test/e2e/case/scope-composition/app');
  const context = {
    workingDirectory: root,
    discoverRoot: async () => root,
    buildEnvironment: buildPackageEnvironment,
    loadApplication: async (environment: Environment) => new ApplicationService(await load(environment)),
    version: '4.0.0',
  };
  const graph = await run(['graph', '--json'], context);
  assert.equal(graph.exitCode, 0);
  const graphBody = JSON.parse(graph.stdout) as GraphBody;
  assert.ok(graphBody.nodes.length > 0);
  const help = await run(['-h'], context);
  assert.equal(help.exitCode, 0);
  assert.match(help.stdout, /^Usage: locus-scope/);
  const version = await run(['version'], context);
  assert.equal(version.exitCode, 0);
  assert.equal(version.stdout, 'locus-scope 4.0.0\n');
  const unreachable = await run(['path', 'api', 'infra:database', '--via', 'type=missing', '--json'], context);
  assert.equal(unreachable.exitCode, 1);
  assert.match(unreachable.stderr, /no directed path/);
  const missing = await run(['entity', 'missing', '--json'], context);
  assert.equal(missing.exitCode, 1);
  assert.match(missing.stderr, /does not contain entity/);
});

test('CLI rejects conflicting positional and JSON mutation identity', async () => {
  const application = new ApplicationService(fixtureWorkspace('before'));
  const response = await run(['relation', 'add', 'api', 'depends_on', 'db', '{"from":"other","type":"links","to":"x"}', '--json'], {
    application,
    reloadWorkspace: async () => application.workspace,
  });
  assert.equal(response.exitCode, 2);
  assert.match(response.stderr, /positional from does not match JSON value/);
});

test('mutation rejects a symlink target outside the root', async t => {
  const root = path.resolve('temp/unit-ts/symlink');
  const outside = path.resolve('temp/unit-ts/symlink-outside.locus.yaml');
  await rm(root, { recursive: true, force: true });
  await mkdir(path.join(root, 'inside'), { recursive: true });
  await writeFile(outside, 'entities:\n  - id: keep\n');
  await writeFile(path.join(root, 'locus.yaml'), 'id: app\n');
  const link = path.join(root, 'inside', 'target.locus.yaml');
  try {
    await symlink(outside, link);
  } catch (error) {
    t.skip(`symlink creation unavailable: ${String(error)}`);
    return;
  }
  try {
    const environment = await buildPackageEnvironment(root);
    const application = new ApplicationService(await load(environment));
    await assert.rejects(
      application.mutate({ kind: 'entity', action: 'add', file: 'inside/target.locus.yaml', object: { id: 'new' }, fields: {} }, async () => load(environment)),
      /remain inside root Scope/,
    );
    assert.equal(await readFile(outside, 'utf8'), 'entities:\n  - id: keep\n');
  } finally {
    await rm(root, { recursive: true, force: true });
    await rm(outside, { force: true });
  }
});
