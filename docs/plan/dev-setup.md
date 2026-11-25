# Development setup on macOS

Run these steps at the repository root. Development state goes to `.dev/local/`, which `.git/info/exclude` covers.

1. Install the tools: `brew install podman mkcert jq`.
2. If no Podman machine exists, run `podman machine init --cpus 4 --memory 8192 --disk-size 30`.
3. Start the machine: `podman machine start`.
4. Install the mkcert root CA one time: `mkcert -install`.
5. Create the certificates:

   ```sh
   mkdir -p .dev/local/certs
   mkcert -cert-file .dev/local/certs/sag.pem -key-file .dev/local/certs/sag-key.pem localhost 127.0.0.1 "$(hostname)"
   mkcert -cert-file .dev/local/certs/pg.pem -key-file .dev/local/certs/pg-key.pem sag-dev-pg
   ```

6. Create the Podman secrets:

   ```sh
   podman secret create sag-tls-cert .dev/local/certs/sag.pem
   podman secret create sag-tls-key .dev/local/certs/sag-key.pem
   podman secret create sag-dev-pg-ca "$(mkcert -CAROOT)/rootCA.pem"
   openssl rand -base64 32 | podman secret create sag-master-key -
   read -rs DSN && printf '%s' "$DSN" | podman secret create sag-orders -
   ```

   The DSN form is `postgres://postgres:<password>@sag-dev-pg:5432/app`. The shell does not echo the typed value.
7. Start the development stack: `make dev-up`. It starts `sag-dev-pg` (PostgreSQL 17 with the mkcert certificate) and `sag` with `deploy/dev/sag.yaml`.
8. Issue a JWT: `export SAG_TOKEN="$(podman exec -t sag sag jwt generate claude-code-dev | tr -d '\r')"`.
9. Start Claude Code with the mkcert CA: `NODE_EXTRA_CA_CERTS="$(mkcert -CAROOT)/rootCA.pem" claude`.
10. Stop the stack: `make dev-down`.

Claude Code MCP configuration, user scope:

```sh
claude mcp add --transport http --scope user sag https://localhost:8443/mcp \
  --header "Authorization: Bearer ${SAG_TOKEN}"
```

Project configuration `.mcp.json` with expansion. Do not commit a literal JWT:

```json
{
  "mcpServers": {
    "sag": {
      "type": "http",
      "url": "https://localhost:8443/mcp",
      "headers": { "Authorization": "Bearer ${SAG_TOKEN}" }
    }
  }
}
```

The mkcert root CA stays on the development machine (PRD R8).
