import assert from 'node:assert/strict';
import test from 'node:test';

import { buildNpmPublishArgs } from '../../../../scripts/npm-publish.mjs';

const TARBALL = 'temp/release/npm/sundw-locus-scope-2.0.0.tgz';

test('release publishing defaults to the official npm registry', () => {
  assert.deepEqual(buildNpmPublishArgs(TARBALL, []), [
    'publish',
    TARBALL,
    '--registry',
    'https://registry.npmjs.org/',
    '--access',
    'public',
    '--no-git-checks',
  ]);
});

test('an explicit registry overrides the official release default', () => {
  assert.deepEqual(buildNpmPublishArgs(TARBALL, ['--', '--registry', 'http://127.0.0.1:4873/', '--tag', 'next']), [
    'publish',
    TARBALL,
    '--registry',
    'http://127.0.0.1:4873/',
    '--tag',
    'next',
    '--access',
    'public',
    '--no-git-checks',
  ]);
  assert.deepEqual(buildNpmPublishArgs(TARBALL, ['--registry=http://127.0.0.1:4873/']), [
    'publish',
    TARBALL,
    '--registry=http://127.0.0.1:4873/',
    '--access',
    'public',
    '--no-git-checks',
  ]);
});
