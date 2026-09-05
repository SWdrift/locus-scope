import { copyFile, mkdir, rename, rm } from 'node:fs/promises'
import { join } from 'node:path'
import { build } from './build.mjs'
import {
  assertOrdinaryDirectory,
  assertOrdinaryFile,
  ensureOrdinaryDirectory,
  isMain,
  parseUserTargetArguments,
  removeOwnedDirectory,
  resolveUserLocusRoot,
  tempRoot
} from './lib/workspace.mjs'


export async function deploy({ user = false, userLocusRoot } = {}) {
  const buildResult = await build({ quiet: true })
  let deploymentRoot

  if (user) {
    const locusRoot = resolveUserLocusRoot(userLocusRoot)
    await assertOrdinaryDirectory(locusRoot, 'user Locus root')
    deploymentRoot = join(locusRoot, 'bin')
    await ensureOrdinaryDirectory(deploymentRoot, 'deployment path')
  } else {
    const localRoot = join(tempRoot, 'local')
    deploymentRoot = join(localRoot, 'bin')
    await assertOrdinaryDirectory(tempRoot, 'deployment path')
    await assertOrdinaryDirectory(localRoot, 'deployment path')
    await ensureOrdinaryDirectory(localRoot, 'deployment path')
    await removeOwnedDirectory(deploymentRoot, 'deployment')
    await mkdir(deploymentRoot)
  }

  for (const name of ['locus-scope', 'locus-pkg']) {
    const fileName = `${name}${buildResult.extension}`
    const sourcePath = join(buildResult.artifactRoot, fileName)
    const destinationPath = join(deploymentRoot, fileName)
    await assertOrdinaryFile(sourcePath, 'build artifact')
    await assertOrdinaryFile(destinationPath, 'deployment file', false)

    if (!user) {
      await copyFile(sourcePath, destinationPath)
      continue
    }

    const temporaryPath = `${destinationPath}.deploy-${process.pid}`
    await assertOrdinaryFile(temporaryPath, 'temporary deployment file', false)
    try {
      await copyFile(sourcePath, temporaryPath)
      await rename(temporaryPath, destinationPath)
    } finally {
      await rm(temporaryPath, { force: true })
    }
  }

  console.log(`Deployed local CLI binaries to ${deploymentRoot}`)
}

if (isMain(import.meta.url)) {
  deploy(parseUserTargetArguments()).catch(error => {
    console.error(error.message)
    process.exitCode = 1
  })
}
