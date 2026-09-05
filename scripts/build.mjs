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

  const ldflags = `-X locus-scope/internal/buildinfo.Version=${version}`
  for (const [name, packagePath] of [
    ['locus-scope', './cmd/locus-scope'],
    ['locus-pkg', './cmd/locus-pkg']
  ]) {
    const outputPath = join(artifactRoot, `${name}${extension}`)
    run('go', ['build', '-trimpath', '-ldflags', ldflags, '-o', outputPath, packagePath])
    if (!quiet) console.log(`Built ${outputPath}`)
  }

  return { artifactRoot, extension, version }
}

function runGoEnv(name) {
  const result = run('go', ['env', name], {
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'inherit']
  })
  return result.stdout.trim()
}

if (isMain(import.meta.url)) {
  build().catch(error => {
    console.error(error.message)
    process.exitCode = 1
  })
}
