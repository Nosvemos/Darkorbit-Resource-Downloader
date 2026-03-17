# DarkOrbit Resource Downloader

A Go-based CLI for mirroring DarkOrbit resource files from `darkorbit.com`, validating the local `darkorbit-files` mirror, and downloading missing or outdated assets.

## Features

- Fetches current manifests from live endpoints
- Parses `filecollection` XML manifests
- Detects missing files and hash mismatches
- Downloads files in parallel
- Auto-tunes request pressure when the server starts rate-limiting
- Keeps an execution log in `app.log` and local state in `.app-state.json`
- Supports category filtering for `spacemap`, `do_img`, `core`, `templates`, `unityApi`, `flashAPI`, and `resources`
- Defaults localized/template downloads to English only, with optional multi-language expansion
- Auto-discovers additional template XML files through `language_*.xml` manifests
- Uses live locale discovery for `--languages=all` when DarkOrbit exposes the current language list

## Project Structure

- `cmd/main.go`: CLI entrypoint
- `internal/app`: command flow, reporting, logging, and planning
- `internal/discovery`: seed discovery and manifest bootstrap
- `internal/manifest`: `filecollection` parsing logic
- `internal/integrity`: file hash validation
- `internal/downloader`: parallel HTTP downloading
- `internal/state`: local state persistence

## Requirements

- Go 1.26+

## Build

```bash
go build ./...
```

To build a standalone binary:

```bash
go build -o app.exe ./cmd
```

## Usage

General form:

```bash
go run ./cmd [command] [flags]
```

Commands:

- `sync`: refreshes manifests and downloads missing or outdated resources
- `plan`: shows what would be downloaded without downloading assets
- `fetch-manifests`: refreshes seed manifests and metadata endpoints
- `verify`: checks the local mirror against current manifests

## Examples

Plan the full sync:

```bash
go run ./cmd plan
```

Verify only the spacemap mirror:

```bash
go run ./cmd verify --category=spacemap
```

Sync only core bootstrap files:

```bash
go run ./cmd sync --category=core
```

Sync spacemap with custom parallelism:

```bash
go run ./cmd sync --category=spacemap --concurrency=8
```

Sync more gently when the server starts rate-limiting:

```bash
go run ./cmd sync --category=all --concurrency=4 --request-interval=750ms
```

Keep a high ceiling, but let the CLI auto-reduce concurrency if 503 responses spike:

```bash
go run ./cmd sync --category=all --concurrency=8 --min-concurrency=2 --auto-tune-concurrency=true
```

Sync English and Turkish localized/template assets:

```bash
go run ./cmd sync --languages=en,tr
```

Sync arbitrary language codes discovered by the game client, for example German and another custom locale:

```bash
go run ./cmd sync --languages=de,au
```

Sync every available language variant:

```bash
go run ./cmd sync --languages=all
```

Force a full refresh:

```bash
go run ./cmd sync --category=all --force
```

## Flags

- `--base-url`: default `https://www.darkorbit.com`
- `--output`: default `darkorbit-files`
- `--concurrency`: default `8`
- `--min-concurrency`: default `1`; lower bound for the runtime concurrency auto-tuner
- `--auto-tune-concurrency`: default `true`; automatically lowers concurrency on repeated `429` / `503` responses and slowly raises it again after stable success
- `--request-interval`: default `250ms`; globally spaces out request starts to reduce rate-limit pressure
- `--category`: default `all`
- `--languages`: default `en`; accepts arbitrary language codes such as `en,de,pt_BR` and affects localized/template paths such as `spacemap/templates/<lang>/...`, `spacemap/templates/language_<lang>.xml`, and `do_img/<lang>/...`
- `--force`: redownloads files even if they already exist locally
- `--log-file`: default `app.log`; pass an empty value to disable file logging

## Tests

Run all tests with:

```bash
go test ./...
```

The test suite is kept in the top-level `tests/` folder.

## Notes

- `darkorbit-files/` is ignored by Git and treated as a local mirror only.
- `verify` reports both missing files and files whose content hash no longer matches the manifest.
- By default, only English localized assets are fetched and verified. Use `--languages=en,tr,es` or `--languages=all` if you want additional language packs.
- Language bootstrap seeds are generated dynamically from the selected `--languages` flag instead of being hardcoded to a fixed locale list.
- When `--languages=all` is used, the CLI first tries to discover the current locale list from `darkorbit.com` and then falls back to locally discovered language seeds if live discovery is unavailable.
- The downloader already retries transient `429` / `503` / `5xx` responses, respects `Retry-After`, and can automatically reduce concurrency at runtime if throttle pressure rises.
- `language_*.xml` files are parsed as template manifests, which allows the CLI to discover additional files such as `resource_chat.xml`, `resource_auction.xml`, `resource_skillTree.xml`, and related template resources.
- The live `unityApi/events` endpoints currently resolve via `.php`, not `.xml`.
- A full `spacemap` sync can download a very large amount of data, especially under `spacemap/3d`.
