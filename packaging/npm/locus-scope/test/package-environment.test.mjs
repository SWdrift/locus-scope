import assert from 'node:assert/strict';
import { realpath, writeFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { buildPackageEnvironment } from '../lib/package-environment.mjs';
import {
  linkDirectory,
  locusManifest,
  semanticDescriptor,
  testDirectory,
  writeJson,
  writePackage,
} from './helpers.mjs';

const EXPECTED_SEMANTICS = {
  root: {
    dependencies: {
      '@example/app': 'npm:@example/app@1.0.0',
      '@example/modern': 'npm:@example/modern@1.0.0',
    },
    hasPackageRoot: true,
  },
  packages: {
    'npm:@example/app@1.0.0': {
      entry: 'locus.yaml',
      dependencies: { '@example/base': 'npm:@example/base@1.1.0' },
    },
    'npm:@example/base@1.1.0': { entry: 'locus.yaml', dependencies: {} },
    'npm:@example/base@2.0.0': { entry: 'locus.yaml', dependencies: {} },
    'npm:@example/modern@1.0.0': {
      entry: 'locus.yaml',
      dependencies: { '@example/base': 'npm:@example/base@2.0.0' },
    },
  },
};

test('hoisted and pnpm symlink layouts produce the same importer-relative descriptor', async (t) => {
  const hoisted = await materializeHoisted(t);
  const pnpm = await materializePnpm(t);

  const hoistedEnvironment = await buildPackageEnvironment(hoisted);
  const pnpmEnvironment = await buildPackageEnvironment(pnpm);

  assert.deepEqual(semanticDescriptor(hoistedEnvironment), EXPECTED_SEMANTICS);
  assert.deepEqual(semanticDescriptor(pnpmEnvironment), EXPECTED_SEMANTICS);
  assert.deepEqual(
    Object.keys(pnpmEnvironment.packages),
    [
      'npm:@example/app@1.0.0',
      'npm:@example/base@1.1.0',
      'npm:@example/base@2.0.0',
      'npm:@example/modern@1.0.0',
    ],
    'package identities are emitted deterministically',
  );
});

test('only a package.json in the exact Scope root becomes the root importer', async (t) => {
  const parent = await testDirectory(t, 'exact-importer');
  await writeJson(path.join(parent, 'package.json'), {
    name: 'parent',
    version: '1.0.0',
    dependencies: { '@example/app': '*' },
  });
  const root = path.join(parent, 'nested-scope');
  await writePackage(
    path.join(parent, 'node_modules', '@example', 'app'),
    locusManifest('@example/app', '1.0.0'),
  );
  await writeJson(path.join(root, 'locus.json'), {});

  assert.deepEqual(await buildPackageEnvironment(root), {
    root: { scopeRoot: await realpath(root), packageRoot: '', dependencies: {} },
    packages: {},
  });
});

test('duplicate semantic identities collapse to the lexical real path only when edges agree', async (t) => {
  const root = await testDirectory(t, 'duplicates');
  await writeFile(path.join(root, 'locus.yaml'), 'id: root\n');
  await writeJson(path.join(root, 'package.json'), {
    name: 'consumer',
    version: '1.0.0',
    dependencies: { '@example/left': '*', '@example/right': '*' },
  });

  const left = path.join(root, 'node_modules', '@example', 'left');
  const right = path.join(root, 'node_modules', '@example', 'right');
  await writePackage(left, locusManifest('@example/left', '1.0.0', { '@example/shared': '*' }));
  await writePackage(right, locusManifest('@example/right', '1.0.0', { '@example/shared': '*' }));

  const leftShared = path.join(left, 'node_modules', '@example', 'shared');
  const rightShared = path.join(right, 'node_modules', '@example', 'shared');
  await writePackage(leftShared, locusManifest('@example/shared', '1.0.0'));
  await writePackage(rightShared, locusManifest('@example/shared', '1.0.0'));

  const environment = await buildPackageEnvironment(root);
  const expectedRoot = [await realpath(leftShared), await realpath(rightShared)].sort()[0];
  assert.equal(environment.packages['npm:@example/shared@1.0.0'].root, expectedRoot);

  await writePackage(
    rightShared,
    locusManifest('@example/shared', '1.0.0', { '@example/leaf': '*' }),
  );
  await writePackage(
    path.join(rightShared, 'node_modules', '@example', 'leaf'),
    locusManifest('@example/leaf', '1.0.0'),
  );

  await assert.rejects(
    buildPackageEnvironment(root),
    /conflicting npm package copies for npm:@example\/shared@1\.0\.0/,
  );
});

test('a Locus package blocked by exports fails before host launch', async (t) => {
  const root = await testDirectory(t, 'blocked-exports');
  await writeFile(path.join(root, 'locus.yaml'), 'id: root\n');
  await writeJson(path.join(root, 'package.json'), {
    name: 'consumer',
    version: '1.0.0',
    dependencies: { '@example/blocked': '*' },
  });
  const blocked = path.join(root, 'node_modules', '@example', 'blocked');
  await writePackage(blocked, {
    name: '@example/blocked',
    version: '1.0.0',
    exports: { '.': './dist/index.js' },
    locus: { entry: 'locus.yaml' },
  });
  await writeJson(path.join(blocked, 'dist', 'package.json'), {
    name: '@example/nested',
    version: '1.0.0',
  });
  await writeFile(path.join(blocked, 'dist', 'index.js'), 'module.exports = {};\n');

  await assert.rejects(
    buildPackageEnvironment(root),
    /Locus package "@example\/blocked" must export "\.\/package\.json"/,
  );
});

async function materializeHoisted(t) {
  const root = await testDirectory(t, 'hoisted');
  await writeFile(path.join(root, 'locus.yaml'), 'id: root\n');
  await writeJson(path.join(root, 'package.json'), {
    name: 'consumer',
    version: '1.0.0',
    dependencies: {
      '@example/app': '^1.0.0',
      '@example/helper': '^1.0.0',
      '@example/modern': '^1.0.0',
    },
  });
  const modules = path.join(root, 'node_modules', '@example');
  await writePackage(
    path.join(modules, 'app'),
    locusManifest('@example/app', '1.0.0', {
      '@example/base': '^1.0.0',
      '@example/helper': '^1.0.0',
    }),
  );
  await writePackage(path.join(modules, 'base'), locusManifest('@example/base', '1.1.0'));
  const modern = path.join(modules, 'modern');
  await writePackage(
    modern,
    locusManifest('@example/modern', '1.0.0', { '@example/base': '^2.0.0' }),
  );
  await writePackage(
    path.join(modern, 'node_modules', '@example', 'base'),
    locusManifest('@example/base', '2.0.0'),
  );
  await writePackage(path.join(modules, 'helper'), {
    name: '@example/helper',
    version: '1.0.0',
    main: './index.js',
  });
  return root;
}

async function materializePnpm(t) {
  const root = await testDirectory(t, 'pnpm');
  await writeFile(path.join(root, 'locus.yaml'), 'id: root\n');
  await writeJson(path.join(root, 'package.json'), {
    name: 'consumer',
    version: '1.0.0',
    dependencies: {
      '@example/app': '^1.0.0',
      '@example/helper': '^1.0.0',
      '@example/modern': '^1.0.0',
    },
  });

  const store = path.join(root, '.store');
  const app = path.join(store, 'app', 'node_modules', '@example', 'app');
  const base = path.join(store, 'base-1', 'node_modules', '@example', 'base');
  const modern = path.join(store, 'modern', 'node_modules', '@example', 'modern');
  const modernBase = path.join(store, 'base-2', 'node_modules', '@example', 'base');
  const helper = path.join(store, 'helper', 'node_modules', '@example', 'helper');
  await writePackage(
    app,
    locusManifest('@example/app', '1.0.0', {
      '@example/base': '^1.0.0',
      '@example/helper': '^1.0.0',
    }),
  );
  await writePackage(base, locusManifest('@example/base', '1.1.0'));
  await writePackage(
    modern,
    locusManifest('@example/modern', '1.0.0', { '@example/base': '^2.0.0' }),
  );
  await writePackage(modernBase, locusManifest('@example/base', '2.0.0'));
  await writePackage(helper, {
    name: '@example/helper',
    version: '1.0.0',
    main: './index.js',
  });

  await linkDirectory(app, path.join(root, 'node_modules', '@example', 'app'));
  await linkDirectory(modern, path.join(root, 'node_modules', '@example', 'modern'));
  await linkDirectory(helper, path.join(root, 'node_modules', '@example', 'helper'));
  await linkDirectory(base, path.join(store, 'app', 'node_modules', '@example', 'base'));
  await linkDirectory(helper, path.join(store, 'app', 'node_modules', '@example', 'helper'));
  await linkDirectory(
    modernBase,
    path.join(store, 'modern', 'node_modules', '@example', 'base'),
  );
  return root;
}
