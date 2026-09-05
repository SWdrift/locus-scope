import { open, readFile, rm, writeFile } from 'node:fs/promises'
import { existsSync, readFileSync, readlinkSync, statSync, unwatchFile, watchFile } from 'node:fs'
import { join, resolve } from 'node:path'
import {
  assertNoLinks,
  assertOrdinaryDirectory,
  assertOrdinaryFile,
  capture,
  ensureOrdinaryDirectory,
  isMain,
  normalizePath,
  pathState,
  repositoryRoot,
  
  spawnDetached,
  tempRoot,
  writeJson
} from './lib/workspace.mjs'

const stateRoot = join(tempRoot, 'verdaccio-dev')
const storageRoot = join(stateRoot, 'storage')
const authRoot = join(stateRoot, 'auth')
const logRoot = join(stateRoot, 'logs')
const configPath = join(stateRoot, 'config.yaml')
const processRecordPath = join(stateRoot, 'process.json')
const authPath = join(authRoot, 'htpasswd')
const serverLogPath = join(logRoot, 'verdaccio.log')
const stdoutLogPath = join(logRoot, 'stdout.log')
const stderrLogPath = join(logRoot, 'stderr.log')
const templatePath = join(repositoryRoot, 'scripts', 'config', 'verdaccio.dev.yaml')
const verdaccioEntryPath = join(repositoryRoot, 'node_modules', 'verdaccio', 'bin', 'verdaccio')
const verdaccioManifestPath = join(repositoryRoot, 'node_modules', 'verdaccio', 'package.json')
const registryUrl = 'http://127.0.0.1:4873/'
const readyUrl = `${registryUrl}-/ping`

async function assertVerdaccioInstallation() {
  await assertOrdinaryFile(verdaccioEntryPath, 'Verdaccio entrypoint')
  await assertOrdinaryFile(verdaccioManifestPath, 'Verdaccio manifest')
  const manifest = JSON.parse(await readFile(verdaccioManifestPath, 'utf8'))
  if (manifest.name !== 'verdaccio' || manifest.version !== '6.10.2') {
    throw new Error(`expected project-local verdaccio@6.10.2 at ${verdaccioManifestPath}`)
  }
}

async function initializeRegistryState() {
  await assertOrdinaryFile(templatePath, 'Verdaccio configuration template')
  for (const path of [tempRoot, stateRoot, storageRoot, authRoot, logRoot]) {
    await ensureOrdinaryDirectory(path, 'registry state path')
  }
  for (const path of [configPath, processRecordPath, authPath, serverLogPath, stdoutLogPath, stderrLogPath]) {
    await assertOrdinaryFile(path, 'registry state file', false)
  }

  const template = await readFile(templatePath, 'utf8')
  const configuration = template
    .replace('__STORAGE_PATH__', JSON.stringify(resolve(storageRoot)))
    .replace('__AUTH_PATH__', JSON.stringify(resolve(authPath)))
    .replace('__LOG_PATH__', JSON.stringify(resolve(serverLogPath)))
  await writeFile(configPath, configuration, 'utf8')
}

async function registryReady() {
  try {
    const response = await fetch(readyUrl, { signal: AbortSignal.timeout(2000) })
    return response.status === 200
  } catch {
    return false
  }
}

function processExists(pid) {
  try {
    process.kill(pid, 0)
    return true
  } catch (error) {
    if (error.code === 'ESRCH') return false
    throw error
  }
}

function inspectProcess(pid) {
  if (process.platform === 'win32') {
    const command = `$p=Get-CimInstance Win32_Process -Filter 'ProcessId = ${pid}'; if ($null -eq $p) { exit 3 }; @{ executable=$p.ExecutablePath; commandLine=$p.CommandLine } | ConvertTo-Json -Compress`
    const result = capture('pwsh', ['-NoProfile', '-Command', command])
    return JSON.parse(result)
  }
  if (process.platform === 'linux') {
    return {
      executable: readlinkSync(`/proc/${pid}/exe`),
      commandLine: readFileSync(`/proc/${pid}/cmdline`, 'utf8').replaceAll('\0', ' ')
    }
  }
  return {
    executable: capture('ps', ['-p', String(pid), '-o', 'comm=']),
    commandLine: capture('ps', ['-p', String(pid), '-o', 'command='])
  }
}


async function managedRegistryProcess() {
  if ((await pathState(processRecordPath)) === null) return null
  let record
  try {
    record = JSON.parse(await readFile(processRecordPath, 'utf8'))
  } catch {
    throw new Error(`invalid Verdaccio process record: ${processRecordPath}`)
  }
  if (!Number.isInteger(record.pid) || record.pid <= 0 || !record.executable || !record.entrypoint || !record.config) {
    throw new Error(`incomplete Verdaccio process record: ${processRecordPath}`)
  }
  if (!processExists(record.pid)) {
    await rm(processRecordPath, { force: true })
    return null
  }

  const actual = inspectProcess(record.pid)
  if (normalizePath(actual.executable) !== normalizePath(record.executable)) {
    throw new Error(`PID ${record.pid} executable '${actual.executable}' does not match '${record.executable}'`)
  }
  if (
    normalizePath(record.entrypoint) !== normalizePath(verdaccioEntryPath) ||
    normalizePath(record.config) !== normalizePath(configPath)
  ) {
    throw new Error(`Verdaccio process record does not belong to this workspace: ${processRecordPath}`)
  }
  const commandLine = process.platform === 'win32' ? actual.commandLine.toLowerCase() : actual.commandLine
  for (const required of [verdaccioEntryPath, configPath, '127.0.0.1:4873']) {
    const expected = process.platform === 'win32' ? required.toLowerCase() : required
    if (!commandLine.includes(expected)) {
      throw new Error(`PID ${record.pid} command line does not match this Verdaccio instance`)
    }
  }
  return record
}

