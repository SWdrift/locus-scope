import { join } from 'node:path'
import { assertOrdinaryFile, isMain, readVersion, repositoryRoot, run } from './lib/workspace.mjs'

const officialNpmRegistry = 'https://registry.npmjs.org/'
const tarballStems = ['sundw-locus-scope']

export function buildNpmPublishArgs(tarball, args) {
  const publishArgs = args[0] === '--' ? args.slice(1) : args
  const hasRegistryOverride = publishArgs.some(arg => arg === '--registry' || arg.startsWith('--registry='))
  return ['publish', tarball, ...(hasRegistryOverride ? [] : ['--registry', officialNpmRegistry]), ...publishArgs, '--access', 'public']
}

export async function publishNpmPackages(args = process.argv.slice(2)) {
  const version = await readVersion()
  const releaseRoot = join(repositoryRoot, 'temp', 'release', 'npm')
  const tarballs = tarballStems.map(stem => join(releaseRoot, `${stem}-${version}.tgz`))
  for (const tarball of tarballs) await assertOrdinaryFile(tarball, 'npm release tarball')
  for (const tarball of tarballs) {
    run(process.platform === 'win32' ? 'npm.cmd' : 'npm', buildNpmPublishArgs(tarball, args), {
      shell: process.platform === 'win32'
    })
  }
}

if (isMain(import.meta.url)) {
  publishNpmPackages().catch(error => {
    console.error(error.message)
    process.exitCode = 1
  })
}
