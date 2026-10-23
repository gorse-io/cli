// Keep aligned with the targets in .goreleaser.yaml.
export const platforms = [
  { os: "linux", cpu: "x64", goos: "linux", goarch: "amd64" },
  { os: "linux", cpu: "arm64", goos: "linux", goarch: "arm64" },
  { os: "linux", cpu: "loong64", goos: "linux", goarch: "loong64" },
  { os: "linux", cpu: "riscv64", goos: "linux", goarch: "riscv64" },
  { os: "win32", cpu: "x64", goos: "windows", goarch: "amd64" },
  { os: "win32", cpu: "arm64", goos: "windows", goarch: "arm64" },
  { os: "darwin", cpu: "arm64", goos: "darwin", goarch: "arm64" },
];
