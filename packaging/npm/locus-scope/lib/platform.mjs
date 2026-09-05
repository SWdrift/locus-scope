import { lstat, readFile, realpath } from 'node:fs/promises';
import { createRequire } from 'node:module';
import path from 'node:path';

const requireFromPackage = createRequire(import.meta.url);
const SUPPORTED_HOSTS = new Map([
  ['darwin/arm64', '@locus/scope-darwin-arm64'],
  ['darwin/x64', '@locus/scope-darwin-x64'],
  ['linux/arm64', '@locus/scope-linux-arm64'],
  ['linux/x64', '@locus/scope-linux-x64'],
  ['win32/x64', '@locus/scope-win32-x64'],
]);

export function platformPackageName(platform, architecture) {
  const packageName = SUPPORTED_HOSTS.get(`${platform}/${architecture}`);
  if (!packageName) {
    throw new Error(
      `unsupported platform ${platform}/${architecture}; supported platforms are ${[...SUPPORTED_HOSTS.keys()].join(', ')}`,
    );
  }
  return packageName;
}

export async function locatePlatformHost({
  platform = process.platform,
  architecture = process.arch,
  resolvePackageJson = (specifier) => requireFromPackage.resolve(specifier),
} = {}) {
  const packageName = platformPackageName(platform, architecture);
  let packageJson;
  try {
    packageJson = resolvePackageJson(`${packageName}/package.json`);
  } catch (error) {
    throw new Error(
      `required platform package ${packageName} is not installed; reinstall @locus/scope with optional dependencies enabled`,
      { cause: error },
    );
  }

  const [adapterManifest, platformManifest] = await Promise.all([
    readManifest(new URL('../package.json', import.meta.url)),
    readManifest(packageJson),
  ]);
  const expectedVersion = adapterManifest.optionalDependencies?.[packageName];
  if (
    platformManifest.name !== packageName ||
    expectedVersion !== adapterManifest.version ||
    platformManifest.version !== expectedVersion ||
    !Array.isArray(platformManifest.os) ||
    platformManifest.os.length !== 1 ||
    platformManifest.os[0] !== platform ||
    !Array.isArray(platformManifest.cpu) ||
    platformManifest.cpu.length !== 1 ||
    platformManifest.cpu[0] !== architecture
  ) {
    throw new Error(
      `platform package ${packageName} does not match @locus/scope ${adapterManifest.version} for ${platform}/${architecture}`,
    );
  }

  const packageRoot = await realpath(path.dirname(packageJson));
  const executableName = platform === 'win32' ? 'locus-scope-node-host.exe' : 'locus-scope-node-host';
  const host = path.join(packageRoot, 'bin', executableName);
  let hostInfo;
  try {
    hostInfo = await lstat(host);
  } catch (error) {
    throw new Error(`platform package ${packageName} is missing ${path.posix.join('bin', executableName)}`, {
      cause: error,
    });
  }
  if (!hostInfo.isFile()) {
    throw new Error(`platform package ${packageName} host is not a regular file`);
  }
  return realpath(host);
}

async function readManifest(filename) {
  let source;
  try {
    source = await readFile(filename, 'utf8');
  } catch (error) {
    throw new Error(`read package manifest ${JSON.stringify(String(filename))}: ${error.message}`, {
      cause: error,
    });
  }
  try {
    const manifest = JSON.parse(source);
    if (manifest === null || typeof manifest !== 'object' || Array.isArray(manifest)) {
      throw new Error('expected a JSON object');
    }
    return manifest;
  } catch (error) {
    throw new Error(`parse package manifest ${JSON.stringify(String(filename))}: ${error.message}`, {
      cause: error,
    });
  }
}
