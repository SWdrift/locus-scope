import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { test } from "node:test";
import { promisify } from "node:util";
const repositoryRoot = path.resolve(".");
const version = (await readFile(path.join(repositoryRoot, "VERSION"), "utf8")).trim();
const root = path.resolve("temp/e2e-run/local-package");
const tarball = path.resolve("temp/release/npm", `sundw-locus-scope-${version}.tgz`);
const pnpm = process.platform === "win32" ? "pnpm.cmd" : "pnpm";
const execFileAsync = promisify(execFile);
async function runPnpm(cwd, args) {
  return execFileAsync(pnpm, args, { cwd, windowsHide: true, shell: process.platform === "win32" });
}

test("local tarball consumer preserves small-workspace CLI and library behavior", async () => {
  await rm(root, { recursive: true, force: true });
  await mkdir(root, { recursive: true });
  await writeFile(
    path.join(root, "package.json"),
    JSON.stringify(
      {
        name: "local-package-consumer",
        version: "1.0.0",
        private: true,
        dependencies: { "@sundw/locus-scope": `file:${tarball}` }
      },
      null,
      2
    ) + "\n"
  );
  await writeFile(path.join(root, "pnpm-workspace.yaml"), 'packages:\n  - "."\n');
  await writeFile(path.join(root, "locus.yaml"), "id: local-consumer\nexports: [api, db]\n");
  await writeFile(
    path.join(root, "entities.locus.yaml"),
    "entities:\n  - id: api\n    type: service\n  - id: db\n    type: database\nrelations:\n  - from: api\n    type: depends_on\n    to: db\n"
  );
  await runPnpm(root, ["install", "--ignore-workspace", "--ignore-scripts"]);
  const validate = await runPnpm(root, ["--silent", "exec", "locus-scope", "validate", "--json"]);
  assert.equal(JSON.parse(validate.stdout).valid, true);
  const graph = await runPnpm(root, ["--silent", "exec", "locus-scope", "graph", "--json"]);
  assert.equal(JSON.parse(graph.stdout).nodes.length, 2);
  const probe = path.join(root, "probe.mjs");
  await writeFile(
    probe,
    'import { ApplicationService, buildPackageEnvironment, loadPackageEnvironment } from "@sundw/locus-scope"; const e = await buildPackageEnvironment(process.cwd()); const r = new ApplicationService(await loadPackageEnvironment(e)).validate(); console.log(JSON.stringify(r));\n'
  );
  const library = await execFileAsync(process.execPath, [probe], { cwd: root, windowsHide: true });
  assert.equal(JSON.parse(library.stdout).valid, true);
  assert.equal(
    (await readFile(path.join(root, "node_modules", "@sundw", "locus-scope", "README.md"), "utf8")).includes(
      "locus-scope"
    ),
    true
  );
});
