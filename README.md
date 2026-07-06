# Hardcover CLI

An unofficial command-line tool for managing your [Hardcover](https://hardcover.app) library; list books, log progress, check stats, track goals, and export your reading history.

## Requirements

- Python 3.8+
- `requests`

```bash
pip install requests
```

## Setup

Get your API token from [Hardcover's account settings](https://hardcover.app/account/api), then run:

```bash
python3 hardcover.py setup
```

You'll be prompted to paste your token, which is saved to `~/.hardcover.json` with `600` permissions.

Alternatively, set the `HARDCOVER_TOKEN` environment variable and skip `setup` entirely.

## Commands

### `whoami`

Show your Hardcover user ID, username, and book count.

```bash
python3 hardcover.py whoami
```

### `library`

List books in your library.

```bash
python3 hardcover.py library
python3 hardcover.py library --status reading
python3 hardcover.py library --status want -l 50 -o 25
```

| Flag | Description |
|---|---|
| `-s, --status` | Filter by `want`, `reading`, `read`, `paused`, `dnf`, `ignored` |
| `-l, --limit` | Max books to show (default: 25) |
| `-o, --offset` | Pagination offset |
| `--json` | Output raw JSON |

### `stats`

Show aggregate reading statistics and active goals.

```bash
python3 hardcover.py stats
```

### `progress`

Show current page progress for books marked "Currently Reading".

```bash
python3 hardcover.py progress
```

### `search`

Search Hardcover's book database.

```bash
python3 hardcover.py search "the unbearable lightness of being"
python3 hardcover.py search "kafka" -l 5
```

### `goals`

Show reading goals.

```bash
python3 hardcover.py goals
python3 hardcover.py goals --all   # include archived goals
```

### `log`

Log progress, update status, or rate a book. Matches by fuzzy title search (falls back to a disambiguation list if multiple books match) or a direct `user_book` ID.

```bash
python3 hardcover.py log "the stranger" --status reading
python3 hardcover.py log "the stranger" --pages 120
python3 hardcover.py log "the stranger" --percent 75
python3 hardcover.py log "the stranger" --rating 4.5
python3 hardcover.py log --id 123456 --status read --rating 5
```

| Flag | Description |
|---|---|
| `--id` | `user_book` ID (skips title search) |
| `--pages` | Cumulative pages read |
| `--percent` | Progress as a percentage (0–100); converted to pages using the book's page count |
| `--status` | `want`, `reading`, `read`, `paused`, `dnf`, `ignored` |
| `--rating` | 0–5, supports halves |

### `export`

Export your reading journal (progress events) to CSV, with a daily pages-read summary printed to the terminal.

```bash
python3 hardcover.py export
python3 hardcover.py export -o reading_log_2026.csv
```

### `daily`

Show a daily reading log with pages read per book and cumulative progress. By default shows the last 7 days.

```bash
python3 hardcover.py daily
python3 hardcover.py daily -d 14
python3 hardcover.py daily --json
```

| Flag | Description |
|---|---|
| `-d, --days` | Number of days to show (default: 7) |
| `--json` | Output raw JSON |

## JSON output

`whoami`, `library`, `stats`, `progress`, `search`, and `goals` all support `--json` for scripting:

```bash
python3 hardcover.py library --status read --json | jq '.[].book.title'
```

## Notes

- Config is stored at `~/.hardcover.json`.
- All commands respect `HARDCOVER_TOKEN` as a fallback if no config file exists.
