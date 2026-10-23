# Gorse CLI

Command-line tools for managing Gorse clusters. Migrated from
`gorse-io/gorse/cmd/gorse-cli` with the existing commands and integration tests.

## Install

With npm (Node.js 18 or later):

```sh
npm install -g @gorse-io/cli
gorse-cli --help
```

Or run without a global installation:

```sh
npx @gorse-io/cli --help
```

The npm package selects a prebuilt native binary through platform-specific optional
dependencies; no Go compiler or install scripts are required. Keep optional
dependencies enabled. Supported platforms match the standalone releases: Linux
(x64, arm64, loong64, riscv64), Windows (x64, arm64), and macOS (arm64).

Build from source:

```sh
git clone https://github.com/gorse-io/cli.git
cd cli
go build -o gorse-cli .
```

Use a source checkout rather than `go install ...@latest`: this module includes
a SQLite replacement required by the integration tests.

Standalone release binaries retain the `gorse-cli` name. Install on Linux or macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/gorse-io/cli/main/install.sh | sh
```

On Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/gorse-io/cli/main/install.ps1 | iex
```

## Usage

```sh
gorse-cli context add dev --endpoint http://localhost:8088
gorse-cli context use dev
gorse-cli --help
```

Contexts and API keys remain in the system keyring under the existing
`gorse-cli` service, so saved contexts continue to work.

## Development

```sh
go test ./...
go vet ./...
go build -o gorse-cli .
GORSE_CLI_TEST_BINARY="$PWD/gorse-cli" node --test npm/test/*.test.js
```

### Publishing npm packages

The release workflow packages the same binaries uploaded by GoReleaser and publishes
platform packages first, followed by `@gorse-io/cli` with exact-version optional
dependencies. Stable releases use npm's `latest` tag; prereleases use `next`.
Configure the repository's `NPM_TOKEN` secret with permission to publish all
`@gorse-io/cli*` packages in the npm organization before publishing a release.
The GitHub release tag must be a canonical npm SemVer version, optionally prefixed
with `v` (for example, `v1.2.3` or `v1.2.3-rc.1`).

To inspect packages locally after downloading all release binaries:

```sh
node npm/scripts/build-packages.js v1.2.3 dist/npm-binaries dist/npm
npm pack ./dist/npm/cli
```

Tests start a real local Gorse Master with temporary SQLite stores. The Gorse
module is pinned in `go.mod`; its SQLite replacement must also be declared here
because Go does not inherit dependency replacement directives.
