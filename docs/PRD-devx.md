# SAG developer experience (devx): PRD addendum

| Field | Value |
|---|---|
| Owner | Ulrich |
| Status | Revision 3 |
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
- The endpoint listens on `127.0.0.1:8443` only. A Tailscale address and a Tailscale certificate are out of scope.
- devx v1 supports two agent harnesses: Claude Code 2.1.332 and pi 1.6.1 (section 12).
- Out of scope: an uninstall command, a self-update, a master key reset, a change of the `agents` list of a resource, a credential replacement command.

### 1.1 Commands

| Command | Runs on | Function |
|---|---|---|
| `sag bootstrap` | Host | Install the pinned dependencies, prepare SAG state, set the start at login, start SAG, set up the agent harnesses. |
| `sag service start` | Host | Start SAG. The start at login runs this command. |
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
| `<cfg>/answers.json` | Stored answers and the absolute path of `podman`, mode 0600. |
| `<cfg>/lock` | Lock file, mode 0600. |
| Start at login, macOS | LaunchAgent `~/Library/LaunchAgents/net.kanthorlabs.sag.plist` |
| Start at login, Ubuntu | Quadlet file `~/.config/containers/systemd/sag.container` of the user manager |
| Agent harness files | Section 12.2 |

- `sag` never reads, stores, or passes a password. Only the Podman install calls `sudo`, and `sudo` reads the password itself.
- With no terminal, bootstrap calls `sudo -n`. A refusal by `sudo` gives exit 3.

### 2.2 Actor T8

| ID | Actor | Capability |
|---|---|---|
| T8 | AI agent with a shell in the OS account of the operator, for example Claude Code or pi | Runs each command that the operator can run. Reads and writes each file of the operator. Opens a pseudo-terminal with `script` or `expect`. |

### 2.3 Guarantees and limits

- No devx command prints, logs, or writes a credential value. The value goes from `/dev/tty` or standard input to the backend, and to the container.
- Thus a credential value does not reach the agent context through a `sag` command or through MCP.
- No devx command prints or logs a JWT that `sag agent setup` issues. The JWT goes only into the MCP configuration file of the harness.
- An agent with a shell in the same account can read or delete each Podman secret with `podman` directly. The `sag credential delete` check stops only an agent that uses `sag`.
- `sag credential delete` reads the typed resource name from the controlling terminal. It stops an agent that runs `sag` through a tool without a terminal. It does not stop an agent that opens a pseudo-terminal or runs `podman secret rm`.
- The same limit applies to the AWS backend. T8 reads the Podman secret `sag-aws-credentials` and then reads each AWS secret that this key can read.
- T8 reads the JWT of each harness from its MCP configuration file. The file mode 0600 stops only other OS users.

### 2.4 Agent command controls

Section 12.7 adds Claude Code deny rules and the pi extension `sag-guard.ts`. Both stop only a direct command. These limits apply:

- A deny rule matches a command prefix. An agent that runs the command through `sh -c` bypasses the rule.
- An agent that runs a command through an absolute path, for example `/usr/bin/script`, bypasses a deny rule.
- The rules and the guard cover no read of a secret, for example `podman secret inspect --showsecret`.
- The rules and the guard cover no other pseudo-terminal tool, for example `expect` or `python`.
- The guard matches command text. An agent that builds the command text at run time, for example from two parts or from base64, bypasses the guard.

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
3. Plan: read the state and collect each needed answer of section 5.4. If an answer is needed and no source gives it, exit 3. A plan conflict of section 5.3 or section 12.3 gives exit 1. Both exits occur before any change.
4. Create `<cfg>` with mode 0700 when it is absent. Take the lock `<cfg>/lock`. If another process holds it, exit 1.
5. Read the state again and plan again. A new needed answer without a source gives exit 3.
6. For each dependency, apply the dependency rule (section 5.3). On macOS, when no Podman machine exists, run `podman machine init`. When the machine is stopped, run `podman machine start`.
7. Pull the image of section 4.3 when it is absent.
8. Create the secret `sag-master-key` when it is absent (section 5.6).
9. Create or renew the endpoint certificate (section 5.7).
10. Create `<cfg>/sag.yaml` when it is absent: `version`, `server`, `auth`, and an empty `resources` map.
11. Apply the start at login (section 5.5).
12. When the container `sag` does not run, or step 8 to step 11 changed an item, run `sag service start` (section 6). Otherwise read `GET /healthz` with `<cfg>/ca.pem` and write `ok sag`.
13. For each harness with the answer `yes`, run the steps of `sag agent setup` (section 12) with the answers of the plan.
14. Print the endpoint `https://localhost:8443/mcp` and the next command: `sag credential create <name> --kind postgres --agent claude-code --agent pi`. For each detected harness without setup, print `sag agent setup <harness>`.

- On a host without `<cfg>`, an exit in step 3 leaves no `<cfg>`.
- Step 13 runs in the bootstrap process under the lock of step 4.
- A second run on a ready host writes only `ok` lines, asks nothing, does not restart SAG, and exits 0.
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
| Agent setup | `--agent-setup <harness>=yes\|no` | `Set up SAG for <harness> in <absolute config directory>? [y/N]` | The harness is detected, and `answers.json` has no `agent_setup.<harness>` value. |
| Answers of agent setup | `--deny-rules`, `--shell-profile`, `--guard` | Section 12.3 | The harness has the agent setup answer `yes`. |

