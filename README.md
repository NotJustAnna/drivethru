# drivethru

Kubernetes operator that provisions static sites on [Garage](https://garagehq.deuxfleurs.fr/) and exposes them through Traefik. Define a `StaticSite` CRD; drivethru reconciles a Garage bucket (with static-website hosting enabled), an S3-credentials `Secret`, and a Traefik `IngressRoute`.

See [`SPEC.md`](SPEC.md) for the full design.

## Quick start

1. **Configure the operator.** Edit `config/manager/manager.yaml` ConfigMap + Secret with your Garage admin endpoint/token, or hand-craft your own:

   | Env var | Description | Default |
   |---|---|---|
   | `GARAGE_HOST` | Convenience host; derives both endpoints | — |
   | `GARAGE_S3_ENDPOINT` | S3 endpoint emitted into generated Secrets | derived from `GARAGE_HOST` |
   | `GARAGE_ADMIN_ENDPOINT` | Garage admin API base URL | derived from `GARAGE_HOST` |
   | `GARAGE_ADMIN_TOKEN` | Bearer token | **required** |
   | `TRAEFIK_ENTRYPOINT` | Traefik entrypoint to attach to | `websecure` |
   | `TRAEFIK_CERT_RESOLVER` | Traefik certResolver | unset |
   | `GARAGE_SERVICE_NAME` | In-cluster Service name for IngressRoute backend | `garage` |
   | `GARAGE_SERVICE_NAMESPACE` | Namespace of that Service | `garage-system` |
   | `GARAGE_WEB_PORT` | Service port to route to | `3902` |

2. **Install the CRD and operator:**

   ```sh
   make install                       # apply the CRD
   make deploy IMG=your-registry/drivethru:tag
   ```

3. **Create a StaticSite:**

   ```yaml
   apiVersion: drivethru.notjustanna.net/v1alpha1
   kind: StaticSite
   metadata:
     name: my-site
     namespace: my-app
   spec:
     host: my-site.example.com
   ```

   The operator will:
   - create a Garage bucket aliased to `my-site.example.com`
   - enable static website hosting on it
   - create a Garage key `drivethru-my-site.example.com` and grant it R/W
   - write a `Secret` `my-site-s3` with `AWS_*` variables ready for `aws s3 sync`
   - emit a Traefik `IngressRoute` matching `Host(\`my-site.example.com\`)`

## Development

```sh
make test    # unit tests
make build   # local binary
make run     # run against current kube context (requires Garage admin reachable)
```

## Scope (v1)

Out of scope: multi-Garage support, custom index pages, CORS/lifecycle rules, non-Traefik ingress controllers.
