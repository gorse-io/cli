import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, mkdirSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const root = fileURLToPath(new URL("../../", import.meta.url));

test("stages declared platform packages with externally built binaries", () => {
  const temporary = mkdtempSync(path.join(os.tmpdir(), "gorse-npm-"));
  try {
    const binaries = path.join(temporary, "binaries");
    const output = path.join(temporary, "packages");
    mkdirSync(binaries);
    const main = JSON.parse(readFileSync(path.join(root, "npm/package.json")));
    const targets = readdirSync(path.join(root, "npm/packages"));
    assert.equal(targets.length, 7);
    assert.equal(Object.keys(main.optionalDependencies).length, targets.length);
    for (const target of targets) {
      const manifest = JSON.parse(readFileSync(path.join(root, "npm/packages", target, "package.json")));
      assert.equal(manifest.name, `@gorse/cli-${target}`);
      assert.equal(main.optionalDependencies[manifest.name], main.version);
      const goos = manifest.os[0] === "win32" ? "windows" : manifest.os[0];
      const goarch = manifest.cpu[0] === "x64" ? "amd64" : manifest.cpu[0];
      const extension = goos === "windows" ? ".exe" : "";
      // Non-executable fixtures check byte-preserving staging, not cross-platform execution.
      writeFileSync(path.join(binaries, `gorse-cli_${goos}_${goarch}${extension}`), `fixture:${target}`);
    }
    execFileSync(process.execPath, ["npm/scripts/build-packages.js", "v1.2.3-beta.1", binaries, output], { cwd: root });
    const stagedMain = JSON.parse(readFileSync(path.join(output, "cli/package.json")));
    assert.equal(stagedMain.version, "1.2.3-beta.1");
    for (const target of targets) {
      const directory = path.join(output, `cli-${target}`);
      const manifest = JSON.parse(readFileSync(path.join(directory, "package.json")));
      assert.equal(manifest.version, stagedMain.version);
      assert.equal(stagedMain.optionalDependencies[manifest.name], stagedMain.version);
      const binary = path.join(directory, "bin", target.startsWith("win32-") ? "gorse-cli.exe" : "gorse-cli");
      assert.equal(readFileSync(binary, "utf8"), `fixture:${target}`);
      if (process.platform !== "win32") assert.equal(statSync(binary).mode & 0o777, 0o755);
      assert.equal(readFileSync(path.join(directory, "LICENSE"), "utf8"), readFileSync(path.join(root, "LICENSE"), "utf8"));
    }
  } finally {
    rmSync(temporary, { recursive: true, force: true });
  }
});
