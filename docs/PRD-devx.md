# SAG developer experience (devx): PRD addendum

| Field | Value |
|---|---|
| Owner | Ulrich |
| Status | Revision 6 |
| Date | 2026-11-26 |
| Base | `docs/PRD.md` revision 16, tag `sag-layer-6` |

## 1. Scope

devx v1 targets developer machines only:

| Platform | Package source for Podman | Test environment |
|---|---|---|
| macOS arm64 | Official Podman installer `podman-installer-macos-arm64.pkg` of the pinned GitHub release | The macOS development machine of Ulrich |
| Ubuntu with `apt-get` (amd64, arm64) | Ubuntu archive through `apt-get` | Ubuntu 24.04, GitHub Actions runner `ubuntu-24.04` |

- Each other OS, architecture, or distribution gets exit 1 from `sag bootstrap` with the message `unsupported platform <os>/<arch>`.
- An Ubuntu release whose `podman` package is older than the pin gets exit 1 from `sag bootstrap` (section 5.3).
- A server install uses PRD 13.5 (`deploy/sag.container`). devx does not change the server path, except the log flags in section 13.
- SAG publishes `127.0.0.1:8443` only. On a host with Tailscale, `tailscale serve` forwards TCP port 8443 of the Tailscale IPv4 address to it (section 5.8).
- A certificate from `tailscale cert` is out of scope.
- devx v1 supports two agent harnesses: Claude Code 2.1.332 and pi 1.6.1 (section 12).
- Out of scope: an uninstall command, a self-update, a master key reset, a change of the `agents` list of a resource, a credential replacement command.

### 1.1 Commands

| Command | Runs on | Function |
|---|---|---|
| `sag bootstrap` | Host | Install the pinned dependencies, prepare SAG state, set the start at login, start SAG, set up the agent harnesses. |
| `sag service start` | Host | Start SAG. On macOS, the start at login runs this command. |
| `sag service stop` | Host | Stop SAG. |
| `sag credential create` | Host | Validate and store one credential, and add its resource. |
| `sag credential list` | Host | Show all resources and their credential metadata. |
| `sag credential inspect` | Host | Show one resource and its credential metadata. |
| `sag credential delete` | Host | Remove one resource and its credentials after a typed confirmation. |
| `sag logs daemon` | Host | Show the daemon log of the container. |
| `sag logs audit` | Host | Show the MCP audit log of the container. |
| `sag agent setup <claude-code\|pi>` | Host | Register SAG as a remote MCP server in one agent harness (section 12). |
| `sag jwt generate`, `sag extension approve`, `sag extension revoke`, `sag resource test` | Host | Host forms of the PRD 13.1 commands (section 1.2). |
| `sag config check`, `sag version` | Host and container | PRD 13.1. |
| `sag serve` | Container only | PRD 13.1. Container entrypoint. |
| `sag credential check` | Container, internal | Validate one credential for `create` (section 7.4). |
| `sag audit read` | Container, internal | Print the audit lines of one day for `sag logs audit`. |
| `sag agent issue` | Container, internal | Issue one JWT for `sag agent setup` (section 12.4). |

- The binary selects the container behavior when `/run/.containerenv` exists. Otherwise it selects the host behavior.
- Each host command refuses UID 0 with exit 1.
- An internal command and `sag serve` run only in the container. On the host they give exit 2.

### 1.2 Host forms

| Host command | Runs |
|---|---|
| `sag jwt generate <sub> [--name <display>]` | `podman exec -it sag sag jwt generate <sub> [--name <display>]` |
| `sag extension approve <resource>` | `podman exec -it sag sag extension approve <resource>` |
| `sag extension revoke <resource>` | `podman exec sag sag extension revoke <resource>` |
| `sag resource test <name>` | `podman exec sag sag resource test <name>` |

- The host forms of `jwt generate` and `extension approve` exit 3 unless standard input and standard output are both terminals. A redirect of standard output to a file thus gives exit 3.
- These two host forms have no `--json`. They give their terminal to `podman exec -it`.

### 1.3 Exit codes and output

| Code | Meaning |
|---|---|
| 0 | Success. A `warn` line keeps code 0. |
| 1 | Failure. |
| 2 | Usage error. An unknown flag gives code 2. |
| 3 | Refusal: a required human answer is absent. Examples: no terminal and no flag for a prompt, no approval for an install, a wrong typed confirmation, a refusal by `sudo`. |

- A prompt goes to `/dev/tty`, never to standard output.
- For a prompt, "a terminal" means that the process can open `/dev/tty`. Section 1.2 has a stricter rule for two host forms.
- Human output of `bootstrap`, `service`, `credential`, and `agent setup` commands is one line per step: `<status> <item> <detail>`. The status is `ok`, `warn`, `install`, `skip`, or `fail`.
- `credential list` and `credential inspect` accept `--json`. No other devx command accepts `--json`.

## 2. Security model

### 2.1 Account

SAG runs under the current OS user. devx creates no service account, no `sudoers` file, and no system unit.

| Item | Owner and location |
|---|---|
| Podman store, Podman secrets, container `sag`, volume `sag-state` | Rootless Podman of the current user. On macOS, the default Podman machine of the current user. |
| Configuration directory `<cfg>` | `$XDG_CONFIG_HOME/sag` when the variable is set, otherwise `~/.config/sag`. Mode 0700. |
| `<cfg>/sag.yaml` | Mode 0644. |
| `<cfg>/ca.pem` | Endpoint CA certificate, mode 0644. |
| `<cfg>/answers.json` | Stored answers, the absolute path of `podman`, and the Tailscale address of the certificate, mode 0600. |
| `<cfg>/lock` | Lock file, mode 0600. |
| Start at login, macOS | LaunchAgent `~/Library/LaunchAgents/net.kanthorlabs.sag.plist` |
| Start at login, Ubuntu | Quadlet file `~/.config/containers/systemd/sag.container` of the user manager |
| Tailscale serve rule | Serve configuration of `tailscaled`, TCP port 8443 |
| Agent harness files | Section 12.2 |

- `sag` never reads, stores, or passes a password. Only the Podman install and the Tailscale operator step of section 5.8 call `sudo`, and `sudo` reads the password itself.
- With no terminal, bootstrap calls `sudo -n`. A refusal by `sudo` gives exit 3.

### 2.2 Actor T8

| ID | Actor | Capability |
|---|---|---|
| T8 | AI agent with a shell in the OS account of the operator, for example Claude Code or pi | Runs each command that the operator can run. Reads and writes each file of the operator. Opens a pseudo-terminal with `script` or `expect`. |

### 2.3 Guarantees and limits

- No devx command prints, logs, or writes a credential value. The value goes from `/dev/tty` or standard input to the backend, and to the container.
- Thus a credential value does not reach the agent context through a `sag` command or through MCP.
- No devx command prints or logs a JWT that `sag agent setup` issues. Agent setup writes the JWT only into the MCP configuration file of the harness.
- Claude Code copies its MCP configuration file to `<dir>/backups`. `claude mcp get sag` prints the JWT.
- An agent with a shell in the same account can read or delete each Podman secret with `podman` directly. The `sag credential delete` check stops only an agent that uses `sag`.
- `sag credential delete` reads the typed resource name from the controlling terminal. It stops an agent that runs `sag` through a tool without a terminal. It does not stop an agent that opens a pseudo-terminal or runs `podman secret rm`.
- The same limit applies to the AWS backend. T8 reads the Podman secret `sag-aws-credentials` and then reads each AWS secret that this key can read.
- T8 reads the JWT of each harness from its MCP configuration file. The file mode 0600 stops only other OS users.
- With the Tailscale serve rule, each tailnet device that the Tailscale ACLs allow reaches port 8443. The JWT check of PRD 11.2 applies to each request except `/healthz`. SAG v1 has no rate limit for a request without a JWT.

### 2.4 Agent command controls

Section 12.7 adds Claude Code deny rules and the pi extension `sag-guard.ts`. Both stop only a direct command. These limits apply:

- A deny rule matches a command prefix. An agent that runs the command through `sh -c` bypasses the rule.
- An agent that runs a command through an absolute path, for example `/usr/bin/script`, bypasses a deny rule.
- The rules and the guard cover no read of a secret, for example `podman secret inspect --showsecret`.
- The rules and the guard cover no other pseudo-terminal tool, for example `expect` or `python`.
- The guard matches command text. An agent that builds the command text at run time, for example from two parts or from base64, bypasses the guard.
- T8 can remove a control: it edits `permissions.deny` or deletes `sag-guard.ts`. The next session then has no control.

## 3. Pins

`deploy/pins` holds one line per item. Spaces separate the fields: `name version platform source locator sha256`.

| Field | Rule |
|---|---|
| `name` | Item name, for example `podman`. |
| `version` | Pin version. The pin is the lowest accepted version. |
| `platform` | `darwin/arm64`, `linux/amd64`, `linux/arm64`, or `linux/*`. One line per item and platform. |
| `source` | `release` or `apt`. |
| `locator` | `release`: the HTTPS URL of the official artifact. `apt`: the package name. |
| `sha256` | `release`: the SHA-256 of the artifact. `apt`: `-`. |

Items of devx v1:

| `name` | `version` | `platform` | `source` | `locator` | `sha256` |
|---|---|---|---|---|---|
| `podman` | `5.5.1` | `darwin/arm64` | `release` | `https://github.com/containers/podman/releases/download/v5.5.1/podman-installer-macos-arm64.pkg` | SHA-256 of that file, from the v5.5.1 release page |
| `podman` | `4.9.3` | `linux/*` | `apt` | `podman` | `-` |

- The binary embeds `deploy/pins` with `go:embed`.
- A `release` artifact downloads to a new directory with mode 0700. Bootstrap computes its SHA-256 and compares it with the pin.
- On a difference, bootstrap deletes the file, installs nothing, and exits 1. On a match, the install command runs on the verified file.
- Checksums cover only files that bootstrap downloads. `apt-get` verifies the archive signature of an `apt` item.
- A pin rises only after the e2e suite passes on the new version. On macOS, the manual check list of section 18.1 also passes.

## 4. Release and images

### 4.1 Image

- The image name is `docker.io/kanthorlabs/sag`.
- The image holds `linux/amd64` and `linux/arm64` in one manifest list.
- The repository keeps `main` as its default branch.
- A push to `main` publishes the dev image `docker.io/kanthorlabs/sag:main`.
- A tag `v*` publishes the release image `docker.io/kanthorlabs/sag:<tag>`.
- CI reads the registry credentials from the GitHub secrets `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN`.

### 4.2 Release workflow

The workflow `.github/workflows/release.yml` has the permissions `contents: write`, `id-token: write`, and `attestations: write`. On a tag `v*`, it does these steps in this order:

1. Build and push the release image. Record the digest of the manifest list.
2. Build `sag_darwin_arm64`, `sag_linux_amd64`, and `sag_linux_arm64` with cgo on native runners.
3. Set the version and the digest through `-ldflags`: `-X github.com/kanthorlabs/sag/internal/cli.Version=<tag> -X github.com/kanthorlabs/sag/internal/deploy.ImageDigest=<digest>`.
4. Write `SHA256SUMS` for the three binaries.
5. Attest each binary with `actions/attest-build-provenance`.
6. Publish the GitHub release with the three binaries and `SHA256SUMS`.

### 4.3 Image selection

