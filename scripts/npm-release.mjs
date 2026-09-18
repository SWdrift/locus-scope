import { chmod, cp, mkdir, readFile, writeFile } from 'node:fs/promises'
import { basename, join } from 'node:path'
import {
  ensureOrdinaryDirectory,
  isMain,
  readVersion,
  removeOwnedDirectory,
  repositoryRoot,
  run,
  tempRoot,
  writeJson
} from './lib/workspace.mjs'

const platforms = [
  ['locus-scope-win32-x64', '@sundw/locus-scope-win32-x64', 'windows', 'amd64', 'win32', 'x64', 'locus-scope-node-host.exe'],
  ['locus-scope-linux-x64', '@sundw/locus-scope-linux-x64', 'linux', 'amd64', 'linux', 'x64', 'locus-scope-node-host'],
  ['locus-scope-linux-arm64', '@sundw/locus-scope-linux-arm64', 'linux', 'arm64', 'linux', 'arm64', 'locus-scope-node-host'],
  ['locus-scope-darwin-x64', '@sundw/locus-scope-darwin-x64', 'darwin', 'amd64', 'darwin', 'x64', 'locus-scope-node-host'],
  ['locus-scope-darwin-arm64', '@sundw/locus-scope-darwin-arm64', 'darwin', 'arm64', 'darwin', 'arm64', 'locus-scope-node-host']
]

export async function releaseNpm() {
  const version = await readVersion()
  const releaseRoot = join(tempRoot, 'release', 'npm')
  const stageRoot = join(releaseRoot, 'stage')
  await ensureOrdinaryDirectory(join(tempRoot, 'release'), 'release path')
  await removeOwnedDirectory(releaseRoot, 'npm release')
  await mkdir(stageRoot, { recursive: true })
  const license = await readFile(join(repositoryRoot, 'LICENSE'))

  for (const [directory, name, goos, goarch, npmOS, npmCPU, binary] of platforms) {
    const source = join(repositoryRoot, 'packaging', 'npm', directory)
    const stage = join(stageRoot, directory)
    await cp(source, stage, { recursive: true, filter: path => basename(path) !== 'node_modules' })
    await writeFile(join(stage, 'LICENSE'), license)
    const manifest = JSON.parse(await readFile(join(stage, 'package.json'), 'utf8'))
    delete manifest.private
    manifest.version = version
    if (npmOS !== 'win32') manifest.bin = { 'locus-scope-node-host': `bin/${binary}` }
    await writeJson(join(stage, 'package.json'), manifest)
    const target = join(stage, 'bin', binary)
    await mkdir(join(stage, 'bin'))
    run('go', ['build', '-trimpath', '-ldflags', `-X locus-scope/internal/buildinfo.Version=${version}`, '-o', target, './cmd/locus-scope-node-host'], {
      env: { ...process.env, GOOS: goos, GOARCH: goarch, CGO_ENABLED: '0' }
    })
    if (npmOS !== 'win32') await chmod(target, 0o755)
    pack(stage, releaseRoot)
    if (manifest.name !== name || manifest.os[0] !== npmOS || manifest.cpu[0] !== npmCPU) throw new Error(`invalid platform manifest: ${name}`)
  }

  const adapter = join(stageRoot, 'locus-scope')
  await cp(join(repositoryRoot, 'packaging', 'npm', 'locus-scope'), adapter, {
    recursive: true,
    filter: path => basename(path) !== 'node_modules'
  })
  await writeFile(join(adapter, 'LICENSE'), license)
  await cp(join(repositoryRoot, 'README.md'), join(adapter, 'README.md'))
  const manifest = JSON.parse(await readFile(join(adapter, 'package.json'), 'utf8'))
  delete manifest.private
  manifest.version = version
  for (const name of Object.keys(manifest.optionalDependencies)) manifest.optionalDependencies[name] = version
  await writeJson(join(adapter, 'package.json'), manifest)
  pack(adapter, releaseRoot)
}

function pack(cwd, destination) {
  const pnpmEntrypoint = process.env.npm_execpath
  if (!pnpmEntrypoint) throw new Error('pnpm entrypoint is unavailable; run through pnpm run release:npm')
  run(process.execPath, [pnpmEntrypoint, 'pack', '--pack-destination', destination], { cwd })
}

if (isMain(import.meta.url)) {
  releaseNpm().catch(error => {
    console.error(error.message)
    process.exitCode = 1
  })
}