- `<harness>` is `claude-code` or `pi`. A harness is detected when its binary, `claude` or `pi`, is in `PATH`.
- The config directory of a harness is in section 12.2.
- A flag replaces the stored answer. Bootstrap writes each answer of `--autostart` and `--agent-setup` to `answers.json`. Agent setup stores its own answers (section 12.3).
- Bootstrap asks only when the answer is needed and no flag gives it.
- With no terminal and no flag for a needed answer, bootstrap exits 3. The message names the flag.
- Bootstrap stores no install approval.

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
- The CA certificate has a critical name constraints extension. It permits only the DNS name `localhost` and the IP ranges `127.0.0.1/32` and `::1/128`.
- Thus the CA trust that `NODE_EXTRA_CA_CERTS` adds covers only the endpoint names.
- Bootstrap signs one server certificate for `localhost`, `127.0.0.1`, and `::1`, with the same validity.
- Bootstrap discards the CA key after it signs.
- The server certificate becomes the secret `sag-tls-cert`. The server key becomes the secret `sag-tls-key`.
- Each value goes to the standard input of `podman secret create <name> -`. The server key never goes to a file, an argument, or an output.
- The CA certificate goes to `<cfg>/ca.pem`.

Bootstrap creates the three items in this order:

1. Write the CA certificate to `<cfg>/ca.pem.new` with mode 0644.
2. Remove `sag-tls-cert` and `sag-tls-key` when they exist.
3. Create `sag-tls-cert` and `sag-tls-key`.
4. Rename `<cfg>/ca.pem.new` to `<cfg>/ca.pem`.

Bootstrap runs this sequence in each of these cases:

- One of `sag-tls-cert`, `sag-tls-key`, and `<cfg>/ca.pem` is absent.
- `<cfg>/ca.pem.new` exists.
- Less than 30 days remain on `<cfg>/ca.pem`.

- Thus a second run repairs a stopped run.
- After a new certificate, step 12 restarts SAG. Each agent client then needs a restart. The path `<cfg>/ca.pem` does not change, so no agent configuration changes.

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
- `start` and `stop` take the lock `<cfg>/lock`.
- On macOS, the run arguments include `--restart=on-failure`.

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
- Build step 1 confirms that `podman container inspect` of Podman 4.9.3 lists the secrets of the container. When it does not, `secrets[].mounted` is not part of v1, and DX-AC20 reads the Quadlet file.

## 9. `sag credential delete`

### 9.1 Synopsis

```
sag credential delete <name>
```

- `delete` has no `--yes`, no `--stdin`, and no `--json`. Each of these flags gives exit 2.
- `delete` opens `/dev/tty`. When `/dev/tty` cannot be opened, `delete` exits 3 and changes nothing.

### 9.2 Ownership

- A resource owns each secret that its `sag.yaml` entry references and that has the label `sag.resource=<name>`.
- A resource also owns each secret with the label `sag.resource=<name>` that no entry references. A failed create leaves such a secret.
- devx manages only the secrets of `sag credential create`. A referenced secret without the label is not owned.

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
9. For each referenced secret that is not owned, print `skip <secret name> not created by sag; not removed`.

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

- `storage.aws.region` is required when a reference starts with `aws-sm:`.
- `storage.aws.credentials` is required in the same case. It references a Podman secret.
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
sag agent setup claude-code [--sub <sub>] [--deny-rules yes|no]
sag agent setup pi [--sub <sub>] [--shell-profile yes|no] [--guard yes|no]
```

- The default `sub` is `claude-code` for Claude Code and `pi` for pi.
- `--sub <sub>` replaces the default. A value that does not match `^[a-z][a-z0-9-]{0,62}$` gives exit 2.
- An unknown harness name gives exit 2.
- Bootstrap runs the same steps in its step 13, under its own lock.

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
| Verify command | `claude mcp list` | `pi mcp list` |

- On the machine of Ulrich, `$CLAUDE_CONFIG_DIR/.claude.json` holds the key `mcpServers` and has mode 0600 `[read]`.
- The shell profile of pi depends on the base name of `$SHELL`:

| Shell | Profile |
|---|---|
| `zsh` | `${ZDOTDIR:-$HOME}/.zshrc` |
| `bash`, Ubuntu | `~/.bashrc` |
| `bash`, macOS | `~/.bash_profile` |

### 12.3 Answers and plan conflicts

| Answer | Flag | Prompt | Needed when |
|---|---|---|---|
| Claude Code deny rules | `--deny-rules yes\|no` | `Add 3 deny rules to <absolute path of settings.json>? [y/N]` | A rule of section 12.7 is absent, and `answers.json` has no `deny_rules` value. |
| pi shell profile | `--shell-profile yes\|no` | `Add "export NODE_EXTRA_CA_CERTS=<cfg>/ca.pem" to <absolute path of the profile>? [y/N]` | The line is absent, and `answers.json` has no `shell_profile` value. |
| pi guard | `--guard yes\|no` | `Install <absolute path of sag-guard.ts> to block SAG secret commands in bash? [y/N]` | The guard file is absent, and `answers.json` has no `guard` value. |

- A flag replaces the stored answer. Agent setup writes each answer to `answers.json`.
- Agent setup asks only when the answer is needed and no flag gives it.
- With no terminal and no flag for a needed answer, agent setup exits 3. The message names the flag.
- Without a `yes` answer, agent setup does not change the file of that answer.

Plan conflicts. Each conflict gives one `fail` line and exit 1 before any change:

- The harness binary, `claude` or `pi`, is not in `PATH`.
- The container `sag` does not run.
- The MCP configuration file or `<dir>/settings.json` exists and is not valid JSON.
- `mcpServers.sag` exists with a `command` key or with another `url`. The line is `fail mcp-server sag in <path> has another form; remove it`.
- `<dir>/skills/sag/SKILL.md` exists without the marker of section 12.6.
- `<dir>/extensions/sag-guard.ts` exists, and its line 1 is not the marker of section 12.7.
- `NODE_EXTRA_CA_CERTS` has a value that section 12.5 does not accept.
- The shell profile answer is `yes`, and `$SHELL` is not `zsh` or `bash`. The line tells the operator to add the `export` line by hand and to run again with `--shell-profile no`.

### 12.4 MCP registration and JWT

- Agent setup registers SAG only as a remote streamable-HTTP MCP server at `https://localhost:8443/mcp`.
- Agent setup never writes a `command` entry or a stdio entry. No harness can start SAG. SAG starts only through `sag service start` or the start at login.
- Claude Code entry: `{"type": "http", "url": "https://localhost:8443/mcp", "headers": {"Authorization": "Bearer <JWT>"}}`.
- pi entry: `{"url": "https://localhost:8443/mcp", "headers": {"Authorization": "Bearer <JWT>"}}`.
- Agent setup writes the MCP configuration file directly. It never calls `claude mcp add` or `pi mcp add`, because their header flag puts the JWT in an argument.
- Write rule: change only `mcpServers.sag`. Keep each other key and value. Write a temporary file with mode 0600 in the same directory, then rename it. The final file has mode 0600.
- When `mcpServers.sag` has the same `url` and an `Authorization` header, agent setup keeps the entry and writes `ok mcp-server`.
- When the verification of a kept entry fails, agent setup issues a new JWT, writes the entry, and verifies one more time.

