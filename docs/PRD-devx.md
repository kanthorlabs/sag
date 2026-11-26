# SAG developer and operator experience (devx): PRD addendum

| Field | Value |
|---|---|
| Owner | Ulrich |
| Status | Draft 1 |
| Date | 2026-11-26 |
| Base | `docs/PRD.md` revision 16, tag `sag-layer-6` |
| Open questions | Section 14. A requirement that depends on a question names it. |

## 1. Scope

This addendum adds five host commands:

| Command | Function |
|---|---|
| `sag bootstrap` | Install the pinned dependencies, set up the service account, start SAG, and set the start at boot. |
| `sag credential create` | Add one resource with its credential. |
| `sag credential list` | Show all resources and their credential metadata. |
| `sag credential inspect` | Show one resource and its credential metadata. |
| `sag credential delete` | Remove one resource and its credential, after a human authentication. |

- The commands run on macOS (arm64, amd64) and on Linux (amd64, arm64) with `apt-get` or `dnf`.
- The commands are the same on a development machine and on a Linux server.
- The commands replace `scripts/dev-up.sh`, `scripts/dev-down.sh`, the `make dev-up` and `make dev-down` targets, and `docs/plan/dev-setup.md`. They also replace the manual steps of PRD 11.1, 12.1, and 13.5.
- Out of scope: a Tailscale publish address, a Tailscale certificate (PRD 6.1), a change of the `agents` list of a resource, a master key reset, an uninstall command.

## 2. Threat model additions

### 2.1 New actor

| ID | Actor | Capability |
|---|---|---|
| T8 | AI agent with a shell in the OS account of the operator, for example Claude Code | Runs any command as the operator. Reads and writes each file that the operator can read and write. Allocates a pseudo-terminal with `script` or `expect` and writes keystrokes to it. Edits shell startup files, `PATH`, the git working copy, and the Homebrew directories. Has no knowledge of the operator password. Cannot touch the fingerprint sensor. |

T8 is a threat actor for credential exfiltration (A1) and for destructive actions on SAG state.

### 2.2 Consequence

In the OS account that owns the rootless Podman store, T8 runs `podman secret inspect --showsecret`, `podman secret rm`, `podman cp`, and `podman exec`, and edits `sag.yaml`. No check inside `sag` stops a direct Podman command. Thus a check in `sag` protects a credential only when the Podman store belongs to an OS account that T8 cannot use.

**Decision: SAG runs in a dedicated service account. Each operation on the service account goes through `sudo` and a fixed command list (Q1).**

### 2.3 Changes to PRD section 7

- 7.1: The assumption "the agent has no access to the container host" becomes "the agent has no access to the service account, to root, or to the operator password".
- 7.3: Add T8.
- 7.6 R8: The bootstrap path uses no mkcert root CA (section 6.6). R8 applies only to a manual mkcert setup.
- 7.6: Add R11 to R16 (section 13).

## 3. Service account

| Item | macOS | Linux |
|---|---|---|
| Account | `_sag`, hidden role account, shell `/usr/bin/false` | `sag`, system account, shell `/usr/sbin/nologin` |
| Home | `/private/var/sag`, mode 0700 | `/var/lib/sag`, mode 0700 |
| Podman | Podman machine of `_sag` | Rootless Podman of `sag`, with linger |
| `sag.yaml` | `/private/var/sag/sag.yaml`, mode 0644 | `/etc/sag/sag.yaml`, mode 0644, directory owner `sag` |
| Unit | LaunchDaemon `/Library/LaunchDaemons/net.kanthorlabs.sag.plist`, `UserName` `_sag` | Quadlet `/var/lib/sag/.config/containers/systemd/sag.container` |
| Admin log | `/private/var/sag/admin.jsonl`, mode 0600 | `/var/lib/sag/admin.jsonl`, mode 0600 |
| Operator record | `<home>/operator`, the operator user name, mode 0600 | Same |

Rules:

- Root owns `/opt/sag` and each file in it. No directory on the path is writable by group or other.
- `/opt/sag/bin/sag` is the host binary. `/opt/sag/share/ca.pem` is the local CA certificate, mode 0644.
- The service account runs only binaries that root owns, in directories that only root can write. The `podman` binary and the Podman helper directory follow this rule. On macOS, bootstrap sets `helper_binaries_dir` in the `containers.conf` of `_sag` to the directory of the Podman package. A Homebrew Podman does not satisfy the rule.
- The service account has no password and no login.

## 4. Command placement

### 4.1 Front process and back process

The host binary `/opt/sag/bin/sag` has two roles. The effective UID selects the role.