async function startRegistry() {
  await assertVerdaccioInstallation()
  await initializeRegistryState()
  const existing = await managedRegistryProcess()
  if (existing) {
    if (await registryReady()) {
      console.log(`Verdaccio is ready at ${registryUrl} with PID ${existing.pid}`)
      return
    }
    throw new Error(`Verdaccio PID ${existing.pid} is running but ${readyUrl} is unavailable`)
  }
  if (await registryReady()) throw new Error(`${registryUrl} is already served by a process not owned by this workspace`)

  await rm(stdoutLogPath, { force: true })
  await rm(stderrLogPath, { force: true })
  const stdout = await open(stdoutLogPath, 'a')
  const stderr = await open(stderrLogPath, 'a')
  const child = spawnDetached(
    process.execPath,
    [verdaccioEntryPath, '--config', configPath, '--listen', '127.0.0.1:4873'],
    { cwd: stateRoot, stdio: ['ignore', stdout.fd, stderr.fd] }
  )
  child.unref()
  await stdout.close()
  await stderr.close()
  await writeJson(processRecordPath, {
    pid: child.pid,
    executable: process.execPath,
    entrypoint: verdaccioEntryPath,
    config: configPath,
    registry: registryUrl
  })

  const deadline = Date.now() + 30_000
  while (Date.now() < deadline) {
    if (!processExists(child.pid)) {
      await rm(processRecordPath, { force: true })
      throw new Error(`Verdaccio exited before becoming ready; see ${stdoutLogPath} and ${stderrLogPath}`)
    }
    if (await registryReady()) {
      console.log(`Verdaccio is ready at ${registryUrl} with PID ${child.pid}`)
      return
    }
    await new Promise(resolvePromise => setTimeout(resolvePromise, 250))
  }
  process.kill(child.pid, 'SIGTERM')
  await rm(processRecordPath, { force: true })
  throw new Error(`Verdaccio did not become ready within 30 seconds; see ${stdoutLogPath} and ${stderrLogPath}`)
}

async function registryStatus() {
  const record = await managedRegistryProcess()
  if (!record) throw new Error('Verdaccio is not running')
  if (!(await registryReady())) {
    throw new Error(`Verdaccio PID ${record.pid} is running but ${readyUrl} is unavailable`)
  }
  console.log(`Verdaccio is ready at ${registryUrl} with PID ${record.pid}`)
}

async function stopRegistry() {
  const record = await managedRegistryProcess()
  if (!record) {
    console.log('Verdaccio is already stopped')
    return
  }
  process.kill(record.pid, 'SIGTERM')
  const deadline = Date.now() + 15_000
  while (Date.now() < deadline && processExists(record.pid)) {
    await new Promise(resolvePromise => setTimeout(resolvePromise, 100))
  }
  if (processExists(record.pid)) throw new Error(`Verdaccio PID ${record.pid} did not stop within 15 seconds`)
  await rm(processRecordPath, { force: true })
  console.log('Verdaccio stopped')
}

async function showLogs(follow) {
  const paths = [serverLogPath, stdoutLogPath, stderrLogPath].filter(existsSync)
  if (paths.length === 0) {
    console.log(`No Verdaccio logs exist under ${logRoot}`)
    return
  }
  const offsets = new Map()
  for (const path of paths) {
    const content = readFileSync(path, 'utf8')
    process.stdout.write(content)
    offsets.set(path, Buffer.byteLength(content))
  }
  if (!follow) return

  for (const path of paths) {
    watchFile(path, { interval: 250 }, () => {
      const size = statSync(path).size
      const previous = Math.min(offsets.get(path), size)
      if (size === previous) return
      const descriptor = readFileSync(path)
      process.stdout.write(descriptor.subarray(previous))
      offsets.set(path, size)
    })
  }
  process.on('SIGINT', () => {
    for (const path of paths) unwatchFile(path)
    process.exit(130)
  })
  await new Promise(() => {})
}

async function resetRegistry(force) {
  if (!force) throw new Error('reset deletes the project-local Registry state; pass --force to continue')
  await assertOrdinaryDirectory(tempRoot, 'registry state path')
  await assertNoLinks(stateRoot, 'registry state')
  await stopRegistry()
  await rm(stateRoot, { recursive: true, force: true })
  console.log(`Removed Verdaccio state at ${stateRoot}`)
}

export async function registry(action, options = {}) {
  if (action === 'start') return startRegistry()
  if (action === 'status') return registryStatus()
  if (action === 'stop') return stopRegistry()
  if (action === 'logs') return showLogs(options.follow)
  if (action === 'reset') return resetRegistry(options.force)
  throw new Error(`unknown Registry action: ${action}`)
}

if (isMain(import.meta.url)) {
  const args = new Set(process.argv.slice(3))
  registry(process.argv[2] ?? 'status', { follow: args.has('--follow'), force: args.has('--force') }).catch(error => {
    console.error(error.message)
    process.exitCode = 1
  })
}
