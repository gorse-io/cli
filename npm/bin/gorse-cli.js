#!/usr/bin/env node
import { spawn } from "node:child_process";
import { createRequire } from "node:module";
import path from "node:path";

const require = createRequire(import.meta.url);
const packageName = `@gorse-io/cli-${process.platform}-${process.arch}`;
let binary;
try {
  binary = path.join(path.dirname(require.resolve(`${packageName}/package.json`)), "bin", process.platform === "win32" ? "gorse-cli.exe" : "gorse-cli");
} catch {
  console.error(`Missing optional dependency ${packageName}. Check that your platform is supported and reinstall with optional dependencies enabled: npm install -g @gorse-io/cli`);
  process.exit(1);
}

const child = spawn(binary, process.argv.slice(2), { stdio: "inherit" });
const handlers = new Map();
for (const signal of ["SIGINT", "SIGTERM", "SIGHUP"]) {
  const handler = () => child.kill(signal);
  handlers.set(signal, handler);
  process.on(signal, handler);
}
child.on("error", (error) => {
  console.error(`Unable to start gorse-cli: ${error.message}`);
  process.exit(1);
});
child.on("exit", (code, signal) => {
  for (const [name, handler] of handlers) process.removeListener(name, handler);
  if (signal) process.kill(process.pid, signal);
  else process.exit(code ?? 1);
});
