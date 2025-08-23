# `tobi`

`tobi` extracts all tags from your Obsidian vault. `tobi` was created because I wanted a way to list all my tags and pass them to an LLM as context so it could suggest tags that fit my existing hierarchical tag structure.

## Features

- **Fast, cached scans**: results are cached and used if no changes are detected.
- **Respects ignore rules**: skips `.git/` and files/directories ignored by `.gitignore` and `.tobiignore`.
- **Flexible output modes**: show only tag names, or with counts, or with relative frequency percentages.
- **Per‑vault tag excludes**: ignore tags via glob patterns in `.tobi.exclude`.

## Installation

[Make sure Go is installed](https://go.dev/doc/install) before running the following command:

```sh
go install github.com/nt54hamnghi/tobi@latest
```

You may need to add the following environment variables to run `tobi`:

```sh
export GOROOT=/usr/local/go
export GOPATH=$HOME/go
# Add Go binary paths and local user binaries to the system PATH
export PATH=$GOPATH/bin:$GOROOT/bin:$HOME/.local/bin:$PATH
```

## Usage

### Basics

If you pass no path, `tobi` uses `OBSIDIAN_VAULT_PATH`. Otherwise, it scans the given directory.

```bash
# Scan the current directory (your vault)
tobi .

# Or set a default vault path and run without arguments
export OBSIDIAN_VAULT_PATH=/path/to/your/vault
tobi

# Show top 10 tag names
tobi . --limit 10

# Show counts
tobi . --mode count

# Show relative frequencies (percent)
tobi . --mode relative

# Force a fresh scan (ignore cache)
tobi . --no-cache
```

### `.gitignore` and `.tobiignore`

`tobi` filters out files using patterns defined in both `.gitignore` and `.tobiignore`.

The `.tobiignore` file is specifically for `tobi`-only exclusions, which is useful if you want to exclude items from `tobi` without modifying your `.gitignore`. It follows the same pattern syntax as `.gitignore`.

### `.tobi.exclude`

The `.tobi.exclude` file excludes specific tags from the output (not files). Place it at your vault root with one glob pattern per line.

Example `.tobi.exclude`:

```text
# skip all tags starting with "personal" or "daily"
{personal,daily}/*

# skip specific tags
work
project
```
