import assert from "node:assert/strict";
import { mkdir, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { test } from "node:test";
import { buildPackageEnvironment } from "@sundw/locus-scope-core";
import { ApplicationError } from "@sundw/locus-scope-core";
import { loadPackageEnvironment as load, secureEntry } from "@sundw/locus-scope-core";

async function writeJson(filename: string, value: unknown): Promise<void> {
  await mkdir(path.dirname(filename), { recursive: true });
  await writeFile(filename, `${JSON.stringify(value, null, 2)}\n`);
}

test("builds importer-relative exact package identities", async () => {
  const root = path.resolve("temp/unit-ts/package-environment");
  await rm(root, { recursive: true, force: true });
  try {
    await writeJson(path.join(root, "package.json"), {
      name: "consumer",
      version: "1.0.0",
      dependencies: { "@example/app": "1.0.0", "@example/base": "3.0.0" }
    });
    await writeJson(path.join(root, "node_modules/@example/app/package.json"), {
      name: "@example/app",
      version: "1.0.0",
      dependencies: { "@example/base": "2.0.0" },
      locus: { entry: "locus.yaml" }
    });
    await writeFile(
      path.join(root, "node_modules/@example/app/locus.yaml"),
      'id: app\nimports:\n  base: "@example/base"\n'
    );
    await writeJson(path.join(root, "node_modules/@example/app/node_modules/@example/base/package.json"), {
      name: "@example/base",
      version: "2.0.0",
      locus: { entry: "locus.yaml" }
    });
    await writeFile(
      path.join(root, "node_modules/@example/app/node_modules/@example/base/locus.yaml"),
      "id: app-base\n"
    );
    await writeJson(path.join(root, "node_modules/@example/base/package.json"), {
      name: "@example/base",
      version: "3.0.0",
      locus: { entry: "locus.yaml" }
    });
    await writeFile(path.join(root, "node_modules/@example/base/locus.yaml"), "id: root-base\n");
    await writeFile(
      path.join(root, "locus.yaml"),
      'id: consumer\nimports:\n  app: "@example/app"\n  base: "@example/base"\n'
    );

    const environment = await buildPackageEnvironment(root);
    assert.equal(environment.rootDependencies["@example/app"], "npm:@example/app@1.0.0");
    assert.equal(environment.rootDependencies["@example/base"], "npm:@example/base@3.0.0");
    assert.equal(
      environment.packages["npm:@example/app@1.0.0"]?.dependencies["@example/base"],
      "npm:@example/base@2.0.0"
    );
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("rejects entry paths that escape a package root", async () => {
  const root = path.resolve("temp/unit-ts/package-entry");
  await rm(root, { recursive: true, force: true });
  await mkdir(root, { recursive: true });
  try {
    await assert.rejects(secureEntry(root, "../locus.yaml"), /relative slash path|remain inside/);
    await writeFile(path.join(root, "locus.yaml"), "id: package\\n");
    assert.equal((await secureEntry(root, "./locus.yaml")).entry, "locus.yaml");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("rejects inherited package identity properties", async () => {
  const root = path.resolve("temp/unit-ts/package-prototype");
  await rm(root, { recursive: true, force: true });
  await mkdir(root, { recursive: true });
  try {
    await writeFile(path.join(root, "locus.yaml"), "id: consumer\n");
    await assert.rejects(
      load({ rootScope: root, rootDependencies: { toString: "npm:missing@1.0.0" }, packages: {} }),
      error => error instanceof ApplicationError && error.reason === "scope.dependency_unresolved"
    );
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
