# SAG developer experience (devx): PRD addendum

| Field | Value |
|---|---|
| Owner | Ulrich |
| Status | Revision 2 |
| Date | 2026-11-26 |
| Base | `docs/PRD.md` revision 16, tag `sag-layer-6` |

## 1. Scope

devx v1 targets developer machines only:

| Platform | Package source for Podman |
|---|---|
| macOS arm64 | Official Podman installer `podman-installer-macos-arm64.pkg` of the pinned GitHub release |
| Ubuntu with `apt-get` (amd64, arm64) | Ubuntu archive through `apt-get` |

- Each other OS, architecture, or distribution gets exit 1 from `sag bootstrap` with the message `unsupported platform <os>/<arch>`.
- A server install uses PRD 13.5 (`deploy/sag.container`). devx does not change the server path, except the log flags in section 12.
- The endpoint listens on `127.0.0.1:8443` only. A Tailscale address and a Tailscale certificate are out of scope.
- Out of scope: an uninstall command, a self-update, a master key reset, a change of the `agents` list of a resource, a credential replacement command.

### 1.1 Commands

| Command | Runs on | Function |
|---|---|---|
| `sag bootstrap` | Host | Install the pinned dependencies, prepare SAG state, set the start at login, start SAG. |
| `sag service start` | Host | Start SAG. The start at login runs this command. |
| `sag service stop` | Host | Stop SAG. |
| `sag credential create` | Host | Validate and store one credential, and add its resource. |
| `sag credential list` | Host | Show all resources and their credential metadata. |
| `sag credential inspect` | Host | Show one resource and its credential metadata. |
| `sag credential delete` | Host | Remove one resource and its credentials after a typed confirmation. |
| `sag logs daemon` | Host | Show the daemon log of the container. |
| `sag logs audit` | Host | Show the MCP audit log of the container. |
| `sag jwt generate`, `sag extension approve`, `sag extension revoke`, `sag resource test` | Host | Host forms of the PRD 13.1 commands (section 1.2). |
| `sag credential check` | Container, internal | Validate one credential for `create` (section 7.4). |
| `sag audit read` | Container, internal | Print the audit lines of one day for `sag logs audit`. |

- The binary selects the container behavior when `/run/.containerenv` exists. Otherwise it selects the host behavior.
- Each host command refuses UID 0 with exit 1.
- An internal command runs only in the container. On the host it gives exit 2.

### 1.2 Host forms

| Host command | Runs |
|---|---|
| `sag jwt generate <sub> [--name <display>]` | `podman exec -it sag sag jwt generate <sub> [--name <display>]` |
| `sag extension approve <resource>` | `podman exec -it sag sag extension approve <resource>` |
| `sag extension revoke <resource>` | `podman exec sag sag extension revoke <resource>` |
| `sag resource test <name>` | `podman exec sag sag resource test <name>` |

- The host forms of `jwt generate` and `extension approve` refuse to run without a terminal, with exit 3.
- These two host forms have no `--json`. They give their terminal to `podman exec -it`.

### 1.3 Exit codes and output

| Code | Meaning |
|---|---|
| 0 | Success. A `warn` line keeps code 0. |
| 1 | Failure. |
| 2 | Usage error. An unknown flag gives code 2. |
| 3 | Refusal: a required human answer is absent. Examples: no terminal and no flag for a prompt, no approval for an install, a wrong typed confirmation, a refusal by `sudo`. |

- A prompt goes to `/dev/tty`, never to standard output.
- "A terminal" means that the process can open `/dev/tty`.
- Human output of `bootstrap`, `service`, and `credential` commands is one line per step: `<status> <item> <detail>`. The status is `ok`, `warn`, `install`, `skip`, or `fail`.
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
| `<cfg>/answers.json` | Stored bootstrap answers, mode 0600. |
| `<cfg>/lock` | Lock file, mode 0600. |
| Start at login, macOS | LaunchAgent `~/Library/LaunchAgents/net.kanthorlabs.sag.plist` |
| Start at login, Ubuntu | Quadlet file `~/.config/containers/systemd/sag.container` of the user manager |

