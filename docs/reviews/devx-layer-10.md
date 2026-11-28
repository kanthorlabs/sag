# Layer 10: Linux port

Phase B, Debian machine `homelab`. Gate commit: pending `D10.4`.

## Machine record

| Item | Value | Verification command |
|---|---|---|
| OS | Debian GNU/Linux 13 (trixie) amd64 | `cat /etc/os-release` |
| Kernel | 6.12.41+deb13-amd64 | `uname -r` |
| `/usr/bin/podman` | 5.4.2+ds1-2+b1 (installed) | `apt-cache policy podman` |
| apt candidate | 5.4.2+ds1-2+b2 | `apt-cache policy podman` |
| `/usr/local/bin/podman` | 6.1.3, hand-installed, appears first in login PATH; test PATH uses `/usr/bin/podman` | `/usr/local/bin/podman --version` |
| Linger | yes | `loginctl show-user cantholabs --property=Linger` |
| systemd | 257 (257.13-1~deb13u1) | `systemctl --version` |
| `sudo -n` | works for `cantholabs` via `/etc/sudoers.d/sag-devx` (temporary) | `sudo -n true` |
| iproute2 `ss` | 6.15.0 | `ss --version` |
| `APT::Install-Recommends` | 1 | `apt-config dump APT::Install-Recommends` |
| `uidmap` | Recommends of `podman`; installs with `apt-get install -y podman` | `apt-cache depends podman` |
| `podman container inspect` secrets | lists the secrets of a container (PRD-devx 8) | verified with a probe on homelab |
