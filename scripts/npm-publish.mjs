import { join } from 'node:path'
import { assertOrdinaryFile, isMain, readVersion, repositoryRoot, run } from './lib/workspace.mjs'

const pnpmEntrypoint = process.env.npm_execpath

const tarballStems = [
  'sundw-locus-scope-win32-x64',
  'sundw-locus-scope-linux-x64',
  'sundw-locus-scope-linux-arm64',
  'sundw-locus-scope-darwin-x64',
  'sundw-locus-scope-darwin-arm64',
  'sundw-locus-scope'
]

export async function publishNpmPackages(args = process.argv.slice(2)) {
  const publishArgs = args[0] === '--' ? args.slice(1) : args
  const version = await readVersion()
  const releaseRoot = join(repositoryRoot, 'temp', 'release', 'npm')
  const tarballs = tarballStems.map(stem => join(releaseRoot, `${stem}-${version}.tgz`))

  for (const tarball of tarballs) await assertOrdinaryFile(tarball, 'npm release tarball')
  if (!pnpmEntrypoint) throw new Error('pnpm entrypoint is unavailable; run this command through: pnpm run publish:npm')
  for (const tarball of tarballs) {
    run(process.execPath, [
      pnpmEntrypoint,
      'publish',
      tarball,
      ...publishArgs,
      '--access',
      'public',
      '--no-git-checks'
    ])
  }
}

if (isMain(import.meta.url)) {
  publishNpmPackages().catch(error => {
    console.error(error.message)
    process.exitCode = 1
  })
}
