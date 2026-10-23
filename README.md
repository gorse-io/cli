# Gorse CLI

Command-line tools for managing Gorse clusters. Migrated from
`gorse-io/gorse/cmd/gorse-cli` with the existing commands and integration tests.

## Install

Build from source:

```sh
git clone https://github.com/gorse-io/cli.git
cd cli
go build -o gorse-cli ./cmd/gorse-cli
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
go build -o gorse-cli ./cmd/gorse-cli
```

Tests start a real local Gorse Master with temporary SQLite stores. The Gorse
module is pinned in `go.mod`; its SQLite replacement must also be declared here
because Go does not inherit dependency replacement directives.