| Role | Runs as | Function |
|---|---|---|
| Front | Operator | Parses arguments. Reads prompts and secret values from `/dev/tty`. Starts the back process with `sudo -u <service account> /opt/sag/bin/sag <command>`. Prints the result. |
| Back | Service account | Validates the input again. Runs Podman. Edits `sag.yaml` and the unit. Writes the admin log. |

- The back process repeats each validation of the front. The back process trusts no front check, because T8 can start the back process directly through `sudo`.
- The back process refuses to run when `SUDO_USER` differs from the operator record.
- Each command refuses to run with UID 0 and inside a container (`/run/.containerenv` exists).
- The container commands of PRD 13.1 stay in the container. On the host, `sag jwt generate`, `sag extension approve`, `sag extension revoke`, and `sag resource test` run the back process, which runs `podman exec` with the same arguments. `podman exec -it` applies when the command needs a terminal.
- `sag version` and `sag config check` keep their PRD 13.1 behavior.

### 4.2 sudoers drop-in

Bootstrap installs `/etc/sudoers.d/sag`, mode 0440. Bootstrap validates the file with `visudo -c -f` before the install. `<op>` is the operator, `<svc>` is the service account:

```
Defaults><svc> timestamp_timeout=0
<op> ALL=(<svc>) NOPASSWD: /opt/sag/bin/sag credential list *, /opt/sag/bin/sag credential inspect *, /opt/sag/bin/sag credential create *, /opt/sag/bin/sag service start, /opt/sag/bin/sag resource test *, /opt/sag/bin/sag extension revoke *
<op> ALL=(<svc>) /opt/sag/bin/sag credential delete *, /opt/sag/bin/sag jwt generate *, /opt/sag/bin/sag extension approve *, /opt/sag/bin/sag service build
```

- The first rule set needs no authentication. Its commands cannot show a secret value and cannot remove a credential.
- The second rule set needs an authentication at each call. Section 9 names this check the human-presence check.
- `credential create` has no flag that replaces a credential. Thus the pattern `credential create *` cannot reach a destructive action.
- `Defaults><svc>` applies to each command that runs as the service account. DX-AC13 proves that sudo asks again for a second call within one minute.
- The front always passes `--json` to the back process and renders the human form itself. Thus each rule with `*` has at least one argument to match.
- The drop-in is the last sudoers file that matches. A rule of the drop-in thus overrides an earlier `NOPASSWD` rule for the same command.
- Q4 can move `credential create` to the second rule set.

### 4.3 First install of the host binary

The operator has a clone of the repository. The first command is `./scripts/bootstrap.sh [flags]`.

The script does exactly these steps:

1. Check the C toolchain. On macOS, the Xcode Command Line Tools (`xcode-select -p`). On Linux, `cc`, `make`, and `git`.
2. Check Go at the version of the `go` pin.
3. For an absent item, apply the approval rule of section 6.3, then install it. Go installs to `$HOME/.local/share/sag/go<version>` without `sudo`.
4. Run `make build`.
5. Run `exec bin/sag bootstrap "$@"`.

- The script and the binary read the same pin file (section 5).
- After the first run, the operator runs `/opt/sag/bin/sag bootstrap`. The front copies a newer `bin/sag` to `/opt/sag/bin/sag` (section 6.2, step 5).
- Q2 decides the later distribution of the host binary.

### 4.4 Exit codes and output

| Code | Meaning |
|---|---|
| 0 | Success. Bootstrap warnings keep code 0. |
| 1 | Failure. |
| 2 | Usage error. |
| 3 | Refusal: a required human action did not occur. Examples: no approval for an install, no terminal for a prompt, a failed sudo authentication, a wrong typed confirmation. |

- The back process uses code 4 for a failure. The front maps code 4 to 1, and maps a sudo refusal (code 1 from `sudo`) to 3.
- Each command accepts `--json`. `bootstrap --json` writes one JSON object per step on standard output. The other commands write one JSON document on standard output.
- Prompts go to `/dev/tty`, never to standard output.
- Human output of bootstrap is one line per step: `<status> <item> <detail>`. The status is `ok`, `warn`, `install`, `skip`, or `fail`.

## 5. Pins

`deploy/pins` holds one line per item, with fields separated by spaces: `name version platform source locator sha256`.

| Field | Rule |
|---|---|
| `name` | `go`, `podman`, `cc`, `make`, `git`, `xcode-clt`. |
| `version` | Exact version, or `-` for an item without a pin. |
| `platform` | `darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64`, or `<os>/*`. |
| `source` | `release`, `distro`, or `vendor`. |
| `locator` | `release`: the HTTPS URL of the official artifact. `distro`: the package names, for example `apt:podman,uidmap dnf:podman`. `vendor`: `-`. |
| `sha256` | `release`: the SHA-256 of the artifact. Other sources: `-`. |