- `sag` never reads, stores, or passes a password. Only the Podman install calls `sudo`, and `sudo` reads the password itself.
- With no terminal, bootstrap calls `sudo -n`. A refusal by `sudo` gives exit 3.

### 2.2 Actor T8

| ID | Actor | Capability |
|---|---|---|
| T8 | AI agent with a shell in the OS account of the operator, for example Claude Code | Runs each command that the operator can run. Reads and writes each file of the operator. Opens a pseudo-terminal with `script` or `expect`. |

### 2.3 Guarantees and limits

- No devx command prints, logs, or writes a credential value. The value goes from `/dev/tty` or standard input to the backend, and to the container.
- Thus a credential value does not reach the agent context through a `sag` command or through MCP.
- An agent with a shell in the same account can read or delete each Podman secret with `podman` directly. The `sag credential delete` check stops only an agent that uses `sag`.
- `sag credential delete` reads the typed resource name from the controlling terminal. It stops an agent that runs `sag` through a tool without a terminal. It does not stop an agent that opens a pseudo-terminal or runs `podman secret rm`.
- The same limit applies to the AWS backend. T8 reads the Podman secret `sag-aws-credentials` and then reads each AWS secret that this key can read.

### 2.4 Claude Code deny rules

Bootstrap asks one time to add these rules to `permissions.deny` in `~/.claude/settings.json`:

```
Bash(sag credential delete:*)
Bash(podman secret rm:*)
Bash(script:*)
```

- Bootstrap adds only absent rules. It keeps each other key and value of the file. It writes a temporary file and renames it.
- Without a `yes` answer, bootstrap does not change the file.
- When `~/.claude` does not exist, bootstrap writes `skip claude-deny-rules` and asks nothing.
- When the file is not valid JSON, bootstrap writes a `fail` line, does not change the file, and exits 1.
- A deny rule matches a command prefix. An agent that runs the command through `sh -c` bypasses the rule.

## 3. Pins

`deploy/pins` holds one line per item. Spaces separate the fields: `name version min platform source locator sha256`.

| Field | Rule |
|---|---|
| `name` | Item name, for example `podman`. |
| `version` | Exact pin version, or `-` for an item without a pin. |
| `min` | Lowest accepted version, or `-`. |
| `platform` | `darwin/arm64`, `linux/amd64`, `linux/arm64`, or `linux/*`. |
| `source` | `release`, `apt`, or `image`. |
| `locator` | `release`: the HTTPS URL of the official artifact. `apt`: the package name. `image`: the image reference with digest. |
| `sha256` | `release`: the SHA-256 of the artifact. Other sources: `-`. |

Items of devx v1:

| Item | `darwin/arm64` | `linux/*` |
|---|---|---|
| `podman` | `release`, `podman-installer-macos-arm64.pkg` from `github.com/containers/podman/releases`, with SHA-256 | `apt`, package `podman` |
| `uidmap` | Not used | `apt`, package `uidmap`, no pin |
| `podman-machine-os` | `image`, `quay.io/podman/machine-os@sha256:<digest>` | Not used |

- The pin values come from the release pages. The PRD does not hold them.
- The binary embeds `deploy/pins` with `go:embed`.
- A `release` artifact downloads to a new directory with mode 0700. Bootstrap computes its SHA-256 and compares it with the pin. On a difference, bootstrap deletes the file, installs nothing, and exits 1.
- An `apt` item has no SHA-256. `apt-get` verifies the archive signature.
- `podman-machine-os` applies only to a new Podman machine (section 5.2, step 5).

## 4. Release and images

### 4.1 Image

- Assumption: the image name is `docker.io/kanthorlabs/sag`.
- The image holds `linux/amd64` and `linux/arm64` in one manifest list.
- A push to `master` publishes the dev image `docker.io/kanthorlabs/sag:master`.
- A tag `v*` publishes the release image `docker.io/kanthorlabs/sag:<tag>`.
- CI reads the registry credentials from the GitHub secrets `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN`.

### 4.2 Release workflow

