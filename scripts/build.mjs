import { mkdir } from 'node:fs/promises'
import { join } from 'node:path'
import {
  assertOrdinaryDirectory,
  ensureOrdinaryDirectory,
  isMain,
  readVersion,
  removeOwnedDirectory,
  run,
  tempRoot
} from './lib/workspace.mjs'

export async function build({ quiet = false } = {}) {
  const version = await readVersion()
  const goos = process.env.GOOS || runGoEnv('GOOS')
  const goarch = process.env.GOARCH || runGoEnv('GOARCH')
  const extension = goos === 'windows' ? '.exe' : ''
  const buildRoot = join(tempRoot, 'build')
  const artifactRoot = join(buildRoot, `${goos}-${goarch}`)

  await assertOrdinaryDirectory(tempRoot, 'build path')
  await assertOrdinaryDirectory(buildRoot, 'build path')
  await ensureOrdinaryDirectory(buildRoot, 'build path')
  await removeOwnedDirectory(artifactRoot, 'build output')
  await mkdir(artifactRoot)

  const outputPath = join(artifactRoot, `locus-scope-node-host${extension}`)
  run('go', ['build', '-trimpath', '-ldflags', `-X locus-scope/internal/buildinfo.Version=${version}`, '-o', outputPath, './cmd/locus-scope-node-host'])
  if (!quiet) console.log(`Built ${outputPath}`)
  return { artifactRoot, extension, outputPath, version }
}

function runGoEnv(name) {
  return run('go', ['env', name], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'inherit'] }).stdout.trim()
}

if (isMain(import.meta.url)) {
  build().catch(error => {
    console.error(error.message)
    process.exitCode = 1
  })
}