Source rules:

- `release`: bootstrap downloads the artifact to a new temporary directory, computes the SHA-256, and compares it with the pin. On a difference, bootstrap deletes the file, installs nothing, and exits 1. The pin hash is the only trust anchor.
- `distro`: the package manager installs its own version. After the install, bootstrap compares the version with the pin. On a difference, bootstrap writes a `warn` line and continues.
- `vendor`: the Apple installer (`xcode-select --install`) installs the item. Bootstrap writes the version and has no pin.
- The binary embeds `deploy/pins` with `go:embed`. A test-only build tag reads another pin file from `SAG_TEST_PINS` (section 12). The product binary has no such path.

Initial pin set:

| Item | macOS | Linux |
|---|---|---|
| C toolchain | `xcode-clt`, vendor | `cc`, `make`, `git`, distro |
| Go | `go` 1.27.4, release, `go.dev/dl` tarball | Same |
| Podman | `podman` 5.5.2, release, `podman-installer-macos-universal.pkg` from the Podman GitHub release | `podman` 5.5.2, distro, `apt:podman,uidmap,passt dnf:podman,passt` |
| Podman machine image | The default image of the pinned Podman version | Not used |

- Podman 5.5.2 is the version that the layer-6 review tested.
- Bootstrap does not install mkcert, OpenSSL, jq, or curl.

## 6. `sag bootstrap`

### 6.1 Synopsis

```
sag bootstrap [--auto-approve <name>@<version>]... [--autostart yes|no] [--json]
```

### 6.2 Steps

Bootstrap runs these steps in this order. Each step first checks the current state. A step with nothing to do writes an `ok` line.

1. Refuse UID 0, a container, and an operator name equal to the service account.
2. Select the pins for the OS and the architecture.
3. For each dependency, apply section 6.3.
4. Create the service account (section 3). On Linux, add a subordinate UID and GID range of 65536 IDs, and run `loginctl enable-linger sag`.
5. Install `bin/sag` to `/opt/sag/bin/sag` when the content differs. Bootstrap prints the commit of `bin/sag`.
6. Install the sudoers drop-in (section 4.2).
7. Run the presence probe: `sudo -k -n -u <svc> /usr/bin/true` and `sudo -k -n /usr/bin/true`. The drop-in does not allow `/usr/bin/true`. If a probe exits 0, another sudoers rule lets the operator run any command as the service account or as root without a password. Bootstrap then exits 3 with the message `sudo runs commands as <svc> or root without a password; remove the NOPASSWD grant for <op>`.
8. macOS: as `_sag`, run `podman machine init` if no machine exists, then `podman machine start`.
9. Build the image `localhost/sag:latest` in the service account with `sag service build` (Q10). The front sends `git archive HEAD` on standard input. Bootstrap refuses a working copy with uncommitted changes. Bootstrap skips the build when the image label `org.opencontainers.image.revision` equals `HEAD`.
10. Create the secret `sag-master-key` when it is absent (section 6.5).
11. Create or renew the endpoint certificate (section 6.6).
12. Create `sag.yaml` when it is absent: `version`, `server`, `auth`, and an empty `resources` map (Q6).
13. Apply the start at boot (section 6.7).
14. Start or restart the container, then wait up to 30 s for `GET /healthz` with `/opt/sag/share/ca.pem`.
15. Print the endpoint `https://localhost:8443/mcp` and the line `NODE_EXTRA_CA_CERTS=/opt/sag/share/ca.pem`.
16. Run `sudo -k`.

Idempotency: a second run on a ready host writes only `ok` lines, asks for no authentication, and exits 0. The exception is step 9 after a new commit.

### 6.3 Dependency rule

| State | Action | Line |
|---|---|---|
| Installed, version equal to the pin | None | `ok podman 5.5.2` |
| Installed, version different from the pin | None. Bootstrap never changes an installed version. | `warn podman installed 5.4.0, pin 5.5.2` |
| Installed, no pin | None | `ok cc <version>` |
| Absent, approved | Install, then check the version again | `install podman 5.5.2` |
| Absent, not approved | Exit 3 | `fail podman absent; run again on a terminal or with --auto-approve podman@5.5.2` |

Approval:

