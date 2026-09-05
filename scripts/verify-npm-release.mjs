import assert from 'node:assert/strict'
import { readFile, rm, writeFile } from 'node:fs/promises'
import { gunzipSync } from 'node:zlib'
import { join } from 'node:path'
import {
  assertOrdinaryDirectory,
  assertOrdinaryFile,
  ensureOrdinaryDirectory,
  isMain,
  run,
  tempRoot,
  writeJson
} from './lib/workspace.mjs'

const unixPlatforms = ['darwin-arm64', 'darwin-x64', 'linux-arm64', 'linux-x64']

export async function verifyNpmRelease(version) {
  if (!/^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(version)) {
    throw new Error(`invalid release version: ${version}`)
  }

  const releaseRoot = join(tempRoot, 'release', 'npm')
  const tarball = suffix => join(releaseRoot, `sundw-locus-scope-${suffix}-${version}.tgz`)
  for (const platform of unixPlatforms) {
    await assertTarEntryMode(tarball(platform), 'package/bin/locus-scope-node-host', 0o755)
  }

  const validationRoot = join(tempRoot, 'release-validation', 'npm')
  await assertOrdinaryDirectory(tempRoot, 'release validation path')
  await ensureOrdinaryDirectory(join(tempRoot, 'release-validation'), 'release validation path')
  await assertOrdinaryDirectory(validationRoot, 'npm release validation')
  await rm(validationRoot, { recursive: true, force: true })
  await ensureOrdinaryDirectory(validationRoot, 'npm release validation')
  await writeJson(join(validationRoot, 'package.json'), { private: true })
  await writeFile(join(validationRoot, 'pnpm-workspace.yaml'), 'packages:\n  - \".\"\n', 'utf8')
  await writeFile(join(validationRoot, 'locus.yaml'), 'id: release-validation\n', 'utf8')

  const pnpmEntrypoint = process.env.npm_execpath
  if (!pnpmEntrypoint) {
    throw new Error('pnpm entrypoint is unavailable; run release through: pnpm run release')
  }
  const platform = `${process.platform}-${process.arch}`
  const platformTarball = tarball(platform)
  const adapterTarball = join(releaseRoot, `sundw-locus-scope-${version}.tgz`)
  await assertOrdinaryFile(platformTarball, 'current platform npm release tarball')
  await assertOrdinaryFile(adapterTarball, 'npm adapter release tarball')

  run(process.execPath, [
    pnpmEntrypoint,
    '--dir',
    validationRoot,
    'add',
    '--ignore-scripts',
    '--save-exact',
    adapterTarball,
    platformTarball
  ])
  const result = run(process.execPath, [
    pnpmEntrypoint,
    '--dir',
    validationRoot,
    'exec',
    'locus-scope-node',
    'validate',
    '--json'
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'inherit'] })
  const validation = JSON.parse(result.stdout)
  assert.equal(validation.valid, true, `release validate result: ${result.stdout}`)
  assert.match(validation.root, /^file:\/\//, `release validate result: ${result.stdout}`)
}

export async function assertTarEntryMode(tarball, entryName, expectedMode) {
  await assertOrdinaryFile(tarball, 'npm release tarball')
  const archive = gunzipSync(await readFile(tarball))
  const entry = findTarEntry(archive, entryName)
  if (!entry) throw new Error(`${tarball} is missing ${entryName}`)
  if ((entry.mode & 0o777) !== expectedMode) {
    throw new Error(`${tarball} entry ${entryName} has mode ${formatMode(entry.mode)}, expected ${formatMode(expectedMode)}`)
  }
}

function findTarEntry(archive, expectedName) {
  for (let offset = 0; offset + 512 <= archive.length;) {
    const header = archive.subarray(offset, offset + 512)
    if (header.every(byte => byte === 0)) return null
    const name = readTarString(header, 0, 100)
    const prefix = readTarString(header, 345, 155)
    const fullName = prefix ? `${prefix}/${name}` : name
    const mode = readTarOctal(header, 100, 8, 'mode', fullName)
    const size = readTarOctal(header, 124, 12, 'size', fullName)
    if (fullName === expectedName) return { mode }
    offset += 512 + Math.ceil(size / 512) * 512
  }
  return null
}

function readTarString(header, offset, length) {
  const end = header.indexOf(0, offset)
  return header.toString('utf8', offset, end === -1 || end > offset + length ? offset + length : end)
}

function readTarOctal(header, offset, length, field, entryName) {
  const value = readTarString(header, offset, length).trim()
  if (!/^[0-7]+$/.test(value)) throw new Error(`invalid tar ${field} for ${entryName}`)
  return Number.parseInt(value, 8)
}

function formatMode(mode) {
  return `0${(mode & 0o777).toString(8)}`
}

if (isMain(import.meta.url)) {
  verifyNpmRelease(process.argv[2] ?? '').catch(error => {
    console.error(error.message)
    process.exitCode = 1
  })
}
