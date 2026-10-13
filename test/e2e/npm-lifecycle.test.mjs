import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { createServer } from 'node:net'
import { cp, mkdir, readFile, readdir, rm, stat, writeFile } from 'node:fs/promises'
import { join, dirname, resolve } from 'node:path'
import { test } from 'node:test'

const repositoryRoot = resolve('.')
const stateRoot = join(repositoryRoot, 'temp', 'e2e-run', 'npm')
const fixtureRoot = join(stateRoot, 'fixtures')
const projectRoot = join(stateRoot, 'projects')
const resultRoot = join(stateRoot, 'results')
const configRoot = join(stateRoot, 'config')
const registryRoot = join(stateRoot, 'registry')
const node = process.execPath
const npm = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const pnpm = process.platform === 'win32' ? 'pnpm.cmd' : 'pnpm'

const sleep = milliseconds => new Promise(resolveSleep => setTimeout(resolveSleep, milliseconds))

async function reservePort() {
  const server = createServer()
  await new Promise((resolveListen, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', resolveListen)
  })
  const port = server.address().port
  await new Promise((resolveClose, reject) => server.close(error => error ? reject(error) : resolveClose()))
  return port
}

async function fetchWithTimeout(url, options = {}, milliseconds = 2_000) {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), milliseconds)
  try {
    return await fetch(url, { ...options, signal: controller.signal })
  } finally {
    clearTimeout(timer)
  }
}

async function readResponse(response) {
  const body = await response.text()
  return { status: response.status, body }
}

function commandEnvironment(root, role, userConfig, registry) {
  const excluded = /^(?:HOME|USERPROFILE|NPM_CONFIG_|npm_config_|PNPM_|pnpm_|YARN_|yarn_|NODE_AUTH_TOKEN|NPM_TOKEN|HTTP_PROXY|HTTPS_PROXY|ALL_PROXY|NO_PROXY)$/u
  const environment = Object.fromEntries(Object.entries(process.env).filter(([key]) => !excluded.test(key)))
  const roleHome = join(root, 'home', role)
  environment.HOME = roleHome
  environment.USERPROFILE = roleHome
  environment.NPM_CONFIG_GLOBALCONFIG = join(root, 'config', 'global.npmrc')
  environment.npm_config_global = 'false'
  environment.npm_config_fund = 'false'
  environment.npm_config_audit = 'false'
  environment.npm_config_update_notifier = 'false'
  environment.CI = '1'
  environment.HTTP_PROXY = ''
  environment.HTTPS_PROXY = ''
  environment.ALL_PROXY = ''
  environment.NO_PROXY = '127.0.0.1,localhost,::1'
  environment.NODE_AUTH_TOKEN = ''
  environment.NPM_TOKEN = ''
  if (userConfig) environment.NPM_CONFIG_USERCONFIG = userConfig
  if (registry) {
    environment.NPM_CONFIG_REGISTRY = `${registry}/`
    environment.npm_config_registry = `${registry}/`
  }
  return environment
}