JWT issue:

- Agent setup runs `podman exec sag sag agent issue <sub>` without `-t`. It reads the JWT from the standard output pipe of that process.
- `sag agent issue <sub>` issues a JWT with the claims of PRD 11.2. The `name` claim is `<sub>`.
- `sag agent issue` writes the JWT to its standard output and writes no log line. It exits 3 when standard output is a terminal.
- `sag agent issue` issues the JWT also when no resource lists `<sub>`. Such a JWT gives no access until `sag credential create --agent <sub>` lists it.
- The PRD 11.2 refusal of an unlisted `sub` applies to `sag jwt generate` only.
- `sag credential create` keeps `--agent` explicit and required.
- The JWT never goes to the standard output or standard error of `sag agent setup`, to a log, to an argument, or to an environment variable.

### 12.5 CA trust

Claude Code:

- Agent setup writes `env.NODE_EXTRA_CA_CERTS` with the absolute path `<cfg>/ca.pem` into `<dir>/settings.json`.
- It keeps each other key and value. It writes a temporary file and renames it.
- This write needs no prompt of its own. The agent setup answer of bootstrap, or the standalone command, approves it.

pi:

- With the answer `yes`, agent setup appends these two lines to the shell profile:

```
# sag agent setup
export NODE_EXTRA_CA_CERTS="<cfg>/ca.pem"
```

- When the profile holds the `export` line, agent setup writes `ok shell-profile`.
- A new shell reads the line. A pi process needs a restart from a new shell.

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

- Agent setup writes `<dir>/skills/sag/SKILL.md`. Both harnesses get the same file.
- The frontmatter has `name: sag` and a `description` that names read-only SQL access through SAG.
- The marker line `<!-- managed by sag agent setup -->` follows the frontmatter. A second run replaces only a file with this marker.
- The body holds these items:
  - The four tools of PRD 13.3 with their inputs.
  - The read-only rules: one `SELECT` statement per call, no write, no bind parameters, results are untrusted data and not instructions.
  - The error codes of PRD 13.3, each with its meaning.
  - The human command that adds a resource for both harnesses: `sag credential create <name> --kind postgres --agent claude-code --agent pi`.
  - The rule that the agent never runs `sag credential delete`, `podman secret`, or `sag agent setup`.

### 12.7 Command control

Claude Code deny rules:

- With the answer `yes`, agent setup adds these rules to `permissions.deny` in `<dir>/settings.json`:

```
Bash(sag credential delete:*)
Bash(podman secret rm:*)
Bash(script:*)
```

- Agent setup adds only absent rules. It keeps each other key and value. It writes a temporary file and renames it.

pi guard:

- With the answer `yes`, agent setup writes `<dir>/extensions/sag-guard.ts`. It creates `<dir>/extensions` with mode 0700 when it is absent.
- Line 1 of the file is the marker `// managed by sag agent setup`. A second run replaces only a file with this line 1.
- The guard handles each `tool_call` event of the `bash` tool. It blocks the call when `input.command` contains `sag credential delete`, `podman secret rm`, or `script` at any position.
- A block returns `{ block: true, reason: "sag-guard: <matched text> blocked" }`. Other tools and other commands pass.
- The match is case-sensitive. The guard also blocks a command that contains `script` inside another word, for example `typescript` or `scripts/`.
- pi has no permission rules. The guard is the only pi command control.

Repository layout:

