# Hardcover CLI

An unofficial command-line tool for managing your [Hardcover](https://hardcover.app) library — list books, log progress, check stats, track goals, and export your reading history.

## Installation

### Homebrew (macOS/Linux)

```bash
brew install KIRKR101/tap/hardcover
```

### Install script (macOS/Linux)

```bash
curl -sSfL https://raw.githubusercontent.com/KIRKR101/hardcover-cli/main/install.sh | sh
```

Or with a specific version:

```bash
curl -sSfL https://raw.githubusercontent.com/KIRKR101/hardcover-cli/main/install.sh | sh -s -- v0.1.0
```

### Go install

```bash
go install github.com/KIRKR101/hardcover-cli@latest
```

Note: The binary will be named `hardcover-cli` when installed via `go install`. Run `hardcover-cli setup` instead of `hardcover setup`, or add an alias: `alias hardcover=hardcover-cli`. Add `$(go env GOPATH)/bin` to your `PATH` if needed.

### Build from source

```bash
git clone https://github.com/KIRKR101/hardcover-cli
cd hardcover-cli
go build -o hardcover .
./hardcover setup
```

## Requirements

- A Hardcover API token

## Setup

Get an API token from [Hardcover's account settings](https://hardcover.app/account/api), then run:

```bash
hardcover setup
```

The token is stored in `~/.hardcover.json` with `0600` permissions. Alternatively, set `HARDCOVER_TOKEN` and skip `setup`.

## Commands

| Command | Description |
|---|---|
| `hardcover whoami` | Show your Hardcover user info |
| `hardcover library [-s status] [-l limit] [-o offset]` | List books in your library |
| `hardcover stats` | Show reading statistics and active goals |
| `hardcover progress` | Show current reading progress |
| `hardcover search <query> [-l limit]` | Search for books on Hardcover |
| `hardcover goals [--all]` | Show reading goals |
| `hardcover log [book] [--id id] [--pages n] [--percent n] [--status s] [--rating r]` | Log progress, status, or rating |
| `hardcover export [-o file]` | Export reading journal to CSV |
| `hardcover daily [-d days]` | Show daily reading log |
| `hardcover completion <bash\|zsh\|fish\|powershell>` | Generate shell completions |

Most commands accept `--json` for raw JSON output suitable for piping into `jq`.

### `log` in detail

`hardcover log` matches a book by fuzzy title search, or by `--id` if you already know the `user_book` id. If multiple books match the title, an interactive picker launches (in TTY mode) with keyboard navigation:

```
> Select a book
  1. The Stranger                 — Albert Camus
  2. The Myth of Sisyphus         — Albert Camus
  3. Stranger in a Strange Land   — Robert A. Heinlein
```

`↑`/`↓` to move, `/` to filter, `enter` to select, `esc`/`ctrl-c` to cancel.

Provide at least one of `--pages`, `--percent`, `--status`, or `--rating`. Examples:

```bash
hardcover log "the stranger" --status reading
hardcover log "the stranger" --pages 120
hardcover log "the stranger" --percent 75
hardcover log "the stranger" --rating 4.5
hardcover log --id 123456 --status read --rating 5
```

## JSON output

For scripting, most commands support `--json`:

```bash
hardcover library --status read --json | jq '.[].book.title'
```

## Flags

- `--no-color` — disable colored output (also respects `NO_COLOR`)
- `--json` — output raw JSON to stdout (per-command)
- `--version` — print the version

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Generic error (bad input, validation, JSON decode failure) |
| 2 | Auth/config error (no token, 401/403 from the API) |
| 3 | Network error (timeout, connection refused, 5xx) |

`--json` mode keeps the data on stdout and any spinner status on stderr, so piping doesn't get ANSI/control codes mixed into the JSON.

## Notes

- Config: `~/.hardcover.json`
- API: `https://api.hardcover.app/v1/graphql`
- Ctrl-C cancels in-flight HTTP requests, not just the UI.
