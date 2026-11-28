# Development setup

Run these steps on macOS arm64. On Ubuntu, use `sag_linux_<arch>` and `sha256sum` instead of `shasum -a 256`.

Prerequisites:

- `gh` 2.49 or later, logged in with `gh auth login`.
- `~/.local/bin` exists and is in `PATH`. Create it with `mkdir -p ~/.local/bin`.

## 1. Install `sag`

```
gh release download <tag> --repo kanthorlabs/sag --pattern sag_darwin_arm64 --pattern SHA256SUMS
shasum -a 256 --ignore-missing -c SHA256SUMS
gh attestation verify sag_darwin_arm64 --repo kanthorlabs/sag
install -m 0755 sag_darwin_arm64 ~/.local/bin/sag
sag bootstrap
```

If a check fails, do not install the binary.

`sag bootstrap` installs or checks Podman, starts SAG, creates the certificate authority `~/.config/sag/ca.pem`, and asks for the agent setup of each harness.

## 2. Register a database

```
sag credential create mydb --kind postgres --agent claude-code
```

`sag` asks for the connection string on the terminal and does not echo it. For a database on this machine, use the host `host.containers.internal` in the connection string.

## 3. Set up a harness

```
sag agent setup claude-code
```

Agent setup writes the MCP entry with its JWT into the harness configuration and adds the certificate authority of SAG. Start a new Claude Code session after the setup.

## Local daemon code

`sag bootstrap` runs only a published image. To run local daemon code, use the e2e harness: `make e2e` builds `localhost/sag:e2e`.
