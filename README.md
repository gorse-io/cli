# Gorse CLI

[![GitHub release](https://img.shields.io/github/v/release/gorse-io/cli)](https://github.com/gorse-io/cli/releases/latest)
[![GitHub downloads](https://img.shields.io/github/downloads/gorse-io/cli/total)](https://github.com/gorse-io/cli/releases)
[![npm version](https://img.shields.io/npm/v/%40gorse%2Fcli)](https://www.npmjs.com/package/@gorse/cli)
[![npm downloads](https://img.shields.io/npm/dm/%40gorse%2Fcli)](https://www.npmjs.com/package/@gorse/cli)

CLI for Gorse recommender system.

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

## Shell Completion

Gorse CLI supports Tab completion for commands and flags in Bash, Zsh, Fish, and PowerShell.

### Bash

Install `bash-completion` using your OS's package manager if it is not already installed, then load completions for the current session:

```bash
source <(gorse-cli completion bash)
```

To enable completions for future sessions, add the command above to `~/.bashrc` (or `~/.bash_profile` on macOS), after loading `bash-completion`.

### Zsh

Enable Zsh's completion system and load completions for the current session:

```zsh
autoload -Uz compinit
compinit
source <(gorse-cli completion zsh)
```

To enable completions for future sessions, add these commands to `~/.zshrc`. If your configuration already runs `compinit`, add only the `source` command after it.

### Fish

Load completions for the current session:

```fish
gorse-cli completion fish | source
```

To enable completions for future sessions, generate a completion file once:

```fish
mkdir -p ~/.config/fish/completions
gorse-cli completion fish > ~/.config/fish/completions/gorse-cli.fish
```

### PowerShell

Load completions for the current session:

```powershell
gorse-cli completion powershell | Out-String | Invoke-Expression
```

To enable completions for future sessions, add the command above to your PowerShell profile (`$PROFILE`). Create the profile if it does not exist:

```powershell
if (!(Test-Path $PROFILE)) {
    New-Item -ItemType File -Path $PROFILE -Force
}
notepad $PROFILE
```

For more installation options, run `gorse-cli completion <shell> --help`.