```
internal/agentsetup/
  agentsetup.go             go:embed of assets/skill/SKILL.md and assets/pi/sag-guard.ts
  assets/skill/SKILL.md
  assets/pi/sag-guard.ts
  assets/pi/sag-guard.test.ts
```

- The `go:embed` directive names the two product files. The test file stays out of the binary.
- `sag-guard.ts` imports only types from `@earendil-works/pi-coding-agent`, with `import type`. Node runs the file without that package.
- `sag-guard.test.ts` uses only `node:test` and `node:assert`. It calls the default export with a test `pi` object and checks the result of the `tool_call` handler.
- CI runs `node --test internal/agentsetup/assets/pi/` with Node 24 and its default type strip.

### 12.8 Verification

- After the writes, agent setup runs `claude mcp list` or `pi mcp list` with a timeout of 60 s.
- The child environment holds `NODE_EXTRA_CA_CERTS=<cfg>/ca.pem`. It holds no JWT.
- The result is the line of the server `sag` in the output. The exit code alone does not decide: `pi mcp list` exits 1 when another enabled server is not connected.
- When `sag` is connected, agent setup writes `ok verify <harness>`. Otherwise it writes `fail verify <harness>` with the output line of `sag` and exits 1. The writes stay.
- Build step 5 records the output form of both commands for Claude Code 2.1.332 and pi 1.6.1.
- The last line is `ok agent <harness>; restart <harness>`. For pi, the line adds `from a new shell`.

## 13. Container flags

- `ContainerFlags` and `ProductFlags` move from `e2e/harness` to the product package `internal/deploy`. `harness.ContainerFlags` refers to `deploy.ContainerFlags`.
- `ContainerFlags` gets the log flags `--log-driver k8s-file --log-opt max-size=10mb`. They leave `TestOnlyFlags`.
- `internal/deploy` holds `ImageDigest` (section 4.3).
- `internal/deploy` builds the `podman run` arguments and the Quadlet file from `ContainerFlags` and `sag.yaml`.
- The secret mounts are each `secret:` reference of `sag.yaml`, the `storage.aws.credentials` reference, and `sag-master-key`. An `aws-sm:` reference gets no mount.
- In the Quadlet file, the log flags are `LogDriver=k8s-file` and `PodmanArgs=--log-opt=max-size=10mb`.
- The e2e harness, `sag service start`, and the Quadlet output use `internal/deploy`.
- `deploy/sag.container` is a golden file. A unit test compares it with the Quadlet output for `deploy/dev/sag.yaml`.
- The devx Quadlet file mounts `<cfg>/sag.yaml` and uses the image of section 4.3 with `Pull=never`.

## 14. Removal of the development scripts

devx removes these items:

- `scripts/dev-up.sh` and `scripts/dev-down.sh`.
- The `dev-up` and `dev-down` targets of `Makefile`.

`docs/plan/dev-setup.md` changes to the steps of section 4.4, a `sag credential create`, and a `sag agent setup`.

| `scripts/dev-up.sh` function | devx replacement |
|---|---|
| mkcert endpoint certificate | Section 5.7 |
| Master key from `openssl rand` | Section 5.6 |
| Image build `localhost/sag-dev:latest` | Image pull, section 4.3 |
| Container run with the PRD 6 flags | `sag service start`, section 13 |
| Wait for `/healthz` | `sag bootstrap` step 12 |
| PostgreSQL fixture `sag-dev-pg` and its secrets | None. A developer registers a database with `sag credential create`. The e2e harness keeps its own fixtures. |

- `deploy/dev/sag.yaml` stays. It is the golden input of `deploy/sag.container` and of DX-AC2. It references `sag-orders` and `sag-dev-pg-ca`, which no devx command creates.
- Bootstrap runs only a published image. A contributor runs local daemon code through the e2e harness, `make e2e`, which builds `localhost/sag:e2e`.

## 15. Teardown

devx has no uninstall command. Do these steps in this order to remove the devx state.

1. Run `sag credential delete <name>` for each resource in `sag credential list`.
2. Run `sag service stop`.
3. On macOS, run `launchctl bootout gui/$(id -u)/net.kanthorlabs.sag`. Then remove `~/Library/LaunchAgents/net.kanthorlabs.sag.plist`.
4. On Ubuntu, remove `~/.config/containers/systemd/sag.container`. Then run `systemctl --user daemon-reload`.
5. Run `podman rm sag` and `podman volume rm sag-state`. This step deletes the audit log and the approvals.
6. Run `podman secret rm sag-master-key sag-tls-cert sag-tls-key`. If `sag-aws-credentials` exists, remove it.
7. Run `podman rmi` for each `docker.io/kanthorlabs/sag` image.
8. Remove `<cfg>`.
9. For Claude Code, remove `mcpServers.sag` from the MCP configuration file. Remove `env.NODE_EXTRA_CA_CERTS` and the deny rules of section 12.7 from `<dir>/settings.json`. Remove `<dir>/skills/sag`.
10. For pi, remove `mcpServers.sag` from `<dir>/mcp.json`. Remove `<dir>/skills/sag` and `<dir>/extensions/sag-guard.ts`. Remove the two lines of section 12.5 from the shell profile.
11. To remove the state of the old scripts, run `scripts/dev-down.sh` from a commit before the removal.

Podman and the Podman machine stay installed.

## 16. Changes to PRD.md