On a tag `v*`, the workflow `.github/workflows/release.yml` does these steps in this order:

1. Build and push the release image. Record the digest of the manifest list.
2. Build `sag_darwin_arm64`, `sag_linux_amd64`, and `sag_linux_arm64` with cgo on native runners.
3. Set the version and the digest through `-ldflags`: `-X github.com/kanthorlabs/sag/internal/cli.Version=<tag> -X github.com/kanthorlabs/sag/internal/release.ImageDigest=<digest>`.
4. Write `SHA256SUMS` for the three binaries.
5. Attest each binary with `actions/attest-build-provenance`.
6. Publish the GitHub release with the three binaries and `SHA256SUMS`.

### 4.3 Image selection

| Build | `release.ImageDigest` | Image that `sag` pulls |
|---|---|---|
| Release binary | Set | `docker.io/kanthorlabs/sag@<digest>` |
| Dev build (`make build`) | Empty | `docker.io/kanthorlabs/sag:master`, with the line `warn image dev build; pulls the master tag` |

- `sag version` prints the version and the image reference.
- devx v1 has no self-update. A new release is a new manual install.

### 4.4 Install steps

Run these steps on macOS arm64. On Ubuntu, use `sag_linux_<arch>` and `sha256sum` instead of `shasum -a 256`.

```
gh release download <tag> --repo kanthorlabs/sag --pattern sag_darwin_arm64 --pattern SHA256SUMS
shasum -a 256 --ignore-missing -c SHA256SUMS
gh attestation verify sag_darwin_arm64 --repo kanthorlabs/sag
install -m 0755 sag_darwin_arm64 ~/.local/bin/sag
sag bootstrap
```

- If a check fails, do not install the binary.
- `~/.local/bin` must be in `PATH`.

## 5. `sag bootstrap`

### 5.1 Synopsis

```
sag bootstrap [--auto-approve <name>@<version>]... [--autostart yes|no] [--claude-deny-rules yes|no]
```

### 5.2 Steps

Bootstrap runs these steps in this order. Each step first reads the current state. A step with nothing to do writes an `ok` line.

1. Refuse UID 0, a container, and an unsupported platform.
2. Take the lock `<cfg>/lock`. If another process holds it, exit 1.
3. Select the pins for the OS and the architecture.
4. Plan: collect each needed answer (section 5.4). If an answer is needed and no source gives it, exit 3 before any change.
5. For each dependency, apply the dependency rule (section 5.3). On macOS, when no Podman machine exists, run `podman machine init --image docker://<podman-machine-os locator>`. Start the machine when it is stopped.
6. Pull the image of section 4.3.
7. Create the secret `sag-master-key` when it is absent (section 5.6).
8. Create the endpoint certificate when it is absent, or renew it (section 5.7).
9. Create `<cfg>/sag.yaml` when it is absent: `version`, `server`, `auth`, and an empty `resources` map.
10. Apply the start at login (section 5.5).
11. Start SAG with `sag service start` (section 6). Wait up to 30 s for `GET /healthz` with `<cfg>/ca.pem`.
12. Apply the Claude Code deny rules (section 2.4).
13. Print the endpoint `https://localhost:8443/mcp`, the line `NODE_EXTRA_CA_CERTS=<cfg>/ca.pem`, and the next commands: `sag credential create`, then `sag jwt generate`.

- A second run on a ready host writes only `ok` lines, asks nothing, and exits 0.
- Bootstrap never replaces `sag-master-key`, `sag.yaml`, or an installed dependency.

### 5.3 Dependency rule

| State | Action | Line |
|---|---|---|
| Installed, version equal to the pin | None | `ok podman 5.5.2` |
| Installed, version not equal to the pin, not below `min` | None | `warn podman installed 5.5.1, pin 5.5.2` |
| Installed, version below `min` | None. Exit 1. | `fail podman installed 4.3.1, min 4.9.3` |
| Installed, no pin | None | `ok uidmap` |
| Absent, approved | Install, then read the version again | `install podman 5.5.2` |
| Absent, not approved | Exit 3 | `fail podman absent; run on a terminal or add --auto-approve podman@5.5.2` |

