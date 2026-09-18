# justimport

> Automatically resolves 'Manual Import required' queue items in Radarr and Sonarr — because clicking Import on obvious single-file matches shouldn't be your job.

[![Test](https://github.com/erkexzcx/justimport/actions/workflows/test.yml/badge.svg)](https://github.com/erkexzcx/justimport/actions/workflows/test.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

---

## 🤖 Vibe Coded

This project was **vibe-coded** — built collaboratively with AI assistance.
Because even the code that automates your lazy clicks was written lazily. 😄

---

## The Problem

You've seen this in Radarr or Sonarr:

> *"Found matching movie via grab history, but release was matched to movie by ID. Manual Import required."*

The file is right there. The match is obvious. But the UI forces you to click **Import** manually for every single one. `justimport` clicks it for you.

---

## How It Works

1. Polls the Radarr/Sonarr queue on a configurable interval (default 60 s)
2. Finds queue items whose status messages contain *"manual import required"* or *"matched to movie by id"*
3. For each stuck item, fetches the available files via `GET /api/v3/manualimport?downloadId=X`
4. Filters out **sample files** (any file whose path contains "sample", case-insensitive)
5. Skips the item if **0 or 2+ files** remain after filtering
6. Skips the item if the single remaining file has any **rejections**
7. Skips the item if the file is **not matched** to a movie or series
8. Otherwise, **auto-imports** the file via `POST /api/v3/manualimport` with `importApproved: true`

**Default mode is DRY RUN** — no changes are made until you set `DRY_RUN=false`.

---

## Safety

- **DRY_RUN defaults to `true`** — you must explicitly opt in to real imports.
- **Only acts on single-file matches** — packs with multiple files are always skipped.
- **Rejection-aware** — files flagged by Radarr/Sonarr are never force-imported.
- **Log deduplication** — each queue item is logged exactly once per run, preventing log spam.

---

## Configuration

All configuration is via environment variables. No config files.

| Variable | Default | Description |
|---|---|---|
| `RADARR_URL` / `RADARR_URL_<N>` | *(unset)* | Base URL of your Radarr instance(s), e.g. `http://radarr:7878` |
| `RADARR_API_KEY` / `RADARR_API_KEY_<N>` | *(unset)* | Radarr API key(s) |
| `SONARR_URL` / `SONARR_URL_<N>` | *(unset)* | Base URL of your Sonarr instance(s), e.g. `http://sonarr:8989` |
| `SONARR_API_KEY` / `SONARR_API_KEY_<N>` | *(unset)* | Sonarr API key(s) |
| `POLL_INTERVAL` | `60s` | How often to check queues (Go duration: `30s`, `1m`, `5m`, …) |
| `DRY_RUN` | `true` | Set to `false` to enable real imports |

> **Note:** You can add as many instances as you want by simply appending an incrementing number to the environment variables (e.g. `RADARR_URL_1`, `RADARR_URL_2`, etc.). You do **not** need both Radarr and Sonarr. Configure only the services you use — at least one valid URL must be set.

---

## Docker Compose

```yaml
services:
  justimport:
    image: ghcr.io/erkexzcx/justimport:latest
    container_name: justimport
    restart: unless-stopped
    environment:
      # Configure Radarr, Sonarr, or both — at least one is required.
      - RADARR_URL=http://radarr:7878
      - RADARR_API_KEY=your-radarr-api-key
      
      # Need multiple instances? Just add a number!
      # - RADARR_URL_1=http://radarr4k:7878
      # - RADARR_API_KEY_1=your-radarr-4k-api-key
      
      # - SONARR_URL=http://sonarr:8989          # uncomment if you use Sonarr
      # - SONARR_API_KEY=your-sonarr-api-key      # uncomment if you use Sonarr
      
      # - SONARR_URL_1=http://sonarr-anime:8989
      # - SONARR_API_KEY_1=your-sonarr-anime-api-key
      
      - POLL_INTERVAL=60s
      - DRY_RUN=true   # change to false when ready
```

---

## Example Output

**Dry run (default):**
```
2026-03-08 12:00:00 INF Starting justimport v1.0.0 (vibe-coded with ❤️)
2026-03-08 12:00:00 INF Mode: DRY RUN (set DRY_RUN=false to enable imports)
2026-03-08 12:00:00 INF Poll interval: 60s
2026-03-08 12:00:00 INF Radarr: http://radarr:7878 ✓ (connected, Radarr v5.3.0)
2026-03-08 12:00:00 INF Sonarr: http://sonarr:8989 ✓ (connected, Sonarr v4.0.0)
2026-03-08 12:00:01 WRN [radarr] "Some.Movie.Pack.2024" → SKIPPED: 4 files found after filtering (expected exactly 1)
2026-03-08 12:00:01 WRN [radarr] "Another.Movie.2024" → SKIPPED: 0 files found after filtering
2026-03-08 12:00:01 INF [radarr] "Galactic.Sunrise.2025.1080p.BluRay.x264" → WOULD IMPORT (1 file, matched to "Galactic Sunrise")
```

**Live mode (`DRY_RUN=false`):**
```
2026-03-08 12:00:01 INF [radarr] "Galactic.Sunrise.2025.1080p.BluRay.x264" → IMPORTED (1 file, matched to "Galactic Sunrise")
```

---

## License

[MIT](LICENSE)