| PRD section | Change |
|---|---|
| 2 G1 | Credentials stay inside the SAG container or in a SecureStorage backend. On a developer machine, section 2.3 of this addendum limits G1. |
| 4 Personas | The operator uses the devx host commands to create secrets, write `sag.yaml`, issue JWTs, approve extensions, set up agent harnesses, and read the audit log. |
| 5 | Add `github.com/aws/aws-sdk-go-v2/config`, `github.com/aws/aws-sdk-go-v2/service/secretsmanager`, and `golang.org/x/term` (hidden terminal read of section 7.3). |
| 6 | Add the log flags of section 13 to the container constraints. "The operator runs CLI commands inside the container with `podman exec`" also allows the host forms of section 1.2. "The daemon reads `sag.yaml` and all secrets at start" applies to `secret:` references; section 11.4 applies to `aws-sm:` references. |
| 6.1 | The macOS development certificate comes from `sag bootstrap` (section 5.7). Its CA has name constraints for `localhost`, `127.0.0.1`, and `::1`. `NODE_EXTRA_CA_CERTS` points to `<cfg>/ca.pem` through section 12.5. |
| 7.1 | On a developer machine, the agent runs on the container host in the account of the operator. Section 2 of this addendum applies. |
| 7.3 | Add T8. |
| 7.4 TB2, TB4 | Add the devx host commands as the operator path. Add the AWS backend as a secret source. |
| 7.6 | R8 applies only to a manual mkcert setup. Add R11 to R25 of section 18. Operator guidance: on a developer machine, `sag agent setup` writes the JWT into the user-level MCP configuration file of the harness. |
| 9.1 | Hint text: `resource <name> has risky objects <list>; to approve, run: sag extension approve <name>`. |
| 9.2 | Command `sag extension approve <resource>`. `sag extension revoke` exits 0 when no approval record exists, after it writes the tombstone. |
| 10.2 | The scrubber adds a credential at the time that `Load` returns it. The scrubber also holds the value of the `storage.aws.credentials` file. |
| 11.1 | `sag bootstrap` creates the master key. |
| 11.2 | Issue command `sag jwt generate <sub>` on the host. The host form exits 3 unless standard input and standard output are terminals, and has no `--json`. The refusal of an unlisted `sub` applies to `sag jwt generate` only. Exception: the internal command `sag agent issue` issues a JWT to a pipe for `sag agent setup` (section 12.4). |
| 12.1 | Credentials come from a SecureStorage backend (section 11). `sag credential create` replaces the manual steps. |
| 13.1 | Add the commands of section 1.1. |
| 13.2 | On a developer machine, `sag agent setup` registers SAG (section 12). A manual registration writes the entry into the MCP configuration file. It never passes the JWT as a command argument. |
| 13.4 | `resources` can be empty. An absent `resources` key equals an empty map. Add `storage.aws.region`, `storage.aws.credentials`, and the `aws-sm:` reference. AC1 keeps its five fixtures. |
| 13.5 | The Quadlet file gets the log keys of section 13. Step 3 text: "The committed file follows the golden input `deploy/dev/sag.yaml` and needs `sag-orders` and `sag-dev-pg-ca`. No devx command creates these secrets." |
| 15.2 | Add a reference to the build order of section 19. |

Code facts that change with the PRD:

- `internal/config/config.go` rejects an empty `resources` map today.
- `internal/adapter/postgres/preflight.go`, `internal/cli/extension.go`, and `internal/cli/jwt_generate.go` hold the text `podman exec -it`.
- `internal/cli/jwt_generate.go` checks only standard output for a terminal today.
- `internal/scrub.Scrubber` is an immutable value from `scrub.New`. Section 11.4 needs an add at `Load` time.
- `go.mod` has no direct `golang.org/x/term` today. `golang.org/x/sys` is only indirect.
- `.github/workflows/ci.yml` grants only `contents: read` today.

## 17. Acceptance criteria

Each criterion has one e2e test, except DX-AC44, which is a Node test. Section 17.1 names the environments.