- Bootstrap never changes an installed version. A Podman from Homebrew or from another source counts as installed.
- Bootstrap writes the path of the `podman` binary that it uses.
- macOS install: `sudo installer -pkg <file> -target /`. The installer asks for the admin password one time.
- Ubuntu install: `sudo apt-get install -y <packages>`.

Approval:

- `--auto-approve <name>@<version>` is repeatable. It approves the install of `<name>` only when `<version>` is equal to the pin version.
- For an item without a pin, the form is `<name>@-`.
- A pre-approval with another version approves nothing. Bootstrap writes `warn podman pre-approval 5.4.0 does not match pin 5.5.2; ignored`.
- Without a valid pre-approval and with a terminal, bootstrap asks `Install podman 5.5.2 from <locator>? [y/N]`. Only `y` or `yes` approves.

### 5.4 Answers

| Answer | Flag | Prompt | Needed when |
|---|---|---|---|
| Install of an absent item | `--auto-approve <name>@<version>` | `Install <name> <version> from <locator>? [y/N]` | The item is absent. |
| Start at login | `--autostart yes\|no` | `Start SAG at login? [y/N]` | `answers.json` has no `autostart` value. |
| Claude Code deny rules | `--claude-deny-rules yes\|no` | `Add the deny rules to ~/.claude/settings.json? [y/N]` | `~/.claude` exists, a rule is absent, and `answers.json` has no `claude_deny_rules` value. |

- A flag replaces the stored answer. Bootstrap writes each answer of `--autostart` and `--claude-deny-rules` to `answers.json`.
- Bootstrap asks only when the answer is needed and no flag gives it.
- With no terminal and no flag for a needed answer, bootstrap exits 3. The message names the flag.
- Install approvals are not stored.

### 5.5 Start at login

| OS | `yes` | `no` |
|---|---|---|
| macOS | Write the LaunchAgent with `RunAtLoad` true and `ProgramArguments` `<absolute path of sag> service start`. Load it with `launchctl bootstrap gui/<uid>`. | Unload and remove the LaunchAgent. |
| Ubuntu | The Quadlet file has `[Install] WantedBy=default.target`. | The Quadlet file has no `[Install]` section. |

- The LaunchAgent writes its output to `~/Library/Logs/sag/service.log`.
- Bootstrap rewrites the LaunchAgent when the path of `sag` changes.
- On Ubuntu, bootstrap does not enable linger. The user manager starts SAG at the first login and stops it at the last logout.

### 5.6 Master key

- Bootstrap reads 32 bytes from `crypto/rand`, encodes them as base64, and writes them to the standard input of `podman secret create sag-master-key -`.
- The value never goes to a file, an argument, or an output.

### 5.7 Endpoint certificate

- Bootstrap creates a CA key and a CA certificate with `crypto/x509`, valid for 397 days.
- Bootstrap signs one server certificate for `localhost`, `127.0.0.1`, and `::1`, with the same validity.
- Bootstrap discards the CA key after it signs.
- The server certificate and key become the secrets `sag-tls-cert` and `sag-tls-key`. The CA certificate goes to `<cfg>/ca.pem`.
- When less than 30 days remain on `<cfg>/ca.pem`, bootstrap creates a new CA and a new server certificate and replaces the three items.
- After a renewal, the agent client needs a restart.

## 6. `sag service start` and `sag service stop`

```
sag service start
sag service stop
```

| OS | `start` | `stop` |
|---|---|---|
| macOS | Start the Podman machine when it is stopped. Run the container with `podman run -d --replace --name sag` and the arguments of section 12. | `podman stop sag` |
| Ubuntu | Write the Quadlet file from `sag.yaml` and `answers.json`. Run `systemctl --user daemon-reload`, then `systemctl --user restart sag`. | `systemctl --user stop sag` |

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
- A piped value is accepted only with `--stdin`.
- Use a password manager CLI in a pipe. `echo "$DSN" |` puts the value in the shell history.
- The value never goes to an argument, an environment variable, or a file.

