import { readdir, rm } from 'node:fs/promises'
import { join } from 'node:path'
import {
  assertOrdinaryDirectory,
  isMain,
  parseUserTargetArguments,
  pathState,
  removeOwnedDirectory,
  resolveUserLocusRoot,
  tempRoot
} from './lib/workspace.mjs'


async function removeUserBinaries(override) {
  const locusRoot = resolveUserLocusRoot(override)
  const binaryRoot = join(locusRoot, 'bin')
  await assertOrdinaryDirectory(locusRoot, 'user Locus root')
  await assertOrdinaryDirectory(binaryRoot, 'user binary path')

  for (const fileName of ['locus-scope', 'locus-scope.exe', 'locus-pkg', 'locus-pkg.exe']) {
    const target = join(binaryRoot, fileName)
    const state = await pathState(target)
    if (state === null) continue
    if (!state.isFile() || state.isSymbolicLink()) {
      throw new Error(`refusing to remove non-ordinary file: ${target}`)
    }
    await rm(target)
    console.log(`Removed ${target}`)
  }

  if ((await pathState(binaryRoot)) !== null && (await readdir(binaryRoot)).length === 0) await rm(binaryRoot)
  if ((await pathState(locusRoot)) !== null && (await readdir(locusRoot)).length === 0) await rm(locusRoot)
}

export async function clean({ user = false, userLocusRoot } = {}) {
  if (user) {
    await removeUserBinaries(userLocusRoot)
    return
  }
  for (const path of [join(tempRoot, 'build'), join(tempRoot, 'local')]) {
    if (await removeOwnedDirectory(path, 'cleanup target')) console.log(`Removed ${path}`)
  }
}

if (isMain(import.meta.url)) {
  clean(parseUserTargetArguments()).catch(error => {
    console.error(error.message)
    process.exitCode = 1
  })
}