| Build | `deploy.ImageDigest` | Image that `sag` pulls |
|---|---|---|
| Release binary | Set | `docker.io/kanthorlabs/sag@<digest>` |
| Dev build (`make build`) | Empty | `docker.io/kanthorlabs/sag:main`, with the line `warn image dev build; pulls the main tag` |

- `sag` pulls the image only when the image is absent from the local Podman store.
- `sag` writes the dev build `warn` line only when it pulls the image.
- A developer gets a newer dev image with `podman pull docker.io/kanthorlabs/sag:main`, then `sag service start`.
- `sag version` prints the version and the image reference.
- devx v1 has no self-update. A new release is a new manual install.

### 4.4 Install steps

Prerequisites:

- `gh` 2.49 or later, logged in with `gh auth login`.
- `~/.local/bin` exists and is in `PATH`. Create it with `mkdir -p ~/.local/bin`.

Run these steps on macOS arm64. On Ubuntu, use `sag_linux_<arch>` and `sha256sum` instead of `shasum -a 256`.

```
gh release download <tag> --repo kanthorlabs/sag --pattern sag_darwin_arm64 --pattern SHA256SUMS
shasum -a 256 --ignore-missing -c SHA256SUMS
gh attestation verify sag_darwin_arm64 --repo kanthorlabs/sag
install -m 0755 sag_darwin_arm64 ~/.local/bin/sag
sag bootstrap
```

- If a check fails, do not install the binary.

## 5. `sag bootstrap`

### 5.1 Synopsis

```
sag bootstrap [--auto-approve <name>@<version>]... [--autostart yes|no]
    [--agent-setup <harness>=yes|no]... [--deny-rules yes|no] [--shell-profile yes|no] [--guard yes|no]
```

### 5.2 Steps

Bootstrap runs these steps in this order. Each step first reads the current state. A step with nothing to do writes an `ok` line.

1. Refuse UID 0, a container, and an unsupported platform.
2. Select the pins for the OS and the architecture.
3. Plan: read the state and collect each needed answer of section 5.4. Read the Tailscale state of section 5.8. Check the holder of `127.0.0.1:8443` (section 6). If bootstrap needs an answer and no source gives it, exit 3. A plan conflict of section 5.3, section 5.8, or section 12.3, or a port conflict of section 6, gives exit 1. Both exits occur before any change.
4. Create `<cfg>` with mode 0700 when it is absent. Take the lock `<cfg>/lock`. If another process holds it, exit 1.
5. Read the state again and plan again. A new needed answer without a source gives exit 3.
6. For each dependency, apply the dependency rule (section 5.3). On macOS, when no Podman machine exists, run `podman machine init`. When the machine is stopped, run `podman machine start`.
7. Pull the image of section 4.3 when it is absent.
8. Create the secret `sag-master-key` when it is absent (section 5.6).
9. Create or renew the endpoint certificate (section 5.7).
10. Create `<cfg>/sag.yaml` when it is absent: `version`, `server`, `auth`, and an empty `resources` map.
11. Apply the start at login (section 5.5).
12. Run `sag service start` (section 6) in each of these cases: the container `sag` does not run, or step 8 to step 11 changed an item. Otherwise read `GET /healthz` with `<cfg>/ca.pem` and write `ok sag`.
13. Apply the Tailscale serve rule (section 5.8).
14. For each harness with the answer `yes`, run the steps of `sag agent setup` (section 12) with the answers of the plan.
15. Print the endpoint `https://localhost:8443/mcp` and the next command: `sag credential create <name> --kind postgres`, with one `--agent <sub>` for each harness that agent setup completed. Without a completed harness, the command holds `--agent <sub>`. For each detected harness without setup, print `sag agent setup <harness>`. When bootstrap uses Tailscale, print the steps of section 5.8 for a second tailnet machine.

- The bootstrap plan omits the conflict "The container `sag` does not run" of section 12.3, because step 12 starts SAG.
- Bootstrap checks the `NODE_EXTRA_CA_CERTS` rule of section 12.5 in step 14, after step 9.
- On a `NODE_EXTRA_CA_CERTS` conflict, agent setup of that harness changes no file and prints the bundle command. Bootstrap completes the other steps and exits 1.
- On a host without `<cfg>`, an exit in step 3 leaves no `<cfg>`.
- Step 14 runs in the bootstrap process under the lock of step 4.
- A second run on a ready host asks nothing, does not restart SAG, and exits 0. It writes only `ok` lines and at most one Tailscale `skip` line. Exception: bootstrap asks the Ubuntu Tailscale operator prompt at each run until the operator step completes.
- Bootstrap never replaces `sag-master-key`, `sag.yaml`, or an installed dependency.

### 5.3 Dependency rule

| State | Action | Line |
|---|---|---|
| Installed, version equal to the pin | None | `ok podman 5.5.1` |
| Installed, version newer than the pin | None | `warn podman installed 5.6.0, pin 5.5.1` |
| Installed, version older than the pin | None. Plan conflict: exit 1 before any change. | `fail podman installed 4.3.1, pin 4.9.3` |
| Absent, approved | Install, then read the version again | `install podman 5.5.1` |
| Absent, not approved | Exit 3 | `fail podman absent; run on a terminal or add --auto-approve podman@5.5.1` |

- Bootstrap never changes an installed version. A Podman from Homebrew or from another source counts as installed.
- The installed version is the client version that `podman --version` prints.
- Bootstrap searches `podman` in this order: `PATH`, `/opt/podman/bin/podman`, `/opt/homebrew/bin/podman`.
- Bootstrap stores the absolute path of the `podman` binary in `answers.json` as `podman_path`. Each devx command uses the stored path.
- When the stored path does not exist, a devx command searches again in the same order and stores the new path.

macOS install:

- The command is `sudo installer -pkg <file> -target /`. `sudo` asks for the admin password one time.
- After the install, the search finds `/opt/podman/bin/podman`.

Ubuntu install:

1. Read the candidate version from `apt-cache policy podman`.
2. Remove an epoch prefix `<n>:`. Remove the text from the first `+`, `-`, or `~` to the end. For example, `4.9.3+ds1-1ubuntu0.1` gives `4.9.3`.
3. When apt has no candidate, or the candidate is older than the pin, exit 1 before any change. This check is part of the plan.
4. Run `sudo apt-get install -y podman`.
5. Check that `newuidmap` is in `PATH`. Otherwise exit 1.

Approval:

- `--auto-approve <name>@<version>` is repeatable. It approves the install of `<name>` only when `<version>` is equal to the pin version.
- On Ubuntu, an approval means "the apt version, at least the pin".
- A pre-approval with another version approves nothing. Bootstrap writes `warn podman pre-approval 5.4.0 does not match pin 5.5.1; ignored`.
- Without a valid pre-approval and with a terminal, bootstrap asks the prompt of section 5.4. Only `y` or `yes` approves.

### 5.4 Answers

| Answer | Flag | Prompt | Needed when |
|---|---|---|---|
| Install of an absent item, macOS | `--auto-approve podman@5.5.1` | `Install podman 5.5.1 from <locator>? [y/N]` | The item is absent. |
| Install of an absent item, Ubuntu | `--auto-approve podman@4.9.3` | `Install podman from apt (candidate <apt version>, pin 4.9.3)? [y/N]` | The item is absent. |
| Start at login | `--autostart yes\|no` | `Start SAG at login? [y/N]` | `answers.json` has no `autostart` value. |
| Tailscale operator, Ubuntu | None | `Run "sudo tailscale set --operator=<user>" so that SAG serves on Tailscale? [y/N]` | Section 5.8. |
| Agent setup | `--agent-setup <harness>=yes\|no` | `Set up SAG for <harness> in <absolute config directory>? [y/N]` | Bootstrap detects the harness, and `answers.json` has no `agent_setup` value for the harness and its absolute config directory. |
| Answers of agent setup | `--deny-rules`, `--shell-profile`, `--guard` | Section 12.3 | The harness has the agent setup answer `yes`. |

- `<harness>` is `claude-code` or `pi`. Bootstrap detects a harness when its binary, `claude` or `pi`, is in `PATH`.
- The config directory of a harness is in section 12.2.
- A flag replaces the stored answer. Bootstrap writes each answer of `--autostart` and `--agent-setup` to `answers.json`. Agent setup stores its own answers (section 12.3).
- `answers.json` keys each `agent_setup` answer by the harness and the absolute path of its config directory.
- Bootstrap asks only when it needs the answer and no flag gives it.
- With no terminal and no flag for a needed answer, bootstrap exits 3. The message names the flag. The Tailscale operator answer is not a needed answer: section 5.8 gives its rule.
- Bootstrap stores no install approval and no Tailscale operator answer.

### 5.5 Start at login

| OS | `yes` | `no` |
|---|---|---|
| macOS | Write the LaunchAgent with `RunAtLoad` true and `ProgramArguments` `<absolute path of sag> service start`. Run `launchctl bootout gui/<uid>/net.kanthorlabs.sag`, then `launchctl bootstrap gui/<uid> <plist>`. | Run `launchctl bootout gui/<uid>/net.kanthorlabs.sag`. Remove the LaunchAgent. |
| Ubuntu | The Quadlet file has `[Install] WantedBy=default.target`. | The Quadlet file has no `[Install]` section. |

- `bootout` of an agent that is not loaded is not a failure.
- When the domain `gui/<uid>` does not exist, for example in an SSH session without a GUI login, bootstrap writes the LaunchAgent file. It writes `warn autostart loads at the next GUI login` and continues.
- The LaunchAgent writes its output to `~/Library/Logs/sag/service.log`.
- Bootstrap rewrites the LaunchAgent when the path of `sag` changes.
- On Ubuntu, bootstrap does not enable linger. The user manager starts SAG at the first login and stops it at the last logout.

### 5.6 Master key

- Bootstrap reads 32 bytes from `crypto/rand`, encodes them as base64, and writes them to the standard input of `podman secret create sag-master-key -`.
- The value never goes to a file, an argument, or an output.

### 5.7 Endpoint certificate

- Bootstrap creates a CA key and a CA certificate with `crypto/x509`, valid for 397 days.
- The CA certificate has no name constraints extension.
- Bootstrap signs one server certificate with the same validity. Its names are `localhost`, `127.0.0.1`, `::1`, and the Tailscale address of section 5.8 when bootstrap uses Tailscale.
- The CA key exists only in memory and never goes to a file.
- Bootstrap discards the CA key after it signs. Thus no party can sign a second certificate with this CA.
- A process that trusts `<cfg>/ca.pem` trusts each name of a certificate that this CA signs. Only the one server certificate exists.
- The server certificate becomes the secret `sag-tls-cert`. The server key becomes the secret `sag-tls-key`.
- Each value goes to the standard input of `podman secret create <name> -`. The server key never goes to a file, an argument, or an output.
- The CA certificate goes to `<cfg>/ca.pem`.

Bootstrap creates the three items in this order:

1. Write the CA certificate to `<cfg>/ca.pem.new` with mode 0644.
2. Remove `sag-tls-cert` and `sag-tls-key` when they exist.
3. Create `sag-tls-cert` and `sag-tls-key`.
4. Rename `<cfg>/ca.pem.new` to `<cfg>/ca.pem`.
5. Write the Tailscale address of the new server certificate to `answers.json` as `tailscale_ip`. When the certificate has no Tailscale address, remove `tailscale_ip`.

Bootstrap runs this sequence in each of these cases:

- One of `sag-tls-cert`, `sag-tls-key`, and `<cfg>/ca.pem` is absent.
- `<cfg>/ca.pem.new` exists.
- Less than 30 days remain on `<cfg>/ca.pem`.
- Bootstrap uses Tailscale, and `answers.json` has no `tailscale_ip` or another `tailscale_ip`.

