import { stat, realpath } from 'node:fs/promises';
import path from 'node:path';

const SCOPE_MANIFESTS = ['locus.yaml', 'locus.yml', 'locus.json'];

export function parseRootOptions(arguments_) {
  const argumentsList = [];
  let scopeDirectory = '';

  for (let index = 0; index < arguments_.length; index += 1) {
    const argument = arguments_[index];
    if (argument === '--scope') {
      if (index + 1 === arguments_.length) {
        throw new Error('--scope requires a directory');
      }
      scopeDirectory = arguments_[index + 1];
      if (!scopeDirectory) {
        throw new Error('--scope requires a directory');
      }
      index += 1;
      continue;
    }
    if (argument.startsWith('--scope=')) {
      scopeDirectory = argument.slice('--scope='.length);
      if (!scopeDirectory) {
        throw new Error('--scope requires a directory');
      }
      continue;
    }
    argumentsList.push(argument);
  }

  return { scopeDirectory, arguments: argumentsList };
}

export async function discoverScopeRoot(workingDirectory, explicitDirectory = '') {
  const absoluteWorkingDirectory = path.resolve(workingDirectory);
  const workingRoot = await requireDirectory(absoluteWorkingDirectory, 'working directory');
  if (explicitDirectory) {
    const explicitRoot = await requireDirectory(
      path.resolve(absoluteWorkingDirectory, explicitDirectory),
      '--scope directory',
    );
    if (!(await containsScopeManifest(explicitRoot))) {
      throw new Error(`no Scope manifest found in --scope directory ${JSON.stringify(explicitRoot)}`);
    }
    return explicitRoot;
  }

  let current = workingRoot;
  while (true) {
    if (await containsScopeManifest(current)) {
      return current;
    }
    const parent = path.dirname(current);
    if (parent === current) {
      throw new Error(`no Scope manifest found from ${JSON.stringify(workingRoot)} to the filesystem root`);
    }
    current = parent;
  }
}

async function requireDirectory(candidate, label) {
  let info;
  try {
    info = await stat(candidate);
  } catch (error) {
    throw new Error(`${label} ${JSON.stringify(candidate)} is not accessible: ${error.message}`, {
      cause: error,
    });
  }
  if (!info.isDirectory()) {
    throw new Error(`${label} ${JSON.stringify(candidate)} is not a directory`);
  }
  return realpath(candidate);
}

async function containsScopeManifest(directory) {
  for (const name of SCOPE_MANIFESTS) {
    try {
      if ((await stat(path.join(directory, name))).isFile()) {
        return true;
      }
    } catch (error) {
      if (error.code !== 'ENOENT' && error.code !== 'ENOTDIR') {
        throw new Error(`inspect Scope manifest ${JSON.stringify(path.join(directory, name))}: ${error.message}`, {
          cause: error,
        });
      }
    }
  }
  return false;
}
