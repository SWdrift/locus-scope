import { lstat, mkdir, readdir, readFile, rm, writeFile } from 'node:fs/promises'
import { spawn, spawnSync } from 'node:child_process'
import { basename, dirname, isAbsolute, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..')
export const tempRoot = join(repositoryRoot, 'temp')

export function isMain(metaUrl) {
  return resolve(fileURLToPath(metaUrl)) === resolve(process.argv[1] ?? '')
}

export function parseUserTargetArguments(args = process.argv.slice(2)) {
  let user = false
  let userLocusRoot
  for (let index = 0; index < args.length; index += 1) {
    if (args[index] === '--user') {
      user = true
      continue
    }
    if (args[index] === '--user-locus-root') {
      if (index + 1 >= args.length) throw new Error('--user-locus-root requires a path')
      userLocusRoot = args[++index]
      continue
    }
    throw new Error(`unknown argument: ${args[index]}`)
  }
  if (userLocusRoot && !user) throw new Error('--user-locus-root requires --user')
  return { user, userLocusRoot }
}

export function resolveUserLocusRoot(override) {
  const candidate = override || (process.env.HOME ? join(process.env.HOME, '.locus') : '')
  if (!candidate) throw new Error('cannot resolve the user Locus directory because HOME is empty')
  const fullPath = resolve(candidate)
  if (basename(fullPath) !== '.locus') {
    throw new Error(`user Locus root must end with '.locus': ${fullPath}`)
  }
  return fullPath
}

export async function pathState(path) {
  try {
    return await lstat(path)
  } catch (error) {
    if (error.code === 'ENOENT') return null
    throw error
  }
}

export async function assertOrdinaryDirectory(path, label = 'path') {
  const state = await pathState(path)
  if (state === null) return
  if (!state.isDirectory() || state.isSymbolicLink()) {
    throw new Error(`${label} must be an ordinary directory: ${path}`)
  }
}

export async function assertOrdinaryFile(path, label = 'file', required = true) {
  const state = await pathState(path)
  if (state === null) {
    if (required) throw new Error(`required ${label} is missing: ${path}`)
    return
  }
  if (!state.isFile() || state.isSymbolicLink()) {
    throw new Error(`${label} must be an ordinary file: ${path}`)
  }
}

export async function assertNoLinks(path, label = 'directory') {
  const state = await pathState(path)
  if (state === null) return
  await assertOrdinaryDirectory(path, label)
  for (const entry of await readdir(path, { withFileTypes: true })) {
    const child = join(path, entry.name)
    const childState = await lstat(child)
    if (childState.isSymbolicLink()) {
      throw new Error(`${label} must not contain a symbolic link or junction: ${child}`)
    }
    if (childState.isDirectory()) await assertNoLinks(child, label)
  }
}

export async function ensureOrdinaryDirectory(path, label = 'path') {
  await assertOrdinaryDirectory(path, label)
  await mkdir(path, { recursive: true })
  await assertOrdinaryDirectory(path, label)
}

export async function removeOwnedDirectory(path, label = 'directory') {
  if ((await pathState(path)) === null) return false
  await assertNoLinks(path, label)
  await rm(path, { recursive: true, force: true })
  return true
}

export function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: repositoryRoot,
    env: process.env,
    stdio: 'inherit',
    shell: false,
    ...options
  })
  if (result.error) throw result.error
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(' ')} exited with code ${result.status}`)
  }
  return result
}

export function capture(command, args, options = {}) {
  const result = run(command, args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], ...options })
  return result.stdout.trim()
}

export async function readVersion() {
  const version = (await readFile(join(repositoryRoot, 'VERSION'), 'utf8')).trim()
  if (!/^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$/.test(version)) {
    throw new Error(`VERSION contains an invalid semantic version: ${version}`)
  }
  return version
}

export async function writeJson(path, value) {
  await writeFile(path, `${JSON.stringify(value, null, 2)}\n`, 'utf8')
}


export function spawnDetached(command, args, options) {
  return spawn(command, args, { detached: true, windowsHide: true, shell: false, ...options })
}

export function normalizePath(path) {
  const fullPath = isAbsolute(path) ? resolve(path) : resolve(repositoryRoot, path)
  return process.platform === 'win32' ? fullPath.toLowerCase() : fullPath
}
