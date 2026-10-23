import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { copyFileSync, mkdtempSync, mkdirSync, writeFileSync, readFileSync, existsSync, rmSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const root = fileURLToPath(new URL("../../", import.meta.url));
const targets = [
  ["linux", "x64", "linux", "amd64"],
  ["linux", "arm64", "linux", "arm64"],
  ["linux", "loong64", "linux", "loong64"],
  ["linux", "riscv64", "linux", "riscv64"],
  ["win32", "x64", "windows", "amd64"],
  ["win32", "arm64", "windows", "arm64"],
  ["darwin", "arm64", "darwin", "arm64"],
];

function fixture(t) {
  const dir = mkdtempSync(path.join(os.tmpdir(), "gorse-npm-"));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  const binaries = path.join(dir, "binaries");
  mkdirSync(binaries);
  for (const [, , goos, goarch] of targets) {
    writeFileSync(path.join(binaries, `gorse-cli_${goos}_${goarch}${goos === "windows" ? ".exe" : ""}`), "test payload");
  }
  return { dir, binaries, output: path.join(dir, "packages") };
}

function build(f, version = "v1.2.3") {
  return spawnSync(process.execPath, ["npm/scripts/build-packages.js", version, f.binaries, f.output], { cwd: root, encoding: "utf8" });
}

function manifest(dir) {
  return JSON.parse(readFileSync(path.join(dir, "package.json"), "utf8"));
}

test("stages exact-version optional packages for every release target", (t) => {
  const f = fixture(t);
  const result = build(f);
  assert.equal(result.status, 0, result.stderr);
  const main = manifest(path.join(f.output, "cli"));
  assert.equal(main.name, "@gorse-io/cli");
  assert.equal(main.version, "1.2.3");
  assert.equal(main.bin["gorse-cli"], "bin/gorse-cli.js");
  assert.equal(Object.keys(main.optionalDependencies).length, targets.length);
  assert.equal(main.scripts?.postinstall, undefined);
  for (const [platform, arch] of targets) {
    const pkg = manifest(path.join(f.output, `cli-${platform}-${arch}`));
    assert.equal(main.optionalDependencies[pkg.name], "1.2.3");
    assert.deepEqual(pkg.os, [platform]);
    assert.deepEqual(pkg.cpu, [arch]);
    assert.equal(pkg.version, main.version);
    assert.ok(existsSync(path.join(f.output, `cli-${platform}-${arch}`, "bin", platform === "win32" ? "gorse-cli.exe" : "gorse-cli")));
  }
});

test("rejects invalid versions and missing release binaries", (t) => {
  const f = fixture(t);
  assert.notEqual(build(f, "../bad").status, 0);
  rmSync(path.join(f.binaries, "gorse-cli_linux_arm64"));
  const result = build(f);
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /gorse-cli_linux_arm64/);
});

test("packed npm install runs the real native CLI without lifecycle scripts", { skip: !process.env.GORSE_CLI_TEST_BINARY }, (t) => {
  const f = fixture(t);
  const target = targets.find(([platform, arch]) => platform === process.platform && arch === process.arch);
  assert.ok(target, "host must be a release target");
  const [platform, arch, goos, goarch] = target;
  copyFileSync(process.env.GORSE_CLI_TEST_BINARY, path.join(f.binaries, `gorse-cli_${goos}_${goarch}${platform === "win32" ? ".exe" : ""}`));
  const result = build(f);
  assert.equal(result.status, 0, result.stderr);
  const tarballs = [];
  for (const name of ["cli", `cli-${platform}-${arch}`]) {
    const packed = spawnSync("npm", ["pack", path.join(f.output, name), "--json", "--pack-destination", f.dir], { encoding: "utf8" });
    assert.equal(packed.status, 0, packed.stderr);
    tarballs.push(path.join(f.dir, JSON.parse(packed.stdout)[0].filename));
  }
  const prefix = path.join(f.dir, "installed");
  const installed = spawnSync("npm", ["install", "--global", "--prefix", prefix, "--offline", "--ignore-scripts", "--no-audit", "--no-fund", ...tarballs], { encoding: "utf8" });
  assert.equal(installed.status, 0, installed.stderr);
  const executable = path.join(prefix, "bin", "gorse-cli");
  for (const arg of ["--help", "--version"]) {
    const run = spawnSync(executable, [arg], { encoding: "utf8" });
    assert.equal(run.status, 0, run.stderr);
    assert.match(run.stdout + run.stderr, arg === "--help" ? /gorse-cli/i : /API version:/);
  }
  const invalid = spawnSync(executable, ["nonexistent-command"], { encoding: "utf8" });
  assert.equal(invalid.status, 1);
});

test("launcher reports a missing optional dependency with reinstall instructions", () => {
  const result = spawnSync(process.execPath, ["npm/bin/gorse-cli.js", "--help"], { cwd: root, encoding: "utf8" });
  assert.equal(result.status, 1);
  assert.match(result.stderr, /Missing optional dependency/);
  assert.match(result.stderr, /npm install -g @gorse-io\/cli/);
});
