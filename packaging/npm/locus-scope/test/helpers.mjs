import { mkdir, mkdtemp, rm, symlink, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const TEST_STATE = fileURLToPath(new URL('../../../temp/node-adapter-tests/', import.meta.url));

export async function testDirectory(t, prefix) {
  await mkdir(TEST_STATE, { recursive: true });
  const directory = await mkdtemp(path.join(TEST_STATE, `${prefix}-`));
  t.after(() => rm(directory, { recursive: true, force: true }));
  return directory;
}

export async function writeJson(filename, value) {
  await mkdir(path.dirname(filename), { recursive: true });
  await writeFile(filename, `${JSON.stringify(value, null, 2)}\n`);
}

export async function writePackage(root, manifest) {
  await mkdir(root, { recursive: true });
  await writeJson(path.join(root, 'package.json'), manifest);
  await writeFile(path.join(root, 'index.js'), 'module.exports = {};\n');
  if (manifest.locus?.entry) {
    const entry = path.join(root, ...manifest.locus.entry.replace(/^\.\//, '').split('/'));
    await mkdir(path.dirname(entry), { recursive: true });
    await writeFile(entry, `id: ${manifest.name.replace(/[^a-z0-9]+/gi, '-')}\n`);
  }
}

export async function linkDirectory(target, link) {
  await mkdir(path.dirname(link), { recursive: true });
  await symlink(target, link, process.platform === 'win32' ? 'junction' : 'dir');
}

export function locusManifest(name, version, dependencies = {}) {
  return {
    name,
    version,
    main: './index.js',
    exports: {
      '.': './index.js',
      './package.json': './package.json',
    },
    dependencies,
    locus: { entry: 'locus.yaml' },
  };
}

export function semanticDescriptor(environment) {
  return {
    root: {
      dependencies: environment.root.dependencies,
      hasPackageRoot: environment.root.packageRoot !== '',
    },
    packages: Object.fromEntries(
      Object.entries(environment.packages).map(([identity, descriptor]) => [
        identity,
        { entry: descriptor.entry, dependencies: descriptor.dependencies },
      ]),
    ),
  };
}

export function captureStream() {
  let value = '';
  return {
    write(chunk) {
      value += String(chunk);
      return true;
    },
    value() {
      return value;
    },
  };
}
