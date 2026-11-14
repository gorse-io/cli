#!/usr/bin/env node
import { chmodSync, copyFileSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

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

function stage(directory, manifest) {
  mkdirSync(directory, { recursive: true });
  writeFileSync(path.join(directory, "package.json"), JSON.stringify(manifest, null, 2) + "\n");
  for (const file of ["README.md", "LICENSE"]) copyFileSync(path.join(repoRoot, file), path.join(directory, file));
}

for (const packageName of Object.keys(main.optionalDependencies)) {
  const name = packageName.split("/")[1];
  const target = name.replace(/^cli-/, "");
  const manifest = JSON.parse(readFileSync(path.join(npmRoot, "packages", target, "package.json"), "utf8"));
  manifest.version = version;
  const goos = manifest.os[0] === "win32" ? "windows" : manifest.os[0];
  const goarch = manifest.cpu[0] === "x64" ? "amd64" : manifest.cpu[0];
  const directory = path.join(output, name);
  const executable = goos === "windows" ? "gorse-cli.exe" : "gorse-cli";
  mkdirSync(path.join(directory, "bin"), { recursive: true });
  copyFileSync(path.join(binaries, `gorse-cli_${goos}_${goarch}${goos === "windows" ? ".exe" : ""}`), path.join(directory, "bin", executable));
  chmodSync(path.join(directory, "bin", executable), 0o755);
  stage(directory, manifest);
  main.optionalDependencies[packageName] = version;
}
const directory = path.join(output, "cli");
mkdirSync(path.join(directory, "bin"), { recursive: true });
copyFileSync(path.join(npmRoot, "bin/gorse-cli.js"), path.join(directory, "bin/gorse-cli.js"));
chmodSync(path.join(directory, "bin/gorse-cli.js"), 0o755);
stage(directory, main);
