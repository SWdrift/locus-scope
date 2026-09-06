import assert from 'node:assert/strict';
import { mkdir, readFile, realpath, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { Readable } from 'node:stream';
import test from 'node:test';
import { main } from '../lib/cli.mjs';
import { discoverScopeRoot, parseRootOptions } from '../lib/options.mjs';
import { captureStream, testDirectory } from './helpers.mjs';

const VERSION = (await readFile(new URL('../../../../VERSION', import.meta.url), 'utf8')).trim();

test('root options work before or after commands and nearest Scope discovery walks ancestors', async (t) => {
  assert.deepEqual(parseRootOptions(['--scope', 'one', 'entity', 'list', '--json']), {
    scopeDirectory: 'one',
    arguments: ['entity', 'list', '--json'],
  });
  assert.deepEqual(parseRootOptions(['validate', '--scope=two']), {
    scopeDirectory: 'two',
    arguments: ['validate'],
  });
  assert.throws(() => parseRootOptions(['validate', '--scope']), /--scope requires a directory/);

  const root = await testDirectory(t, 'root-discovery');
  const nested = path.join(root, 'a', 'b');
  await mkdir(nested, { recursive: true });
  await writeFile(path.join(root, 'locus.yml'), 'id: root\n');
  const canonicalRoot = await realpath(root);
  assert.equal(await discoverScopeRoot(nested), canonicalRoot);
  assert.equal(await discoverScopeRoot(nested, '../..'), canonicalRoot);
});

test('version and help use the shared host without discovering a Scope', async () => {
  for (const arguments_ of [['version'], ['--version'], ['--json', 'version'], []]) {
    const stdout = captureStream();
    const stderr = captureStream();
    let request;
    const exitCode = await main(arguments_, {
      workingDirectory: path.parse(process.cwd()).root,
      stdout,
      stderr,
      discoverRoot: () => {
        throw new Error('must not discover a Scope');
      },
      locateHost: async () => 'host',
      invokeHost: async (_host, value, output) => {
        request = value;
        output.write(arguments_.length === 0 ? 'help\n' : 'version\n');
        return 0;
      },
    });
    assert.equal(exitCode, 0);
    assert.equal(stderr.value(), '');
    assert.equal(request.version, 2);
    assert.equal(request.root.scopeRoot, '');
    assert.deepEqual(request.packages, {});
    assert.match(stdout.value(), /^(version|help)\n$/);
  }
});

test('adapter sends the exact versioned request and forwards the enclosed exit code', async () => {
  const stdout = captureStream();
  const stderr = captureStream();
  const root = path.resolve('synthetic-scope');
  const environment = {
    root: { scopeRoot: root, packageRoot: '', dependencies: {} },
    packages: {},
  };
  let captured;

  const exitCode = await main(['entity', 'list', '--scope', root, '--json'], {
    workingDirectory: path.resolve('consumer', 'nested'),
    stdout,
    stderr,
    discoverRoot: async (workingDirectory, explicit) => {
      assert.equal(workingDirectory, path.resolve('consumer', 'nested'));
      assert.equal(explicit, root);
      return root;
    },
    buildEnvironment: async (scopeRoot) => {
      assert.equal(scopeRoot, root);
      return environment;
    },
    locateHost: async ({ platform, architecture }) => {
      assert.equal(platform, process.platform);
      assert.equal(architecture, process.arch);
      return 'host';
    },
    invokeHost: async (host, request) => {
      assert.equal(host, 'host');
      captured = request;
      return 2;
    },
  });

  assert.equal(exitCode, 2);
  assert.deepEqual(captured, {
    version: 2,
    workingDirectory: path.resolve('consumer', 'nested'),
    arguments: ['entity', 'list', '--json'],
    root: environment.root,
    packages: environment.packages,
  });
  assert.equal(stdout.value(), '');
  assert.equal(stderr.value(), '');
});

test('adapter forwards user stdin only for dash operands through protocol v2', async () => {
  const root = path.resolve('stdin-scope');
  let captured;
  const exitCode = await main(['graph', '-', '--scope', root, '--json'], {
    stdin: Readable.from(['[{\"key\":{\"scope\":\"file:///root\",\"id\":\"api\"}}]']),
    stdout: captureStream(),
    stderr: captureStream(),
    discoverRoot: async () => root,
    buildEnvironment: async () => ({ root: { scopeRoot: root, packageRoot: '', dependencies: {} }, packages: {} }),
    locateHost: async () => 'host',
    invokeHost: async (_host, request) => {
      captured = request;
      return 0;
    },
  });
  assert.equal(exitCode, 0);
  assert.equal(captured.version, 2);
  assert.equal(captured.stdin, '[{\"key\":{\"scope\":\"file:///root\",\"id\":\"api\"}}]');
});

test('adapter errors preserve JSON failure shape', async () => {
  const stdout = captureStream();
  const stderr = captureStream();
  const exitCode = await main(['validate', '--json'], {
    stdout,
    stderr,
    locateHost: async () => 'host',
    discoverRoot: async () => {
      throw new Error('no usable Scope');
    },
  });
  assert.equal(exitCode, 1);
  assert.equal(stdout.value(), '');
  assert.deepEqual(JSON.parse(stderr.value()), { error: 'no usable Scope' });
});