### 7.4 Validation

`create` checks these rules in this order. The first failure gives exit 1 and a fixed message. No secret exists after a failure.

1. `<name>` matches `^[a-z][a-z0-9-]{0,62}$`.
2. `sag.yaml` has no resource `<name>`. Otherwise: `resource <name> exists; delete it first`.
3. No secret with a target name of section 7.5 exists in the selected backend. Otherwise: `secret <secret name> exists; run sag credential delete <name>`.
4. `--kind` is a creatable kind. Each `--agent` matches `^[a-z][a-z0-9-]{0,62}$`, and at least one `--agent` is present.
5. `--description` has at most 500 characters. `--tls-ca` holds at least one PEM certificate.
6. `--backend aws-sm` needs `storage.aws` in `sag.yaml`.
7. The container `sag` runs. Otherwise: `SAG is not running; run sag service start`.
8. Read the value (section 7.3).
9. `ValidateCredential` passes. A failure gives exit 1 also with `--accept-failed-check`.
10. When `LoopbackHost` is set, write `warn dsn host is a loopback address; in the container it is the container; for a database on this machine use host.containers.internal`.
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

Each secret gets the labels `sag.resource=<name>`, `sag.kind=<kind>`, and `sag.role=connection` or `sag.role=tls_ca`.

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
| `secrets[].created_at`, `secrets[].updated_at` | `Metadata` of the backend, or `null` |
| `secrets[].mounted` | Podman backend only: the secret list of `podman container inspect sag` |
| `container_running` (inspect only) | `podman container inspect sag` |

- List and inspect never print a secret value, a part of a value, the value length, or a hash of the value.
- List and inspect never call `Load`.
- No source file outside tests contains the string `--showsecret`.
- When less than 30 days remain on `<cfg>/ca.pem`, list writes `warn endpoint certificate expires <date>; run sag bootstrap` to standard error.
- An unreachable backend gives a `warn` line on standard error and exit 0.
- `--json` of list: `{"credentials": [<object>]}`. `--json` of inspect: one object. The object has exactly the fields of the table.
- Inspect of an unknown name gives exit 1 with `resource <name> not found`.

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
2. Collect the `sag.yaml` entry and the owned secrets in each configured backend. If neither exists, exit 1 with `resource <name> not found`.
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
	UpdatedAt time.Time
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
- The daemon reads each Podman secret at start, as PRD 6 states. It reads each `aws-sm:` secret when it opens the resource.
- If SAG cannot reach AWS, each resource whose credential lives only in AWS is unavailable. Every other resource is served.
- The open retry of PRD 9.1 (every 60 s) brings the resource back when AWS is reachable again.
- A request to such a resource returns `resource_unavailable`.
- The scrubber adds each credential at the time that `Load` returns it.

## 12. Container flags

- `ContainerFlags` and `ProductFlags` move from `e2e/harness` to the product package `internal/deploy`. `harness.ContainerFlags` refers to `deploy.ContainerFlags`.
- `ContainerFlags` gets the log flags `--log-driver k8s-file --log-opt max-size=10mb`. They leave `TestOnlyFlags`.
- `internal/deploy` builds the `podman run` arguments and the Quadlet file from `ContainerFlags` and `sag.yaml`. The secret mounts come from `Config.SecretNames()` and `sag-master-key`.
- In the Quadlet file, the log flags are `LogDriver=k8s-file` and `PodmanArgs=--log-opt=max-size=10mb`.
- The e2e harness, `sag service start`, and the Quadlet output use `internal/deploy`.
- `deploy/sag.container` is a golden file. A unit test compares it with the Quadlet output for `deploy/dev/sag.yaml`.
- The devx Quadlet file mounts `<cfg>/sag.yaml` and uses the image of section 4.3 with `Pull=never`.

## 13. Removal of the development scripts

devx removes these items:

- `scripts/dev-up.sh` and `scripts/dev-down.sh`.
- The `dev-up` and `dev-down` targets of `Makefile`.

`docs/plan/dev-setup.md` changes to the steps of section 4.4 and a `sag credential create`.