| ID | Criterion | Environment |
|---|---|---|
| DX-AC1 | `bootstrap --autostart yes`, with no harness binary in `PATH`, exits 0. `GET /healthz` with `<cfg>/ca.pem` returns 200. `systemctl --user list-dependencies default.target` holds `sag.service`. | Ubuntu CI |
| DX-AC2 | For the container from DX-AC1, `podman exec sag sag config check` exits 0. The secret count of `podman container inspect sag` equals the mount count of section 13. `podman inspect` shows log driver `k8s-file` with `max-size` 10mb. The Quadlet output for `deploy/dev/sag.yaml` equals `deploy/sag.container`. `<cfg>/ca.pem` has a critical name constraints extension that permits exactly `localhost`, `127.0.0.1/32`, and `::1/128`. | Ubuntu CI |
| DX-AC3 | A second `bootstrap` without a terminal and without flags exits 0 and writes only `ok` lines. The `CreatedAt` of `sag-master-key` and the start time of the container do not change. A JWT from before the run still authenticates. | Ubuntu CI |
| DX-AC4 | On a host without `<cfg>`, `bootstrap` without a terminal and without flags exits 3 within 30 s. The message names `--autostart`. No secret and no `<cfg>` exist after the run. | Ubuntu CI |
| DX-AC5 | With Podman removed, `bootstrap --auto-approve podman@4.9.3 --autostart no` installs Podman and exits 0. `newuidmap` is in `PATH`. | Ubuntu CI, separate job |
| DX-AC6 | Without a terminal and without a pre-approval, an absent test item gives exit 3 and no install. With `--auto-approve <item>@<other version>`, `bootstrap` writes the `ignored` `warn` line and exits 3. | Ubuntu CI, test pins |
| DX-AC7 | A `release` test item with a wrong SHA-256 gives exit 1. The artifact is absent from the download directory, and the test install command does not run. With the correct SHA-256, the test install command runs one time on the path of the verified file. | Ubuntu CI, test pins |
| DX-AC8 | An installed item newer than the test pin gives one `warn` line and exit 0. An installed item older than the test pin gives exit 1, and no `<cfg>` exists after the run. An `apt` test item whose candidate is older than the test pin gives exit 1 and no install. The installed version does not change. | Ubuntu CI, test pins |
| DX-AC9 | `credential create canary` with a canary DSN typed on a pseudo-terminal exits 0. The DSN host is `host.containers.internal` with the published port of a PostgreSQL container. `sag resource test canary` passes. | Ubuntu CI, PostgreSQL container |
| DX-AC10 | After DX-AC9, a search for the canary password, host, user, and database finds no match. The search covers the command output, the pseudo-terminal echo, `<cfg>`, `answers.json`, and the Quadlet file. It also covers `podman logs sag`, the audit file, `podman container inspect sag`, `podman secret inspect` of each secret, and the user journal. | Ubuntu CI |
| DX-AC11 | `create` exits 1 for a DSN with `?sslmode=disable`, with two hosts, and without a database. It also exits 1 for an invalid name and for a name in use. No new secret exists. No message holds a canary part. | Ubuntu CI |
| DX-AC12 | `create` with a DSN to a closed port exits 1, and no secret exists. With `--accept-failed-check`, it exits 0 with one `warn` line, and the secret exists. | Ubuntu CI |
| DX-AC13 | A DSN with host `127.0.0.1` gives the loopback `warn` line. The line holds no canary part. | Ubuntu CI |
| DX-AC14 | `create --stdin` with a piped DSN exits 0. Input of 64 KiB plus one byte gives exit 1. Input with a NUL byte gives exit 1. A piped DSN without `--stdin` and without a terminal gives exit 3. | Ubuntu CI |
| DX-AC15 | `create` exits 1 when `sag-res-<name>.conn` exists before the call. The value of that secret does not change. | Ubuntu CI |
| DX-AC16 | A test hook makes step 3 of `create` fail after step 1. `create` exits 1. The new secret is absent, `sag.yaml` is unchanged, and `/healthz` returns 200. | Ubuntu CI, test build |
| DX-AC17 | `credential list` and `credential inspect canary`, in text and with `--json`, show the fields of section 8. They contain no canary part. The JSON object has exactly the field set of section 8. | Ubuntu CI |
| DX-AC18 | `credential delete canary` without a terminal exits 3. After the attempt, the secret exists and `sag resource test canary` passes. | Ubuntu CI |
| DX-AC19 | `credential delete canary --yes` exits 2. A wrong typed name on a pseudo-terminal gives exit 3. After each attempt, the secret exists. | Ubuntu CI |
| DX-AC20 | `credential delete canary` with the typed name on a pseudo-terminal exits 0. The secrets are absent, and `sag.yaml` has no `canary`. The container has no mount of the secrets, and the revoke tombstone exists. | Ubuntu CI |
| DX-AC21 | `delete` succeeds for a resource without an approval record, and with the container stopped. | Ubuntu CI |
| DX-AC22 | A test hook makes step 8 of `delete` fail. A second `delete` exits 0 and removes the secrets that remain. | Ubuntu CI, test build |
| DX-AC23 | For a resource whose referenced secret has no `sag.resource` label, `delete` prints the `skip` line. The secret still exists. | Ubuntu CI |
| DX-AC24 | Host `sag jwt generate claude-code-dev` without a terminal exits 3. On a pseudo-terminal with standard output redirected to a file, it exits 3 and the file is empty. With `--json`, it exits 2. On a pseudo-terminal, it prints a JWT that authenticates at `/mcp`. | Ubuntu CI |
| DX-AC25 | `sag logs daemon` prints the line `sag starting`. After one `tools/call`, `sag logs audit` prints one line with its `request_id`. Both commands exit 0 in a process without a controlling terminal. | Ubuntu CI |
| DX-AC26 | With a `settings.json` that holds other keys, `agent setup claude-code --deny-rules yes` adds the three rules and keeps the other keys. A second run adds no duplicate. `--deny-rules no` leaves the rules unchanged. | Ubuntu CI, harness stubs |
| DX-AC27 | `bootstrap --autostart no` writes a Quadlet file without `[Install]`, and `list-dependencies default.target` does not hold `sag.service`. `--autostart yes` writes it with `WantedBy=default.target`. | Ubuntu CI |
| DX-AC28 | After `sag service stop`, the container does not run and `/healthz` fails. After `sag service start`, `/healthz` returns 200. | Ubuntu CI |
| DX-AC29 | With a test certificate validity of 20 days, `credential list` writes the expiry `warn` line. `bootstrap` then replaces `<cfg>/ca.pem`, and `/healthz` returns 200 with the new file. A test hook stops a renewal after step 3 of section 5.7. The next `bootstrap` exits 0, removes `<cfg>/ca.pem.new`, and `/healthz` returns 200 with `<cfg>/ca.pem`. | Ubuntu CI, test build |
| DX-AC30 | A binary with an embedded test digest pulls the image by that digest. A binary without a digest writes the dev build `warn` line. | Ubuntu CI, test build, local registry |
| DX-AC31 | A resource with `connection: aws-sm:<name>` passes `sag resource test`. `create --backend aws-sm`, `list`, and `delete` work on the AWS backend. The tags of each created AWS secret are exactly the three labels of section 7.5. | Ubuntu CI, LocalStack |
| DX-AC32 | Three resources reference `aws-sm:` secrets, one resource references a Podman secret, and the AWS endpoint drops packets. `/healthz` returns 200 within 30 s after `sag service start`. The Podman resource is served, and each AWS resource returns `resource_unavailable`. Within 120 s after the endpoint returns, each AWS resource is served. | Ubuntu CI, LocalStack |
| DX-AC33 | The product binary ignores each `SAG_TEST_*` variable. Its symbol table has no `devxtest` symbol. No non-test source file contains `--showsecret`. | Ubuntu CI, build check |
| DX-AC34 | On macOS with an installed Podman, `bootstrap --autostart yes --agent-setup claude-code=no --agent-setup pi=no` writes an `ok` or `warn` line for Podman and does not change it. `answers.json` holds the absolute `podman_path`. `/healthz` returns 200. `launchctl print gui/<uid>/net.kanthorlabs.sag` shows the agent. After `sag service stop`, `launchctl kickstart -k gui/<uid>/net.kanthorlabs.sag` gives `/healthz` 200 within 30 s. | macOS dev machine |
| DX-AC35 | On macOS, `credential create`, `list`, and `delete` behave as DX-AC9, DX-AC17, and DX-AC20. | macOS dev machine |
| DX-AC36 | Each host command as UID 0 exits 1. `sag agent issue a`, `sag credential check`, and `sag serve` on the host exit 2. A host command exits 1 while another process holds `<cfg>/lock`. | Ubuntu CI |
| DX-AC37 | `create --tls-ca <file>` against a PostgreSQL with a private CA exits 0, and `sag resource test` passes. The CA secret has the label `sag.role=tls_ca`. A test hook makes step 3 fail after step 2, and both new secrets are absent. `delete` removes both secrets. | Ubuntu CI, TLS PostgreSQL container |
| DX-AC38 | A test flag marks `podman` absent, and a test flag stops `bootstrap` before the install command. `bootstrap` downloads the pinned pkg, accepts its SHA-256, exits 0, and prints the path of the verified file. `installer` does not run. With a wrong test SHA-256, `bootstrap` exits 1 and the file is absent. | macOS dev machine, test build |
| DX-AC39 | With `CLAUDE_CONFIG_DIR` set to a test directory whose `.claude.json` and `settings.json` hold other keys, `agent setup claude-code --deny-rules no` exits 0. `.claude.json` has mode 0600, keeps the other keys, and has `mcpServers.sag` with `type` `http`, the endpoint URL, an `Authorization` header, and no `command` key. `settings.json` has `env.NODE_EXTRA_CA_CERTS` equal to `<cfg>/ca.pem`. `skills/sag/SKILL.md` exists. The JWT of the entry authenticates at `/mcp` with `sub` `claude-code`. The stub `claude` received `mcp list`. Without `CLAUDE_CONFIG_DIR`, the entry goes to `~/.claude.json` and the skill to `~/.claude/skills/sag/SKILL.md`. | Ubuntu CI, harness stubs |
| DX-AC40 | After DX-AC39 and DX-AC41, a search for each issued JWT finds no match outside the MCP configuration files. The search covers the output of `agent setup`, the arguments and environment of each child process, `podman logs sag`, the audit file, `<cfg>`, and the user journal. | Ubuntu CI, harness stubs |
| DX-AC41 | With `PI_CODING_AGENT_DIR` set to a test directory and `SHELL=/bin/bash`, `agent setup pi --shell-profile yes --guard yes` exits 0. `mcp.json` has mode 0600 and `mcpServers.sag` with `url` and `headers`, and no `command` key. The skill exists. `extensions/sag-guard.ts` exists with the marker in line 1. `~/.bashrc` holds the `export` line one time. A second run without a terminal and without flags writes only `ok` lines. | Ubuntu CI, harness stubs |
| DX-AC42 | With `NODE_EXTRA_CA_CERTS` set to another file, `agent setup pi` exits 1, prints the bundle command, and changes no file. After the bundle command and `NODE_EXTRA_CA_CERTS` set to the bundle, `agent setup pi` exits 0. The same holds for `env.NODE_EXTRA_CA_CERTS` in the `settings.json` of Claude Code. | Ubuntu CI, harness stubs |
| DX-AC43 | Without a terminal and without flags, `agent setup claude-code` exits 3 and names `--deny-rules`. `agent setup pi` exits 3 and names `--shell-profile`. A `mcpServers.sag` entry with a `command` key gives exit 1. A `sag-guard.ts` without the marker gives exit 1. A stub that reports `sag` as not connected gives exit 1. No file changes on an exit in the plan. | Ubuntu CI, harness stubs |
| DX-AC44 | `node --test internal/agentsetup/assets/pi/` passes. The tests show that the guard blocks a `bash` call that contains `sag credential delete x`, `sh -c "podman secret rm x"`, or `/usr/bin/script`. The guard passes `ls -la` and each call of another tool. | Ubuntu CI, Node 24 |
| DX-AC45 | With no resource that lists `claude-code`, `agent setup claude-code` exits 0. With its JWT, `list_resources` returns an empty list. After `credential create x --agent claude-code`, `list_resources` returns `x`. Host `sag jwt generate claude-code` on a pseudo-terminal still exits 1 for an unlisted `sub`. `--sub Bad` gives exit 2. | Ubuntu CI, harness stubs |
| DX-AC46 | With the stubs `claude` and `pi` in `PATH`, `bootstrap --autostart no --agent-setup claude-code=yes --agent-setup pi=no --deny-rules no` sets up only Claude Code. Without a terminal and without `--agent-setup`, `bootstrap` exits 3 and names `--agent-setup`. | Ubuntu CI, harness stubs |
| DX-AC47 | With Claude Code 2.1.332, after `agent setup claude-code --deny-rules yes`, `claude mcp list` shows `sag` connected. `claude -p` from a shell without `NODE_EXTRA_CA_CERTS` lists the resources. A Bash tool call `sag credential delete x` gets a deny. | macOS dev machine |
| DX-AC48 | With pi 1.6.1, after `agent setup pi --shell-profile yes --guard yes`, `pi mcp list` from a new shell shows `sag` connected. In a pi session, a `bash` tool call `podman secret rm x` gets a block from `sag-guard`. | macOS dev machine |

