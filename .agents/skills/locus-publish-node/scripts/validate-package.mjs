#!/usr/bin/env node

import { lstat, readFile, realpath } from 'node:fs/promises';
import path from 'node:path';

const root = path.resolve(process.argv[2] ?? '.');
const manifestPath = path.join(root, 'package.json');
const errors = [];

let manifest;
try {
  manifest = JSON.parse(await readFile(manifestPath, 'utf8'));
} catch (error) {
  console.error(`Invalid package.json: ${error.message}`);
  process.exit(1);
}

const packageName = /^(?:@[a-z0-9][a-z0-9._-]*\/[a-z0-9][a-z0-9._-]*|[a-z0-9][a-z0-9._-]*)$/;
const version = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;

if (typeof manifest.name !== 'string' || !packageName.test(manifest.name)) {
  errors.push('package.json name must be a lowercase npm package name');
}
if (typeof manifest.version !== 'string' || !version.test(manifest.version)) {
  errors.push('package.json version must be a valid SemVer version');
}
if (manifest.private === true) {
  errors.push('package.json private must not be true');
}

if (!Array.isArray(manifest.files) || manifest.files.length === 0) {
  errors.push('package.json files must be a non-empty array');
} else {
  for (const file of manifest.files) {
    if (
      typeof file !== 'string' ||
      file.length === 0 ||
      path.isAbsolute(file) ||
      file.includes('\\') ||
      file.split('/').some((part) => part === '' || part === '.' || part === '..') ||
      /[*?{}[\]]/.test(file)
    ) {
      errors.push(`files entry must be a literal relative path: ${JSON.stringify(file)}`);
    }
  }
}

const entry = manifest.locus?.entry;
if (
  typeof entry !== 'string' ||
  !/(?:^|\/)locus\.(?:yaml|yml|json)$/.test(entry) ||
  path.isAbsolute(entry) ||
  entry.includes('\\') ||
  entry.split('/').some((part) => part === '' || part === '.' || part === '..')
) {
  errors.push('locus.entry must be a relative path ending in locus.yaml, locus.yml, or locus.json');
} else {
  const included = Array.isArray(manifest.files) && manifest.files.some(
    (file) => typeof file === 'string' && (entry === file || entry.startsWith(`${file.replace(/\/$/, '')}/`)),
  );
  if (!included) {
    errors.push(`locus.entry is not included by package.json files: ${entry}`);
  }

  try {
    const entryPath = path.join(root, ...entry.split('/'));
    const [rootRealPath, entryRealPath, entryStat] = await Promise.all([
      realpath(root),
      realpath(entryPath),
      lstat(entryPath),
    ]);
    if (!entryStat.isFile() || entryStat.isSymbolicLink()) {
      errors.push(`locus.entry must be a regular file: ${entry}`);
    }
    if (entryRealPath !== rootRealPath && !entryRealPath.startsWith(`${rootRealPath}${path.sep}`)) {
      errors.push(`locus.entry resolves outside the package root: ${entry}`);
    }
  } catch (error) {
    errors.push(`cannot read locus.entry ${entry}: ${error.message}`);
  }
}

if (manifest.exports !== undefined) {
  if (
    manifest.exports === null ||
    typeof manifest.exports !== 'object' ||
    Array.isArray(manifest.exports) ||
    manifest.exports['./package.json'] !== './package.json'
  ) {
    errors.push('package.json exports must include "./package.json": "./package.json"');
  }
}

if (errors.length > 0) {
  for (const error of errors) console.error(`- ${error}`);
  process.exit(1);
}

console.log(`${manifest.name}@${manifest.version}: basic Locus package checks passed`);
