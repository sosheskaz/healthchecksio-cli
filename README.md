# Healthchecksio-CLI

A simple CLI for healthchecks.io

Usage:

```
healthchecksio-cli <check_id> [<signal>]
```

`signal` is optional, and may be any signal supported by healthchecks.io.

The binary has no dependencies outside of the standard library, so it is quite small. Container is
distroless, to minimize footprint.