### 17.1 Test environment

Ubuntu CI:

- The DX jobs run on the `ubuntu-24.04` runner of `.github/workflows/ci.yml`. The tests drive terminals with `github.com/creack/pty`.
- Job setup: `sudo loginctl enable-linger "$USER"`, then `XDG_RUNTIME_DIR=/run/user/$(id -u)`. The systemd user manager then runs for the runner user.
- The DX tests use the real `HOME` of the runner, because the user manager and rootless Podman read it. They run in series, because the container name `sag` and port 8443 are global.
- The section 15 teardown runs between test groups. DX-AC1 to DX-AC4 run first, in this order.
- An agent setup test sets `CLAUDE_CONFIG_DIR` or `PI_CODING_AGENT_DIR` to a test directory, except the default path case of DX-AC39. A test that changes `~/.bashrc` restores the file after the test.
- Harness stubs: CI has no Claude Code and no pi. The test files `claude` and `pi` in a test `PATH` directory record their arguments and environment. They print a `sag` line with the state that the test sets.
- DX-AC44 runs in a CI step with `actions/setup-node` and Node 24.
- The build tag `devxtest` enables `SAG_TEST_PINS`, `SAG_TEST_IMAGE`, `SAG_TEST_CERT_DAYS`, `SAG_TEST_ABSENT`, `SAG_TEST_STOP_BEFORE_INSTALL`, and the failure hooks of DX-AC16, DX-AC22, DX-AC29, and DX-AC37. The product binary has no such path.
- Test pins hold a `release` item that a local HTTP server serves, and the apt package `hello`.
- The test-pin criteria and DX-AC5 run in jobs separate from the other criteria.
- LocalStack runs as a container pinned by digest. The test sets `AWS_ENDPOINT_URL_SECRETS_MANAGER` in the container environment. Build step 7 confirms the LocalStack image and its access rules.
- A unit test checks the argument list of each child process of `create`, `delete`, `bootstrap`, and `agent setup`. No argument holds the canary or a JWT.
- The Ubuntu CI criteria need a push by Ulrich, the same as AC35.

