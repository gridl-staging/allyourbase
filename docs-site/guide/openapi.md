---
title: OpenAPI Specification
layout: page
---

# OpenAPI Specification
<!-- audited 2026-03-20 -->

AYB ships two OpenAPI surfaces plus a docs UI:

- `GET /api/openapi.yaml` serves the bundled YAML spec (`openapi.Spec`) with `Content-Type: application/yaml` and `Cache-Control: public, max-age=3600`.
- `GET /api/openapi.json` serves a generated JSON OpenAPI document built from the live schema cache.
- `GET /api/docs` serves Swagger UI HTML wired to `/api/openapi.json`.

## JSON spec behavior (`/api/openapi.json`)

- Returns `503` with `schema cache not ready` when schema cache data is not available.
- Returns `200` JSON with `ETag` and `Cache-Control: public, max-age=60` when available.
- Honors `If-None-Match`; matching ETag returns `304 Not Modified`.
- Regenerates the JSON spec when schema cache `BuiltAt` changes.

## Docs UI behavior (`/api/docs`)

- Returns HTML (Swagger UI assets from `swagger-ui-dist@5` CDN).
- UI loads the JSON surface at `/api/openapi.json`.

<ClientOnly>
  <Redoc />
</ClientOnly>
