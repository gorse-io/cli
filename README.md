# Gorse CLI

[![GitHub release](https://img.shields.io/github/v/release/gorse-io/cli)](https://github.com/gorse-io/cli/releases/latest)
[![GitHub downloads](https://img.shields.io/github/downloads/gorse-io/cli/total)](https://github.com/gorse-io/cli/releases)
[![npm version](https://img.shields.io/npm/v/%40gorse%2Fcli)](https://www.npmjs.com/package/@gorse/cli)
[![npm downloads](https://img.shields.io/npm/dm/%40gorse%2Fcli)](https://www.npmjs.com/package/@gorse/cli)

CLI for Gorse recommender system.

## Commands

Data commands use resource-first syntax:

```sh
gorse-cli item get <item-id>
gorse-cli item list -n 10
gorse-cli item list <query...> -n 10
gorse-cli user get <user-id>
gorse-cli user list -n 10
gorse-cli feedback list --type click --user <user-id> -n 10
gorse-cli category list
```

These replace `get item`, `get items`, `get user`, `get users`,
`get feedback`, and `get categories`; the top-level `get` command is removed.
Authentication flags, filters, and output formats are unchanged. The `context`,
`pipeline`, `recommend`, `cluster-info`, `stats`, `ps`, `dump`, and `restore`
commands retain their existing syntax.

## Install

- Install using bash:

```sh
curl -fsSL https://gorse.io/install.sh | sh
```

- Install using PowerShell:

```powershell
irm https://gorse.io/install.ps1 | iex
```

- Install using npm:

```sh
npm install -g @gorse/cli
```

- Install using Homebrew:

```sh
brew tap gorse-io/tap
brew install gorse-cli
```

- Install using scoop:

```sh
scoop bucket add gorse https://github.com/gorse-io/scoop-bucket
scoop install gorse-cli
```

- Install using Go:

```sh
go install github.com/gorse-io/cli@latest
```