- Thus a second run repairs a stopped run.
- After a new certificate, step 12 restarts SAG. Each agent client then needs a restart. The path `<cfg>/ca.pem` does not change, so no agent configuration changes.
- A Tailscale skip at bootstrap does not trigger the sequence. The certificate keeps its Tailscale address.
- During a skip with the reason `down`, a renewal keeps the stored `tailscale_ip` as a name of the new certificate.

### 5.8 Tailscale

Tailscale is optional, and bootstrap detects it. Bootstrap has no Tailscale flag. Bootstrap uses Tailscale when each of these conditions holds:

1. The `tailscale` command is in `PATH`.
2. `tailscale ip -4` exits 0, and its first output line parses as an IPv4 address in `100.64.0.0/10`. This address is the Tailscale address.
3. On Ubuntu, the field `OperatorUser` of `tailscale debug prefs` is the current user, or the operator step below gets approval.

When a condition fails, bootstrap writes one line `skip tailscale <reason>; loopback only` and continues. `<reason>` is `command absent`, `down`, `address invalid`, or `operator not set`.

Endpoint:

- SAG publishes `127.0.0.1:8443:8443` only. SAG never binds the Tailscale address, `0.0.0.0`, or another address.
- `tailscale serve --tcp` listens on the Tailscale address and forwards the raw TCP stream to `127.0.0.1:8443`. The TLS session runs from the client to SAG.
- SAG sees each tailnet client as `127.0.0.1` (R27).
- Bootstrap adds the Tailscale address to the server certificate (section 5.7).

Serve rule:

- In step 3, when bootstrap uses Tailscale, bootstrap reads `tailscale serve status --json`. When this command fails, bootstrap writes `skip tailscale down; loopback only`.
- The SAG rule is the entry `TCP."8443"` equal to `{"TCPForward": "127.0.0.1:8443"}`, and no key of `AllowFunnel` that ends in `:8443` is true.
- When `TCP."8443"` exists and is not the SAG rule, bootstrap writes `fail tailscale TCP 8443 serves another target` and exits 1 before any change.
- When a key of `AllowFunnel` ends in `:8443` and is true, bootstrap writes `fail tailscale funnel is on for port 8443` and exits 1 before any change.
- In step 13, when `TCP."8443"` is absent, bootstrap runs `tailscale serve --bg --tcp 8443 tcp://127.0.0.1:8443`. When the SAG rule exists, bootstrap writes `ok tailscale-serve`.
- `tailscaled` keeps the rule across restarts. `sag service start` does not run `tailscale`.
- Bootstrap never runs `tailscale funnel`.

Ubuntu operator step:

- On Ubuntu, `tailscale serve` needs root or the current user as the operator of `tailscaled`.
- When the `OperatorUser` check fails and bootstrap has a terminal, bootstrap asks the prompt of section 5.4. Only `y` or `yes` approves.
- With approval, step 13 runs `sudo tailscale set --operator=<user>` before the serve rule.
- Without a terminal or without approval, bootstrap writes `skip tailscale operator not set; loopback only`.

Agent on a second tailnet machine. Bootstrap prints these steps with the Tailscale address:

1. On the second machine, install `sag` with the steps of section 4.4, without `sag bootstrap`.
2. On the second machine, create `<cfg>` with mode 0700. Copy `<cfg>/ca.pem` of the SAG host to `<cfg>/ca.pem` of the second machine by hand.
3. On the SAG host, list the `sub` of the agent with `sag credential create <name> --kind <kind> --agent <sub>`.
4. On the SAG host, run `sag jwt generate <sub>` on a terminal.
5. On the second machine, run `sag agent setup <harness> --url https://<tailscale IP>:8443/mcp --sub <sub>` and type the JWT at the hidden prompt (section 12.4).
6. After each certificate renewal, copy `<cfg>/ca.pem` again.

## 6. `sag service start` and `sag service stop`

```
sag service start
sag service stop
```

| OS | `start` | `stop` |
|---|---|---|
| macOS | Start the Podman machine when it is stopped. Run the container with `podman run -d --replace --name sag` and the arguments of section 13. | `podman stop sag` |
| Ubuntu | Write the Quadlet file from `sag.yaml` and `answers.json`. Run `systemctl --user daemon-reload`, then `systemctl --user restart sag`. | `systemctl --user stop sag` |

- `start` replaces a container that runs. In-flight requests fail during the restart.
- `start` waits up to 30 s for `GET /healthz`. A timeout gives exit 1 and the last 50 lines of the daemon log.
- `start` and `stop` take the lock `<cfg>/lock`. A command that holds the lock runs the start steps in its own process and does not take the lock again.
- On macOS, the run arguments include `--restart=on-failure`.
- `start` publishes `127.0.0.1:8443:8443` only (section 5.8).

Port check:

- Bootstrap in step 3 and `start` before it starts a container check the holder of `127.0.0.1:8443`.
- When `podman ps` shows a container that publishes port 8443, this container is the holder. This check comes first.
- Else the holder is the process that listens on TCP port 8443. On macOS, `lsof -nP -iTCP:8443 -sTCP:LISTEN` names it. On Ubuntu, `ss -ltnp` names it.
- On macOS, `lsof` shows `gvproxy` for each Podman publish.
- When no process holds the port, the command continues.
- When the holder is the container `sag`, bootstrap writes `ok port-8443`, and `start` continues.
- When the holder is another container, the command writes `fail port 8443 held by container <name>` and exits 1 before any change.
- When the holder is a process outside Podman, the command writes `fail port 8443 held by <process>` and exits 1 before any change.

## 7. `sag credential create`

### 7.1 Synopsis

```
sag credential create <name> --kind <kind> --agent <sub> [--agent <sub>]... [--description <text>]
    [--tls verify-full|insecure-skip-verify|disable] [--tls-ca <pem-file>]
    [--backend podman|aws-sm] [--stdin] [--accept-failed-check]
```

### 7.2 Kind registry

- The kind registry is the adapter registry of `internal/core`.
- A kind is creatable when its adapter implements `core.CredentialKind`:

```go
type CredentialKind interface {
	CredentialPrompt() string
	ValidateCredential(cred secret.Credential) (CredentialNotes, error)
	CheckCredential(ctx context.Context, cfg ResourceConfig, cred secret.Credential) error
}

type CredentialNotes struct {
	LoopbackHost bool
}
```

- `ValidateCredential` checks the form without a network call. `CheckCredential` connects to the resource.
- Each error has fixed text only. The text never holds a part of the value.
- v1 kind `postgres`:
  - `ValidateCredential` is the `parseDSN` function of the adapter. Thus `create` accepts exactly the values that the daemon accepts.
  - `ValidateCredential` sets `LoopbackHost` for the host `localhost`, an address in `127.0.0.0/8`, or `::1`.
  - `CheckCredential` opens one connection with the `tls` and `tls_ca` of the resource, then runs the `Engine` and `Version` checks of PRD 9.1.
- A later kind, for example `cloudwatch_logs`, adds its own prompt, form check, and connection check.

### 7.3 Secret value input

- Without `--stdin`, `create` opens `/dev/tty`, writes `<CredentialPrompt> for <name> (input hidden): `, and reads one line with echo off. No terminal gives exit 3.
- With `--stdin`, `create` reads the value from standard input. These limits apply:
  - When standard input is a terminal, exit 3.
  - At most 64 KiB. A longer input gives exit 1.
  - A NUL byte or an inner line break gives exit 1.
  - `create` removes one final `\n` or `\r\n`.
- `create` reads a piped value only with `--stdin`.
- Use a password manager CLI in a pipe. A value typed in a command line goes to the shell history.
- The value never goes to an argument, an environment variable, or a file.

### 7.4 Validation

`create` checks these rules in this order. The first failure gives exit 1 and a fixed message. No secret exists after a failure.

1. `<name>` matches `^[a-z][a-z0-9-]{0,62}$`.
2. `sag.yaml` has no resource `<name>`. Otherwise: `resource <name> exists; delete it first`.
3. No secret with a target name of section 7.5 exists in the selected backend. Otherwise: `secret <secret name> exists; run sag credential delete <name>`.
4. `--kind` is a creatable kind. Each `--agent` matches `^[a-z][a-z0-9-]{0,62}$`, and at least one `--agent` is present.
5. `--description` has at most 500 characters. `--tls-ca` holds at least one PEM certificate.
6. `--backend aws-sm` needs `storage.aws` in `sag.yaml`.
7. The container `sag` runs. Otherwise: `container sag stopped; run sag service start`.
8. Read the value (section 7.3).
9. `ValidateCredential` passes. A failure gives exit 1 also with `--accept-failed-check`.
10. When `LoopbackHost` is set, write `warn dsn host is a loopback address; in the container it is the container; for a database on this machine use host.containers.internal; on Linux the database must listen on a non-loopback address`.
11. `CheckCredential` passes.

Connection check (rule 11):

- `create` runs `podman exec -i sag sag credential check --kind <kind> --tls <mode>`. It writes `{"value": <value>, "tls_ca": <PEM text or null>}` to the standard input of `podman exec`.
- The check runs in the container, so it uses the network view of the daemon.
- `sag credential check` exits 0 on success. On failure, it prints one fixed error and exits 1.
- On a failed check, `create` prints the error and exits 1.
- With `--accept-failed-check`, `create` prints the error as a `warn` line and continues.

### 7.5 Effects

| Backend | Connection secret | CA secret |
|---|---|---|
| `podman` (default) | `sag-res-<name>.conn` | `sag-res-<name>.ca` |
| `aws-sm` | `sag/<name>/conn` | `sag/<name>/ca` |

Each secret gets the labels `sag.resource=<name>`, `sag.kind=<kind>`, and `sag.role=connection` or `sag.role=tls_ca`. On the AWS backend, these three labels are the only tags.

`create` holds the lock `<cfg>/lock` and does these steps in this order:

1. `Put` the connection secret.
2. If `--tls-ca` is present, `Put` the CA secret.
3. Add `resources.<name>` to `sag.yaml` with `kind`, `description`, `connection`, `tls`, `tls_ca`, and `agents`. Keep all other content. Validate the new file with the `config check` rules. Write a temporary file and rename it.
4. Run `sag service start`.

- On a failure in steps 2 to 4, `create` restores the previous `sag.yaml` and deletes only the secrets that this call created.
- After the rollback, `create` runs `sag service start` and exits 1.
- The output is `resource <name> created; SAG restarted; run sag resource test <name>`.
- A create restarts SAG. In-flight requests fail during the restart.

## 8. `sag credential list` and `sag credential inspect`

```
sag credential list [--json]
sag credential inspect <name> [--json]
```

| Field | Source |
|---|---|
| `name`, `kind`, `description`, `tls`, `agents` | `sag.yaml` |
| `secrets[].name`, `secrets[].backend`, `secrets[].role` | `sag.yaml` |
| `secrets[].exists` | `Metadata` of the backend. `null` when the backend is unreachable. |
| `secrets[].owned` | `true` when the secret has the label `sag.resource=<name>`. |
| `secrets[].created_at` | `Metadata` of the backend, or `null` |
| `secrets[].mounted` | Podman backend only: the secret list of `podman container inspect sag` |
| `container_running` (inspect only) | `podman container inspect sag` |

