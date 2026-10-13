#!/usr/bin/env node
process.env.LOCUS_SCOPE_VERSION ??= process.env.npm_package_version ?? "development";
await import("../lib/bin.js");