macOS:

- macOS criteria run on the macOS development machine of Ulrich only. No GitHub Actions job runs devx on macOS.
- The machine has Podman 5.5.1 and a Podman machine. DX-AC38 thus stops before `installer`.
- DX-AC34, DX-AC35, DX-AC47, and DX-AC48 change the user state. Section 15 removes it.
- The installer run, `podman machine init`, and `podman machine start` have no e2e test. Section 18.1 covers them.

## 18. Residual risks

- **R11.** T8 reads and deletes each Podman secret with `podman` in the account of the operator. The devx checks stop only an agent that uses `sag`.
- **R12.** T8 bypasses a Claude Code deny rule with `sh -c`, another wrapper, or an absolute path. The deny rules and the pi guard cover no secret read, for example `podman secret inspect --showsecret`, and no other pseudo-terminal tool, for example `expect` or `python`.
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
- **R25.** The pi guard blocks each `bash` command that contains `script`, also a command without risk, for example `npx typescript`.

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
| 2 | `bootstrap` and `service` on Ubuntu, pins, certificate. Confirm that `apt-get install -y podman` installs `uidmap` on Ubuntu 24.04. | DX-AC1 to DX-AC8, DX-AC27 to DX-AC29, DX-AC33, DX-AC36 |
| 3 | `credential create`, `list`, `inspect`, `credential check` | DX-AC9 to DX-AC17, DX-AC37 |
| 4 | `credential delete`, host forms, `logs`, `audit read` | DX-AC18 to DX-AC25 |
| 5 | `agent setup`, `agent issue`, skill, pi guard, bootstrap step 13. Record the output form of `claude mcp list` and `pi mcp list`. | DX-AC26, DX-AC39 to DX-AC46 |
| 6 | Release workflow, image selection | DX-AC30 |
| 7 | AWS Secrets Manager backend | DX-AC31, DX-AC32 |
| 8 | macOS | DX-AC34, DX-AC35, DX-AC38, DX-AC47, DX-AC48, section 18.1 |
| 9 | Removal of the development scripts (section 14) | None |
