import { cp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";
import {
  ensureOrdinaryDirectory,
  isMain,
  readVersion,
  removeOwnedDirectory,
  repositoryRoot,
  run,
  tempRoot,
  writeJson
} from "./lib/workspace.mjs";
import { build } from "./build.mjs";

export async function releaseNpm() {
  const version = await readVersion();
  const { outputRoot } = await build({ quiet: true });
  const releaseRoot = join(tempRoot, "release", "npm");
  const stage = join(releaseRoot, "locus-scope");
  await removeOwnedDirectory(releaseRoot, "npm release");
  await mkdir(stage, { recursive: true });
  await cp(join(repositoryRoot, "packaging", "npm", "locus-scope"), stage, {
    recursive: true,
    filter: source => !source.split(/[\\/]/u).includes("node_modules")
  });
  await rm(join(stage, "lib"), { recursive: true, force: true });
  await mkdir(join(stage, "lib"));
  await cp(outputRoot, join(stage, "lib"), { recursive: true });
  await writeFile(join(stage, "README.md"), await readFile(join(repositoryRoot, "README.md")));
  await writeFile(
    join(stage, "bin", "locus-scope.mjs"),
    `#!/usr/bin/env node\nprocess.env.LOCUS_SCOPE_VERSION = ${JSON.stringify(version)}\nawait import('../lib/bin.js')\n`
  );
  const manifest = JSON.parse(await readFile(join(stage, "package.json"), "utf8"));
  delete manifest.private;
  manifest.version = version;
  await writeJson(join(stage, "package.json"), manifest);
  const pnpmEntrypoint = process.env.npm_execpath;
  if (!pnpmEntrypoint) throw new Error("pnpm entrypoint is unavailable; run through pnpm run release:npm");
  run(process.execPath, [pnpmEntrypoint, "pack", "--pack-destination", releaseRoot], { cwd: stage });
  console.log(`Packed ${join(releaseRoot, `sundw-locus-scope-${version}.tgz`)}`);
}

if (isMain(import.meta.url)) {
  releaseNpm().catch(error => {
    console.error(error.message);
    process.exitCode = 1;
  });
}