| `scripts/dev-up.sh` function | devx replacement |
|---|---|
| mkcert endpoint certificate | Section 5.7 |
| Master key from `openssl rand` | Section 5.6 |
| Image build `localhost/sag-dev:latest` | Image pull, section 4.3 |
| Container run with the PRD 6 flags | `sag service start`, section 12 |
| Wait for `/healthz` | `sag bootstrap` step 11 |
| PostgreSQL fixture `sag-dev-pg` and its secrets | None. A developer registers a database with `sag credential create`. The e2e harness keeps its own fixtures. |

## 14. Teardown

devx has no uninstall command. Do these steps in this order to remove the devx state.

1. Run `sag credential delete <name>` for each resource in `sag credential list`.
2. Run `sag service stop`.
3. On macOS, run `launchctl bootout gui/$(id -u)/net.kanthorlabs.sag`. Then remove `~/Library/LaunchAgents/net.kanthorlabs.sag.plist`.
4. On Ubuntu, remove `~/.config/containers/systemd/sag.container`. Then run `systemctl --user daemon-reload`.
5. Run `podman rm sag` and `podman volume rm sag-state`. This step deletes the audit log and the approvals.
6. Run `podman secret rm sag-master-key sag-tls-cert sag-tls-key`. If `sag-aws-credentials` exists, remove it.
7. Run `podman rmi` for each `docker.io/kanthorlabs/sag` image.
8. Remove `<cfg>`.
9. Remove the deny rules of section 2.4 from `~/.claude/settings.json` by hand.
10. To remove the state of the old scripts, run `scripts/dev-down.sh` from a commit before the removal.

Podman and the Podman machine stay installed.

## 15. Changes to PRD.md

| PRD section | Change |
|---|---|
| 2 G1 | Credentials stay inside the SAG container or in a SecureStorage backend. On a developer machine, section 2.3 of this addendum limits G1. |
| 4 Personas | The operator uses the devx host commands to create secrets, write `sag.yaml`, issue JWTs, approve extensions, and read the audit log. |
| 5 | Add `github.com/aws/aws-sdk-go-v2/config` and `github.com/aws/aws-sdk-go-v2/service/secretsmanager`. |
| 6 | Add the log flags of section 12 to the container constraints. "The operator runs CLI commands inside the container with `podman exec`" also allows the host forms of section 1.2. |
| 6.1 | The macOS development certificate comes from `sag bootstrap` (section 5.7). `NODE_EXTRA_CA_CERTS` points to `<cfg>/ca.pem`. |
| 7.1 | On a developer machine, the agent runs on the container host in the account of the operator. Section 2 of this addendum applies. |
| 7.3 | Add T8. |
| 7.4 TB2, TB4 | Add the devx host commands as the operator path. Add the AWS backend as a secret source. |
| 7.6 | R8 applies only to a manual mkcert setup. Add R11 to R19 of section 17. |
| 9.1 | Hint text: `resource <name> has risky objects <list>; to approve, run: sag extension approve <name>`. |
| 9.2 | Command `sag extension approve <resource>`. `sag extension revoke` exits 0 when no approval record exists, after it writes the tombstone. |
| 10.2 | The scrubber adds a credential at the time that `Load` returns it. |
| 11.1 | `sag bootstrap` creates the master key. |
| 11.2 | Issue command `sag jwt generate <sub>` on the host. The host form refuses without a terminal and has no `--json`. |
| 12.1 | Credentials come from a SecureStorage backend (section 11). `sag credential create` replaces the manual steps. |
| 13.1 | Add the commands of section 1.1. |
| 13.4 | `resources` can be empty. An absent `resources` key equals an empty map. Add `storage.aws.region`, `storage.aws.credentials`, and the `aws-sm:` reference. AC1 keeps its five fixtures. |
| 13.5 | The Quadlet file gets the log keys of section 12. |
| 15.2 | Add a reference to the build order of section 18. |

Code facts that change with the PRD:

- `internal/config/config.go` rejects an empty `resources` map today.
- `internal/adapter/postgres/preflight.go`, `internal/cli/extension.go`, and `internal/cli/jwt_generate.go` hold the text `podman exec -it`.

## 16. Acceptance criteria

Each criterion has one e2e test. Section 16.1 names the environments.

| ID | Criterion | Environment |
|---|---|---|
| DX-AC1 | `bootstrap --autostart yes --claude-deny-rules no` exits 0. `GET /healthz` with `<cfg>/ca.pem` returns 200. `systemctl --user is-enabled sag` prints `generated` or `enabled`. | Ubuntu CI |
| DX-AC2 | The container from DX-AC1 passes the AC32 and AC33 checks. `podman inspect` shows log driver `k8s-file` with `max-size` 10mb. The Quadlet output for `deploy/dev/sag.yaml` equals `deploy/sag.container`. | Ubuntu CI |
| DX-AC3 | A second `bootstrap` without a terminal and without flags exits 0 and writes only `ok` lines. The `CreatedAt` of `sag-master-key` does not change. A JWT from before the run still authenticates. | Ubuntu CI |
| DX-AC4 | On a host without `<cfg>`, `bootstrap` without a terminal and without flags exits 3 within 30 s. The message names `--autostart`. No secret and no `<cfg>` exist after the run. | Ubuntu CI |
| DX-AC5 | With Podman removed, `bootstrap --auto-approve podman@<pin> --auto-approve uidmap@- --autostart no` installs Podman and exits 0. | Ubuntu CI, separate job |
| DX-AC6 | Without a terminal and without a pre-approval, an absent test item gives exit 3 and no install. With `--auto-approve <item>@<other version>`, `bootstrap` writes the `ignored` `warn` line and exits 3. | Ubuntu CI, test pins |
| DX-AC7 | A `release` test item with a wrong SHA-256 gives exit 1. The artifact is absent from the download directory, and nothing is installed. | Ubuntu CI, test pins |
| DX-AC8 | An installed item with a version not equal to the test pin gives one `warn` line and exit 0. A test pin with a `min` above the installed version gives exit 1. The installed version does not change. | Ubuntu CI, test pins |
| DX-AC9 | `credential create canary` with a canary DSN typed on a pseudo-terminal exits 0. `sag resource test canary` passes. | Ubuntu CI, PostgreSQL container |
| DX-AC10 | After DX-AC9, a search for the canary password, host, user, and database finds no match. The search covers the command output, the pseudo-terminal echo, `<cfg>`, and the Quadlet file. | Ubuntu CI |
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
| DX-AC24 | Host `sag jwt generate claude-code-dev` without a terminal exits 3. With `--json`, it exits 2. On a pseudo-terminal, it prints a JWT that authenticates at `/mcp`. | Ubuntu CI |
| DX-AC25 | `sag logs daemon` prints the line `sag starting`. After one `tools/call`, `sag logs audit` prints one line with its `request_id`. Neither command opens `/dev/tty`. | Ubuntu CI |
| DX-AC26 | With a `settings.json` that holds other keys, `--claude-deny-rules yes` adds the three rules and keeps the other keys. A second run adds no duplicate. `--claude-deny-rules no` leaves the file unchanged. | Ubuntu CI |
| DX-AC27 | `bootstrap --autostart no` writes a Quadlet file without `[Install]`. `--autostart yes` writes it with `WantedBy=default.target`. | Ubuntu CI |
| DX-AC28 | After `sag service stop`, the container does not run and `/healthz` fails. After `sag service start`, `/healthz` returns 200. | Ubuntu CI |
| DX-AC29 | With a test certificate validity of 20 days, `credential list` writes the expiry `warn` line. `bootstrap` then replaces `<cfg>/ca.pem`, and `/healthz` returns 200 with the new file. | Ubuntu CI, test build |
| DX-AC30 | A binary with an embedded test digest pulls the image by that digest. A binary without a digest writes the dev build `warn` line. | Ubuntu CI, test build, local registry |
| DX-AC31 | A resource with `connection: aws-sm:<name>` passes `sag resource test`. `create --backend aws-sm`, `list`, and `delete` work on the AWS backend. | Ubuntu CI, LocalStack |
| DX-AC32 | With the AWS endpoint unreachable at start, the daemon starts. The Podman resource is served, and the AWS resource returns `resource_unavailable`. Within 120 s after the endpoint returns, the AWS resource is served. | Ubuntu CI, LocalStack |
| DX-AC33 | The product binary ignores each `SAG_TEST_*` variable. Its symbol table has no `devxtest` symbol. No non-test source file contains `--showsecret`. | Ubuntu CI, build check |
| DX-AC34 | On macOS with an installed Podman, `bootstrap --autostart yes --claude-deny-rules no` writes an `ok` or `warn` line for Podman and does not change it. `/healthz` returns 200. `launchctl print gui/<uid>/net.kanthorlabs.sag` shows the agent. | macOS, local only |
| DX-AC35 | On macOS, `credential create`, `list`, and `delete` behave as DX-AC9, DX-AC17, and DX-AC20. | macOS, local only |

