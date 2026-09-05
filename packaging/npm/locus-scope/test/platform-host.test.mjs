import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { mkdir, realpath, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { decodeResponse, runHost } from '../lib/host.mjs';
import { locatePlatformHost, platformPackageName } from '../lib/platform.mjs';
import { captureStream, testDirectory, writeJson } from './helpers.mjs';

test('platform lookup selects only the exact supported package and validates its manifest', async (t) => {
  assert.equal(platformPackageName('win32', 'x64'), '@sundw/locus-scope-win32-x64');
  assert.equal(platformPackageName('linux', 'arm64'), '@sundw/locus-scope-linux-arm64');
  assert.throws(() => platformPackageName('win32', 'arm64'), /unsupported platform win32\/arm64/);

  const root = await testDirectory(t, 'platform');
  const packageRoot = path.join(root, 'scope-linux-x64');
  const packageJson = path.join(packageRoot, 'package.json');
  const host = path.join(packageRoot, 'bin', 'locus-scope-node-host');
  await mkdir(path.dirname(host), { recursive: true });
  await writeJson(packageJson, {
    name: '@sundw/locus-scope-linux-x64',
    version: '1.0.1',
    os: ['linux'],
    cpu: ['x64'],
  });
  await writeFile(host, 'host');

  assert.equal(
    await locatePlatformHost({
      platform: 'linux',
      architecture: 'x64',
      resolvePackageJson: () => packageJson,
    }),
    await realpath(host),
  );

  await writeJson(packageJson, {
    name: '@sundw/locus-scope-linux-x64',
    version: '9.9.9',
    os: ['linux'],
    cpu: ['x64'],
  });
  await assert.rejects(
    locatePlatformHost({
      platform: 'linux',
      architecture: 'x64',
      resolvePackageJson: () => packageJson,
    }),
    /does not match @sundw\/locus-scope 1\.0\.1 for linux\/x64/,
  );

  await writeJson(packageJson, {
    name: '@sundw/locus-scope-linux-x64',
    version: '1.0.1',
    os: ['linux'],
    cpu: ['x64'],
  });
  await rm(host);
  await assert.rejects(
    locatePlatformHost({
      platform: 'linux',
      architecture: 'x64',
      resolvePackageJson: () => packageJson,
    }),
    /is missing bin[\\/]locus-scope-node-host/,
  );

  await assert.rejects(
    locatePlatformHost({
      platform: 'linux',
      architecture: 'x64',
      resolvePackageJson: () => {
        throw new Error('not installed');
      },
    }),
    /reinstall @sundw\/locus-scope with optional dependencies enabled/,
  );
});

test('host invocation sends one request and forwards response output and exit code', async (t) => {
  const root = await testDirectory(t, 'host-output');
  const fakeHost = path.join(root, 'fake-host.mjs');
  await writeFile(
    fakeHost,
    `let input = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', (chunk) => { input += chunk; });
process.stdin.on('end', () => {
  const request = JSON.parse(input);
  process.stdout.write(JSON.stringify({
    version: 1,
    exitCode: 2,
    stdout: 'seen:' + request.arguments.join(',') + '\\n',
    stderr: 'query failed\\n'
  }) + '\\n');
  process.exitCode = 2;
});
`,
  );
  const stdout = captureStream();
  const stderr = captureStream();
  const request = {
    version: 1,
    workingDirectory: root,
    arguments: ['resolve', 'missing'],
    root: { scopeRoot: root, packageRoot: '', dependencies: {} },
    packages: {},
  };

  const exitCode = await runHost('synthetic-host', request, stdout, stderr, {
    spawnProcess: (_host, options) => spawn(process.execPath, [fakeHost], options),
  });
  assert.equal(exitCode, 2);
  assert.equal(stdout.value(), 'seen:resolve,missing\n');
  assert.equal(stderr.value(), 'query failed\n');
});

test('host protocol rejects malformed, extended, and exit-mismatched responses', async (t) => {
  assert.throws(() => decodeResponse('not-json'), /returned invalid JSON/);
  assert.throws(
    () => decodeResponse('{"version":1,"exitCode":0,"stdout":"","stderr":"","extra":true}'),
    /response fields are invalid/,
  );
  assert.throws(
    () => decodeResponse('{"version":2,"exitCode":0,"stdout":"","stderr":""}'),
    /response version 2 is unsupported/,
  );

  const root = await testDirectory(t, 'host-mismatch');
  const fakeHost = path.join(root, 'fake-host.mjs');
  await writeFile(
    fakeHost,
    `process.stdin.resume();
process.stdin.on('end', () => {
  process.stdout.write(JSON.stringify({version: 1, exitCode: 1, stdout: '', stderr: ''}));
});
`,
  );
  await assert.rejects(
    runHost('synthetic-host', { version: 1 }, captureStream(), captureStream(), {
      spawnProcess: (_host, options) => spawn(process.execPath, [fakeHost], options),
    }),
    /protocol exit mismatch: process exited 0, response declared 1/,
  );
});
