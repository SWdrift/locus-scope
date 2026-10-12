import assert from 'node:assert/strict'
import { join } from 'node:path'
import {
  assertOrdinaryFile,
  ensureOrdinaryDirectory,
  isMain,
  readVersion,
  run,
  tempRoot,
  writeJson
} from './lib/workspace.mjs'
import { mkdir, rm, writeFile } from 'node:fs/promises'

export async function verifyNpmRelease(version) {
  if (!/^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(version)) throw new Error(`invalid release version: ${version}`)
  const releaseRoot = join(tempRoot, 'release', 'npm')
  const tarball = join(releaseRoot, `sundw-locus-scope-${version}.tgz`)
  await assertOrdinaryFile(tarball, 'npm release tarball')

  const validationRoot = join(tempRoot, 'release-validation', 'npm')
  await ensureOrdinaryDirectory(join(tempRoot, 'release-validation'), 'release validation path')
  await rm(validationRoot, { recursive: true, force: true })
  await mkdir(validationRoot, { recursive: true })
  await writeJson(join(validationRoot, 'package.json'), { private: true })
  await writeFile(join(validationRoot, 'pnpm-workspace.yaml'), 'packages:\n  - "."\n', 'utf8')
  await writeFile(join(validationRoot, 'locus.yaml'), 'id: release-validation\n', 'utf8')

  const pnpmEntrypoint = process.env.npm_execpath
  if (!pnpmEntrypoint) throw new Error('pnpm entrypoint is unavailable; run release through pnpm run verify:npm-release')
  run(process.execPath, [pnpmEntrypoint, '--dir', validationRoot, 'add', '--ignore-scripts', '--save-exact', tarball])
  const result = run(process.execPath, [pnpmEntrypoint, '--dir', validationRoot, 'exec', 'locus-scope', 'validate', '--json'], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'inherit'] })
  const validation = JSON.parse(result.stdout)
  assert.equal(validation.valid, true, `release validate result: ${result.stdout}`)
  assert.match(validation.root, /^file:\/\//, `release validate result: ${result.stdout}`)
  const library = run(process.execPath, [
    '--input-type=module',
    '-e',
    'import { ApplicationService, buildPackageEnvironment, loadPackageEnvironment } from "@sundw/locus-scope"; const environment = await buildPackageEnvironment(process.cwd()); const result = new ApplicationService(await loadPackageEnvironment(environment)).validate(); if (!result.valid) process.exit(1); console.log(JSON.stringify(result));'
  ], { cwd: validationRoot, encoding: 'utf8', stdio: ['ignore', 'pipe', 'inherit'] })
  assert.match(library.stdout, /"valid":true/, `library validate result: ${library.stdout}`)
}

if (isMain(import.meta.url)) {
  verifyNpmRelease(process.argv[2] || await readVersion()).catch(error => {
    console.error(error.message)
    process.exitCode = 1
  })
}
