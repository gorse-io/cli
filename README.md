# Gorse CLI

Command-line tools for managing Gorse clusters. Migrated from
`gorse-io/gorse/cmd/gorse-cli` with the existing commands and integration tests.

## Install

With npm (Node.js 18 or later):

```sh
npm install -g @gorse/cli
gorse-cli --help
```

Or run without a global installation:

```sh
npx @gorse/cli --help
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

### Package manager release publishing

Stable releases automatically update `bucket/gorse-cli.json` on the `main`
branch of `gorse-io/scoop-bucket` through GoReleaser. The manifest includes
Windows amd64 and arm64 ZIP download URLs and SHA256 hashes. Existing standalone
release binaries remain available. Prerelease tags do not update the bucket.

Before publishing, configure the `SCOOP_BUCKET_TOKEN` Actions secret in this
repository with a fine-grained personal access token restricted to
`gorse-io/scoop-bucket`, with **Contents: Read and write** permission. The token
must be allowed to push to the bucket's `main` branch (and approved by the
organization if required). The default `GITHUB_TOKEN` remains responsible for
uploading CLI release assets; it cannot write to the separate bucket repository.

The same release also updates `Formula/gorse-cli.rb` on the `main` branch of
`gorse-io/homebrew-tap` for macOS arm64 and Linux amd64/arm64. Configure a second
Actions secret, `HOMEBREW_TAP_TOKEN`, restricted to that repository with
**Contents: Read and write** permission and access to push to `main`.
Prerelease tags do not update the tap either. Homebrew uses TAR.GZ archives;
Scoop uses ZIP archives, while standalone binaries remain unchanged.

The Homebrew publisher uses GoReleaser's deprecated but still supported `brews`
configuration to update the existing Formula in place. Migrating the tap to
Casks and adding tap migration metadata is outside this change.

Validate release artifacts locally without publishing:

```sh
goreleaser release --snapshot --clean
```

`goreleaser check` reports the intentional `brews` deprecation as a nonzero
exit; snapshot generation validates both package manager outputs without
publishing.

```sh
go test ./...
go vet ./...
go build -o gorse-cli .
```

### Publishing npm packages

The release workflow packages the same binaries uploaded by GoReleaser and publishes
platform packages first, followed by `@gorse/cli` with exact-version optional
dependencies. Stable releases use npm's `latest` tag; prereleases use `next`.
Configure the repository's `NPM_TOKEN` secret with permission to publish all
`@gorse/cli*` packages in the npm organization before publishing a release.
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
