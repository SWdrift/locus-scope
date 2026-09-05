import assert from 'node:assert/strict';
import { mkdir, realpath, writeFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { main } from '../lib/cli.mjs';
import { discoverScopeRoot, parseRootOptions } from '../lib/options.mjs';
import { captureStream, testDirectory } from './helpers.mjs';

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

test('version and help are local and require no Scope or platform host', async () => {
  for (const arguments_ of [['version'], ['--version'], ['--json', 'version']]) {
    const stdout = captureStream();
    const stderr = captureStream();
    const exitCode = await main(arguments_, {
      workingDirectory: path.parse(process.cwd()).root,
      stdout,
      stderr,
      discoverRoot: () => {
        throw new Error('must not discover a Scope');
      },
      locateHost: () => {
        throw new Error('must not locate a host');
      },
    });
    assert.equal(exitCode, 0);
    assert.equal(stderr.value(), '');
    if (arguments_.includes('--json')) {
      assert.deepEqual(JSON.parse(stdout.value()), { name: 'locus-scope-node', version: '1.0.0' });
    } else {
      assert.equal(stdout.value(), 'locus-scope-node 1.0.0\n');
    }
  }

  const stdout = captureStream();
  assert.equal(await main([], { stdout, stderr: captureStream() }), 0);
  assert.match(stdout.value(), /^Usage:\n  locus-scope-node/);
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
    version: 1,
    workingDirectory: path.resolve('consumer', 'nested'),
    arguments: ['entity', 'list', '--json'],
    root: environment.root,
    packages: environment.packages,
  });
  assert.equal(stdout.value(), '');
  assert.equal(stderr.value(), '');
});

test('adapter errors preserve JSON failure shape', async () => {
  const stdout = captureStream();
  const stderr = captureStream();
  const exitCode = await main(['validate', '--json'], {
    stdout,
    stderr,
    discoverRoot: async () => {
      throw new Error('no usable Scope');
    },
  });
  assert.equal(exitCode, 1);
  assert.equal(stdout.value(), '');
  assert.deepEqual(JSON.parse(stderr.value()), { error: 'no usable Scope' });
});
