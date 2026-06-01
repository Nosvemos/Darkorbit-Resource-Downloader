# DarkOrbit Resource Downloader

A premium, high-performance Go-based command-line interface (CLI) for mirroring, validating, and syncing resource files from `darkorbit.com` (and compatible clone game servers). It supports manifest parsing, parallel asset downloading, adaptive rate-limiting mitigation, dynamic concurrency auto-tuning, and robust local state persistence.

---

## Key Features

- **Manifest Parsing:** Seamlessly parses `filecollection` XML manifests to discover and map server-side game assets.
- **Parallel Downloading:** Leverages Go's high-concurrency capabilities to download multiple files in parallel.
- **Dynamic Rate-Limit Mitigation:** Features a built-in global request pacer that paces sequential HTTP triggers to prevent server throttle responses.
- **Adaptive Concurrency Auto-Tuner:** Automatically decreases download concurrency when encountering HTTP `429` (Too Many Requests) or `503` (Service Unavailable) errors, and slowly increases it again during stable successful streaks.
- **Flexible Category Filters:** Supports targeted synchronization for specific game components (e.g. `spacemap`, `do_img`, `core`, `templates`, `unityApi`, `flashAPI`, `resources`).
- **Comprehensive Localization Syncing:** Fetches assets for specific languages (`--languages=en,de,tr`) or automatically discovers and downloads every available game localization seed (`--languages=all`).
- **GitHub Actions Release Pipeline:** Ready-to-go CI/CD workflow to compile version-linked binaries for Windows and release them directly to GitHub.

---

## Project Structure

```
├── .github/workflows/
│   └── release.yml        # CI/CD automated build and release pipeline
├── cmd/
│   └── main.go            # Command-line entry point and signal handler
├── internal/
│   ├── app/               # Application run loop, reporting, console output, and planning
│   ├── discovery/         # Bootstrap seed mapping and language discovery
│   ├── downloader/        # Adaptive pacer and dynamic parallel HTTP downloading
│   ├── integrity/         # MD5/SHA manifest-based hash validation
│   ├── manifest/          # XML filecollection parser
│   ├── model/             # Shared data definitions and model structs
│   └── state/             # Local state tracking (.app-state.json)
└── tests/                 # Full suite of automated unit and integration tests
```

---

## Commands Reference

The CLI is structured around clear execution commands:

| Command | Action | Recommended Use Cases |
| :--- | :--- | :--- |
| `sync` | Fetches the latest manifests/metadata seeds and downloads all missing or outdated game resources. | Primary command used to keep the local mirror fully synchronized with the live server. |
| `plan` | Performs the entire sync sequence *except* downloading actual assets. Prints a complete summary of planned downloads. | Used to inspect size, category counts, and sync plans without writing any large game files. |
| `fetch-manifests` | Downloads only the manifest and XML template files, bypassing heavy graphics assets. | Ideal for scanning structural changes or updating local manifests rapidly. |
| `verify` | Compares all local files against the currently stored local manifests, reporting missing files and hash mismatches. | Offline integrity checking to detect corrupted assets. |
| `version` | Displays the binary version, runtime Go compiler version, target OS, and architecture. | Verifying compile-time information and environment context. |

---

## Command-Line Flags (Arguments)

Fine-tune Downloader behavior using standard command-line flags.

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--base-url` | `string` | `https://www.darkorbit.com` | The base URL of the targeted game server. Allows mirroring private clone servers (e.g. `https://pinkgalaxy.net`). |
| `--output` | `string` | `darkorbit-files` | Output directory where the downloaded asset structure is written. |
| `--category` | `string` | `all` | Comma-separated list of asset categories to download or verify. Supported: `spacemap`, `do_img`, `core`, `templates`, `unityApi`, `flashAPI`, `resources`, or `all`. |
| `--languages` | `string` | `en` | Comma-separated localization codes to sync (e.g., `en,de,tr`) or `all` to auto-discover all available languages. |
| `--concurrency` | `int` | `8` | The maximum number of concurrent download threads/workers. |
| `--min-concurrency` | `int` | `1` | Lower limit the dynamic auto-concurrency tuner is allowed to scale down to under heavy rate-limiting pressure. |
| `--auto-tune-concurrency` | `bool` | `true` | Enables/disables the dynamic concurrency auto-tuner. |
| `--request-interval` | `duration` | `250ms` | Minimum enforced delay between sequential request launches (e.g. `100ms`, `500ms`, `1s`) to smooth out request bursts. |
| `--force` | `bool` | `false` | If enabled, forces redownloading of every planned asset even if its size, path, and hash match local states. |
| `--log-file` | `string` | `app.log` | Local path where execution history is logged. Pass an empty string (`""`) to disable logging to disk. |

