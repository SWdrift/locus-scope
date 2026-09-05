import { readdir } from 'node:fs/promises'
import { join } from 'node:path'
import { assertOrdinaryFile, isMain, repositoryRoot, run } from './lib/workspace.mjs'

async function assertDependencies() {
  for (const path of [
    join(repositoryRoot, 'node_modules', 'verdaccio', 'package.json'),
    join(repositoryRoot, 'node_modules', 'remark-cli', 'package.json'),
    join(repositoryRoot, 'node_modules', 'remark-validate-links', 'package.json')
  ]) {
    try {
      await assertOrdinaryFile(path, 'Node dependency')
    } catch {
      throw new Error('root Node dependencies are not installed; run: pnpm install --frozen-lockfile')
    }
  }
}

export async function test(suite = 'all') {
  if (!['all', 'e2e'].includes(suite)) throw new Error(`unknown test suite: ${suite}`)
  await assertDependencies()
  if (suite === 'e2e') {
    run('go', ['test', './test/e2e', '-count=1'])
    return
  }
  run('go', ['test', './...', '-count=1'])
  const nodeTestRoot = join(repositoryRoot, 'packaging', 'npm', 'locus-scope', 'test')
  const nodeTests = (await readdir(nodeTestRoot))
    .filter(name => name.endsWith('.test.mjs'))
    .sort()
    .map(name => join(nodeTestRoot, name))
  run(process.execPath, ['--test', ...nodeTests])
}

if (isMain(import.meta.url)) {
  test(process.argv[2] ?? 'all').catch(error => {
    console.error(error.message)
    process.exitCode = 1
  })
}