- List and inspect never print a secret value, a part of a value, the value length, or a hash of the value.
- List and inspect never call `Load`.
- No source file outside tests contains the string `--showsecret`.
- When less than 30 days remain on `<cfg>/ca.pem`, list writes `warn endpoint certificate expires <date>; run sag bootstrap` to standard error.
- An unreachable backend gives a `warn` line on standard error and exit 0.
- `--json` of list: `{"credentials": [<object>]}`. `--json` of inspect: one object. The object has exactly the fields of the table.
- Inspect of an unknown name gives exit 1 with `resource <name> not found`.
- Build step 1 confirms that `podman container inspect` of Podman 4.9.3 lists the secrets of the container. When it does not, `secrets[].mounted` is not part of v1, and DX-AC20 reads the Quadlet file. In that case DX-AC2 counts the `Secret=` lines of the Quadlet file.

## 9. `sag credential delete`

### 9.1 Synopsis

```
sag credential delete <name>
```

- `delete` has no `--yes`, no `--stdin`, and no `--json`. Each of these flags gives exit 2.
- `delete` opens `/dev/tty`. When `delete` cannot open `/dev/tty`, it exits 3 and changes nothing.

### 9.2 Ownership

- A resource owns each secret that its `sag.yaml` entry references and that has the label `sag.resource=<name>`.
- A resource also owns each secret with the label `sag.resource=<name>` that no entry references. A failed create leaves such a secret.
- devx manages only the secrets of `sag credential create`. A resource does not own a referenced secret without the label.

### 9.3 Flow

1. Take the lock `<cfg>/lock`.
2. Collect the `sag.yaml` entry and the owned secrets in each configured backend. When a configured backend is unreachable, exit 1 with `backend <backend> unreachable; no change`. If neither an entry nor an owned secret exists, exit 1 with `resource <name> not found`.
3. Print the resource name, the kind, the agents, and the owned secret names on `/dev/tty`.
4. Ask the operator to type the resource name. Input that is not exactly equal gives exit 3 and no change.
5. Revoke the approval of the resource:
   - Container runs: `podman exec sag sag extension revoke <name>`.
   - Container stopped: a one-shot container from the same image, with the same state volume and `sag-master-key`, runs `sag extension revoke <name>`.
6. Remove `resources.<name>` from `sag.yaml` with a temporary file and a rename.
7. When the container ran at step 5, run `sag service start`. Otherwise keep SAG stopped and, on Ubuntu, write the Quadlet file.
8. `Delete` each owned secret.
9. For each referenced secret that the resource does not own, print `skip <secret name> not created by sag; not removed`.

- `delete` is idempotent. A second `delete` after a failure completes the steps that remain.
- `delete` of a resource without an approval record succeeds.

## 10. `sag logs`

```
sag logs daemon [--follow] [--since <duration>] [--tail <n>]
sag logs audit [--date <YYYY-MM-DD>]
```

| Command | Runs |
|---|---|
| `sag logs daemon` | `podman logs sag`, with each given flag |
| `sag logs audit` | `podman exec sag sag audit read --date <date>` |

- `sag audit read` prints the lines of `/var/lib/sag/audit/audit-<date>.jsonl` without change. The default date is the current UTC date.
- Both commands only read. They need no confirmation and no terminal.

## 11. SecureStorage

### 11.1 Interface

Package `internal/secret` defines one interface. Each backend implements it.

```go
type Metadata struct {
	Name      string
	Labels    map[string]string
	CreatedAt time.Time
}

var (
	ErrExists      = errors.New("secret exists")
	ErrNotFound    = errors.New("secret not found")
	ErrUnavailable = errors.New("secret backend unavailable")
)

type SecureStorage interface {
	Put(ctx context.Context, name string, value Credential, labels map[string]string) error
	Load(ctx context.Context, name string) (Credential, error)
	Delete(ctx context.Context, name string) error
	Metadata(ctx context.Context, name string) (Metadata, error)
	List(ctx context.Context, selector map[string]string) ([]Metadata, error)
}
```

| Operation | Contract |
|---|---|
| `Put` | Create the secret with the value and the labels. Return `ErrExists` when the name exists. Never replace a value. |
| `Load` | Return the value as a `Credential` with the reference text as its name. Return `ErrNotFound` or `ErrUnavailable` on failure. |
| `Delete` | Remove the secret. Return nil when the name does not exist. |
| `Metadata` | Return the metadata without a read of the value. Return `ErrNotFound` when the name does not exist. |
| `List` | Return the metadata of each secret whose labels contain each key and value of `selector`. Never read a value. |

- The daemon calls only `Load`. The host commands call `Put`, `Delete`, `Metadata`, and `List`, and never `Load`.
- An error never holds a value or a part of a value.
- Each AWS call has a timeout of 10 s.

### 11.2 Backends

| Backend | Reference in `sag.yaml` | Status |
|---|---|---|
| Podman secrets | `secret:<name>` | Default. devx v1. |
| AWS Secrets Manager | `aws-sm:<name>` | devx v1. |
| GCP Secret Manager | Not defined | v2. |
| Azure Key Vault | Not defined | v2. |

The reference prefix selects the backend of each secret. A resource can mix backends for `connection` and `tls_ca`.

Podman backend:

| Operation | Method |
|---|---|
| `Put` | `podman secret create --label <k>=<v> <name> -`, with the value on standard input |
| `Load` | Read `/run/secrets/<name>` in the container with the PRD 6 owner and mode rules. The current `secret.Load` function. |
| `Delete` | `podman secret rm <name>` |
| `Metadata` | `podman secret inspect <name>`, without `--showsecret` |
| `List` | `podman secret ls`, then `Metadata` of each name, then a label filter in SAG |

AWS Secrets Manager backend:

| Operation | Method |
|---|---|
| `Put` | `CreateSecret` with `SecretString` and the labels as tags |
| `Load` | `GetSecretValue`, field `SecretString` |
| `Delete` | `DeleteSecret` with `ForceDeleteWithoutRecovery` true |
| `Metadata` | `DescribeSecret` |
| `List` | `ListSecrets` with a `tag-key` filter, then a label filter in SAG |

- An `aws-sm:` name matches `^[A-Za-z0-9/_+=.@-]{1,512}$`.
- `server.tls_cert`, `server.tls_key`, `storage.aws.credentials`, and `sag-master-key` accept only `secret:` references.
- `sag config check` in the container validates the file owner and mode only for `secret:` references. It skips each `aws-sm:` reference.

### 11.3 AWS configuration

```yaml
storage:
  aws:
    region: eu-central-1
    credentials: secret:sag-aws-credentials
```

- A reference that starts with `aws-sm:` needs `storage.aws.region`.
- The same reference also needs `storage.aws.credentials`. It references a Podman secret.
- The value of that secret is an AWS shared credentials file with one `[default]` profile. The operator creates it with `podman secret create`.
- The daemon builds its AWS configuration with the standard credential chain of the AWS SDK for Go v2: `config.LoadDefaultConfig` with `config.WithRegion` and `config.WithSharedCredentialsFiles(["/run/secrets/<name>"])`.
- The host commands use `config.LoadDefaultConfig` with `config.WithRegion` and the default chain of the operator.
- `sag.yaml` never holds an AWS key.
- The key of the daemon needs only `secretsmanager:GetSecretValue` on the names that `sag.yaml` references.

### 11.4 Availability

- SAG never depends on AWS to start or to serve.
- The daemon reads each Podman secret at start, as PRD 6 states.
- An `aws-sm:` read never delays the listener. The daemon adds each resource with an `aws-sm:` reference as unopened and starts the listener.
- For each such resource, a background open starts at once. The open reads the `aws-sm:` secret with `Load`.
- If SAG cannot reach AWS, each resource whose credential lives only in AWS is unavailable. Every other resource is served.
- The open retry of PRD 9.1 (every 60 s) calls `Load` again. It brings the resource back when AWS is reachable again.
- A request to such a resource returns `resource_unavailable`.
- The scrubber adds each credential at the time that `Load` returns it.
- At start, the daemon gives the value of the `storage.aws.credentials` file to the scrubber.

## 12. `sag agent setup`

### 12.1 Synopsis

```
sag agent setup claude-code [--sub <sub>] [--url <url>] [--deny-rules yes|no]
sag agent setup pi [--sub <sub>] [--url <url>] [--shell-profile yes|no] [--guard yes|no]
```

- The default `sub` is `claude-code` for Claude Code and `pi` for pi.
- The default URL is `https://localhost:8443/mcp`. This is the local mode.
- `--url <url>` selects the remote mode for an agent on a second tailnet machine (section 5.8). The URL has the scheme `https`, a host, the port 8443, and the path `/mcp`. Another form gives exit 2.
- In the remote mode, agent setup does not use Podman or the container. The JWT comes from a hidden prompt (section 12.4).
- `--sub <sub>` replaces the default. A value that does not match `^[a-z][a-z0-9-]{0,62}$` gives exit 2.
- An unknown harness name gives exit 2.
- Bootstrap runs the same steps in its step 14, under its own lock.

Steps:

1. Refuse UID 0 and a container.
2. Plan: read the state, collect each needed answer of section 12.3, and check each plan conflict of section 12.3. A needed answer without a source gives exit 3. A conflict gives exit 1. Both exits occur before any change.
3. In a standalone run, take the lock `<cfg>/lock`. Read the state again and plan again.
4. Register SAG in the MCP configuration file (section 12.4).
5. Install the skill (section 12.6).
6. Apply the CA trust (section 12.5).
7. Apply the command control (section 12.7).
8. Verify the connection (section 12.8).

- A second run on a ready harness writes only `ok` lines, asks nothing, and exits 0.

### 12.2 Paths

| Item | Claude Code | pi |
|---|---|---|
| Config directory `<dir>` | `$CLAUDE_CONFIG_DIR` when the variable is set, otherwise `~/.claude` | `$PI_CODING_AGENT_DIR` when the variable is set, otherwise `~/.pi/agent` |
| MCP configuration file | `$CLAUDE_CONFIG_DIR/.claude.json` when the variable is set, otherwise `~/.claude.json`. Key `mcpServers.sag`. | `<dir>/mcp.json`. Key `mcpServers.sag`. |
| Skill | `<dir>/skills/sag/SKILL.md` | `<dir>/skills/sag/SKILL.md` |
| CA trust | `env.NODE_EXTRA_CA_CERTS` in `<dir>/settings.json` | One `export` line in the shell profile |
| Command control | `permissions.deny` in `<dir>/settings.json` | `<dir>/extensions/sag-guard.ts` |
| Verify command | `claude mcp list` | `pi mcp list --json` |

- The shell profile of pi depends on the base name of `$SHELL`:

| Shell | Profile |
|---|---|
| `zsh` | `${ZDOTDIR:-$HOME}/.zshrc` |
| `bash`, Ubuntu | `~/.bashrc` |
| `bash`, macOS | `~/.bash_profile` |

### 12.3 Answers and plan conflicts

| Answer | Flag | Prompt | Needed when |
|---|---|---|---|
| Claude Code deny rules | `--deny-rules yes\|no` | `Add 3 deny rules to <absolute path of settings.json>? [y/N]` | A rule of section 12.7 is absent, and `answers.json` has no `deny_rules` value for that `settings.json`. |
| pi shell profile | `--shell-profile yes\|no` | `Add "export NODE_EXTRA_CA_CERTS=<cfg>/ca.pem" to <absolute path of the profile>? [y/N]` | The line is absent, and `answers.json` has no `shell_profile` value for that profile. |
| pi guard | `--guard yes\|no` | `Install <absolute path of sag-guard.ts> to block SAG secret commands in bash? [y/N]` | The guard file is absent, and `answers.json` has no `guard` value for that guard file. |