- `--auto-approve <name>@<version>` is repeatable. It approves the install of the item `<name>` only when `<version>` is equal to the pin version.
- A pre-approval with another version approves nothing. Bootstrap writes `warn podman pre-approval 5.4.0 does not match pin 5.5.2; ignored`.
- For an item without a pin, the form is `<name>@-`.
- Without a valid pre-approval and with a terminal, bootstrap asks on `/dev/tty`: `Install podman 5.5.2 from <locator>? [y/N]`. Only `y` or `yes` approves.
- Without a valid pre-approval and without a terminal, bootstrap never installs and exits 3.
- A pre-approval does not replace a sudo authentication.

### 6.4 Privileges

- Bootstrap calls `sudo` for each step that needs root: an install, the service account, `/opt/sag`, the sudoers drop-in, the LaunchDaemon, the subordinate IDs, linger.
- Bootstrap calls `sudo -u <svc>` for each step in the service account.
- Bootstrap never reads, stores, or passes a password. `sudo` reads the password from the terminal.
- Step 16 removes the sudo timestamp that bootstrap created.

### 6.5 Master key

- The back process reads 32 bytes from `crypto/rand`, encodes them as base64, and writes them to the standard input of `podman secret create sag-master-key -`.
- The value never goes to a file, an argument, or an output.
- Bootstrap never replaces a `sag-master-key` that exists. Devx has no master key reset.

### 6.6 Endpoint certificate

- The back process creates a CA key and a CA certificate with `crypto/x509`, valid for 397 days.
- The back process signs one server certificate for `localhost`, `127.0.0.1`, and `::1`, valid for 397 days.
- The back process discards the CA key after it signs. No process can sign another certificate with that CA.
- The server certificate and key become the secrets `sag-tls-cert` and `sag-tls-key`. The CA certificate goes to `/opt/sag/share/ca.pem`.
- Bootstrap renews the CA and the server certificate when less than 30 days remain. A renewal is not destructive. The agent client needs a restart after a renewal.

### 6.7 Start at boot

- `--autostart yes|no` sets the state. Without the flag and with a terminal, bootstrap asks `Start SAG at boot? [y/N]`. Without the flag and without a terminal, bootstrap keeps the current state and writes `skip autostart`.
- macOS `yes`: install the LaunchDaemon with `UserName` `_sag`, `RunAtLoad` true, and the program `/opt/sag/bin/sag service start`. `service start` starts the Podman machine and the container.
- Linux `yes`: the Quadlet file has `[Install] WantedBy=default.target`. Linger starts the user manager of `sag` at boot.
- `no`: macOS removes the LaunchDaemon. Linux writes the Quadlet file without `[Install]`.
- The start is at host boot, before any login (Q7).

### 6.8 One source for the container flags (layer-6 S14)

- `ContainerFlags` moves from `e2e/harness` to the product package `internal/deploy`.
- `internal/deploy` builds the `podman run` arguments and the Quadlet file from `ContainerFlags` and from `sag.yaml`. The secret mounts come from `Config.SecretNames()` and `sag-master-key`.
- The e2e harness, the macOS `service start`, and the Linux Quadlet file use `internal/deploy`.
- `deploy/sag.container` becomes a golden file. A unit test compares it with the Quadlet output for `deploy/dev/sag.yaml`.
- `scripts/dev-up.sh` and `scripts/dev-down.sh` are removed (Q9).

## 7. `sag credential create`

### 7.1 Synopsis

```
sag credential create <name> --kind <kind> --agent <sub> [--agent <sub>]... [--description <text>]
    [--tls verify-full|insecure-skip-verify|disable] [--tls-ca <pem-file>] [--stdin] [--json]
```

### 7.2 Kind registry

- The kind registry is the adapter registry of `internal/core`.
- A kind is creatable when its adapter implements `core.CredentialKind`:

```go
type CredentialKind interface {
	CredentialPrompt() string
	ValidateCredential(cred secret.Credential) error
}
```

- `ValidateCredential` returns errors with fixed text only. The text never contains a part of the value.
- v1 kind: `postgres`. Its validator is the `parseDSN` function of the adapter, so `create` accepts exactly the values that the daemon accepts. A later `cloudwatch_logs` adapter adds its own prompt and validator.

### 7.3 Secret value input

- Without `--stdin`, the front opens `/dev/tty`, writes the prompt `postgres DSN for <name> (input hidden): `, and reads one line with echo off (`golang.org/x/term`). No terminal gives exit 3.
- With `--stdin`, the front reads the value from standard input up to EOF and removes one final newline (Q5).
- The value never appears in an argument, an environment variable, or a file.
- The front sends one JSON document to the standard input of `sudo`: `{"value": <secret>, "tls_ca": <PEM text or null>}`. `sudo` reads its password from `/dev/tty`, not from this pipe.

