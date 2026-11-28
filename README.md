# SAG — Secure Access Gateway

A container gateway that lets AI coding agents (Claude Code, pi) query databases over MCP without ever receiving a credential.

Agents connect to SAG with a JWT bearer token over TLS. SAG holds all database credentials in Podman secrets or AWS Secrets Manager. No credential enters the agent context through SAG or through MCP.

---

## Table of contents

1. [Why SAG](#why-sag)
2. [Key features](#key-features)
3. [Requirements](#requirements)
4. [Quick start](#quick-start)
5. [Configuration](#configuration)
6. [Daily usage](#daily-usage)
7. [Connecting agents](#connecting-agents)
8. [Remote agents over Tailscale](#remote-agents-over-tailscale)
9. [Security model](#security-model)
10. [Troubleshooting](#troubleshooting)
11. [FAQ](#faq)
12. [Uninstall](#uninstall)
13. [Development and contributing](#development-and-contributing)
14. [License](#license)
15. [Further reading](#further-reading)

---

## Why SAG

An AI agent that queries a database needs a credential. A credential in an agent context can leak through prompts, logs, tool output, or prompt injection. SAG removes the credential from the agent entirely. The agent sends a read-only SQL query over MCP. SAG executes it, enforces read-only controls, writes an audit record, and returns data only.

---

## Key features

- **Credential isolation.** Credentials live in Podman secrets or AWS Secrets Manager. No credential reaches the agent.
- **Read-only enforcement.** SAG validates each query with a PostgreSQL AST parser, runs it in read-only engine mode, and rolls back the transaction. Write statements are rejected before execution.
- **Per-agent JWT identity.** Each agent harness has its own JWT. Each resource lists the agents that can use it. SAG denies by default.
- **Audit log.** SAG writes one record for each tool call and each authentication failure before it returns data.
- **Preflight checks.** SAG refuses a resource that has unapproved risky database extensions. Rechecks run every 10 minutes by default.
- **One-command setup.** `sag bootstrap` installs Podman, starts SAG, generates a local CA, and registers the agent harnesses.

---

## Requirements

| Item | Requirement |
|---|---|
| Operating system | macOS arm64, or Debian 13 (amd64 or arm64) |
| Podman | 5.5.1 on macOS arm64 (bootstrap installs it); 5.4.2 or later from `apt` on Debian 13 |
| `gh` CLI | 2.49 or later, logged in with `gh auth login` |
| Agent harness | Claude Code 2.1.332 or pi 1.6.1 |
| `~/.local/bin` | Must exist and be in `PATH` before you install `sag` (Quick start step 1) |
| Tailscale | Optional; needed for remote agents on a second machine |

Ubuntu and other Linux distributions are not supported. On any other OS, distribution, or architecture, `sag bootstrap` exits 1 with `unsupported platform <os>/<arch>` (for example `unsupported platform linux/amd64` on Ubuntu).

Every `sag` host command refuses to run as root (UID 0) and exits 1. Run `sag` as your normal user.

`sag` requires cgo, because the PostgreSQL parser embeds the C library libpg_query. Build it on a native machine for the target architecture. The release workflow builds three binaries on native runners: `sag_darwin_arm64`, `sag_linux_amd64`, and `sag_linux_arm64`.

---

## Quick start

### 1. Put `~/.local/bin` in `PATH`

```sh
mkdir -p ~/.local/bin
# Add to your shell profile if it is not already there:
export PATH="$HOME/.local/bin:$PATH"
```

### 2. Install `sag`

No release tag has been published yet. Build from source today:

```sh
git clone https://github.com/kanthorlabs/sag
cd sag
CGO_ENABLED=1 go build -trimpath -o ~/.local/bin/sag ./cmd/sag
```

When a release tag exists, install the signed binary instead:

```sh
# macOS arm64
gh release download <tag> --repo kanthorlabs/sag \
  --pattern sag_darwin_arm64 --pattern SHA256SUMS
shasum -a 256 --ignore-missing -c SHA256SUMS
gh attestation verify sag_darwin_arm64 --repo kanthorlabs/sag
install -m 0755 sag_darwin_arm64 ~/.local/bin/sag

# Debian amd64 or arm64: use sag_linux_amd64 or sag_linux_arm64,
# and use sha256sum --ignore-missing -c SHA256SUMS instead of shasum
```

`sag` has no self-update command. To upgrade, repeat these download, verify, and install steps with the new tag.

Do not install the binary when a check fails.

### 3. Run bootstrap

```sh
sag bootstrap
```

Bootstrap installs or verifies Podman, generates a local CA at `~/.config/sag/ca.pem`, starts the SAG container, and asks whether to set up each detected agent harness. Each step writes one `ok`, `warn`, `install`, `skip`, or `fail` line.

On macOS, bootstrap downloads the Podman 5.5.1 installer `https://github.com/containers/podman/releases/download/v5.5.1/podman-installer-macos-arm64.pkg`, verifies its SHA-256 against `deploy/pins`, and asks for the admin password one time to install it.

On Debian 13, bootstrap installs Podman with `sudo apt-get install -y podman`. Before any change, it reads the candidate version from `apt-cache policy podman`. When apt has no candidate, or the candidate is older than 5.4.2, bootstrap exits 1 and changes nothing. After the install, bootstrap checks that `newuidmap` is in `PATH`.

Bootstrap never upgrades or downgrades an installed Podman. An installed Podman older than the pin gives `fail podman installed <version>, pin <pin>` and exit 1 before any change. Upgrade Podman yourself, then run bootstrap again. A newer installed Podman gives one `warn` line and exit 0.

Bootstrap creates the master key: it reads 32 random bytes from `crypto/rand`, encodes them as base64, and pipes them to `podman secret create sag-master-key -`. The key never goes to a file, a command argument, or any output.

With `--autostart yes`, SAG starts at login:

- macOS: the LaunchAgent `~/Library/LaunchAgents/net.kanthorlabs.sag.plist` with `RunAtLoad` true. Logs go to `~/Library/Logs/sag/`.
- Debian: the Quadlet file `~/.config/containers/systemd/sag.container`. `sag service start` runs `systemctl --user daemon-reload`, then `systemctl --user restart sag`.

When bootstrap has no terminal and needs an answer, it exits 3 and names the flag that gives the answer.

To run without prompts, pass the Podman pin of your platform: `podman@5.5.1` on macOS arm64, `podman@5.4.2` on Debian 13. A wrong version is ignored with a `warn` line. On macOS:

```sh
sag bootstrap \
  --auto-approve podman@5.5.1 \
  --autostart yes \
  --agent-setup claude-code=yes \
  --deny-rules yes
```

### 4. Register a database

```sh
sag credential create mydb --kind postgres --agent claude-code
```

SAG asks for the PostgreSQL connection string on the terminal. The value is not echoed. For a database on this machine, use `host.containers.internal` as the host in the connection string. On Linux, that database must listen on a non-loopback address.

The container `sag` must run. Otherwise `create` prints `container sag stopped; run sag service start`, exits 1, and writes no secret. SAG checks the credential from inside the container before it saves anything. On success, `create` prints:

```
resource mydb created; SAG restarted; run sag resource test mydb
```

### 5. Test the connection

```sh
sag resource test mydb
```

---

## Configuration

The configuration file is `~/.config/sag/sag.yaml` (or `$XDG_CONFIG_HOME/sag/sag.yaml`). Bootstrap creates it. `sag credential create` adds resources to it. Avoid editing it by hand; use the credential commands.

| Path | Purpose | Mode |
|---|---|---|
| `~/.config/sag/sag.yaml` | Gateway configuration | 0644 |
| `~/.config/sag/ca.pem` | Endpoint CA certificate for agent clients | 0644 |
| `~/.config/sag/answers.json` | Stored bootstrap answers and Podman path | 0600 |

The container reads `sag.yaml` and all Podman secrets at start. A change to either requires `sag service start` to restart the container.

### AWS Secrets Manager

Add this block to `sag.yaml` to use AWS Secrets Manager as a credential backend:

```yaml
storage:
  aws:
    region: eu-central-1
    credentials: secret:sag-aws-credentials
```

Create `sag-aws-credentials` as a Podman secret. Its value is an AWS shared credentials file with one `[default]` profile.

Pass `--backend aws-sm` to `sag credential create` to store a credential in AWS Secrets Manager.

---

## Daily usage

### Service control

```sh
sag service start    # Start the SAG container
sag service stop     # Stop the SAG container
```

`start` waits up to 30 seconds for the health check. It prints the last 50 daemon log lines on timeout.

To get a newer dev image, run `podman pull docker.io/kanthorlabs/sag:main`, then `sag service start`.

To check that SAG listens only on loopback, run `ss -ltnp` on Debian or `lsof -nP -iTCP:8443 -sTCP:LISTEN` on macOS. Port 8443 must show `127.0.0.1` only, never `0.0.0.0` or a public address.

### Credentials

```sh
# Register a new PostgreSQL database
sag credential create mydb --kind postgres --agent claude-code [--agent pi]

# List all resources
sag credential list [--json]

# Show one resource
sag credential inspect mydb [--json]

# Remove a resource (requires typed confirmation on the terminal)
sag credential delete mydb
```

`credential list` and `credential inspect` never print a credential value.

`credential delete` reads the typed resource name from the terminal (`/dev/tty`) only. It has no `--yes`, `--stdin`, or `--json` flag; each of these flags gives exit 2. Without a terminal, or with a wrong typed name, it exits 3 and changes nothing.

### Logs

```sh
sag logs daemon [--follow] [--since 1h] [--tail 100]
sag logs audit [--date 2026-11-28]
```

### JWT management

```sh
# Issue a JWT for a named subject
sag jwt generate <sub>
```

`sag jwt generate` exits 3 unless both standard input and standard output are terminals. Do not run it from a script, and do not redirect its output to a file. It has no `--json` flag (exit 2). It refuses a `<sub>` that no resource lists in its agents.

### Version

```sh
sag version
```

---

## Connecting agents

### Claude Code

Bootstrap runs `sag agent setup claude-code` when Claude Code is detected. To run it separately:

```sh
sag agent setup claude-code [--deny-rules yes|no]
```

Agent setup writes the MCP entry with its JWT into `~/.claude.json`. The entry shape is:

```json
{"type": "http", "url": "https://localhost:8443/mcp", "headers": {"Authorization": "Bearer <JWT>"}}
```

It also sets `NODE_EXTRA_CA_CERTS` in `~/.claude/settings.json` to trust the SAG CA, and optionally adds three deny rules:

```
Bash(sag credential delete:*)
Bash(podman secret rm:*)
Bash(script:*)
```

The last output line is `ok agent claude-code; restart claude-code`. Start a new Claude Code session after setup. `claude mcp list` shows `sag` connected.

### pi

Bootstrap runs `sag agent setup pi` when pi is detected. To run it separately:

```sh
sag agent setup pi [--shell-profile yes|no] [--guard yes|no]
```

Agent setup writes the MCP entry with its JWT into `~/.pi/agent/mcp.json`, appends `export NODE_EXTRA_CA_CERTS=~/.config/sag/ca.pem` to the shell profile, and optionally installs `~/.pi/agent/extensions/sag-guard.ts` to block SAG secret commands.

The last output line is `ok agent pi; restart pi from a new shell`. Start pi from a new shell after setup. `pi mcp list --json` shows `sag` connected.

### MCP endpoint

The SAG MCP endpoint is `https://localhost:8443/mcp`. All connections require a JWT bearer token. The agent clients find the JWT in their MCP configuration file; never pass it as a command argument.

---

## Remote agents over Tailscale

SAG publishes `127.0.0.1:8443` only. When Tailscale is running on the SAG host, `tailscale serve` forwards TCP port 8443 of the Tailscale address to `127.0.0.1:8443`. Bootstrap detects Tailscale and applies this rule automatically.

To set up an agent on a second Tailscale machine:

1. Install `sag` on the second machine (without running bootstrap).
2. Create `~/.config/sag/` with mode 0700 on the second machine. Copy `~/.config/sag/ca.pem` from the SAG host to the second machine by hand.
3. On the SAG host, run `sag jwt generate <sub>` on a terminal. Copy the printed JWT.
4. On the second machine, run:
   ```sh
   sag agent setup claude-code \
     --url https://<tailscale-ip>:8443/mcp \
     --sub <sub>
   ```
   Type the JWT at the hidden prompt.
5. After each certificate renewal on the SAG host, copy `ca.pem` again.

On Debian, `tailscale serve` requires the current user as the Tailscale operator. Bootstrap reads the field `OperatorUser` of `tailscale debug prefs`. When the current user is not the operator and a terminal is present, bootstrap asks:

```
Run "sudo tailscale set --operator=<user>" so that SAG serves on Tailscale? [y/N]
```

Only `y` or `yes` approves. With approval, bootstrap runs `sudo tailscale set --operator=<user>`. Without a terminal or without approval, bootstrap does not exit 3: it prints `skip tailscale operator not set; loopback only` and continues in loopback-only mode.

---

## Security model

### What SAG protects

- **Credentials never leave the SAG boundary.** Values go from the terminal to Podman secrets (or AWS Secrets Manager). No `sag` command prints, logs, or passes a credential value. Podman mounts each secret into the container at `/run/secrets/<name>` with owner UID 65532 and mode 0400. The master key is the secret `sag-master-key`; the TLS certificate and key are `sag-tls-cert` and `sag-tls-key`.
- **SAG runs as a non-root user.** Inside the container, SAG runs as UID 65532. On the host, every `sag` command refuses UID 0 and exits 1.
- **Read-only enforcement is on SAG, not on the database credential.** SAG assumes the credential has write privileges and enforces read-only at the query level: AST validation, engine read-only mode (`BEGIN TRANSACTION READ ONLY`), and transaction rollback (never `COMMIT`). No `sag.yaml` key turns off any of these three controls.
- **Each agent has its own identity.** A JWT identifies the agent. Each resource lists its permitted agents. SAG denies access by default.
- **Every tool call has an audit record.** SAG writes one record before returning data.

### Threat assumptions

- The operator is the only person with shell access to the container host.
- The SAG port is reachable only from `127.0.0.1` and from tailnet devices that Tailscale ACLs allow.
- The agent has no access to the container, the secret mount, or the config file.

### Limits on a developer machine

An AI agent with a shell in the operator account (threat actor T8) can read Podman secrets directly with `podman`, bypass deny rules with `sh -c` or an absolute path, and read the JWT from the MCP configuration file. The deny rules and the pi guard stop only a direct command. See `docs/PRD-devx.md` section 2.3 for the full list.

---

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `fail port 8443 held by container <name>` | Another container publishes port 8443. | Stop that container. |
| `fail port 8443 held by <process>` | A non-Podman process listens on port 8443. | Stop that process. |
| `skip tailscale operator not set; loopback only` | On Debian, the Tailscale operator is not set. | Run `sudo tailscale set --operator=<user>`, then run `sag bootstrap` again. |
| `skip tailscale command absent; loopback only` | Tailscale is not installed. | Install Tailscale, or use loopback only. |
| `skip tailscale down; loopback only` | Tailscale is installed but not connected. | Run `tailscale up`. |
| `fail tailscale TCP 8443 serves another target` | Port 8443 of `tailscale serve` is used by another rule. | Remove that rule manually. |
| `resource <name> not found` | The resource name is unknown. | Run `sag credential list` to see the registered names. |
| `container sag stopped; run sag service start` | The SAG container is not running. No secret was written. | Run `sag service start`, then run the command again. |
| `unsupported platform <os>/<arch>` | The OS, distribution, or architecture is not supported. | Use macOS arm64 or Debian 13 (amd64 or arm64). |
| `fail podman installed <version>, pin <pin>` | The installed Podman is older than the pin. Bootstrap does not change an installed Podman. | Upgrade Podman yourself, then run `sag bootstrap` again. |
| `sag jwt generate` exits 3 | Standard input or standard output is not a terminal. | Run it in an interactive terminal without a redirect. |
| `warn endpoint certificate expires <date>; run sag bootstrap` | The endpoint CA is expiring. | Run `sag bootstrap` to renew. |
| Agent TLS error after certificate renewal | The agent client cached the old CA. | Restart the agent. The CA path `~/.config/sag/ca.pem` does not change. |
| Exit code 1 | General failure. | Read the `fail` line for the reason. |
| Exit code 2 | Usage error (unknown flag or wrong arguments). | Check `sag <command> --help`. |
| Exit code 3 | Refusal: a required human answer is absent (no terminal, wrong typed confirmation, or `sudo` refused). | Run the command on a terminal, or add the matching flag (for example `--auto-approve`, `--autostart`, `--deny-rules`). |

For daemon errors, run `sag logs daemon --tail 100`.

---

## FAQ

**Can I use SAG with a database on this machine?**
Use `host.containers.internal` as the host in the connection string. `localhost` and `127.0.0.1` resolve to the container, not the host. When the DSN host is a loopback address, `sag credential create` prints:

```
warn dsn host is a loopback address; in the container it is the container; for a database on this machine use host.containers.internal; on Linux the database must listen on a non-loopback address
```

**Can I register multiple databases?**
Run `sag credential create` once per database. Each resource has its own credential and its own list of agent identities.

**Can I use the same agent identity for multiple resources?**
Pass the same `--agent <sub>` to each `sag credential create` call. The JWT for that sub grants access to each resource that lists it.

**What is the JWT lifetime?**
The JWT is long-lived. Increment `auth.version` in `sag.yaml` and restart SAG to invalidate all current JWTs. This forces each agent to reconnect and report an authentication failure.

**Can I run SAG without Tailscale?**
Yes. SAG runs in loopback-only mode. Only agents on the same machine reach the endpoint at `https://localhost:8443/mcp`.

**A second run of `sag bootstrap` changes nothing. Is that expected?**
Yes. A second run on a ready host exits 0, writes only `ok` lines (and at most one Tailscale `skip` line), and does not restart SAG.

**Does SAG support MySQL or other databases?**
Not in v1. PostgreSQL is the first supported kind. MySQL (kind `mysql`) is planned for v1.1.

**Is Ubuntu supported?**
No. Supported Linux distributions: Debian 13 amd64 and arm64 only. On Ubuntu, `sag bootstrap` exits 1 with `unsupported platform linux/<arch>`.

**Is Windows or an Intel Mac supported?**
No. Supported platforms: macOS arm64 and Debian 13 (amd64 or arm64).

**How do I update `sag`?**
`sag` has no self-update command. Install the new release by hand with the steps of [Install `sag`](#2-install-sag). To update the dev container image, run `podman pull docker.io/kanthorlabs/sag:main`, then `sag service start`.

---

## Uninstall

SAG has no uninstall command. Run these steps in order. Step 1 comes first because `credential delete` also revokes the resource approval with `sag extension revoke` and removes the owned secrets. It needs `sag.yaml`, the state volume, and `sag-master-key`, which later steps remove.

1. Delete each registered resource:
   ```sh
   sag credential delete <name>   # once per resource in: sag credential list
   ```

2. Stop SAG and remove the Tailscale rule:
   ```sh
   sag service stop
   # If the SAG tailscale serve rule exists:
   tailscale serve --tcp 8443 off
   # On Debian, if bootstrap set the operator:
   sudo tailscale set --operator=
   ```

3. Remove the start-at-login entry:

   macOS:
   ```sh
   launchctl bootout gui/$(id -u)/net.kanthorlabs.sag
   rm ~/Library/LaunchAgents/net.kanthorlabs.sag.plist
   rm -rf ~/Library/Logs/sag/
   ```

   Debian:
   ```sh
   rm ~/.config/containers/systemd/sag.container
   systemctl --user daemon-reload
   ```

4. Remove the container, state volume, and secrets:
   ```sh
   podman rm sag
   podman volume rm sag-state
   podman secret rm sag-master-key sag-tls-cert sag-tls-key
   # If present:
   podman secret rm sag-aws-credentials
   ```

5. Remove the SAG image:
   ```sh
   podman rmi docker.io/kanthorlabs/sag
   ```

6. Remove the configuration directory:
   ```sh
   rm -rf ~/.config/sag
   ```

7. Remove the agent harness entries:

   Claude Code:
   ```sh
   claude mcp remove sag -s user
   # Also remove from ~/.claude/settings.json:
   #   env.NODE_EXTRA_CA_CERTS
   #   the three deny rules (sag credential delete, podman secret rm, script)
   rm -rf ~/.claude/skills/sag
   # Remove each file in ~/.claude/backups/ that holds mcpServers.sag:
   grep -l '"sag"' ~/.claude/backups/* 2>/dev/null
   ```

   pi:
   ```sh
   pi mcp remove sag
   rm -rf ~/.pi/agent/skills/sag ~/.pi/agent/extensions/sag-guard.ts
   # Remove the two lines that sag agent setup added to the shell profile.
   ```

Podman and the Podman machine stay installed.

---

## Development and contributing

### Build

```sh
CGO_ENABLED=1 go build -o bin/sag ./cmd/sag
```

Requires cgo. Build on a native machine for the target architecture: macOS arm64, Linux amd64, or Linux arm64.

### Test

```sh
go vet ./...
go test -count=1 ./...
```

Integration tests (need a PostgreSQL container):

```sh
make test-integration PG=17
```

### E2E tests

```sh
make e2e
```

Builds the container image `localhost/sag:e2e` and runs the full acceptance suite. Requires a running Podman machine on macOS. `sag bootstrap` uses only a published image, never a local build, so `make e2e` is the way to test local daemon code.

The devx acceptance criteria (`DX-AC*`) run on the development machine, not in GitHub Actions. See `docs/PRD-devx.md` section 17 for the full list and their environments.

### Project layout

| Path | Contents |
|---|---|
| `cmd/sag/` | Binary entry point |
| `internal/cli/` | Host and container commands |
| `internal/core/` | Adapter interface and registry |
| `internal/adapter/` | Resource adapters (PostgreSQL in v1) |
| `internal/secret/` | SecureStorage interface and backends |
| `internal/deploy/` | Container flags and Quadlet builder |
| `deploy/` | Containerfile, Quadlet golden file, pins |
| `docs/PRD.md` | Gateway product requirements |
| `docs/PRD-devx.md` | Developer experience requirements |
| `e2e/` | End-to-end test suite |

### Pins

`deploy/pins` lists the pinned dependency versions. Each line has the fields `name version platform source locator sha256`. Bootstrap verifies the SHA-256 of each downloaded artifact before it runs the installer. A pin rises only after the e2e suite passes on the new version.

### Container image

| Event | Image |
|---|---|
| Push to `main` | `docker.io/kanthorlabs/sag:main` (dev) |
| Tag `v*` | `docker.io/kanthorlabs/sag:<tag>` |

A dev build (`make build`) pulls `docker.io/kanthorlabs/sag:main` and prints `warn image dev build; pulls the main tag`. A release binary pins the image by digest.

---

## License

A LICENSE file has not been published yet.

---

## Further reading

- `docs/PRD.md` — gateway architecture, MCP tools, read-only controls, audit, approval workflow
- `docs/PRD-devx.md` — developer experience commands, security model, Tailscale setup, acceptance criteria
- `docs/plan/dev-setup.md` — step-by-step first-time setup
