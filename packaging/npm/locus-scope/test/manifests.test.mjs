import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const MATRIX = [
  ['darwin-arm64', 'darwin', 'arm64', 'bin/locus-scope-node-host'],
  ['darwin-x64', 'darwin', 'x64', 'bin/locus-scope-node-host'],
  ['linux-arm64', 'linux', 'arm64', 'bin/locus-scope-node-host'],
  ['linux-x64', 'linux', 'x64', 'bin/locus-scope-node-host'],
  ['win32-x64', 'win32', 'x64', 'bin/locus-scope-node-host.exe'],
];
const PROJECT_LINKS = {
  repository: {
    type: 'git',
    url: 'git+https://github.com/SWdrift/locus-scope.git',
  },
  homepage: 'https://github.com/SWdrift/locus-scope#readme',
  bugs: {
    url: 'https://github.com/SWdrift/locus-scope/issues',
  },
};

test('@sundw/locus-scope source manifest defers release versions to VERSION', async () => {
  const manifest = await readManifest(new URL('../package.json', import.meta.url));
  assert.equal(manifest.version, undefined);
  assert.equal(manifest.private, true);
  assertProjectLinks(manifest);
  assert.equal(manifest.type, 'module');
  assert.deepEqual(manifest.engines, { node: '>=20.6' });
  assert.deepEqual(manifest.exports, {});
  assert.deepEqual(manifest.bin, { 'locus-scope-node': 'bin/locus-scope-node.mjs' });
  assert.deepEqual(manifest.files, ['bin', 'lib']);
  assert.deepEqual(
    manifest.optionalDependencies,
    Object.fromEntries(MATRIX.map(([suffix]) => [`@sundw/locus-scope-${suffix}`, 'workspace:*'])),
  );
});

test('platform package manifests match the five-platform host matrix', async () => {
  for (const [suffix, operatingSystem, architecture, host] of MATRIX) {
    const manifest = await readManifest(new URL(`../../locus-scope-${suffix}/package.json`, import.meta.url));
    assert.equal(manifest.name, `@sundw/locus-scope-${suffix}`);
    assert.equal(manifest.version, undefined);
    assert.equal(manifest.private, true);
    assertProjectLinks(manifest);
    assert.deepEqual(manifest.os, [operatingSystem]);
    assert.deepEqual(manifest.cpu, [architecture]);
    assert.deepEqual(manifest.exports, { './package.json': './package.json' });
    assert.deepEqual(manifest.files, [host]);
  }
});

function assertProjectLinks(manifest) {
  assert.deepEqual(
    {
      repository: manifest.repository,
      homepage: manifest.homepage,
      bugs: manifest.bugs,
    },
    PROJECT_LINKS,
  );
}

async function readManifest(filename) {
  return JSON.parse(await readFile(filename, 'utf8'));
}