- A flag replaces the stored answer. Agent setup writes each answer to `answers.json`.
- `answers.json` keys each answer by the absolute path of the file that the answer approves.
- Agent setup asks only when it needs the answer and no flag gives it.
- With no terminal and no flag for a needed answer, agent setup exits 3. The message names the flag.
- Without a `yes` answer, agent setup does not change the file of that answer.

Plan conflicts. Each conflict gives one `fail` line and exit 1 before any change:

- The harness binary, `claude` or `pi`, is not in `PATH`.
- In the local mode, the container `sag` does not run. The bootstrap plan omits this conflict (section 5.2).
- In the remote mode, `<cfg>/ca.pem` is absent.
- The MCP configuration file or `<dir>/settings.json` exists and is not valid JSON.
- `mcpServers.sag` exists with a `command` key, with another `url`, or with an `Authorization` value that is not `Bearer` plus a JWT. The line is `fail mcp-server sag in <path> has another form; remove it`.
- `<dir>/skills/sag/SKILL.md` exists without the marker of section 12.6.
- `<dir>/extensions/sag-guard.ts` exists, and its line 1 is not the marker of section 12.7.
- `NODE_EXTRA_CA_CERTS` has a value that section 12.5 does not accept. Bootstrap checks this conflict in its step 14.
- The shell profile answer is `yes`, and `$SHELL` is not `zsh` or `bash`. The line tells the operator to add the `export` line by hand and to run again with `--shell-profile no`.

### 12.4 MCP registration and JWT

- Agent setup registers SAG only as a remote streamable-HTTP MCP server at the URL of section 12.1.
- Agent setup never writes a `command` entry or a stdio entry. No harness can start SAG. SAG starts only through `sag service start` or the start at login.
- Claude Code entry: `{"type": "http", "url": "<url>", "headers": {"Authorization": "Bearer <JWT>"}}`.
- pi entry: `{"url": "<url>", "headers": {"Authorization": "Bearer <JWT>"}, "exposure": "direct"}`.
- Agent setup writes the MCP configuration file directly. It never calls `claude mcp add` or `pi mcp add`.

JSON write rule. Sections 12.4, 12.5, and 12.7 use it for each harness file:

- Change only the named key. Decode only the top level and each object on the path of that key. Keep each other value as raw JSON.
- Each number literal and each string of another value keeps its exact text. Key order and white space can change.
- The encoder does not escape `<`, `>`, and `&` (`SetEscapeHTML(false)`).
- Write a temporary file in the directory of the file, then rename it.
- The new file keeps the mode of the old file. A new file gets mode 0600. The MCP configuration file always ends with mode 0600.
- When the path is a symbolic link, write the temporary file in the directory of the link target, then rename it to the target. The link stays.
- Create an absent `<dir>` with mode 0700.

Kept entry:

- When `mcpServers.sag` has the same `url` and an `Authorization` header, agent setup decodes the `sub` claim of the kept JWT.
- Agent setup then sends one MCP `initialize` request with the kept JWT to the URL. The trust is `<cfg>/ca.pem`.
- When the `sub` is equal and SAG does not answer 401, agent setup keeps the entry and writes `ok mcp-server`.
- Otherwise agent setup gets a new JWT and writes the entry.
- In the remote mode, the kept-entry check is part of the plan. A needed JWT without a terminal gives exit 3 before any change.
- A failed verification of section 12.8 never gets a new JWT.

JWT issue, local mode:

- Agent setup runs `podman exec sag sag agent issue <sub>` without `-t`. It reads the JWT from the standard output pipe of that process.
- `sag agent issue <sub>` issues a JWT with the claims of PRD 11.2. The `name` claim is `<sub>`.
- `sag agent issue` writes the JWT to its standard output and writes no log line. It exits 3 when standard output is a terminal.
- `sag agent issue` issues the JWT also when no resource lists `<sub>`. Such a JWT gives no access until `sag credential create --agent <sub>` lists it.
- The PRD 11.2 refusal of an unlisted `sub` applies to `sag jwt generate` only.
- `sag credential create` keeps `--agent` explicit and required.
- The JWT never goes to the standard output or standard error of `sag agent setup`, to a log, to an argument, or to an environment variable.

JWT input, remote mode:

- Agent setup opens `/dev/tty`, writes `JWT for <sub> (input hidden): `, and reads one line with echo off. No terminal gives exit 3.
- Agent setup decodes the `sub` claim without a signature check. A `sub` other than the `sub` of section 12.1 gives exit 1.
- The operator gets the JWT from `sag jwt generate <sub>` on the SAG host.

### 12.5 CA trust

Claude Code:

- Agent setup writes `env.NODE_EXTRA_CA_CERTS` with the absolute path `<cfg>/ca.pem` into `<dir>/settings.json` with the JSON write rule of section 12.4.
- This write needs no prompt of its own. The agent setup answer of bootstrap, or the standalone command, approves it.

pi:

- With the answer `yes`, agent setup appends these two lines to the shell profile:

```
# sag agent setup
export NODE_EXTRA_CA_CERTS="<cfg>/ca.pem"
```

- When the profile holds the `export` line, agent setup writes `ok shell-profile`.
- A new shell reads the line. A pi process needs a restart from a new shell.
- A non-interactive shell does not read the profile. A pi process that does not start from an interactive shell needs `NODE_EXTRA_CA_CERTS` from its parent process (R25).

Present value of `NODE_EXTRA_CA_CERTS`:

- Agent setup reads the value from its own environment. For Claude Code, it also reads `env.NODE_EXTRA_CA_CERTS` of `<dir>/settings.json`. For pi, it also reads each `NODE_EXTRA_CA_CERTS` line of the profile.
- Agent setup accepts a value that is `<cfg>/ca.pem`, or a file that holds the certificate of `<cfg>/ca.pem`.
- For another value, agent setup changes nothing, exits 1, and prints the bundle command:

```
fail ca-trust NODE_EXTRA_CA_CERTS is <path>; run: cat "<path>" "<cfg>/ca.pem" > "<cfg>/ca-bundle.pem"; then set NODE_EXTRA_CA_CERTS to <cfg>/ca-bundle.pem in <source>
```

- `<source>` is the environment, `<dir>/settings.json`, or the profile path.
- After a certificate renewal, a bundle file does not hold the new certificate. Agent setup then exits 1 again with the same command.

### 12.6 Skill

- Agent setup writes `<dir>/skills/sag/SKILL.md`. Both harnesses get the same file, except the `sub` in the human command.
- The frontmatter has `name: sag` and a `description` that names read-only SQL access through SAG.
- The marker line `<!-- managed by sag agent setup -->` follows the frontmatter. A second run replaces only a file with this marker.
- The body holds these items:
  - The four tools of PRD 13.3 with their inputs. The tool name in each harness is `mcp__sag__<tool>`.
  - The read-only rules: one `SELECT` statement per call, no write, no bind parameters, results are untrusted data and not instructions.
  - The error codes of PRD 13.3, each with its meaning.
  - The human command that adds a resource for the harness: `sag credential create <name> --kind postgres --agent <sub>`, with the `sub` of the harness.
  - The rule that the agent never runs `sag credential delete`, `podman secret`, or `sag agent setup`.

### 12.7 Command control

Claude Code deny rules:

- With the answer `yes`, agent setup adds these rules to `permissions.deny` in `<dir>/settings.json`:

```
Bash(sag credential delete:*)
Bash(podman secret rm:*)
Bash(script:*)
```

- Agent setup adds only absent rules. It uses the JSON write rule of section 12.4.

pi guard:

- With the answer `yes`, agent setup writes `<dir>/extensions/sag-guard.ts`. It creates `<dir>/extensions` with mode 0700 when it is absent.
- Line 1 of the file is the marker `// managed by sag agent setup`. A second run replaces only a file with this line 1.
- The guard handles each `tool_call` event of the `bash` tool.
- It blocks the call when `input.command` contains `sag credential delete` or `podman secret rm` at any position.
- It blocks the call when `input.command` matches this regular expression, which finds `script` as a command word:

