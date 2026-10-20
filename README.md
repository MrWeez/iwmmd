# IWMMD

Self-hosted domain expiration monitor for a small team. IWMMD checks domains with RDAP first and WHOIS as a fallback, then delivers expiry and system alerts through email, a generic webhook, Discord, or Telegram.

The stack is a Go modular monolith, SQLite with WAL, and a React + Vite single-page app. One process, one data directory, no Redis or external database.

## Features

- Dashboard with domain health, upcoming expiries, and an expiry horizon.
- Manual and scheduled checks with RDAP discovery and WHOIS fallback.
- Alert offsets in calendar days or months, before or after expiry.
- Per-rule send time, scope, and channel selection.
- Global, per-domain, and per-channel mute windows.
- Durable SQLite job queue and notification outbox with retries.
- Email (SMTP), generic webhook, Discord, and Telegram delivery.
- OIDC authorization-code login with PKCE and server-side sessions.
- Role-based access: viewer, member, and admin.
- Audit log, incident history, check history, and delivery attempts.
- Dark and light themes, built-in TLS, or reverse-proxy deployment.

## Quick start with the published image

Generate a master key:

```sh
docker run --rm ghcr.io/mrweez/iwmmd:latest gen-key
```

Copy the environment example, fill in the key and OIDC settings, then start Compose:

```sh
cp .env.example .env
$EDITOR .env
docker compose up -d
```

Open <http://localhost:8080>. OIDC is the only authentication method, so the instance is unusable until the provider settings are complete.

The Compose file binds the host port to `127.0.0.1` by default. Set `IWMMD_BIND_ADDR=0.0.0.0` only when a reverse proxy or firewall protects the port.

Health endpoints:

- `GET /healthz` is liveness.
- `GET /readyz` checks SQLite and the schema.

## Standalone static binary

Each release includes statically linked Linux binaries for `amd64` and `arm64`, as raw executables and tar archives. Download the matching archive and `checksums.txt` from the release page:

```sh
VERSION=1.1.0
ARCH=amd64
curl -LO "https://github.com/MrWeez/iwmmd/releases/download/v${VERSION}/iwmmd_${VERSION}_linux_${ARCH}.tar.gz"
curl -LO "https://github.com/MrWeez/iwmmd/releases/download/v${VERSION}/checksums.txt"
sha256sum -c --ignore-missing checksums.txt
tar -xzf "iwmmd_${VERSION}_linux_${ARCH}.tar.gz"
sudo install -m 0755 "iwmmd_${VERSION}_linux_${ARCH}/iwmmd" /usr/local/bin/iwmmd
```

The binary is built with `CGO_ENABLED=0` and Go's `netgo` resolver. It has no runtime C library dependency. Use the same configuration and data directory as the container deployment.

## Build locally

The local build uses the same root `Dockerfile` as the published image:

```sh
docker compose -f compose.build.yaml up -d --build
```

That tags the result as `iwmmd:local`. To build and run the image directly:

```sh
make docker-build
docker run --rm -e IWMMD_SECRET_KEY="$(make -s key)" -p 8080:8080 iwmmd:local
```

## Configuration

Configuration precedence is:

1. `IWMMD_` environment variables
2. a TOML file
3. built-in defaults

TOML keys are general names because the file is project-scoped. Environment variables use the project prefix. For example, `[oidc] issuer` becomes `IWMMD_OIDC_ISSUER`.

The server looks for a config file in this order:

1. `--config PATH`
2. `IWMMD_CONFIG`
3. `/etc/iwmmd/config.toml`
4. the user config directory
5. `./config.toml`

See [`config.example.toml`](config.example.toml) and [`.env.example`](.env.example).

Generate a key at any time:

```sh
go run ./cmd/iwmmd gen-key
```

### OIDC

OIDC is the only authentication method. The callback URL is:

```text
https://your-host/api/v1/auth/callback
```

Set the provider issuer, client ID, client secret, redirect URL, and at least one authorization source. For example:

