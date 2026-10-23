#!/usr/bin/env node
import { chmodSync, copyFileSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { platforms } from "../platforms.js";

const [tag, binaries, output] = process.argv.slice(2);
const version = tag?.replace(/^v/, "");
// npm versions must be canonical SemVer; reject build metadata (npm strips it).
const number = "(?:0|[1-9][0-9]*)";
const identifier = `(?:${number}|[0-9]*[A-Za-z-][0-9A-Za-z-]*)`;
if (!version || !new RegExp(`^${number}\\.${number}\\.${number}(?:-${identifier}(?:\\.${identifier})*)?$`).test(version) || !binaries || !output) {
  throw new Error("Usage: node npm/scripts/build-packages.js <version> <binaries-dir> <output-dir> (version must be canonical SemVer)");
}
const npmRoot = fileURLToPath(new URL("../", import.meta.url));
const repoRoot = path.resolve(npmRoot, "..");
const main = JSON.parse(readFileSync(path.join(npmRoot, "package.json"), "utf8"));
main.version = version;
main.optionalDependencies = {};

function stage(directory, manifest) {
  mkdirSync(directory, { recursive: true });
  writeFileSync(path.join(directory, "package.json"), JSON.stringify(manifest, null, 2) + "\n");
  for (const file of ["README.md", "LICENSE"]) copyFileSync(path.join(repoRoot, file), path.join(directory, file));
}

for (const target of platforms) {
  const name = `cli-${target.os}-${target.cpu}`;
  const packageName = `@gorse-io/${name}`;
  const directory = path.join(output, name);
  const executable = target.os === "win32" ? "gorse-cli.exe" : "gorse-cli";
  mkdirSync(path.join(directory, "bin"), { recursive: true });
  copyFileSync(path.join(binaries, `gorse-cli_${target.goos}_${target.goarch}${target.os === "win32" ? ".exe" : ""}`), path.join(directory, "bin", executable));
  chmodSync(path.join(directory, "bin", executable), 0o755);
  stage(directory, {
    name: packageName, version, description: main.description, license: main.license,
    repository: main.repository, publishConfig: main.publishConfig,
    os: [target.os], cpu: [target.cpu], files: ["bin"],
  });
  main.optionalDependencies[packageName] = version;
}
const directory = path.join(output, "cli");
mkdirSync(path.join(directory, "bin"), { recursive: true });
copyFileSync(path.join(npmRoot, "bin/gorse-cli.js"), path.join(directory, "bin/gorse-cli.js"));
chmodSync(path.join(directory, "bin/gorse-cli.js"), 0o755);
stage(directory, main);