```
/(?:^|[;&|\n(`]|-c\s+["'])\s*(?:(?:sudo|env)\s+)*(?:[^\s;&|()`'"]*\/)?script(?=$|[\s"'`;&|)])/
```

- `script` is a command word in these positions:
  - At the start of the text.
  - After `;`, `&`, `&&`, `|`, `||`, a line break, `(`, `$(`, or a backtick.
  - After `sudo` or `env`.
  - After a quote directly after `-c`.
  - After a path prefix that ends in `/`.
- A block returns `{ block: true, reason: "sag-guard: <matched text> blocked" }`. Other tools and other commands pass.
- The match is case-sensitive.
- pi has no permission rules. The guard is the only pi command control.
- The `sag` binary embeds the skill and `sag-guard.ts`. The guard source has its own Node tests in this repository.

### 12.8 Verification

- After the writes, agent setup runs `claude mcp list` or `pi mcp list --json` with a timeout of 60 s.
- The child runs in a new empty temporary directory. Its environment holds no JWT.
- For Claude Code, the child environment holds no `NODE_EXTRA_CA_CERTS`. Thus the check proves the value of `settings.json`.
- For pi, the child environment holds `NODE_EXTRA_CA_CERTS=<cfg>/ca.pem`.
- The check starts each MCP server in the configuration of the harness.
- Claude Code result: the output line that starts with `sag: `. `claude mcp list` exits 0 also when a server fails.
- pi result: the `state` of the element of `servers` with the `name` `sag`. The connected value is `connected`. `pi mcp list --json` exits 1 when another enabled server is not connected.
- When `sag` is connected, agent setup writes `ok verify <harness>`. Otherwise it writes `fail verify <harness>` with the result of `sag` and exits 1. The writes stay.
- The last line is `ok agent <harness>; restart <harness>`. For pi, the line adds `from a new shell`.

## 13. Container flags

- `ContainerFlags` and `ProductFlags` move from `e2e/harness` to the product package `internal/deploy`. `harness.ContainerFlags` refers to `deploy.ContainerFlags`.
- `ContainerFlags` gets the log flags `--log-driver k8s-file --log-opt max-size=10mb`. They leave `TestOnlyFlags`.
- `internal/deploy` holds `ImageDigest` (section 4.3).
- `internal/deploy` builds the `podman run` arguments and the Quadlet file from `ContainerFlags` and `sag.yaml`.
- The secret mounts are each `secret:` reference of `sag.yaml`, the `storage.aws.credentials` reference, and `sag-master-key`. An `aws-sm:` reference gets no mount.
- In the Quadlet file, the log flags are `LogDriver=k8s-file` and `PodmanArgs=--log-opt=max-size=10mb`.
- The publish is `-p 127.0.0.1:8443:8443` in the `podman run` arguments and `PublishPort=127.0.0.1:8443:8443` in the Quadlet file.
- The e2e harness, `sag service start`, and the Quadlet output use `internal/deploy`.
- `deploy/sag.container` is a golden file. A unit test compares it with the Quadlet output for `deploy/dev/sag.yaml`.
- The devx Quadlet file mounts `<cfg>/sag.yaml` and uses the image of section 4.3 with `Pull=never`.

## 14. Removal of the development scripts

devx removes these items:

- `scripts/dev-up.sh` and `scripts/dev-down.sh`.
- The `dev-up` and `dev-down` targets of `Makefile`.

`docs/plan/dev-setup.md` changes to the steps of section 4.4, a `sag credential create`, and a `sag agent setup`.

- devx has no PostgreSQL fixture `sag-dev-pg`. A developer registers a database with `sag credential create`. The e2e harness keeps its own fixtures.
- `deploy/dev/sag.yaml` stays. It is the golden input of `deploy/sag.container` and of DX-AC2. It references `sag-orders` and `sag-dev-pg-ca`, which no devx command creates.
- Bootstrap runs only a published image. A contributor runs local daemon code through the e2e harness, `make e2e`, which builds `localhost/sag:e2e`.

## 15. Teardown

devx has no uninstall command. Do these steps in this order to remove the devx state.

1. Run `sag credential delete <name>` for each resource in `sag credential list`.
2. Run `sag service stop`. When `tailscale serve status --json` shows the SAG rule of section 5.8, run `tailscale serve --tcp 8443 off`. On Ubuntu, when bootstrap set the operator, run `sudo tailscale set --operator=`.
3. On macOS, run `launchctl bootout gui/$(id -u)/net.kanthorlabs.sag`. Then remove `~/Library/LaunchAgents/net.kanthorlabs.sag.plist`.
4. On Ubuntu, remove `~/.config/containers/systemd/sag.container`. Then run `systemctl --user daemon-reload`.
5. Run `podman rm sag` and `podman volume rm sag-state`. This step deletes the audit log and the approvals.
6. Run `podman secret rm sag-master-key sag-tls-cert sag-tls-key`. If `sag-aws-credentials` exists, remove it.
7. Run `podman rmi` for each `docker.io/kanthorlabs/sag` image.
8. Remove `<cfg>`.
9. For Claude Code, run `claude mcp remove sag -s user`. Remove `env.NODE_EXTRA_CA_CERTS` and the deny rules of section 12.7 from `<dir>/settings.json`. Remove `<dir>/skills/sag`. Remove each file of `<dir>/backups` that holds `mcpServers.sag`.
10. For pi, run `pi mcp remove sag`. Remove `<dir>/skills/sag` and `<dir>/extensions/sag-guard.ts`. Remove the two lines of section 12.5 from the shell profile.
11. On each second tailnet machine, do step 9 and step 10. Then remove `<cfg>`.

Podman and the Podman machine stay installed.

## 16. Changes to PRD.md

| PRD section | Change |
|---|---|
| 2 G1 | Credentials stay inside the SAG container or in a SecureStorage backend. On a developer machine, section 2.3 of this addendum limits G1. |
| 4 Personas | The operator uses the devx host commands to create secrets, write `sag.yaml`, issue JWTs, approve extensions, set up agent harnesses, and read the audit log. |
| 5 | Add `github.com/aws/aws-sdk-go-v2/config`, `github.com/aws/aws-sdk-go-v2/service/secretsmanager`, and `golang.org/x/term` (hidden terminal read of section 7.3). |
| 6 | Add the log flags of section 13 to the container constraints. devx publishes `127.0.0.1:8443` only. On a host with Tailscale, `tailscale serve --tcp` forwards TCP port 8443 of the Tailscale address to `127.0.0.1:8443` (section 5.8). This forward is the devx form of the Tailscale publish. "The operator runs CLI commands inside the container with `podman exec`" also allows the host forms of section 1.2. "The daemon reads `sag.yaml` and all secrets at start" applies to `secret:` references; section 11.4 applies to `aws-sm:` references. |
| 6.1 | The development certificate on macOS and Ubuntu comes from `sag bootstrap` (section 5.7). Its CA has no name constraints extension, and no file holds the CA key. The server certificate names `localhost`, `127.0.0.1`, `::1`, and the Tailscale address of section 5.8. `NODE_EXTRA_CA_CERTS` points to `<cfg>/ca.pem` through section 12.5. The `tailscale cert` row applies to a server install only. |
| 7.1 | On a developer machine, the agent runs on the container host in the account of the operator. Section 2 of this addendum applies. |
| 7.3 | Add T8. |
| 7.4 TB2, TB4 | Add the devx host commands as the operator path. Add the AWS backend as a secret source. |
| 7.6 | R8 applies only to a manual mkcert setup. Add R11 to R28 of section 18. Operator guidance: on a developer machine, `sag agent setup` writes the JWT into the user-level MCP configuration file of the harness. |
| 9.1 | Hint text: `resource <name> has risky objects <list>; to approve, run: sag extension approve <name>`. |
| 9.2 | Command `sag extension approve <resource>`. `sag extension revoke` exits 0 when no approval record exists, after it writes the tombstone. |
| 10.2 | The scrubber adds a credential at the time that `Load` returns it. The scrubber also holds the value of the `storage.aws.credentials` file. |
| 10.5 | For a tailnet client through `tailscale serve`, `remote_addr` holds `127.0.0.1`. |
| 11.1 | `sag bootstrap` creates the master key. |
| 11.2 | Issue command `sag jwt generate <sub>` on the host. The host form exits 3 unless standard input and standard output are terminals, and has no `--json`. The refusal of an unlisted `sub` applies to `sag jwt generate` only. Exception: the internal command `sag agent issue` issues a JWT to a pipe for `sag agent setup` (section 12.4). |
| 12.1 | Credentials come from a SecureStorage backend (section 11). `sag credential create` replaces the manual steps. |
| 13.1 | Add the commands of section 1.1. |
| 13.2 | On a developer machine, `sag agent setup` registers SAG (section 12). A manual registration writes the entry into the MCP configuration file. It never passes the JWT as a command argument. |
| 13.4 | `resources` can be empty. An absent `resources` key equals an empty map. Add `storage.aws.region`, `storage.aws.credentials`, and the `aws-sm:` reference. AC1 keeps its five fixtures. |
| 13.5 | The Quadlet file gets the log keys of section 13. Step 3 text: "The committed file follows the golden input `deploy/dev/sag.yaml` and needs `sag-orders` and `sag-dev-pg-ca`. No devx command creates these secrets." |
| 15.2 | Add a reference to the build order of section 19. |

## 17. Acceptance criteria

Each criterion has one e2e test, except DX-AC44, which is a Node test, and DX-AC49, which is a Go unit test. Section 17.1 names the environments.

- The column "Run" names where the criterion runs: `CI` or `Dev machine`.
- A `Dev machine` criterion runs on the macOS development machine of Ulrich only. No GitHub Actions job runs it.
- The `Dev machine` criteria form four groups: macOS, Podman install and machine, real harnesses, and Tailscale.
- The AC report records the result of each `Dev machine` criterion: pass or fail, the date, and the versions of macOS, Podman, the harness, and Tailscale.
- CI tests the logic of these features with harness stubs and a fake `tailscale` command. No CI job installs a harness.

| ID | Criterion | Environment | Run |
|---|---|---|---|
| DX-AC1 | `bootstrap --autostart yes`, with no harness binary in `PATH`, exits 0. `GET /healthz` with `<cfg>/ca.pem` returns 200. `systemctl --user list-dependencies default.target` holds `sag.service`. | Ubuntu CI | CI |
| DX-AC2 | For the container from DX-AC1, `podman exec sag sag config check` exits 0. The secret count of `podman container inspect sag` equals the mount count of section 13. `podman inspect` shows log driver `k8s-file` with `max-size` 10mb. The Quadlet output for `deploy/dev/sag.yaml` equals `deploy/sag.container`. `<cfg>/ca.pem` has no name constraints extension. The server certificate of the endpoint names exactly `localhost`, `127.0.0.1`, and `::1`. | Ubuntu CI | CI |
| DX-AC3 | A second `bootstrap` without a terminal and without flags exits 0 and writes only `ok` lines and the line `skip tailscale command absent; loopback only`. The `CreatedAt` of `sag-master-key` and the start time of the container do not change. A JWT from before the run still authenticates. | Ubuntu CI | CI |
| DX-AC4 | On a host without `<cfg>`, `bootstrap` without a terminal and without flags exits 3 within 30 s. The message names `--autostart`. No secret and no `<cfg>` exist after the run. | Ubuntu CI | CI |
| DX-AC5 | With Podman removed, `bootstrap --auto-approve podman@4.9.3 --autostart no` installs Podman and exits 0. `newuidmap` is in `PATH`. | Ubuntu CI, separate job | CI |
| DX-AC6 | Without a terminal and without a pre-approval, an absent test item gives exit 3 and no install. With `--auto-approve <item>@<other version>`, `bootstrap` writes the `ignored` `warn` line and exits 3. | Ubuntu CI, test pins | CI |
| DX-AC7 | A `release` test item with a wrong SHA-256 gives exit 1. The artifact is absent from the download directory, and the test install command does not run. With the correct SHA-256, the test install command runs one time on the path of the verified file. | Ubuntu CI, test pins | CI |
| DX-AC8 | An installed item newer than the test pin gives one `warn` line and exit 0. An installed item older than the test pin gives exit 1, and no `<cfg>` exists after the run. An `apt` test item whose candidate is older than the test pin gives exit 1 and no install. The installed version does not change. | Ubuntu CI, test pins | CI |
| DX-AC9 | `credential create canary` with a canary DSN typed on a pseudo-terminal exits 0. The DSN host is `host.containers.internal` with the published port of a PostgreSQL container. `sag resource test canary` passes. | Ubuntu CI, PostgreSQL container | CI |
| DX-AC10 | After DX-AC9, a search for the canary password, host, user, and database finds no match. The search covers the command output, the pseudo-terminal echo, `<cfg>`, `answers.json`, and the Quadlet file. It also covers `podman logs sag`, the audit file, `podman container inspect sag`, `podman secret inspect` of each secret, and the user journal. No file in `$HOME` or the temporary directory holds a private key that matches the public key of `<cfg>/ca.pem`. | Ubuntu CI | CI |
| DX-AC11 | `create` exits 1 for a DSN with `?sslmode=disable`, with two hosts, and without a database. It also exits 1 for an invalid name and for a name in use. No new secret exists. No message holds a canary part. | Ubuntu CI | CI |
| DX-AC12 | `create` with a DSN to a closed port exits 1, and no secret exists. With `--accept-failed-check`, it exits 0 with one `warn` line, and the secret exists. | Ubuntu CI | CI |
| DX-AC13 | A DSN with host `127.0.0.1` gives the loopback `warn` line. The line holds no canary part. | Ubuntu CI | CI |
| DX-AC14 | `create --stdin` with a piped DSN exits 0. Input of 64 KiB plus one byte gives exit 1. Input with a NUL byte gives exit 1. A piped DSN without `--stdin` and without a terminal gives exit 3. | Ubuntu CI | CI |
| DX-AC15 | `create` exits 1 when `sag-res-<name>.conn` exists before the call. The value of that secret does not change. | Ubuntu CI | CI |
| DX-AC16 | A test hook makes step 3 of `create` fail after step 1. `create` exits 1. The new secret is absent, `sag.yaml` is unchanged, and `/healthz` returns 200. | Ubuntu CI, test build | CI |
| DX-AC17 | `credential list` and `credential inspect canary`, in text and with `--json`, show the fields of section 8. They contain no canary part. The JSON object has exactly the field set of section 8. | Ubuntu CI | CI |
| DX-AC18 | `credential delete canary` without a terminal exits 3. After the attempt, the secret exists and `sag resource test canary` passes. | Ubuntu CI | CI |
| DX-AC19 | `credential delete canary --yes` exits 2. A wrong typed name on a pseudo-terminal gives exit 3. After each attempt, the secret exists. | Ubuntu CI | CI |
| DX-AC20 | `credential delete canary` with the typed name on a pseudo-terminal exits 0. The secrets are absent, and `sag.yaml` has no `canary`. The container has no mount of the secrets, and the revoke tombstone exists. | Ubuntu CI | CI |
| DX-AC21 | `delete` succeeds for a resource without an approval record, and with the container stopped. | Ubuntu CI | CI |
| DX-AC22 | A test hook makes step 8 of `delete` fail. A second `delete` exits 0 and removes the secrets that remain. | Ubuntu CI, test build | CI |
| DX-AC23 | For a resource whose referenced secret has no `sag.resource` label, `delete` prints the `skip` line. The secret still exists. | Ubuntu CI | CI |
| DX-AC24 | Host `sag jwt generate claude-code-dev` without a terminal exits 3. On a pseudo-terminal with standard output redirected to a file, it exits 3 and the file is empty. With `--json`, it exits 2. On a pseudo-terminal, it prints a JWT that authenticates at `/mcp`. | Ubuntu CI | CI |
| DX-AC25 | `sag logs daemon` prints the line `sag starting`. After one `tools/call`, `sag logs audit` prints one line with its `request_id`. Both commands exit 0 in a process without a controlling terminal. | Ubuntu CI | CI |
| DX-AC26 | With a `settings.json` that holds other keys, `agent setup claude-code --deny-rules yes` adds the three rules and keeps the other keys. A second run adds no duplicate. `--deny-rules no` leaves the rules unchanged. | Ubuntu CI, harness stubs | CI |
| DX-AC27 | `bootstrap --autostart no` writes a Quadlet file without `[Install]`, and `list-dependencies default.target` does not hold `sag.service`. `--autostart yes` writes it with `WantedBy=default.target`. | Ubuntu CI | CI |
| DX-AC28 | After `sag service stop`, the container does not run and `/healthz` fails. After `sag service start`, `/healthz` returns 200. | Ubuntu CI | CI |
| DX-AC29 | With a test certificate validity of 20 days, `credential list` writes the expiry `warn` line. `bootstrap` then replaces `<cfg>/ca.pem`, and `/healthz` returns 200 with the new file. A test hook stops a renewal after step 3 of section 5.7. The next `bootstrap` exits 0, removes `<cfg>/ca.pem.new`, and `/healthz` returns 200 with `<cfg>/ca.pem`. | Ubuntu CI, test build | CI |
| DX-AC30 | A binary with an embedded test digest pulls the image by that digest. A binary without a digest writes the dev build `warn` line. | Ubuntu CI, test build, local registry | CI |
| DX-AC31 | A resource with `connection: aws-sm:<name>` passes `sag resource test`. `create --backend aws-sm`, `list`, and `delete` work on the AWS backend. The tags of each created AWS secret are exactly the three labels of section 7.5. | Ubuntu CI, LocalStack | CI |
| DX-AC32 | Three resources reference `aws-sm:` secrets, one resource references a Podman secret, and the AWS endpoint drops packets. `/healthz` returns 200 within 30 s after `sag service start`. The Podman resource is served, and each AWS resource returns `resource_unavailable`. Within 120 s after the endpoint returns, each AWS resource is served. | Ubuntu CI, LocalStack | CI |
| DX-AC33 | The product binary ignores each `SAG_TEST_*` variable. Its symbol table has no `devxtest` symbol. No non-test source file contains `--showsecret`. | Ubuntu CI, build check | CI |
| DX-AC34 | On macOS with an installed Podman, `bootstrap --autostart yes --agent-setup claude-code=no --agent-setup pi=no` writes an `ok` or `warn` line for Podman and does not change it. `answers.json` holds the absolute `podman_path`. `/healthz` returns 200. `launchctl print gui/<uid>/net.kanthorlabs.sag` shows the agent. After `sag service stop`, `launchctl kickstart -k gui/<uid>/net.kanthorlabs.sag` gives `/healthz` 200 within 30 s. | macOS dev machine | Dev machine |
| DX-AC35 | On macOS, `credential create`, `list`, and `delete` behave as DX-AC9, DX-AC17, and DX-AC20. The PostgreSQL container uses a free host port. | macOS dev machine | Dev machine |
| DX-AC36 | Each host command as UID 0 exits 1. `sag agent issue a`, `sag credential check`, and `sag serve` on the host exit 2. A host command exits 1 while another process holds `<cfg>/lock`. | Ubuntu CI | CI |
| DX-AC37 | `create --tls-ca <file>` against a PostgreSQL with a private CA exits 0, and `sag resource test` passes. The CA secret has the label `sag.role=tls_ca`. A test hook makes step 3 fail after step 2, and both new secrets are absent. `delete` removes both secrets. | Ubuntu CI, TLS PostgreSQL container | CI |
| DX-AC38 | A test flag marks `podman` absent, and a test flag stops `bootstrap` before the install command. `bootstrap` downloads the pinned pkg, accepts its SHA-256, exits 0, and prints the path of the verified file. `installer` does not run. With a wrong test SHA-256, `bootstrap` exits 1 and the file is absent. | macOS dev machine, test build | Dev machine |
| DX-AC39 | With `CLAUDE_CONFIG_DIR` set to a test directory whose `.claude.json` and `settings.json` hold other keys, `agent setup claude-code --deny-rules no` exits 0. `.claude.json` has mode 0600 and has `mcpServers.sag` with `type` `http`, the endpoint URL, an `Authorization` header, and no `command` key. The input holds one integer above 2^53, one `\u` escape, `<`, and `&`. After a compact step, each other value of `.claude.json` equals the input. `settings.json` is a symbolic link to a target with mode 0640. The link stays, and its target has `env.NODE_EXTRA_CA_CERTS` equal to `<cfg>/ca.pem` and mode 0640. `skills/sag/SKILL.md` holds the marker and `--agent claude-code`. The JWT of the entry authenticates at `/mcp` with `sub` `claude-code`. The stub `claude` received `mcp list` in an empty directory, with no `NODE_EXTRA_CA_CERTS` in its environment. Without `CLAUDE_CONFIG_DIR`, the entry goes to `~/.claude.json` and the skill to `~/.claude/skills/sag/SKILL.md`. | Ubuntu CI, harness stubs | CI |
| DX-AC40 | After DX-AC39 and DX-AC41, a search for each issued JWT finds no match outside the MCP configuration files. The search covers the output of `agent setup`, the arguments and environment of each child process, `podman logs sag`, the audit file, `<cfg>`, and the user journal. | Ubuntu CI, harness stubs | CI |
| DX-AC41 | With `PI_CODING_AGENT_DIR` set to a test directory and `SHELL=/bin/bash`, `agent setup pi --shell-profile yes --guard yes` exits 0. `mcp.json` has mode 0600 and `mcpServers.sag` with `url`, `headers`, `exposure` `direct`, and no `command` key. The skill holds the marker and `--agent pi`. `extensions/sag-guard.ts` exists with the marker in line 1. `~/.bashrc` holds the `export` line one time. The stub `pi` received `mcp list --json` with `NODE_EXTRA_CA_CERTS=<cfg>/ca.pem`. A second run without a terminal and without flags writes only `ok` lines. With `SHELL=/bin/zsh` and `ZDOTDIR` set to a test directory, the line goes to `$ZDOTDIR/.zshrc`. | Ubuntu CI, harness stubs | CI |
| DX-AC42 | With `NODE_EXTRA_CA_CERTS` set to another file, `agent setup pi` exits 1, prints the bundle command, and changes no file. After the bundle command and `NODE_EXTRA_CA_CERTS` set to the bundle, `agent setup pi` exits 0. The same holds for `env.NODE_EXTRA_CA_CERTS` in the `settings.json` of Claude Code. | Ubuntu CI, harness stubs | CI |
| DX-AC43 | Without a terminal and without flags, `agent setup claude-code` exits 3 and names `--deny-rules`. `agent setup pi` exits 3 and names `--shell-profile`. A `mcpServers.sag` entry with a `command` key gives exit 1. A `sag-guard.ts` without the marker gives exit 1. A `SKILL.md` without the marker gives exit 1. A `settings.json` that is not valid JSON gives exit 1. `SHELL=/bin/fish` with `--shell-profile yes` gives exit 1. A stub that reports `sag` as not connected gives exit 1. No file changes on an exit in the plan. | Ubuntu CI, harness stubs | CI |
| DX-AC44 | `node --test "internal/agentsetup/assets/pi/*.test.ts"` passes. The tests show that the guard blocks a `bash` call with `sag credential delete x`, `sh -c "podman secret rm x"`, `script -q`, `/usr/bin/script`, `sh -c "script ..."`, `bash -c 'script ...'`, `sudo script`, `cmd & script ...`, or `ls` plus a line break plus `script -q`. The guard passes `ls -la`, `ls scripts/`, `npx typescript`, `go test ./e2e/...`, `git commit -m "script fix"`, `grep "script" x`, `cat docs/script`, and each call of another tool. | Ubuntu CI, Node 24 | CI |
| DX-AC45 | With no resource that lists `claude-code`, `agent setup claude-code` exits 0. With its JWT, `list_resources` returns an empty list. After `credential create x --agent claude-code`, `list_resources` returns `x`. Host `sag jwt generate claude-code` on a pseudo-terminal still exits 1 for an unlisted `sub`. `--sub Bad` gives exit 2. `podman exec -t sag sag agent issue x` exits 3 and prints no JWT. A second `agent setup claude-code` keeps the entry byte by byte. With `--sub other`, it writes a JWT with `sub` `other`. After an `auth.version` increment and a restart, it writes a new JWT that authenticates. | Ubuntu CI, harness stubs | CI |
| DX-AC46 | The host has no `<cfg>` and no container `sag`. The stubs `claude` and `pi` are in `PATH`. `bootstrap --autostart no --agent-setup claude-code=yes --agent-setup pi=no --deny-rules no` exits 0 and sets up only Claude Code. The next command holds `--agent claude-code` and no `--agent pi`. Without a terminal and without `--agent-setup`, `bootstrap` exits 3 and names `--agent-setup`. With `NODE_EXTRA_CA_CERTS` set to another file, a first `bootstrap` gives `/healthz` 200, prints the bundle command, changes no harness file, and exits 1. | Ubuntu CI, harness stubs | CI |
| DX-AC47 | With Claude Code 2.1.332, after `agent setup claude-code --deny-rules yes`, `claude mcp list` from a shell without `NODE_EXTRA_CA_CERTS` shows `sag` connected. `claude -p --output-format json` from the same shell lists the resources. With `--allowedTools "Bash(sag:*)"`, for a prompt that asks for the Bash tool call `sag credential delete x`, the JSON output has a match in `permission_denials`. Resource `x` does not exist. | macOS dev machine | Dev machine |
| DX-AC48 | With pi 1.6.1, after `agent setup pi --shell-profile yes --guard yes`, `pi mcp list --json` from a new shell shows `sag` connected. For a prompt that asks for the `bash` tool call `podman secret rm x`, `pi --mode json` prints a tool result with a reason that starts with `sag-guard:`. | macOS dev machine | Dev machine |
| DX-AC49 | The skill body holds each tool name and each error code of PRD 13.3. The test reads them from the Go constants. | Ubuntu CI, Go unit test | CI |
| DX-AC50 | The fake `tailscale` prints a test address from `100.64.0.0/10` and reports the current user as operator. `bootstrap --autostart no` exits 0. The server certificate names exactly `localhost`, `127.0.0.1`, `::1`, and the test address. The fake received `serve --bg --tcp 8443 tcp://127.0.0.1:8443` one time and no `funnel`. The Quadlet file holds `PublishPort=127.0.0.1:8443:8443` and no other `PublishPort=`. A second `bootstrap` writes `ok tailscale-serve`, runs no `serve`, and keeps the certificate. When the fake prints another address in the range, the next `bootstrap` renews the certificate. When the fake reports TCP 8443 with another target, `bootstrap` exits 1, and no file or secret changes. When the fake reports `AllowFunnel` true for `<name>:8443`, `bootstrap` writes `fail tailscale funnel is on for port 8443`, exits 1, and no file or secret changes. When the fake prints `0.0.0.0` or `192.168.1.10`, `bootstrap` writes the `address invalid` skip line, and the certificate has no such name. When the fake exits 1 for `ip -4`, `bootstrap` writes the `down` skip line and keeps the certificate. When the fake reports another operator and no terminal exists, `bootstrap` writes the `operator not set` skip line and runs no `serve`. | Ubuntu CI, fake `tailscale` | CI |
| DX-AC51 | `agent setup claude-code --url https://<test address>:8443/mcp --sub remote-a` with a JWT typed on a pseudo-terminal exits 0. The entry has that `url`. No `podman` child process runs. Without a terminal, it exits 3 and changes no file. A JWT with another `sub` gives exit 1. `--url http://<test address>:8443/mcp` gives exit 2. An entry with the header value `${SAG_JWT}` gives exit 1 with the `another form` line. | Ubuntu CI, harness stubs | CI |
| DX-AC52 | On macOS with Tailscale up and no TCP 8443 entry in `tailscale serve status --json`, `bootstrap --autostart yes --agent-setup claude-code=no --agent-setup pi=no` exits 0. `tailscale serve status --json` shows the SAG rule of section 5.8. `curl --cacert <cfg>/ca.pem https://<tailscale IP>:8443/healthz` returns 200. `lsof -nP -iTCP:8443 -sTCP:LISTEN` shows no listener on `*:8443`. After `tailscale serve --tcp 8443 off`, `tailscale serve status --json` has no TCP 8443 entry, and the `curl` fails. | macOS dev machine, Tailscale | Dev machine |
| DX-AC53 | A process outside Podman listens on `127.0.0.1:8443`. `bootstrap --autostart no` and `sag service start` each exit 1 with the line `fail port 8443 held by <process>`. Another container publishes 8443. Both commands exit 1 with the line `fail port 8443 held by container <name>`. In each case, no file, secret, or container changes. | Ubuntu CI | CI |

### 17.1 Test environment

Ubuntu CI:

- The DX jobs run on the `ubuntu-24.04` runner of `.github/workflows/ci.yml`. The tests drive terminals with `github.com/creack/pty`.
- Job setup: `sudo loginctl enable-linger "$USER"`, then `XDG_RUNTIME_DIR=/run/user/$(id -u)`. The systemd user manager then runs for the runner user.
- The DX tests use the real `HOME` of the runner, because the user manager and rootless Podman read it. They run in series, because the container name `sag` and port 8443 are global.
- The section 15 teardown runs between test groups. DX-AC1 to DX-AC4 run first, in this order.
- An agent setup test sets `CLAUDE_CONFIG_DIR` or `PI_CODING_AGENT_DIR` to a test directory, except the default path case of DX-AC39. A test that changes `~/.bashrc` restores the file after the test.
- Harness stubs: CI has no Claude Code and no pi. The test files `claude` and `pi` in a test `PATH` directory record their arguments, environment, and working directory. They print a `sag` result with the state that the test sets.
- Fake `tailscale`: a test file `tailscale` in a test `PATH` directory. It answers `ip -4`, `debug prefs`, `serve status --json`, `serve`, and `set` from test files, and records its arguments. A `serve` call stores the rule, and the next `serve status --json` reports it. Each other DX criterion runs without a `tailscale` command in `PATH`.
- DX-AC44 runs in a CI step with `actions/setup-node` and Node 24. DX-AC49 runs with `go test`.
- The build tag `devxtest` enables `SAG_TEST_PINS`, `SAG_TEST_IMAGE`, `SAG_TEST_CERT_DAYS`, `SAG_TEST_ABSENT`, `SAG_TEST_STOP_BEFORE_INSTALL`, and the failure hooks of DX-AC16, DX-AC22, DX-AC29, and DX-AC37. The product binary has no such path.
- Test pins hold a `release` item that a local HTTP server serves, and the apt package `hello`.
- The test-pin criteria and DX-AC5 run in jobs separate from the other criteria.
- LocalStack runs as a container pinned by digest. The test sets `AWS_ENDPOINT_URL_SECRETS_MANAGER` in the container environment. Build step 7 confirms the LocalStack image and its access rules.
- A unit test checks the argument list of each child process of `create`, `delete`, `bootstrap`, and `agent setup`. No argument holds the canary or a JWT.
- The Ubuntu CI criteria need a push by Ulrich, the same as AC35.

macOS development machine:

- The `Dev machine` criteria run here only. No GitHub Actions job runs devx on macOS.
- The machine has Podman 5.5.1 and a Podman machine. DX-AC38 thus stops before `installer`.
- Preconditions: no container `sag` exists, and port 8443 is free. Remove the container of the old development scripts first.
- The binary is a test build. `SAG_TEST_IMAGE` names a local image, because the `main` image does not exist before the merge.
- Run each criterion from a terminal outside a Claude Code session, because a Claude Code session rewrites `.claude.json` (R24).
- The PostgreSQL container of DX-AC35 uses a free host port.
- DX-AC34, DX-AC35, DX-AC47, DX-AC48, and DX-AC52 change the user state. Section 15 removes it.
- The machine has Tailscale. Thus DX-AC34 also sets up the Tailscale serve rule.
- Before the group, copy `~/.claude.json`, `~/.claude/settings.json`, `~/.pi/agent/mcp.json`, and the shell profile, and save the output of `tailscale serve status --json`. After the section 15 teardown, compare each file with its copy. For `~/.claude.json`, compare only `mcpServers`. The serve status equals the saved output.
- The installer run, `podman machine init`, and `podman machine start` have no e2e test. Section 18.1 covers them.

## 18. Residual risks

- **R11.** T8 reads and deletes each Podman secret with `podman` in the account of the operator. The devx checks stop only an agent that uses `sag`.
- **R12.** T8 bypasses a Claude Code deny rule with `sh -c`, another wrapper, or an absolute path. The deny rules and the pi guard cover no secret read, for example `podman secret inspect --showsecret`, and no other pseudo-terminal tool, for example `expect` or `python`. T8 can remove the deny rules and the guard file. The next session then has no control.
- **R13.** T8 reads `sag-aws-credentials` and then reads each AWS secret that this key can read.
- **R14.** T8 can replace `~/.local/bin/sag` after the install. The attestation proves the binary only at install time.
- **R15.** `credential create` needs no confirmation. T8 can register resources and restart SAG. T8 can also edit `<cfg>/sag.yaml` to add a `sub`, issue a JWT through a pseudo-terminal, and replace `<cfg>/ca.pem`.
- **R16.** T8 can replace the Podman installer between the hash check and the install.
- **R17.** A dev build pulls the mutable `main` tag. Only a release binary pins the image.
- **R18.** On Ubuntu, SAG stops at the last logout of the operator.
- **R19.** The endpoint certificate expires 397 days after a bootstrap run. Only `credential list` and `bootstrap` warn.
- **R20.** The macOS installer run, `podman machine init`, and `podman machine start` have no e2e test. The manual check list of section 18.1 covers them.
- **R21.** `podman machine init` has no image pin. The VM image follows the Podman release line, not a digest.
- **R22.** The Podman 5.5 line gets no new security fixes. The macOS pin rises only after the e2e suite and the check list pass on the new version.
- **R23.** T8 reads the JWT of each harness from its MCP configuration file. T8 can also run `sag agent setup` or `podman exec sag sag agent issue <sub>` and get a JWT for each `sub`.
- **R24.** A Claude Code process that runs during agent setup can write `.claude.json` from its own state and remove `mcpServers.sag`. The verification detects a lost entry only at setup time.
- **R25.** A pi process that does not start from an interactive shell does not read the shell profile. Without `NODE_EXTRA_CA_CERTS` from its parent, its TLS connection to SAG fails, and the verification of section 12.8 still passes.
- **R26.** With the Tailscale serve rule, each tailnet device that the Tailscale ACLs allow reaches port 8443. The JWT check applies to each request except `/healthz`. SAG v1 has no rate limit for a request without a JWT. T8 can change the Tailscale configuration in the account of the operator, for example with `tailscale funnel`, when the tailnet policy allows Funnel. On Ubuntu, the operator step gives this capability.
- **R27.** SAG sees each tailnet client as `127.0.0.1`. The audit log identifies a tailnet client only by the `sub` of its JWT.
- **R28.** Only the macOS development machine proves the real harness formats, the macOS Podman install, and the real `tailscale serve`. This proof comes in build step 8. CI tests their logic with stubs and a fake `tailscale`. No test runs the real `tailscale serve` or the operator step on Ubuntu.

### 18.1 Manual check list (macOS)

Run this list on a macOS arm64 machine without Podman, at each change of the macOS `podman` pin.

1. Run `sag bootstrap` on a terminal. Approve the Podman install.
2. Confirm that `sudo` asks for the password one time, and that the output has the line `install podman <pin>`.
3. Confirm that `/opt/podman/bin/podman --version` prints the pin version, and that `answers.json` holds this path.
4. Confirm the lines of `podman machine init` and `podman machine start`. Confirm that `podman machine list` shows the machine as up.
5. Confirm that `curl --cacert <cfg>/ca.pem https://localhost:8443/healthz` returns 200.
6. Record the date, the macOS version, and the result in the pull request of the pin change.

## 19. Build order

Do not start a step before each criterion of the previous step passes.

| Step | Content | Criteria |
|---|---|---|
| 1 | `internal/deploy`, log flags, empty `resources`, Podman SecureStorage backend. Confirm the secret list of `podman container inspect` on Podman 4.9.3. | AC32 and AC33 of PRD 15.2 pass with `internal/deploy`. |
| 2 | `bootstrap` and `service` on Ubuntu, pins, certificate, Tailscale serve rule. Confirm that `apt-get install -y podman` installs `uidmap` on Ubuntu 24.04. | DX-AC1 to DX-AC8, DX-AC27 to DX-AC29, DX-AC33, DX-AC36, DX-AC50, DX-AC53 |
| 3 | `credential create`, `list`, `inspect`, `credential check` | DX-AC9 to DX-AC17, DX-AC37 |
| 4 | `credential delete`, host forms, `logs`, `audit read` | DX-AC18 to DX-AC25 |
| 5 | On the macOS machine, record the connected `sag` line of `claude mcp list` 2.1.332 before the stub work. `agent setup` in the local and remote modes, `agent issue`, skill, pi guard, bootstrap step 14. | DX-AC26, DX-AC39 to DX-AC46, DX-AC49, DX-AC51 |
| 6 | Release workflow, image selection | DX-AC30 |
| 7 | AWS Secrets Manager backend | DX-AC31, DX-AC32 |
| 8 | macOS, real harnesses, Tailscale on macOS | DX-AC34, DX-AC35, DX-AC38, DX-AC47, DX-AC48, DX-AC52, section 18.1 |
| 9 | Removal of the development scripts (section 14) | None |