```sh
IWMMD_OIDC_PROVIDER=keycloak
IWMMD_OIDC_ISSUER=https://id.example.com/realms/team
IWMMD_OIDC_CLIENT_ID=iwmmd
IWMMD_OIDC_CLIENT_SECRET=...
IWMMD_OIDC_REDIRECT_URL=https://iwmmd.example.com/api/v1/auth/callback
IWMMD_OIDC_ALLOWED_GROUPS=iwmmd-users
IWMMD_OIDC_ADMIN_GROUPS=iwmmd-admins
```

Provider presets are available for Google, Microsoft Entra, Okta, Auth0, GitLab, Keycloak, Authentik, Authelia, Pocket ID, Zitadel, and generic OIDC. GitHub is excluded because it is OAuth 2.0, not OIDC.

Authorization is evaluated in this order: claims and groups, then a verified email domain, then an explicit email. Admin access requires an admin group or an explicit admin email. Existing users can have their role changed in the web UI. Environment and TOML changes require a restart.

### Built-in TLS

Set `IWMMD_TLS_ENABLED=true`, mount a certificate and key, and set `IWMMD_BASE_URL` to the matching `https://` URL. The alternative is a reverse proxy, which is the default for Compose. When TLS is enabled, the HTTP listener redirects to the base URL and sends HSTS.

If you enable TLS in Compose, remove or override the image healthcheck because it probes the HTTP port:

```yaml
healthcheck:
  test: ["CMD", "/iwmmd", "healthcheck", "--url", "https://127.0.0.1:8443/healthz", "--insecure"]
```

### TOML file with Compose

Copy the example, fill in the key and provider values, and uncomment the bind mount in `compose.yaml`:

```sh
cp config.example.toml config.toml
```

```yaml
volumes:
  - iwmmd-data:/data
  - ./config.toml:/etc/iwmmd/config.toml:ro
```

### Runtime settings

SMTP, notification defaults, retention, and check intervals are configured in the web UI and stored in SQLite. Channel secrets and the SMTP password are encrypted with the master key. Keep `IWMMD_SECRET_KEY` stable; changing it makes stored secrets unreadable.

## Development

Prerequisites: Go 1.27+, Node 24+, and pnpm 10+.

```sh
make web-install
cp .env.example .env
$EDITOR .env
make dev-api
```

In a second terminal:

```sh
make dev-web
```

Vite serves the app at <http://localhost:5173> and proxies API requests to `http://127.0.0.1:8080`. Override the proxy target with `IWMMD_DEV_API`.

Common commands:

```sh
make test          # Go tests
make lint          # go vet + oxlint
make build         # embedded SPA + bin/iwmmd
make web-build     # rebuild internal/webui/dist
make key           # generate a master key
```

## CLI

```text
iwmmd serve       [--config PATH]
iwmmd migrate     [--config PATH]
iwmmd backup      [--config PATH] [--out PATH]
iwmmd healthcheck [--url URL] [--insecure]
iwmmd gen-key
iwmmd version
```

`backup` uses SQLite `VACUUM INTO` and does not take the instance lock. `serve` and `migrate` take the single-instance lock, so only one of them can use a data directory at a time.

## Project layout

```text
cmd/iwmmd/            CLI entry point
internal/alert/       alert scheduling and notification generation
internal/app/         lifecycle, TLS reload, HTTP servers
internal/auth/        OIDC, sessions, roles, provider presets
internal/checker/     RDAP, WHOIS, and check orchestration
internal/config/      TOML + environment loader and validation
internal/domain/      domain lifecycle and incident handling
internal/httpapi/     API, security middleware, SPA fallback
internal/notify/      outbox dispatcher and channel senders
internal/settings/    runtime settings
internal/store/       SQLite pools, migrations, instance lock
internal/webui/       embedded SPA
migrations/sqlite/    versioned schema
web/                  React, Vite, Tailwind, TanStack Router/Query
```

The full design is in [`ARCHITECTURE.md`](ARCHITECTURE.md).

## License

IWMMD is licensed under the [GNU Affero General Public License v3.0](LICENSE), SPDX identifier `AGPL-3.0-only`.
