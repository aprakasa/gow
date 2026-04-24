<div align="center">

# gow

WordPress on OpenLiteSpeed, simplified.

[![CI](https://github.com/aprakasa/gow/actions/workflows/ci.yml/badge.svg)](https://github.com/aprakasa/gow/actions/workflows/ci.yml)
[![Release](https://github.com/aprakasa/gow/actions/workflows/release.yml/badge.svg)](https://github.com/aprakasa/gow/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/aprakasa/gow)](https://goreportcard.com/report/github.com/aprakasa/gow)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A single-command CLI tool for managing WordPress (and plain PHP/HTML) sites on OpenLiteSpeed web server. Handles the entire server stack lifecycle — from installation to site creation, SSL, backups, and monitoring.

</div>

---

## Features

- **Single Binary** — Zero runtime dependencies, one static binary
- **Full Stack in One Command** — OpenLiteSpeed, LSPHP (8.1–8.5), MariaDB, Redis, WP-CLI, Composer, Certbot
- **Site Lifecycle** — Create, update, delete, clone, backup, and restore WordPress and HTML sites
- **Auto-Renewing SSL** — Let's Encrypt with HTTP-01 and DNS-01 (wildcard), deploy hook reloads OLS on renewal
- **Performance Tuning** — Six presets (lite → heavy) with auto-sized PHP workers based on server RAM and CPU
- **Per-Site Isolation** — Dedicated system users for multi-tenant security
- **Built-in Caching** — LSCache page cache + Redis object cache wired automatically
- **Crash-Safe Operations** — File-level state locking, automatic rollback on failure, idempotent retry with `--force`
- **Live Monitoring** — Disk, database, Redis, and OLS request metrics
- **Scheduled Backups** — Daily or weekly with configurable retention
- **WP-CLI Passthrough** — Run `wp` commands scoped to any site

## Requirements

| | Minimum | Recommended |
|---|---|---|
| **OS** | Ubuntu 22.04 / Debian 12 | Ubuntu 24.04 / Debian 12 |
| **Architecture** | amd64 (x86_64) | amd64 |
| **RAM** | 1 GB | 2 GB+ |
| **Disk** | 10 GB | 20 GB+ |
| **Access** | root | root |

> `gow` must run as root. It manages system packages, writes to `/usr/local/lsws`, `/var/www`, `/etc`, and creates system users.

**Stack installed by `gow stack install`:**

| Component | Purpose |
|---|---|
| OpenLiteSpeed | Web server |
| LSPHP 8.1–8.5 | PHP processor |
| MariaDB | Database |
| Redis | Object cache |
| WP-CLI | WordPress management |
| Composer | PHP dependency management |
| Certbot | SSL certificates (Let's Encrypt) |

## Install

```bash
wget -qO gow https://raw.githubusercontent.com/aprakasa/gow/main/install.sh && sudo bash gow
```

Or download directly from [releases](https://github.com/aprakasa/gow/releases/latest):

```bash
curl -SL https://github.com/aprakasa/gow/releases/latest/download/gow -o /usr/local/bin/gow
chmod +x /usr/local/bin/gow
```

Uninstall:

```bash
sudo bash install.sh --purge
```

## Usage

Provision the server and create a WordPress site:

```bash
sudo gow stack install
sudo gow site create example.com --type wp --tune standard --php 83
```

Plain HTML or PHP site?

```bash
sudo gow site create static.example.com --type html
```

Need SSL?

```bash
sudo gow site ssl example.com --email admin@example.com
```

Running a WooCommerce store?

```bash
sudo gow site create shop.example.com --type wp --tune woocommerce
```

WordPress multisite?

```bash
sudo gow site create network.example.com --type wp --multisite subdirectory
```

Update PHP version or tuning?

```bash
sudo gow site update example.com --php 84
sudo gow site update example.com --tune business
```

Want daily backups?

```bash
sudo gow site backup-schedule example.com              # daily, keep 7
sudo gow site backup-schedule example.com --schedule weekly --retain 14
```

Clone a site for staging?

```bash
sudo gow site clone example.com staging.example.com
```

Check logs or server health?

```bash
gow site log example.com                    # error log (default)
gow site log example.com --access           # access log
gow status
gow metrics
```

> Full command reference: [`docs/gow.md`](docs/gow.md)

## Tuning Presets

| Preset | PHP Memory | Workers | Use Case |
|--------|-----------|---------|----------|
| `lite` | 128 MB | 64 MB | Small static sites |
| `standard` | 256 MB | 128 MB | Typical blog (alias: `blog`) |
| `business` | 384 MB | 192 MB | High-traffic site |
| `woocommerce` | 512 MB | 256 MB | WooCommerce store |
| `heavy` | 768 MB | 384 MB | Large multisite |
| `custom` | user-defined | user-defined | Full control (`--memory` and `--workers`) |

Workers are auto-sized from server RAM and CPU. Override defaults with `/etc/gow/policy.yaml`.

## Development

```bash
make build       # build binary
make test        # run tests (race detector on)
make vet         # go vet
make lint        # golangci-lint
make coverage    # test coverage report
make cross-build # linux/amd64 cross-compile
```

CI runs on every push and PR: build, vet, test, golangci-lint, govulncheck, and smoke tests.

Releases are cut by pushing a `v*` tag — GoReleaser builds the binary and publishes a GitHub release.

## Acknowledgements

`gow` drew inspiration from [EasyEngine](https://github.com/EasyEngine/easyengine) and [WordOps](https://github.com/WordOps/WordOps).

## License

[MIT](LICENSE)
