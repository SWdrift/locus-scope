import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { test } from 'node:test'
import path from 'node:path'
import { promisify } from 'node:util'

const execFileAsync = promisify(execFile)
const root = path.resolve('test/e2e/case/scope-composition/app')
const executable = path.resolve('temp/ts-build/bin.js')

test('built CLI preserves composed Scope fixture behavior', async () => {
  const { stdout } = await execFileAsync(process.execPath, [executable, '--scope', root, 'validate', '--json'], { cwd: process.cwd() })
  const summary = JSON.parse(stdout)
  assert.deepEqual(
    { valid: summary.valid, scopes: summary.scopes, entities: summary.entities, relations: summary.relations },
    { valid: true, scopes: 2, entities: 5, relations: 4 },
  )

  const graph = await execFileAsync(process.execPath, [executable, '--scope', root, 'graph', 'api', '--depth', '1', '--json'], { cwd: process.cwd() })
  const graphResult = JSON.parse(graph.stdout)
  assert.equal(graphResult.seeds[0].key.id, 'api')
  assert.equal(graphResult.relations.length, 2)
})
