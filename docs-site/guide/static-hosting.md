<!-- audited 2026-03-20 -->
# Static Hosting

AYB's hosting support is a Phase 1 feature for serving prebuilt static sites. It is intentionally narrow: build your frontend locally, then deploy the output directory with the CLI.

## What Phase 1 includes

- CLI-first deploys with `ayb sites deploy <site-id-or-slug> --dir <build-dir>`
- Admin dashboard site CRUD and deploy history actions
- Host-based serving for derived domains and verified custom domains
- SPA fallback via per-site `spa_mode`
- Deploy lifecycle management: uploading, live, superseded, failed

## Deploy flow

Build your frontend locally first, then upload the generated directory:

```bash
npm run build
ayb sites deploy marketing --dir ./dist
```

The deploy directory must include `index.html`. AYB uploads files in deterministic order and promotes the resulting deploy separately from the upload step.

## Runtime behavior

Static hosting only handles normal site traffic. AYB bypasses site-runtime serving for:

- `/api`
- the configured admin path
- `/health`
- `/favicon.ico`

Only `GET` and `HEAD` requests are served by the site runtime. Other methods bypass site hosting and continue to the normal server handlers.

When `spa_mode` is enabled, AYB serves `index.html` for missing application routes. When `spa_mode` is disabled, missing paths return a normal 404.

## What Phase 1 does not include

These are intentional Phase 2+ deferrals, not broken features:

- Git-triggered deploys
- Server-side build execution
- Dashboard file uploads for deploy artifacts
- Preview or staging deploy URLs
- Per-site TLS management
- Build logs
- Deploy diffing

## Related guides

- [CLI Reference](/guide/cli)
- [Custom Domains](/guide/custom-domains)
- [Configuration](/guide/configuration)
- [Deployment](/guide/deployment)