### 7.4 Validation

The front and the back process each check these rules. The first failure gives exit 1 and a fixed message.

- `<name>` matches `^[a-z][a-z0-9-]{0,62}$`, and no resource has this name. A name in use gives `resource <name> exists; delete it first`.
- `--kind` is a creatable kind.
- Each `--agent` matches `^[a-z][a-z0-9-]{0,62}$`. At least one `--agent` is present.
- `--description` has at most 500 characters.
- `--tls-ca` holds at least one PEM certificate.
- `ValidateCredential` passes. For `postgres`: scheme `postgres` or `postgresql`, one host, a user, a database, no query, no fragment. A TLS mode in the value fails, because the value has no query (PRD 12.2).

### 7.5 Effects

The back process does these steps in this order:

1. Create the secret `sag-res-<name>.conn` with `--label sag.resource=<name> --label sag.kind=<kind>`. The value goes to the standard input of `podman secret create`.
2. If `--tls-ca` is present, create the secret `sag-res-<name>.ca` with the same labels.
3. Add `resources.<name>` to `sag.yaml` with `kind`, `description`, `connection`, `tls`, `tls_ca`, and `agents`. The edit keeps all other content. The back process validates the new file with the `config check` rules, writes a temporary file, and renames it.
4. Linux: write the Quadlet file and run `systemctl --user daemon-reload`. Both OSes: restart the container with the new mounts.
5. Wait up to 30 s for `GET /healthz`.
6. Write the admin log line (section 10.2).

- On a failure in steps 3 to 5, the back process restores the previous `sag.yaml` and unit, removes the new secrets, restarts the container, and exits 4.
- The output is `resource <name> created; SAG restarted; run sag resource test <name>`.
- A resource name never contains `.`, so the secret names of two resources never collide.

### 7.6 Create by an agent

- A create needs no human-presence check (Q4). A create cannot show a value and cannot change or remove a credential.
- T8 can register a resource with a value that T8 knows. T8 then reads through SAG what it can already read with that value.
- A create restarts SAG. In-flight requests fail during the restart.

## 8. `sag credential list` and `sag credential inspect`

```
sag credential list [--json]
sag credential inspect <name> [--json]
```

Fields:

| Field | Source |
|---|---|
| `name`, `kind`, `description`, `tls`, `agents` | `sag.yaml` |
| `secrets[].name`, `secrets[].role` (`connection` or `tls_ca`) | `sag.yaml` |
| `secrets[].created_at`, `secrets[].updated_at` | `podman secret inspect` without `--showsecret` |
| `secrets[].exists` | `podman secret exists` |
| `secrets[].mounted` | The secret list of `podman container inspect sag` |
| `container_running` (inspect only) | `podman container inspect sag` |

- List and inspect never show the host, the port, the database, the user, the password, the value length, or a hash of the value. PRD 12.1 puts these parts inside the protected value. A hash of a value with a weak password is a guess oracle.
- No source file outside tests contains the string `--showsecret`.
- The back process never reads a secret value for list or inspect.
- List and inspect show no preflight state. `sag resource test <name>` shows it (PRD 9.1).
- `--json` of list: `{"credentials": [<object>]}`. `--json` of inspect: one object. The object has exactly the fields of the table.
- Inspect of an unknown name gives exit 1 with `resource <name> not found`.

## 9. `sag credential delete` and the human-presence check

### 9.1 Synopsis and flow

```
sag credential delete <name> [--json]
```

1. The front refuses to run without a terminal on standard input and standard output (exit 3).
2. The front runs `sudo -k -u <svc> /opt/sag/bin/sag credential delete <name>`.
3. `sudo` authenticates the operator through PAM. No cached timestamp applies: `-k` and `timestamp_timeout=0` for the runas account.
4. The back process checks `SUDO_USER` against the operator record.
5. The back process opens `/dev/tty`, prints the resource name, the kind, the agents, and the secret names, and asks the operator to type the resource name. Other input gives exit 3.
6. The back process runs `sag extension revoke <name>` in the container. The revoke tombstone stops the reuse of an old approval by a new resource with the same name and target (PRD 9.2).
7. The back process removes `resources.<name>` from `sag.yaml`, writes the unit, and restarts the container without the mounts of the resource.
8. The back process removes the secrets `sag-res-<name>.conn` and `sag-res-<name>.ca`.
9. The back process writes the admin log line.

### 9.2 Human-presence check

