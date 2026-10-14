import { cp, mkdir, readFile, rm } from "node:fs/promises";
import { join } from "node:path";
import {
  ensureOrdinaryDirectory,
  isMain,
  readVersion,
  repositoryRoot,
  run,
  tempRoot,
  writeJson
} from "./lib/workspace.mjs";
import { build } from "./build.mjs";

const packages = [
  {
    name: "core",
    packaging: join(repositoryRoot, "packaging", "npm", "locus-scope-core")
  },
  {
    name: "locus-scope",
    packaging: join(repositoryRoot, "packaging", "npm", "locus-scope")
  }
];

function replaceWorkspaceDependencies(manifest, version) {
  for (const section of ["dependencies", "devDependencies", "optionalDependencies", "peerDependencies"]) {
    for (const [name, value] of Object.entries(manifest[section] ?? {})) {
      if (typeof value === "string" && value.startsWith("workspace:")) manifest[section][name] = version;
    }
  }
}

async function stagePackage(package_, outputRoot, version) {
  const stage = join(outputRoot, package_.name);
  await mkdir(stage, { recursive: true });
  await cp(package_.packaging, stage, {
    recursive: true,
    filter: source => source.split(/[\\/]/u).at(-1) !== "package.json"
  });
  await readFile(join(stage, "README.md"));
  const manifest = JSON.parse(await readFile(join(package_.packaging, "package.json"), "utf8"));
  delete manifest.private;
  manifest.version = version;
  replaceWorkspaceDependencies(manifest, version);
  await writeJson(join(stage, "package.json"), manifest);
  await cp(join(tempRoot, "build", package_.name === "locus-scope" ? "cli" : package_.name), join(stage, "lib"), {
    recursive: true
  });
  return stage;
}

export async function releaseNpm() {
  const version = await readVersion();
  await build({ quiet: true });
  const releaseRoot = join(tempRoot, "release", "npm");
  await ensureOrdinaryDirectory(tempRoot, "release path");
  await rm(releaseRoot, { recursive: true, force: true });
  await mkdir(releaseRoot, { recursive: true });
  const pnpmEntrypoint = process.env.npm_execpath;
  if (!pnpmEntrypoint) throw new Error("pnpm entrypoint is unavailable; run through pnpm run release:npm");
  for (const package_ of packages) {
    const stage = await stagePackage(package_, releaseRoot, version);
    run(process.execPath, [pnpmEntrypoint, "pack", "--pack-destination", releaseRoot], { cwd: stage });
  }
  const tarballs = packages.map(package_ =>
    join(releaseRoot, `sundw-locus-scope-${package_.name === "locus-scope" ? "" : `${package_.name}-`}${version}.tgz`)
  );
  console.log(`Packed ${tarballs.join(", ")}`);
  return { releaseRoot, tarballs, version };
}

if (isMain(import.meta.url)) {
  releaseNpm().catch(error => {
    console.error(error.message);
    process.exitCode = 1;
  });
}
