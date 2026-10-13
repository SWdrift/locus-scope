import { readdir } from "node:fs/promises";
import { join } from "node:path";
import { assertOrdinaryFile, isMain, repositoryRoot, run } from "./lib/workspace.mjs";

async function assertDependencies() {
  for (const path of [
    join(repositoryRoot, "node_modules", "verdaccio", "package.json"),
    join(repositoryRoot, "node_modules", "remark-cli", "package.json"),
    join(repositoryRoot, "node_modules", "typescript", "package.json"),
    join(repositoryRoot, "node_modules", "tsx", "package.json")
  ]) {
    try {
      await assertOrdinaryFile(path, "Node dependency");
    } catch {
      throw new Error("root Node dependencies are not installed; run: pnpm install --frozen-lockfile");
    }
  }
}

async function testFiles(directory, suffix) {
  return (await readdir(join(repositoryRoot, directory)))
    .filter(name => name.endsWith(suffix))
    .sort()
    .map(name => join(repositoryRoot, directory, name));
}

export async function test(suite = "all") {
  if (!["all", "e2e"].includes(suite)) throw new Error(`unknown test suite: ${suite}`);
  await assertDependencies();
  if (suite === "e2e") {
    const { releaseNpm } = await import("./npm-release.mjs");
    const { verifyNpmRelease } = await import("./verify-npm-release.mjs");
    const { readVersion } = await import("./lib/workspace.mjs");
    await releaseNpm();
    await verifyNpmRelease(await readVersion());
    const e2eTests = await testFiles("test/e2e", ".test.mjs");
    if (e2eTests.length > 0) run(process.execPath, ["--test", ...e2eTests]);
    return;
  }
  run(process.execPath, ["node_modules/typescript/bin/tsc", "--noEmit"]);
  const unitTests = await testFiles("test/unit", ".test.ts");
  if (unitTests.length > 0) run(process.execPath, ["node_modules/tsx/dist/cli.mjs", "--test", ...unitTests]);
}
if (isMain(import.meta.url)) {
  test(process.argv[2] ?? "all").catch(error => {
    console.error(error.message);
    process.exitCode = 1;
  });
}