The check is a PAM authentication of the operator by `sudo`, at each call, for a command that runs as the service account.

| OS | Authentication |
|---|---|
| macOS | The PAM stack of `sudo`: the operator password, or Touch ID when the operator enables `pam_tid.so` in `/etc/pam.d/sudo_local`. Bootstrap does not change PAM. |
| Linux | The PAM stack of `sudo`: the operator password, or a fingerprint when the PAM stack has `pam_fprintd`. Bootstrap does not change PAM. |

Why T8 fails:

- T8 does not know the password and cannot touch the sensor.
- T8 has a terminal from `script`, but `sudo` asks on that terminal, and T8 has no answer.
- T8 cannot reuse a timestamp of the operator for the runas account, because `timestamp_timeout=0` applies to each call.
- T8 cannot run Podman in the service account, because the Podman store and socket belong to that account, with mode 0700.
- T8 cannot change `/opt/sag`, the sudoers drop-in, or the binaries that the service account runs, because root owns them.
- The typed name in step 5 is not a control against T8. It confirms the intent of the operator.

### 9.3 Commands under the same rule

| Command | Human-presence check | Reason |
|---|---|---|
| `credential delete` | Yes | Destructive. |
| Replacement of a credential | Yes | Devx has no replace command. A replacement is `delete`, then `create`. |
| Master key reset | Not applicable | Bootstrap never replaces the master key. No command resets it. |
| `jwt generate` | Yes (Q8) | A JWT for another `sub` crosses resource tags (PRD 11.3). |
| `extension approve` | Yes | An approval accepts R3 for a resource. |
| `service build` | Yes | A new image runs with all secrets. |
| `credential create`, `list`, `inspect`, `resource test`, `extension revoke`, `service start` | No (Q4 for `create`) | No value output and no removal. |

## 10. Interaction with product rules

### 10.1 Audit and scrub

- The MCP audit log (PRD 10.5) does not change. Admin commands write no MCP audit record.
- The daemon scrub (PRD 10.2) does not change. The front and the back process hold each value in a `secret.Credential` from the first read. `secret.Credential` prints `[REDACTED]`.
- The back process writes the value only to the standard input of `podman secret create`.

### 10.2 Admin log

The back process appends one JSON line to the admin log for each `credential create`, `credential delete`, `jwt generate`, `extension approve`, `extension revoke`, and `service build`:

| Field | Content |
|---|---|
| `ts` | RFC 3339 time with milliseconds, UTC |
| `actor` | `SUDO_USER` |
| `command` | For example `credential.delete` |
| `resource` | Resource name, or `null` |
| `secrets` | Secret names, never values |
| `outcome` | `ok`, `refused`, or `error:<fixed code>` |

- The admin log never holds a value, a JWT, a host, or a user of a credential.
- A refusal by `sudo` writes no admin log line, because the back process does not run. The system log of `sudo` records the refusal.

### 10.3 PRD changes

| PRD section | Change |
|---|---|
| 4 Personas | The operator acts on SAG through the host commands and `sudo`. |
| 6 Operational rules | "The operator runs CLI commands inside the container with `podman exec`" becomes: the host commands run `podman exec` in the service account. |
| 6.1 | The development certificate comes from bootstrap (section 6.6). |
| 7 | Section 2.3 of this addendum. |
| 11.2 | The host form `sag jwt generate <sub>` replaces `podman exec -it sag sag jwt generate <sub>`. |
| 12.1 | Secret names `sag-res-<name>.conn` and `sag-res-<name>.ca`. `sag credential create` replaces the manual steps. |
| 13.1 | Add the host commands of section 1 and the host forms of section 4.1. |
| 13.4 | `resources` can be empty (Q6). AC1 keeps its five fixtures. |
| 13.5 | `sag bootstrap` replaces the install steps. `deploy/sag.container` becomes a golden file. |

## 11. Acceptance criteria

Each criterion has one e2e test. Section 12 names the test environment. "Linux VM" is a disposable Linux VM with a test operator `sagop` that has a known password and no `NOPASSWD` grant.

