import path from 'node:path';
import { buildPackageEnvironment } from './package-environment.mjs';
import { runHost } from './host.mjs';
import { discoverScopeRoot, parseRootOptions } from './options.mjs';
import { locatePlatformHost } from './platform.mjs';

export async function main(
  arguments_,
  {
    workingDirectory = process.cwd(),
    stdin = process.stdin,
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
    const absoluteWorkingDirectory = path.resolve(workingDirectory);
    const host = await locateHost({ platform, architecture });
    const request = {
      version: 2,
      workingDirectory: absoluteWorkingDirectory,
      arguments: options.arguments,
      root: { scopeRoot: '', packageRoot: '', dependencies: {} },
      packages: {},
    };
    if (!isRootless(options.arguments)) {
      const scopeRoot = await discoverRoot(absoluteWorkingDirectory, options.scopeDirectory);
      const environment = await buildEnvironment(scopeRoot);
      request.root = environment.root;
      request.packages = environment.packages;
    }
    if (usesStdin(options.arguments)) {
      request.stdin = await readStream(stdin);
    }
    return await invokeHost(host, request, stdout, stderr);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (jsonOutput) stderr.write(`${JSON.stringify({ error: message })}\n`);
    else stderr.write(`locus-scope-node: ${message}\n`);
    return 1;
  }
}

function isRootless(arguments_) {
  const command = arguments_.filter((argument) => argument !== '--json');
  return command.length === 0 || (command.length === 1 && ['help', '--help', '-h', 'version', '--version'].includes(command[0]));
}

function usesStdin(arguments_) {
  return arguments_.includes('-');
}

async function readStream(stream) {
  const chunks = [];
  for await (const chunk of stream) chunks.push(Buffer.from(chunk));
  return Buffer.concat(chunks).toString('utf8');
}
