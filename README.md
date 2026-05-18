# external-dns-scp-webhook

ExternalDNS webhook provider for Samsung Cloud Platform (SCP) DNS.

## How it works

This webhook runs as a sidecar alongside ExternalDNS. ExternalDNS calls the webhook over `localhost:8888`, and the webhook syncs records to SCP DNS using the OpenAPI.

```
ExternalDNS → localhost:8888 (webhook) → SCP OpenAPI → DNS records
```

SCP stores DNS records as short names without a public suffix (e.g. `myapp.gs12345.sds`). Set `SCP_DOMAIN_FILTER` to the zone's public suffix (e.g. `example.com`) so the webhook appends it on read and strips it on write.

Set `domainFilters` in the ExternalDNS Helm chart to the exact FQDN you want ExternalDNS to manage (e.g. `myapp.gs12345.sds.example.com`). This ensures only that specific record is managed and others in the zone are left untouched.

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `SCP_API_URL` | `https://openapi.samsungsdscloud.com` | SCP OpenAPI base URL |
| `SCP_ACCESS_KEY` | — | SCP access key (required) |
| `SCP_SECRET_KEY` | — | SCP secret key (required) |
| `SCP_PROJECT_ID` | — | SCP project ID (required) |
| `SCP_ZONE_ID` | — | DNS domain ID, e.g. `DNS_DOMAIN_SERVICE-xxx` (required) |
| `SCP_DOMAIN_FILTER` | — | Zone public suffix for name translation, e.g. `example.com` — appended on read, stripped on write |
| `SCP_TTL` | `300` | TTL in seconds for created/updated records (SCP valid range: 300–86400) |
| `DRY_RUN` | `false` | Log changes without applying |
| `WEBHOOK_HOST` | `localhost` | Webhook listen host |
| `WEBHOOK_PORT` | `8888` | Webhook listen port |
| `HEALTH_HOST` | `0.0.0.0` | Health server listen host |
| `HEALTH_PORT` | `8080` | Health server listen port |

## Deploy with ExternalDNS Helm chart

```yaml
provider: webhook

domainFilters:
  - example.com  # must match SCP_DOMAIN_FILTER below

sidecars:
  - name: external-dns-scp-webhook
    image: ghcr.io/z1yoon/external-dns-scp-webhook:latest
    ports:
      - name: webhook
        containerPort: 8888
      - name: health
        containerPort: 8080
    livenessProbe:
      httpGet:
        path: /health
        port: health
    readinessProbe:
      httpGet:
        path: /ready
        port: health
    env:
      - name: SCP_ACCESS_KEY
        valueFrom:
          secretKeyRef:
            name: scp-external-dns-credentials
            key: access_key
      - name: SCP_SECRET_KEY
        valueFrom:
          secretKeyRef:
            name: scp-external-dns-credentials
            key: secret_key
      - name: SCP_PROJECT_ID
        value: "PROJECT-xxxxxxxx"
      - name: SCP_ZONE_ID
        value: "DNS_DOMAIN_SERVICE-xxxxxxxx"
      - name: SCP_DOMAIN_FILTER
        value: "example.com"

extraArgs:
  webhook-provider-url: http://localhost:8888
```

## Release

Tag a commit to trigger the release workflow:

```sh
git tag vX.Y.Z
git push origin vX.Y.Z
```

The image is published to `ghcr.io/z1yoon/external-dns-scp-webhook`.