| ID | Criterion | Environment |
|---|---|---|
| DX-AC1 | On a fresh Linux VM, `bootstrap --auto-approve` for each absent item and `--autostart yes` exits 0. `GET /healthz` with `/opt/sag/share/ca.pem` returns 200. `systemctl --user -M sag@ is-enabled sag` prints `enabled` (`generated` accepted). | Linux VM |
| DX-AC2 | A second `bootstrap` on the same VM, without a terminal, exits 0, writes only `ok` lines, and runs no install command. The `CreatedAt` of `sag-master-key` does not change. A JWT from before the run still authenticates. | Linux VM |
| DX-AC3 | Without a terminal and without a pre-approval, `bootstrap` with an absent test item exits 3 and installs nothing. With `--auto-approve <item>@<other version>`, it writes the `ignored` warning and exits 3. | Linux VM, test pins |
| DX-AC4 | A `release` test item with a wrong SHA-256 gives exit 1. The artifact is not in the install directory or the temporary directory. | Linux VM, test pins, local HTTP server |
| DX-AC5 | An installed distro item with a version different from the test pin gives one `warn` line and exit 0. The installed version does not change. | Linux VM, test pins |
| DX-AC6 | The container from bootstrap passes the AC32 checks, has `ReadOnlyTmpfs` false, and has the state volume with `noexec`. The Quadlet output for `deploy/dev/sag.yaml` is equal to `deploy/sag.container`. | Linux VM |
| DX-AC7 | For an operator with `NOPASSWD: ALL`, `bootstrap` stops at the presence probe with exit 3 and the fixed message. | Linux VM, operator `sagnopw` |
| DX-AC8 | `credential create canary` with a canary DSN typed on a pseudo-terminal exits 0. `sag resource test canary` passes. The canary password, host, user, and database appear in no output, in no pseudo-terminal echo, in the admin log, in `sag.yaml`, in the Quadlet file, and in the process arguments that a 10 ms `ps` sampler records during the command. | Linux VM, PostgreSQL container |
| DX-AC9 | `credential create` exits 1 with a fixed message for a DSN with `?sslmode=disable`, for two hosts, for no database, for an invalid name, and for a name in use. No new secret exists. No message contains the canary. | Linux VM |
| DX-AC10 | `credential list` and `credential inspect canary`, in text and with `--json`, show the fields of section 8 and contain no canary part. The JSON object has exactly the field set of section 8. | Linux VM |
| DX-AC11 | As `sagop`, `credential delete canary` without a terminal exits 3. Under `script` with no password input, it exits 3. A direct `sudo -u sag /opt/sag/bin/sag credential delete canary` without a password fails. After each attempt, the secret exists and `sag resource test canary` passes. | Linux VM |
| DX-AC12 | As `sagop`, `credential delete canary` with the password and the typed name on a pseudo-terminal exits 0. The secrets are absent, `sag.yaml` has no `canary`, the container has no mount of the secrets, the revoke tombstone exists, and the admin log has one `credential.delete` line with `outcome` `ok`. | Linux VM |
| DX-AC13 | After one authenticated `credential delete`, a second `credential delete` on the same pseudo-terminal within one minute asks for the password again. | Linux VM |
| DX-AC14 | Host `sag jwt generate claude-code-dev` without the password exits 3. With the password, it prints a JWT that authenticates at `/mcp`. | Linux VM |
| DX-AC15 | The product binary ignores `SAG_TEST_PINS`. Its symbol table has no test-pin symbol. No non-test source file contains `--showsecret`. | Linux VM, build check |
| DX-AC16 | On macOS, after `bootstrap --autostart yes`, `launchctl print system/net.kanthorlabs.sag` shows user `_sag`. After `launchctl kickstart -k system/net.kanthorlabs.sag`, `GET /healthz` returns 200. `ps` shows the Podman machine processes as `_sag`. | macOS, local only |
| DX-AC17 | On macOS, `credential create`, `list`, and `delete` behave as DX-AC8, DX-AC10, and DX-AC12, with the password through a pseudo-terminal. | macOS, local only |

## 12. Test environment

- Linux tier: the CI `ubuntu-24.04` runner is a disposable VM. The test setup creates `sagop` and `sagnopw`. The runner user has `NOPASSWD: ALL`, so no test runs a command under test as the runner user.
- The tests drive terminals with `github.com/creack/pty`, as the e2e harness does today. The test is the human: it knows the password of `sagop`.
- Test pins: a build tag `devxtest` enables `SAG_TEST_PINS`. Test pins hold a `release` item served by a local HTTP server and a small distro package (`hello` on apt) for the install path. Real Podman stays as the runner has it. DX-AC5 uses a test pin with a different Podman version.
- DX-AC1 needs a VM without SAG. The CI job runs DX-AC1 first, then the other Linux criteria in order.
- The Linux tier needs a git remote and a push by Ulrich, the same as AC35.
- macOS tier: local only. The `macos-26` runner has no nested virtualization, so a Podman machine cannot start there. DX-AC16 and DX-AC17 run on Ulrich's machine and change the system: they add `_sag`, `/opt/sag`, and the sudoers drop-in.
- Not tested: Touch ID, `pam_fprintd`, a start after a real host reboot, a distro other than Ubuntu 24.04. The fingerprint path uses the same PAM stack as the password path, which DX-AC11 to DX-AC14 test.

