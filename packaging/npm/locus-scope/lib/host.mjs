import { spawn } from 'node:child_process';

const RESPONSE_FIELDS = ['exitCode', 'stderr', 'stdout', 'version'];

export async function runHost(host, request, stdout, stderr, { spawnProcess = spawn } = {}) {
  const child = spawnProcess(host, {
    stdio: ['pipe', 'pipe', 'pipe'],
    windowsHide: true,
  });
  const responseChunks = [];
  const errorChunks = [];
  child.stdout.on('data', (chunk) => responseChunks.push(Buffer.from(chunk)));
  child.stderr.on('data', (chunk) => errorChunks.push(Buffer.from(chunk)));

  const completion = new Promise((resolve, reject) => {
    child.once('error', (error) => reject(new Error(`launch Node host ${JSON.stringify(host)}: ${error.message}`, { cause: error })));
    child.once('close', (code, signal) => resolve({ code, signal }));
  });

  child.stdin.on('error', () => {});
  child.stdin.end(`${JSON.stringify(request)}\n`);
  const { code, signal } = await completion;
  const rawStderr = Buffer.concat(errorChunks).toString('utf8');
  if (signal !== null) {
    throw new Error(`Node host terminated by signal ${signal}${rawStderr ? `: ${rawStderr.trim()}` : ''}`);
  }

  const response = decodeResponse(Buffer.concat(responseChunks).toString('utf8'));
  if (code !== response.exitCode) {
    throw new Error(
      `Node host protocol exit mismatch: process exited ${String(code)}, response declared ${response.exitCode}${rawStderr ? `; stderr: ${rawStderr.trim()}` : ''}`,
    );
  }
  if (rawStderr) {
    throw new Error(`Node host wrote outside the JSON protocol: ${rawStderr.trim()}`);
  }

  if (response.stdout) {
    stdout.write(response.stdout);
  }
  if (response.stderr) {
    stderr.write(response.stderr);
  }
  return response.exitCode;
}

export function decodeResponse(source) {
  let response;
  try {
    response = JSON.parse(source);
  } catch (error) {
    throw new Error(`Node host returned invalid JSON: ${error.message}`, { cause: error });
  }
  if (response === null || typeof response !== 'object' || Array.isArray(response)) {
    throw new Error('Node host response must be a JSON object');
  }
  const fields = Object.keys(response).sort();
  if (JSON.stringify(fields) !== JSON.stringify(RESPONSE_FIELDS)) {
    throw new Error(`Node host response fields are invalid: ${fields.join(', ')}`);
  }
  if (response.version !== 1) {
    throw new Error(`Node host response version ${JSON.stringify(response.version)} is unsupported`);
  }
  if (!Number.isInteger(response.exitCode) || response.exitCode < 0 || response.exitCode > 255) {
    throw new Error(`Node host response exitCode ${JSON.stringify(response.exitCode)} is invalid`);
  }
  if (typeof response.stdout !== 'string' || typeof response.stderr !== 'string') {
    throw new Error('Node host response stdout and stderr must be strings');
  }
  return response;
}
