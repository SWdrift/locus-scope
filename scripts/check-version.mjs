import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { isMain, readVersion, repositoryRoot } from "./lib/workspace.mjs";

async function readJson(filename) {
  return JSON.parse(await readFile(filename, "utf8"));
}

export async function checkVersion() {
  const version = await readVersion();
  for (const packageName of ["core", "cli"]) {
    const manifest = await readJson(join(repositoryRoot, "packages", packageName, "package.json"));
    if (manifest.version !== version)
      throw new Error(`packages/${packageName}/package.json version ${manifest.version} does not match VERSION ${version}`);
  }
  const releaseManifest = await readJson(join(repositoryRoot, "packaging", "npm", "locus-scope", "package.json"));
  if (releaseManifest.dependencies?.["@sundw/locus-scope-core"] !== "workspace:*")
    throw new Error("packaging/npm/locus-scope/package.json must use workspace:* for the Core dependency");
  console.log(`Version ${version} is synchronized`);
}

if (isMain(import.meta.url)) {
  checkVersion().catch(error => {
    console.error(error.message);
    process.exitCode = 1;
  });
}
