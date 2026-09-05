import { existsSync } from 'node:fs'
import { resolve } from 'node:path'
import { capture, isMain, repositoryRoot, run } from './lib/workspace.mjs'

function git(...args) {
  return run('git', args)
}

export function synchronizeBranches() {
  if (capture('git', ['status', '--porcelain=v1']) !== '') {
    throw new Error('Working tree must be clean before synchronizing branches.')
  }

  const mergeHeadPath = resolve(repositoryRoot, capture('git', ['rev-parse', '--git-path', 'MERGE_HEAD']))
  try {
    git('switch', 'dev')
    git('fetch', 'gitee', 'main')
    git('fetch', 'github', 'master')

    git('switch', 'main')
    git('merge', '--ff-only', 'gitee/main')
    git('merge', '--no-ff', '--no-edit', 'dev')

    git('switch', 'master')
    git('merge', '--ff-only', 'github/master')
    git('merge', '--no-ff', '--no-edit', 'dev')

    git('push', 'gitee', 'main:main')
    git('push', 'github', 'master:master')
    git('switch', 'dev')
    console.log('Synchronized dev into gitee/main and github/master; current branch is dev.')
  } catch (error) {
    if (existsSync(mergeHeadPath)) {
      try {
        git('merge', '--abort')
      } catch {
        console.warn('Failed to abort the in-progress merge.')
      }
    }
    try {
      const currentBranch = capture('git', ['branch', '--show-current'])
      if (currentBranch && currentBranch !== 'dev') git('switch', 'dev')
    } catch {
      console.warn('Failed to return to dev; resolve the repository state manually.')
    }
    throw error
  }
}

if (isMain(import.meta.url)) {
  try {
    synchronizeBranches()
  } catch (error) {
    console.error(error.message)
    process.exitCode = 1
  }
}