## 13. Residual risks

- **R11.** The operator password or the fingerprint is the only barrier. An operator who types the password into a prompt that T8 shows, or who has `NOPASSWD: ALL`, gives T8 every admin command. The presence probe detects only the second case.
- **R12.** A root sudo timestamp of the operator on a terminal that T8 shares gives T8 root, and root controls the service account. The operator sets `Defaults timestamp_timeout=0` to remove this risk.
- **R13.** An agent that runs as root, or under the service account, has full control. SAG does not protect against it.
- **R14.** The host binary and the image build from the working copy, and T8 can write the working copy and the Go toolchain directory. A human authentication of `service build` proves presence, not a code review. The operator reviews the commits before bootstrap. Q2 removes this risk with signed release artifacts.
- **R15.** T8 can edit shell startup files and put a false `sag` in `PATH`. The false `sag` can start a different admin command while the operator authenticates. The operator runs `/opt/sag/bin/sag` with the full path. The typed name of step 5 in section 9.1 shows the target, but T8 can also proxy that terminal.
- **R16.** T8 can register resources with values that it knows (section 7.6) and can restart SAG through `create`. It cannot read or remove a credential that it did not create.

## 14. Open questions for Ulrich

Answer each question in the form `Q<n>: <answer>`. The first option is the recommendation of Aelita.

**Q1. Isolation boundary for credentials.**
- Recommended: a dedicated service account (sections 2 and 3). Only this boundary stops T8 from `podman secret inspect --showsecret` and `podman secret rm`.
- Alternative: SAG stays in the operator account. Setup is simpler, but T8 reads and removes every credential with Podman, and the delete check only slows T8.

**Q2. Distribution of the host binary.**
- Recommended now: source build through `scripts/bootstrap.sh` (section 4.3). The repository has no remote and no release pipeline today.
- Alternative: release binaries with a SHA-256 list and a signature, verified against a key in the pins. This removes R14 but needs a remote, a release job, and a signature key.
- Alternative: the operator installs Go and the C toolchain by hand. This is the smallest change, but bootstrap then cannot install everything.

**Q3. macOS Podman machine under `_sag`.** No test shows yet that a Podman machine starts from a LaunchDaemon under a role account.
- Recommended: build DX-AC16 first as a spike. If it fails, Aelita stops and asks Ulrich.
- Alternative: on macOS, SAG stays in the operator account (the second option of Q1). Setup is simpler, but on macOS the delete check only slows T8.

**Q4. Human-presence check for `credential create`.**
- Recommended: no check for a new name (section 7.6). A create cannot show or remove a credential, and a replacement needs `delete`.
- Alternative: check each create. T8 then cannot register a resource or restart SAG, with one more password prompt for each create.

**Q5. Secret value from piped standard input.**
- Recommended: allow it only with `--stdin`, for a pipe from a password manager CLI. The sudo rule, not the terminal, is the security control.
- Alternative: terminal only. This stops scripted setup on a server and gives no security gain against T8, which can allocate a terminal.

**Q6. Empty `resources` map in `sag.yaml`.**
- Recommended: allow it. Bootstrap then starts SAG before the first credential, and `list_resources` returns an empty list.
- Alternative: keep the PRD rule. Bootstrap leaves SAG stopped until the first `credential create`, and the first create starts it.

**Q7. Start at boot or at login.**
- Recommended: at host boot (section 6.7). The service account has no login session.
- Alternative: at operator login through a LaunchAgent or a user unit of the operator. This puts SAG back into the operator account and breaks Q1.

**Q8. Human-presence check for `jwt generate`.**
- Recommended: yes. A JWT for another `sub` gives access to the resources of that `sub`.
- Alternative: no check. Token issue is faster, but T8 can issue a JWT for each `sub` in `sag.yaml`.

**Q9. Development PostgreSQL fixture.**
- Recommended: remove `scripts/dev-up.sh` and `scripts/dev-down.sh`. The e2e harness has its own fixtures, and a developer registers any database with `credential create`.
- Alternative: keep a `make dev-db` target that starts only the fixture database. This keeps a local database for manual tests, with one more script to maintain.

**Q10. Image source for `service build`.**
- Recommended: `git archive HEAD` with a clean working copy (section 6.2, step 9). The image then matches one commit.
- Alternative: the working copy with uncommitted changes. The development loop is faster, but the image matches no commit.
