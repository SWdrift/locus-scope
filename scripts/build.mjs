import { join } from "node:path";
import { ensureOrdinaryDirectory, isMain, readVersion, removeOwnedDirectory, run, tempRoot } from "./lib/workspace.mjs";

export async function build({ quiet = false } = {}) {
  const version = await readVersion();
  const outputRoot = join(tempRoot, "ts-build");
  await ensureOrdinaryDirectory(tempRoot, "build path");
  await removeOwnedDirectory(outputRoot, "TypeScript build output");
  run(process.execPath, ["node_modules/typescript/bin/tsc", "--project", "tsconfig.json"]);
  if (!quiet) console.log(`Built TypeScript output in ${outputRoot}`);
  return { outputRoot, outputPath: join(outputRoot, "bin.js"), version };
}

if (isMain(import.meta.url)) {
  build().catch(error => {
    console.error(error.message);
    process.exitCode = 1;
  });
}