async function runCommand(name, cwd, environment, executable, args, timeout = 300_000) {
  const child = spawn(executable, args, {
    cwd,
    env: environment,
    shell: process.platform === 'win32' && /\.cmd$/u.test(executable),
    windowsHide: true,
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  const stdout = []
  const stderr = []
  child.stdout.on('data', chunk => stdout.push(Buffer.from(chunk)))
  child.stderr.on('data', chunk => stderr.push(Buffer.from(chunk)))
  let timer
  const result = await new Promise(resolveResult => {
    let settled = false
    const finish = value => {
      if (settled) return
      settled = true
      clearTimeout(timer)
      resolveResult(value)
    }
    child.once('error', error => finish({ code: null, error }))
    child.once('close', (code, signal) => finish({ code, signal }))
    timer = setTimeout(() => {
      if (process.platform === 'win32' && child.pid !== undefined) {
        const killer = spawn('taskkill.exe', ['/PID', String(child.pid), '/T', '/F'], { windowsHide: true, stdio: 'ignore' })
        killer.unref()
      } else {
        child.kill('SIGTERM')
      }
      finish({ code: null, error: new Error(`command timed out after ${timeout}ms`) })
    }, timeout)
  })
  const output = { ...result, stdout: Buffer.concat(stdout).toString('utf8'), stderr: Buffer.concat(stderr).toString('utf8') }
  await writeFile(join(resultRoot, `${name}.stdout`), output.stdout)
  await writeFile(join(resultRoot, `${name}.stderr`), output.stderr)
  await writeFile(join(resultRoot, `${name}.exit`), `${output.code ?? -1}\n`)
  return output
}

function requireSuccess(result, name) {
  assert.equal(result.code, 0, `${name} failed\nstdout:\n${result.stdout}\nstderr:\n${result.stderr}`)
  return result.stdout
}

async function writeNpmrc(path, registry, token = '') {
  const host = new URL(registry).host
  const lines = [`registry=${registry}/`, 'fund=false', 'audit=false']
  if (token) lines.push(`//${host}/:_authToken=${token}`)
  await mkdir(dirname(path), { recursive: true })
  await writeFile(path, `${lines.join('\n')}\n`, { mode: 0o600 })
}

async function materialize(name, source, parent = fixtureRoot) {
  const destination = join(parent, name)
  await cp(source, destination, { recursive: true })
  return destination
}

async function publishPackage(name, directory, environment, registry) {
  const result = await runCommand(`publish-${name}`, directory, environment, npm, [
    'publish', '.', '--access', 'public', '--ignore-scripts', '--registry', registry,
  ], 60_000)
  requireSuccess(result, `publish ${name}`)
}

async function startRegistry(environment, endpoint) {
  const configPath = join(registryRoot, 'config.yaml')
  const logPath = join(registryRoot, 'verdaccio.log')
  const storagePath = join(registryRoot, 'storage')
  const authPath = join(registryRoot, 'auth', 'htpasswd')
  await mkdir(storagePath, { recursive: true })
  await mkdir(dirname(authPath), { recursive: true })
  const yamlPath = value => JSON.stringify(value.replaceAll('\\', '/'))
  await writeFile(configPath, `storage: ${yamlPath(storagePath)}\nauth:\n  htpasswd:\n    file: ${yamlPath(authPath)}\n    max_users: 20\nuplinks:\n  npmjs:\n    url: https://registry.npmjs.org/\npackages:\n  '@example/private':\n    access: $authenticated\n    publish: $authenticated\n    unpublish: $authenticated\n  '@sundw/locus-scope':\n    access: $all\n    publish: $authenticated\n    unpublish: $authenticated\n  '**':\n    access: $all\n    publish: $authenticated\n    unpublish: $authenticated\n    proxy: npmjs\nlog:\n  type: stdout\n  format: pretty\n  level: http\n`)
  const child = spawn(node, [
    join(repositoryRoot, 'node_modules', 'verdaccio', 'bin', 'verdaccio'),
    '--config', configPath, '--listen', new URL(endpoint).host,
  ], { cwd: repositoryRoot, env: environment, stdio: ['ignore', 'pipe', 'pipe'], windowsHide: true })
  const stdout = []
  const stderr = []
  child.stdout.on('data', chunk => stdout.push(Buffer.from(chunk)))
  child.stderr.on('data', chunk => stderr.push(Buffer.from(chunk)))
  const record = { pid: child.pid, endpoint, config: configPath }
  await writeFile(join(registryRoot, 'process.json'), `${JSON.stringify(record, null, 2)}\n`)
  let ready = false
  for (let attempt = 0; attempt < 150; attempt += 1) {
    if (child.exitCode !== null) break
    try {
      const response = await fetchWithTimeout(`${endpoint}/-/ping`)
      if (response.ok) {
        ready = true
        break
      }
    } catch {
      // Verdaccio is still starting.
    }
    await sleep(200)
  }
  if (!ready) {
    if (child.exitCode === null) child.kill()
    if (child.exitCode === null) await new Promise(resolveClose => child.once('close', resolveClose))
    await writeFile(logPath, Buffer.concat([...stdout, ...stderr]))
    throw new Error(`Verdaccio did not become ready at ${endpoint}`)
  }
  await writeFile(join(registryRoot, 'endpoint.txt'), `${endpoint}\n`)
  return {
    child,
    async stop() {
      if (child.pid !== record.pid) throw new Error('refusing to stop an unowned Verdaccio process')
      const waitForClose = () => child.exitCode !== null
        ? Promise.resolve()
        : new Promise(resolveClose => child.once('close', resolveClose))
      if (process.platform === 'win32' && child.pid !== undefined) {
        await new Promise(resolveKill => {
          const killer = spawn('taskkill.exe', ['/PID', String(child.pid), '/T', '/F'], { windowsHide: true, stdio: 'ignore' })
          killer.once('close', resolveKill)
        })
      } else if (child.exitCode === null) {
        child.kill('SIGINT')
      }
      await Promise.race([waitForClose(), sleep(10_000)])
      if (child.exitCode === null) child.kill()
      await waitForClose()
      await writeFile(logPath, Buffer.concat([...stdout, ...stderr]))
      await writeFile(join(registryRoot, 'stopped.json'), `${JSON.stringify({ pid: record.pid, endpoint, config: configPath }, null, 2)}\n`)
    },
  }
}

async function createUser(endpoint) {
  const username = 'locus-e2e'
  const response = await fetchWithTimeout(`${endpoint}/-/user/org.couchdb.user:${username}`, {
    method: 'PUT',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({
      _id: `org.couchdb.user:${username}`,
      name: username,
      password: 'locus-e2e-password',
      type: 'user',
      roles: [],
      date: new Date(0).toISOString(),
    }),
  })
  const result = await readResponse(response)
  assert.equal(result.status, 201, `create Verdaccio user: ${result.body}`)
  const body = JSON.parse(result.body)
  assert.equal(typeof body.token, 'string')
  assert.notEqual(body.token, '')
  return body.token
}

async function assertNoSecret(root, secret) {
  const entries = await readdir(root, { withFileTypes: true })
  for (const entry of entries) {
    const path = join(root, entry.name)
    if (entry.isDirectory()) {
      await assertNoSecret(path, secret)
    } else if (entry.isFile()) {
      const body = await readFile(path, 'utf8')
      assert.equal(body.includes(secret), false, `credential leaked to ${path}`)
    }
  }
}

async function makeLibraryProbe(project) {
  const probe = join(project, 'library-probe.mjs')
  await writeFile(probe, `import { ApplicationService, buildPackageEnvironment, loadPackageEnvironment } from '@sundw/locus-scope'\nconst environment = await buildPackageEnvironment(process.cwd())\nconst application = new ApplicationService(await loadPackageEnvironment(environment))\nconst from = application.getEntity('app:application', false)\nconst to = application.getEntity('app:base:database', false)\nconst responses = [application.validate(), to, await application.path(from.key, to.key, [], false)]\nprocess.stdout.write(JSON.stringify(responses))\n`)
  return probe
}

async function readVersion() {
  return (await readFile(join(repositoryRoot, 'VERSION'), 'utf8')).trim()
}

async function prepareState() {
  await rm(stateRoot, { recursive: true, force: true })
  await mkdir(join(configRoot), { recursive: true })
  await mkdir(fixtureRoot, { recursive: true })
  await mkdir(projectRoot, { recursive: true })
  await mkdir(resultRoot, { recursive: true })
  await mkdir(join(registryRoot, 'storage'), { recursive: true })
  await writeFile(join(configRoot, 'global.npmrc'), '')
}

test('npm and pnpm preserve the isolated registry lifecycle', async () => {
  await prepareState()
  const port = await reservePort()
  const endpoint = `http://127.0.0.1:${port}`
  const globalEnvironment = commandEnvironment(stateRoot, 'registry', '', endpoint)
  const registry = await startRegistry(globalEnvironment, endpoint)
  try {
    const token = await createUser(endpoint)
    const authConfig = join(configRoot, 'auth.npmrc')
    const anonymousConfig = join(configRoot, 'anonymous.npmrc')
    const wrongConfig = join(configRoot, 'wrong.npmrc')
    await writeNpmrc(authConfig, endpoint, token)
    await writeNpmrc(anonymousConfig, endpoint)
    await writeNpmrc(wrongConfig, endpoint, 'not-the-e2e-token')
    const authEnvironment = commandEnvironment(stateRoot, 'auth', authConfig, endpoint)
    const anonymousEnvironment = commandEnvironment(stateRoot, 'anonymous', anonymousConfig, endpoint)
    const wrongEnvironment = commandEnvironment(stateRoot, 'wrong', wrongConfig, endpoint)

    for (const [name, source] of [
      ['base', join(repositoryRoot, 'test', 'e2e', 'case', 'npm', 'packages', 'base', '1.0.0')],
      ['base-2.0', join(repositoryRoot, 'test', 'e2e', 'case', 'npm', 'packages', 'base', '2.0.0')],
      ['helper', join(repositoryRoot, 'test', 'e2e', 'case', 'npm', 'packages', 'helper')],
      ['modern', join(repositoryRoot, 'test', 'e2e', 'case', 'npm', 'packages', 'modern')],
      ['app', join(repositoryRoot, 'test', 'e2e', 'case', 'npm', 'packages', 'app')],
      ['private', join(repositoryRoot, 'test', 'e2e', 'case', 'npm', 'packages', 'private')],
    ]) {
      await publishPackage(name, await materialize(name, source), authEnvironment, endpoint)
    }

    const version = await readVersion()
    const releaseTarball = join(repositoryRoot, 'temp', 'release', 'npm', `sundw-locus-scope-${version}.tgz`)
    await stat(releaseTarball)
    const stagedTarball = join(fixtureRoot, `sundw-locus-scope-${version}.tgz`)
    await cp(releaseTarball, stagedTarball)
    const tarballPublish = await runCommand('publish-locus-scope', fixtureRoot, authEnvironment, npm, [
      'publish', stagedTarball, '--access', 'public', '--ignore-scripts', '--registry', endpoint,
    ])
    requireSuccess(tarballPublish, 'publish current locus-scope tarball')
    const consumer = await materialize('consumer', join(repositoryRoot, 'test', 'e2e', 'case', 'npm', 'consumer'), projectRoot)
    await writeFile(join(consumer, 'pnpm-workspace.yaml'), 'packages:\n  - "."\nminimumReleaseAge: 0\n')
    const consumerManifest = JSON.parse(await readFile(join(consumer, 'package.json'), 'utf8'))
    consumerManifest.dependencies = {
      '@example/app': '1.0.0',
      '@example/modern': '1.0.0',
      '@sundw/locus-scope': version,
    }
    await writeFile(join(consumer, 'package.json'), `${JSON.stringify(consumerManifest, null, 2)}\n`)
    assert.equal(JSON.parse(await readFile(join(consumer, 'package.json'), 'utf8')).dependencies['@sundw/locus-scope'], version)
    const install = await runCommand('consumer-install', consumer, anonymousEnvironment, pnpm, [
      'install', '--ignore-workspace', '--ignore-scripts', '--registry', endpoint,
    ])
    requireSuccess(install, 'install isolated consumer')

    const cliPath = join(consumer, 'node_modules', '@sundw', 'locus-scope', 'bin', 'locus-scope.mjs')
    const validate = await runCommand('consumer-validate', consumer, anonymousEnvironment, node, [cliPath, 'validate', '--json'])
    const validateValue = JSON.parse(requireSuccess(validate, 'validate installed graph'))
    assert.deepEqual(
      { valid: validateValue.valid, scopes: validateValue.scopes, entities: validateValue.entities, relations: validateValue.relations },
      { valid: true, scopes: 5, entities: 5, relations: 4 },
    )
    const entity = await runCommand('consumer-entity', consumer, anonymousEnvironment, node, [cliPath, 'entity', 'app:base:database', '--json'])
    const entityValue = JSON.parse(requireSuccess(entity, 'query transitive entity'))
    assert.equal(entityValue.key.scope, 'npm:@example/base@1.0.0')
    const pathResult = await runCommand('consumer-path', consumer, anonymousEnvironment, node, [cliPath, 'path', 'app:application', 'app:base:database', '--json'])
    const pathValue = JSON.parse(requireSuccess(pathResult, 'query transitive path'))
    assert.equal(pathValue.relations[0].object.type, 'uses')

    const probe = await makeLibraryProbe(consumer)
    const library = await runCommand('consumer-library', consumer, anonymousEnvironment, node, [probe])
    const libraryResponses = JSON.parse(requireSuccess(library, 'query library API'))
    assert.equal(libraryResponses.length, 3)
    assert.deepEqual(libraryResponses[0], validateValue)
    assert.deepEqual(libraryResponses[1], entityValue)
    assert.deepEqual(libraryResponses[2], pathValue)

    const anonymousPrivate = await runCommand('anonymous-private', consumer, anonymousEnvironment, npm, [
      'view', '@example/private', 'version', '--registry', endpoint,
    ])
    assert.notEqual(anonymousPrivate.code, 0, 'anonymous private package access unexpectedly succeeded')
    const wrongPrivate = await runCommand('wrong-private', consumer, wrongEnvironment, npm, [
      'view', '@example/private', 'version', '--registry', endpoint,
    ])
    assert.notEqual(wrongPrivate.code, 0, 'wrong credentials unexpectedly succeeded')
    const privateMetadata = await fetchWithTimeout(`${endpoint}/@example%2fprivate`)
    assert.equal(privateMetadata.status >= 400, true)
    const publicMetadata = await fetchWithTimeout(`${endpoint}/@example%2fapp`)
    assert.equal(publicMetadata.status, 200)

    await assertNoSecret(consumer, token)
    await assertNoSecret(resultRoot, token)
    const authText = await readFile(authConfig, 'utf8')
    assert.equal(authText.includes(token), true)
  } finally {
    await registry.stop()
  }
})
