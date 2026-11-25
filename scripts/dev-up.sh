#!/usr/bin/env bash
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
state="$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")/.dev/local/dev"
image=localhost/sag-dev:latest
pg_image=docker.io/library/postgres:17.12-bookworm
sag_uid=65532
pg_uid=999

"$root/scripts/dev-down.sh"

umask 077
mkdir -p "$state/certs"
if [[ ! -f "$state/certs/sag.pem" ]]; then
  env -u JAVA_HOME mkcert -cert-file "$state/certs/sag.pem" -key-file "$state/certs/sag-key.pem" localhost 127.0.0.1 "$(hostname)"
fi
if [[ ! -f "$state/certs/pg.pem" ]]; then
  env -u JAVA_HOME mkcert -cert-file "$state/certs/pg.pem" -key-file "$state/certs/pg-key.pem" sag-dev-pg
fi
if [[ ! -f "$state/master-key" ]]; then
  openssl rand -base64 32 > "$state/master-key"
fi
if [[ ! -f "$state/pg-password" ]]; then
  openssl rand -hex 24 > "$state/pg-password"
fi

podman secret create sag-dev-tls-cert "$state/certs/sag.pem" >/dev/null
podman secret create sag-dev-tls-key "$state/certs/sag-key.pem" >/dev/null
podman secret create sag-dev-pg-cert "$state/certs/pg.pem" >/dev/null
podman secret create sag-dev-pg-key "$state/certs/pg-key.pem" >/dev/null
podman secret create sag-dev-pg-ca "$(mkcert -CAROOT)/rootCA.pem" >/dev/null
podman secret create sag-dev-master-key "$state/master-key" >/dev/null
tr -d '\n' < "$state/pg-password" | podman secret create sag-dev-pg-password - >/dev/null
printf 'postgres://postgres:%s@sag-dev-pg:5432/app' "$(tr -d '\n' < "$state/pg-password")" | podman secret create sag-dev-orders - >/dev/null

make -C "$root" image IMAGE="$image"

podman network create sag-dev >/dev/null
podman volume create sag-dev-state >/dev/null

podman run --detach --name sag-dev-pg --network sag-dev \
  --secret "sag-dev-pg-password,type=mount,target=pg-password,uid=$pg_uid,gid=$pg_uid,mode=0400" \
  --secret "sag-dev-pg-cert,type=mount,target=server.crt,uid=$pg_uid,gid=$pg_uid,mode=0644" \
  --secret "sag-dev-pg-key,type=mount,target=server.key,uid=$pg_uid,gid=$pg_uid,mode=0600" \
  --env POSTGRES_PASSWORD_FILE=/run/secrets/pg-password \
  --env POSTGRES_DB=app \
  "$pg_image" \
  -c ssl=on -c ssl_cert_file=/run/secrets/server.crt -c ssl_key_file=/run/secrets/server.key >/dev/null

for _ in $(seq 1 60); do
  if podman exec sag-dev-pg pg_isready -q -h 127.0.0.1 -U postgres -d app; then
    break
  fi
  sleep 1
done
podman exec sag-dev-pg pg_isready -q -h 127.0.0.1 -U postgres -d app

podman run --detach --name sag-dev --network sag-dev \
  --read-only --cap-drop=all --security-opt=no-new-privileges --user "$sag_uid:$sag_uid" \
  --publish 127.0.0.1:8443:8443 \
  --volume "$root/deploy/dev/sag.yaml:/etc/sag/sag.yaml:ro" \
  --volume sag-dev-state:/var/lib/sag \
  --secret "sag-dev-tls-cert,type=mount,target=sag-tls-cert,uid=$sag_uid,mode=0400" \
  --secret "sag-dev-tls-key,type=mount,target=sag-tls-key,uid=$sag_uid,mode=0400" \
  --secret "sag-dev-master-key,type=mount,target=sag-master-key,uid=$sag_uid,mode=0400" \
  --secret "sag-dev-orders,type=mount,target=sag-orders,uid=$sag_uid,mode=0400" \
  --secret "sag-dev-pg-ca,type=mount,target=sag-dev-pg-ca,uid=$sag_uid,mode=0400" \
  "$image" >/dev/null

ca="$(mkcert -CAROOT)/rootCA.pem"
for _ in $(seq 1 30); do
  if curl -fsS --cacert "$ca" -o /dev/null https://localhost:8443/healthz 2>/dev/null; then
    echo "sag-dev ready at https://localhost:8443"
    exit 0
  fi
  if [[ "$(podman inspect --format '{{.State.Running}}' sag-dev)" != "true" ]]; then
    podman logs sag-dev >&2
    exit 1
  fi
  sleep 1
done
podman logs sag-dev >&2
exit 1
