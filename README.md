# Healthchecksio-CLI

A simple CLI for healthchecks.io

Usage:

```bash
healthchecksio-cli <check_id> [<signal>]
```

The check ID can also be supplied with `HEALTHCHECKSIO_CHECK_ID`:

```bash
HEALTHCHECKSIO_CHECK_ID=<check_id> healthchecksio-cli [<signal>]
```

or run a command and report its exit status:

```bash
healthchecksio-cli exec --check <check_id> -- <command> [args...]
```

or:

```bash
HEALTHCHECKSIO_CHECK_ID=<check_id> healthchecksio-cli exec -- <command> [args...]
```

or

```bash
docker run --rm ghcr.io/sosheskaz/healthchecksio-cli <check_id> [<signal>]
```

`signal` is optional. Supported values are `start`, `success`, `failure`, `fail`, `true`, `false`,
`log`, or a numeric exit status.

## Environment configuration

Explicit flags and positional arguments override environment variables. Environment variables
override compiled defaults. An empty environment variable is treated as unset.

| Environment variable | Equivalent input | Default |
| --- | --- | --- |
| `HEALTHCHECKSIO_CHECK_ID` | `<check_id>` or `exec --check` | None |
| `HEALTHCHECKSIO_ATTEMPTS` | `--attempts` | `5` |
| `HEALTHCHECKSIO_RETRY_MAX_BACKOFF` | `--retry-max-backoff` | `30s` |
| `HEALTHCHECKSIO_CONNECTION_TIMEOUT` | `--connection-timeout` | `5s` |
| `HEALTHCHECKSIO_TOTAL_PING_TIMEOUT` | `--total-ping-timeout` | `5m` |

Duration values use Go duration syntax, such as `500ms`, `30s`, or `5m`.

## Retry and timeout options

The retry and timeout flags apply to direct pings and to both the start and completion pings sent by
`exec`:

| Flag | Default | Description |
| --- | --- | --- |
| `--attempts` | `5` | Total HTTP attempts per ping. Set to `0` to retry indefinitely within the total ping timeout. |
| `--retry-max-backoff` | `30s` | Maximum delay between attempts. |
| `--connection-timeout` | `5s` | Timeout for DNS/TCP connection setup and the TLS handshake. |
| `--total-ping-timeout` | `5m` | Hard deadline for one ping operation, including attempts and backoff waits. |

For `exec`, the start and completion pings each receive a separate total ping timeout. The wrapped
command is not constrained by `--total-ping-timeout`.

For example, retry a ping until its two-minute deadline:

```bash
healthchecksio-cli --attempts 0 --total-ping-timeout 2m <check_id>
```

The container is distroless to minimize runtime footprint.
