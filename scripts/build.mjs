import { cp, mkdir, rm } from "node:fs/promises";
import { join } from "node:path";
import {
  ensureOrdinaryDirectory,
  isMain,
  readVersion,
  removeOwnedDirectory,
  repositoryRoot,
  run,
  tempRoot
} from "./lib/workspace.mjs";

const packages = [
  { name: "core", directory: join(repositoryRoot, "packages", "core") },
  { name: "cli", directory: join(repositoryRoot, "packages", "cli") }
];

export async function build({ quiet = false } = {}) {
  const version = await readVersion();
  const outputRoot = join(tempRoot, "build");
  await ensureOrdinaryDirectory(tempRoot, "build path");
  await removeOwnedDirectory(outputRoot, "TypeScript build output");
  for (const package_ of packages) {
    await removeOwnedDirectory(join(package_.directory, "lib"), `${package_.name} build output`);
    await rm(join(package_.directory, "tsconfig.tsbuildinfo"), { force: true });
  }
  await mkdir(outputRoot, { recursive: true });
  try {
    run(process.execPath, ["node_modules/typescript/bin/tsc", "--build", "packages/core", "--pretty", "false"]);
    run(process.execPath, ["node_modules/typescript/bin/tsc", "--build", "packages/cli", "--pretty", "false"]);
    for (const package_ of packages) {
      await cp(join(package_.directory, "lib"), join(outputRoot, package_.name), { recursive: true });
    }
  } finally {
    for (const package_ of packages) {
      await removeOwnedDirectory(join(package_.directory, "lib"), `${package_.name} build output`);
      await rm(join(package_.directory, "tsconfig.tsbuildinfo"), { force: true });
    }
  }
  if (!quiet) console.log(`Built workspace packages in ${outputRoot}`);
  return {
    outputRoot,
    outputPaths: Object.fromEntries(packages.map(package_ => [package_.name, join(outputRoot, package_.name)])),
    version
  };
}

if (isMain(import.meta.url)) {
  build().catch(error => {
    console.error(error.message);
    process.exitCode = 1;
  });
}