### 16.1 Test environment

- Ubuntu CI: the `ubuntu-24.04` runner of `.github/workflows/ci.yml`. The tests drive terminals with `github.com/creack/pty`.
- Each test sets its own `HOME` and `XDG_CONFIG_HOME`. DX-AC1 to DX-AC4 run first, in this order.
- The build tag `devxtest` enables `SAG_TEST_PINS`, `SAG_TEST_IMAGE`, `SAG_TEST_CERT_DAYS`, and the failure hooks of DX-AC16 and DX-AC22. The product binary has no such path.
- Test pins hold a `release` item that a local HTTP server serves, and the apt package `hello`.
- The test-pin criteria and DX-AC5 run in jobs separate from the other criteria.
- LocalStack runs as a container pinned by digest. The test sets `AWS_ENDPOINT_URL_SECRETS_MANAGER` in the container environment.
- A unit test checks the argument list of each child process of `create`, `delete`, and `bootstrap`. No argument holds the canary.
- macOS: local only, on the machine of Ulrich. DX-AC34 and DX-AC35 change the user state. Section 14 removes it.
- The Ubuntu CI criteria need a push by Ulrich, the same as AC35.

## 17. Residual risks

- **R11.** T8 reads and deletes each Podman secret with `podman` in the account of the operator. The devx checks stop only an agent that uses `sag`.
- **R12.** T8 bypasses a Claude Code deny rule with `sh -c` or another wrapper.
- **R13.** T8 reads `sag-aws-credentials` and then reads each AWS secret that this key can read.
- **R14.** T8 can replace `~/.local/bin/sag` after the install. The attestation proves the binary only at install time.
- **R15.** `credential create` needs no confirmation. T8 can register resources and restart SAG.
- **R16.** T8 can replace the Podman installer between the hash check and the install.
- **R17.** A dev build pulls the mutable `master` tag. Only a release binary pins the image.
- **R18.** On Ubuntu, SAG stops at the last logout of the operator.
- **R19.** The endpoint certificate expires 397 days after a bootstrap run. Only `credential list` and `bootstrap` warn.

## 18. Build order

Do not start a step before each criterion of the previous step passes.

| Step | Content | Criteria |
|---|---|---|
| 1 | `internal/deploy`, log flags, empty `resources`, Podman SecureStorage backend | AC32 and AC33 of PRD 15.2 pass with `internal/deploy`. |
| 2 | `bootstrap` and `service` on Ubuntu, pins, certificate, deny rules | DX-AC1 to DX-AC8, DX-AC26 to DX-AC29, DX-AC33 |
| 3 | `credential create`, `list`, `inspect`, `credential check` | DX-AC9 to DX-AC17 |
| 4 | `credential delete`, host forms, `logs`, `audit read` | DX-AC18 to DX-AC25 |
| 5 | Release workflow, image selection | DX-AC30 |
| 6 | AWS Secrets Manager backend | DX-AC31, DX-AC32 |
| 7 | macOS | DX-AC34, DX-AC35 |
| 8 | Removal of the development scripts (section 13) | None |
