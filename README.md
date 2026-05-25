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

The container is distroless to minimize runtime footprint.
