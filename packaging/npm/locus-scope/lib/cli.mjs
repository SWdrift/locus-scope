import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { buildPackageEnvironment } from './package-environment.mjs';
import { runHost } from './host.mjs';
import { discoverScopeRoot, parseRootOptions } from './options.mjs';
import { locatePlatformHost } from './platform.mjs';

export async function main(
  arguments_,
  {
    workingDirectory = process.cwd(),
    stdout = process.stdout,
    stderr = process.stderr,
    platform = process.platform,
    architecture = process.arch,
    discoverRoot = discoverScopeRoot,
    buildEnvironment = buildPackageEnvironment,
    locateHost = locatePlatformHost,
    invokeHost = runHost,
  } = {},
) {
  const jsonOutput = arguments_.includes('--json');
  try {
    const options = parseRootOptions(arguments_);
    if (isHelp(options.arguments)) {
      stdout.write(usage);
      return 0;
    }
    if (isVersion(options.arguments)) {
      const { version } = await packageManifest();
      if (jsonOutput) {
        stdout.write(`${JSON.stringify({ name: 'locus-scope-node', version })}\n`);
      } else {
        stdout.write(`locus-scope-node ${version}\n`);
      }
      return 0;
    }

    const absoluteWorkingDirectory = path.resolve(workingDirectory);
    const scopeRoot = await discoverRoot(absoluteWorkingDirectory, options.scopeDirectory);
    const environment = await buildEnvironment(scopeRoot);
    const request = {
      version: 1,
      workingDirectory: absoluteWorkingDirectory,
      arguments: options.arguments,
      root: environment.root,
      packages: environment.packages,
    };
    const host = await locateHost({ platform, architecture });
    return await invokeHost(host, request, stdout, stderr);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (jsonOutput) {
      stderr.write(`${JSON.stringify({ error: message })}\n`);
    } else {
      stderr.write(`locus-scope-node: ${message}\n`);
    }
    return 1;
  }
}

function isHelp(arguments_) {
  return (
    arguments_.length === 0 ||
    arguments_.includes('--help') ||
    arguments_.includes('-h') ||
    (arguments_.length === 1 && arguments_[0] === 'help')
  );
}

function isVersion(arguments_) {
  const command = arguments_.filter((argument) => argument !== '--json');
  return command.length === 1 && (command[0] === 'version' || command[0] === '--version');
}

async function packageManifest() {
  let source;
  try {
    source = await readFile(new URL('../package.json', import.meta.url), 'utf8');
  } catch (error) {
    throw new Error(`read @locus/scope package manifest: ${error.message}`, { cause: error });
  }
  try {
    return JSON.parse(source);
  } catch (error) {
    throw new Error(`parse @locus/scope package manifest: ${error.message}`, { cause: error });
  }
}

const usage = `Usage:
  locus-scope-node [--scope <dir>] [--json] validate
  locus-scope-node [--scope <dir>] [--json] scope show
  locus-scope-node [--scope <dir>] [--json] scope list
  locus-scope-node [--scope <dir>] [--json] entity list
  locus-scope-node [--scope <dir>] [--json] entity show <ref>
  locus-scope-node [--scope <dir>] [--json] relation list
  locus-scope-node [--scope <dir>] [--json] resolve <ref>
  locus-scope-node [--json] version
  locus-scope-node help
`;
