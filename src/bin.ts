import { readFileSync } from "node:fs";
import path from "node:path";
import { main } from "./cli/index.js";
import { findScope, loadDefinitionFile } from "./domain/index.js";
import { load as loadWorkspace, type Environment } from "./packageenv.js";
import { buildPackageEnvironment } from "./package-environment.js";
import { ApplicationService } from "./application/index.js";
import { createError } from "./domain/errors.js";

let version = process.env.LOCUS_SCOPE_VERSION ?? process.env.npm_package_version;
if (!version) {
  try {
    version = readFileSync(path.resolve(process.cwd(), "VERSION"), "utf8").trim();
  } catch {
    version = "development";
  }
}
let currentEnvironment: Environment | undefined;

const explicitScope = async (workingDirectory: string, value?: string): Promise<string> =>
  value === undefined ? findScope(workingDirectory) : path.resolve(workingDirectory, value);

const loadPath = async (value: string) => {
  const target = path.resolve(value);
  if (/\.locus\.(yaml|yml|json)$/u.test(target)) return loadDefinitionFile(target);
  const environment = await buildPackageEnvironment(target);
  return loadWorkspace(environment);
};

const exitCode = await main(process.argv.slice(2), {
  version,
  discoverRoot: explicitScope,
  buildEnvironment: buildPackageEnvironment,
  loadApplication: async environment => {
    currentEnvironment = environment;
    return new ApplicationService(await loadWorkspace(environment));
  },
  reloadWorkspace: async () => {
    if (!currentEnvironment) throw createError("scope.internal", "Scope environment is unavailable");
    return loadWorkspace(currentEnvironment);
  },
  loadPath
});
process.exitCode = exitCode;
