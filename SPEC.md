# drivethru

Kubernetes operator — static site provisioning for Garage + Traefik.
`github.com/notjustanna/drivethru`

---

## Overview

drivethru watches `StaticSite` CRDs and reconciles three things: a Garage bucket with static hosting enabled, S3 credentials in a `Secret`, and a Traefik `IngressRoute` pointed at Garage's HTTP endpoint. No cert management — Traefik handles TLS as long as the host and entrypoint are declared correctly.

Operator configuration (Garage endpoint, admin token, Traefik entrypoint/certResolver) lives in env vars on the operator Deployment or a ConfigMap mounted into it. No CRD for this.

---

## CRD: StaticSite

Namespace-scoped.

```yaml
apiVersion: drivethru.notjustanna.net/v1alpha1
kind: StaticSite
metadata:
  name: my-site
  namespace: my-app
spec:
  host: my-site.notjustanna.net
  generatedSecretName: my-site-s3   # optional, defaults to {name}-s3; false to disable
  retain: false                     # optional, default false
```

| Field | Description |
|---|---|
| `host` | Public hostname. Used as Garage bucket name, website host, and Traefik `Host()` rule. |
| `generatedSecretName` | Name of the generated S3 credentials Secret. Defaults to `{name}-s3`. Set to `false` to skip Secret generation entirely. Union type: `string \| false`. |
| `retain` | If `true`, bucket and credentials are NOT deleted when the resource is removed. |

---

## Operator configuration

Provided as env vars on the operator Deployment, or via a mounted ConfigMap.

| Env var | Description |
|---|---|
| `GARAGE_HOST` | Convenience var. Derives `GARAGE_S3_ENDPOINT` as `http://{host}:3900` and `GARAGE_ADMIN_ENDPOINT` as `http://{host}:3903`. Overrideable by either var individually. |
| `GARAGE_S3_ENDPOINT` | S3-compatible endpoint. If absent and `GARAGE_HOST` is unset, `AWS_ENDPOINT_URL` is omitted from generated Secrets. |
| `GARAGE_ADMIN_ENDPOINT` | Admin API endpoint. Required — startup fails if neither this nor `GARAGE_HOST` is set. |
| `GARAGE_ADMIN_TOKEN` | Garage admin token. Prefer sourcing from a Secret via `valueFrom`. |
| `TRAEFIK_ENTRYPOINT` | Traefik entrypoint name. Defaults to `websecure`. |
| `TRAEFIK_CERT_RESOLVER` | Traefik cert resolver name. If omitted, TLS block is emitted without a resolver. |

Resolution order: `GARAGE_HOST` derives both endpoints as defaults; `GARAGE_S3_ENDPOINT` and `GARAGE_ADMIN_ENDPOINT` override individually. If only `GARAGE_ADMIN_ENDPOINT` is set, the operator functions normally but omits `AWS_ENDPOINT_URL` from generated Secrets.

---

## Reconciliation loop

1. **Ensure bucket** — call Garage admin API, create bucket if absent, enable static website hosting. Idempotent.
2. **Ensure credentials** — create a Garage key scoped to the bucket (read/write). Write or update a `Secret` in the `StaticSite`'s namespace.
3. **Ensure IngressRoute** — emit a Traefik `IngressRoute` in the same namespace, routing `Host(`{host}`)` to Garage's S3 endpoint. TLS block uses configured entrypoint + certResolver.
4. **Update status** — set `status.ready`, `status.bucketName`, `status.secretName` on the resource.

---

## Generated Secret shape

```
AWS_ENDPOINT_URL=http://garage.garage-system.svc.cluster.local:3900  # omitted if GARAGE_S3_ENDPOINT is unset
AWS_ACCESS_KEY_ID=GKxxxxxxxxxxxxxxxx
AWS_SECRET_ACCESS_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
BUCKET_NAME=my-site.notjustanna.net
AWS_DEFAULT_REGION=garage
```

Standard AWS SDK env var names so `aws s3 sync` works out of the box. `AWS_DEFAULT_REGION=garage` is a dummy value — Garage ignores region but the AWS SDK requires it. `BUCKET_NAME` is always the full `host` value.

---

## Deletion / finalizer

- Operator registers a finalizer on every `StaticSite`.
- On delete: if `retain: false`, revoke Garage key, delete bucket, delete Secret, delete IngressRoute, then remove finalizer.
- If `retain: true`, skip bucket/key deletion, remove finalizer immediately.

> **Note:** `retain` defaults to `false`, which means `kubectl delete staticsite` will nuke the bucket. Consider making this field required rather than optional, or flipping the default for production use.

---

## Tech stack

- Go + `controller-runtime` (kubebuilder scaffolding)
- Garage admin API — thin HTTP wrapper, no SDK exists
- Traefik CRD types via `traefik/traefik` Go module
- Deployed as a single `Deployment` in `drivethru-system`

---

## Out of scope (v1)

- Multi-Garage support
- Custom index/error page configuration
- Bucket lifecycle rules / CORS
- Non-Traefik ingress controllers