---

## Usage Examples

### 1. Perform a Full Sync with Dynamic Tuning
Sync all available files, utilizing 8 workers which will automatically scale down if the server rate-limits:
```bash
go run ./cmd sync --category=all --concurrency=8 --auto-tune-concurrency=true
```

### 2. Gentle Sync for Clone Sites
Space out request starts to 500ms to be extremely gentle on a private server's security firewall:
```bash
go run ./cmd sync --base-url=https://pinkgalaxy.net --request-interval=500ms --concurrency=4
```

### 3. Sync Specific Languages
Synchronize only English, German, and Turkish localized assets:
```bash
go run ./cmd sync --languages=en,de,tr
```

### 4. Create a Dry-Run Sync Plan
See exactly which assets would be downloaded due to hash mismatch or missing files:
```bash
go run ./cmd plan --category=spacemap
```

### 5. Validate Local Mirror Offline
Run an offline verification scan to locate any corrupted files:
```bash
go run ./cmd verify --output=darkorbit-files
```

---

## Build and Release Guide

### Requirements
- **Go:** Version `1.22` or later.

### Local Compilation
To compile a native standalone executable for your current operating system:
```bash
go build -o darkorbit-downloader.exe ./cmd
```

To compile a production build with a custom version string injected into the binary:
```bash
go build -ldflags "-X darkorbit-resource-downloader/internal/app.Version=v1.2.3" -o darkorbit-downloader.exe ./cmd
```

---

## GitHub Actions Release (CI/CD)

The project includes an advanced, pre-configured GitHub Actions workflow located in `.github/workflows/release.yml` that handles versioning and building automatically entirely on GitHub.

### How to Release
You can trigger a release using two flexible methods:

1. **Tag-Based Release (Automatic):**
   Simply tag a commit and push it to your repository:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```
   The workflow will trigger, compile the Windows `.exe` with the version string `v1.0.0` injected, create a new GitHub Release page, and upload the binary asset.

2. **UI-Based Release (Manual):**
   - Navigate to the **Actions** tab on your GitHub Repository page.
   - Select the **Release** workflow from the left sidebar.
   - Click the **Run workflow** dropdown on the right.
   - Input your desired version string (e.g. `1.1.0` or `v1.1.0`) and optional release notes.
   - Click **Run workflow**. The pipeline will automatically create the Git tag on GitHub, build the binary, and publish the release.

---

## Technical Architecture & Design

### Dynamic Concurrency & Rate-Limiting
When syncing tens of thousands of resources, web servers often respond with HTTP `429` or `503` rate limits. The Downloader addresses this with two layers of defense:
1. **Adaptive Pacer:** Enforces a thread-safe delay interval (`--request-interval`) between individual HTTP requests. If a request is throttled, the pacer dynamically applies a temporary cooldown backing off further requests.
2. **Dynamic Concurrency Tuner:** Modulates the concurrent worker count. When multiple HTTP errors occur, workers are dynamically paused, reducing concurrent pressure. As soon as a successful request streak is established, concurrency is restored.

### File Verification
The mirror uses local manifest metadata files to build exact file integrity mappings. If a file exists locally, the downloader verifies its MD5/SHA manifest hash against the actual file bytes. If the hash matches, the download is skipped, saving bandwidth. Changes to server-side file versions are automatically detected via hash updates or URL query version changes (`?__cv=...`).

---

## Running Tests

Verify the entire integrity and downloader subsystems by running the test suite:
```bash
go test ./... -v
```
