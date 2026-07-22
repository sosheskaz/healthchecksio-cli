# Healthchecksio-CLI

A simple CLI for healthchecks.io

Usage:

```bash
healthchecksio-cli <check_id> [<signal>]
```

or run a command and report its exit status:

```bash
healthchecksio-cli exec --check <check_id> -- <command> [args...]
```

or

```bash
docker run --rm ghcr.io/sosheskaz/healthchecksio-cli <check_id> [<signal>]
```

`signal` is optional. Supported values are `start`, `success`, `failure`, `fail`, `true`, `false`,
`log`, or a numeric exit status.

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
