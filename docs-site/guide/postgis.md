<!-- audited 2026-03-20 -->
# PostGIS Extension Support

AYB supports PostgreSQL databases with the [PostGIS](https://postgis.net/) extension for geospatial data — GPS routes, geographic points of interest, proximity queries, and spatial analysis.

## Overview

When you connect AYB to a PostgreSQL database with PostGIS installed, AYB automatically:

- **Detects PostGIS** at startup and exposes the version in the schema API
- **Introspects geometry/geography columns** with type, SRID, and subtype metadata
- **Serializes spatial values as GeoJSON** (RFC 7946) in API responses — no EWKB hex
- **Accepts GeoJSON input** for INSERT and UPDATE operations
- **Propagates GeoJSON** through expand, batch, and realtime SSE events

When using an external PostgreSQL instance with PostGIS already installed, no additional AYB configuration is required — spatial columns just work.

When using AYB's managed PostgreSQL mode, set `managed_pg.postgis = true` in your `ayb.toml` to include PostGIS in the managed instance's extension list. This prepends `"postgis"` to the `managed_pg.extensions` array at startup.

::: info Embedded mode limitation
AYB's embedded PostgreSQL mode does not bundle the PostGIS extension binaries. If you need geospatial features, use an external PostgreSQL instance with PostGIS installed, or use managed PostgreSQL mode with the `postgis` toggle enabled.
:::

## Setting up PostGIS

### Docker (recommended for development)

Use the official `postgis/postgis` image:

```bash
docker run -d \
  --name ayb-postgis \
  -e POSTGRES_USER=ayb \
  -e POSTGRES_PASSWORD=ayb \
  -e POSTGRES_DB=ayb \
  -p 5432:5432 \
  postgis/postgis:16-3.4
```

Then start AYB:

```bash
ayb start --database-url "postgresql://ayb:ayb@localhost:5432/ayb"
```

### Docker Compose

```yaml
services:
  ayb:
    image: ghcr.io/gridlhq/allyourbase
    ports:
      - "8090:8090"
    environment:
      AYB_DATABASE_URL: "postgresql://ayb:ayb@postgres:5432/ayb?sslmode=disable"
      AYB_AUTH_ENABLED: "true"
      AYB_AUTH_JWT_SECRET: "change-me-to-a-secure-random-string-at-least-32-chars"
    depends_on:
      postgres:
        condition: service_healthy

  postgres:
    image: postgis/postgis:16-3.4
    environment:
      POSTGRES_USER: ayb
      POSTGRES_PASSWORD: ayb
      POSTGRES_DB: ayb
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ayb"]
      interval: 5s
      timeout: 3s
      retries: 5

volumes:
  pgdata:
```

### Cloud PostgreSQL

Most managed PostgreSQL services support PostGIS:

| Provider | How to enable |
|----------|--------------|
| **AWS RDS** | Enable the `postgis` extension in parameter groups, then `CREATE EXTENSION postgis` |
| **Supabase** | PostGIS is pre-installed. Run `CREATE EXTENSION postgis` in the SQL editor |
| **Neon** | Run `CREATE EXTENSION postgis` — supported on all plans |
| **Railway** | Run `CREATE EXTENSION postgis` on the attached Postgres |
| **Google Cloud SQL** | Enable PostGIS in database flags, then `CREATE EXTENSION postgis` |
| **Azure Database for PostgreSQL** | Enable via Azure portal → Server parameters, then `CREATE EXTENSION postgis` |

### Local development (macOS / Linux)

```bash
# macOS
brew install postgis

# Ubuntu/Debian
sudo apt install postgresql-16-postgis-3

# Then enable the extension
psql -d mydb -c "CREATE EXTENSION postgis"
```

## Creating spatial tables

Create tables with PostGIS `geometry` or `geography` column types:

```sql
CREATE TABLE places (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  location geometry(Point, 4326),
  boundary geometry(Polygon, 4326),
  created_at TIMESTAMPTZ DEFAULT now()
);

-- Add a spatial index for efficient queries
CREATE INDEX idx_places_location ON places USING GIST (location);
```

```sql
CREATE TABLE routes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  path geography(LineString, 4326),
  created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_routes_path ON routes USING GIST (path);
```

::: tip geometry vs geography
Use `geometry` for projected/Cartesian data and `geography` for lat/lng on a sphere. For most GPS-based apps, `geography` gives accurate distance calculations. Both serialize as GeoJSON through AYB.
:::

## API usage

### Creating records with GeoJSON

```bash
curl -X POST http://localhost:8090/api/collections/places \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Central Park",
    "location": {
      "type": "Point",
      "coordinates": [-73.9654, 40.7829]
    }
  }'
```

**Response (201 Created):**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Central Park",
  "location": {
    "type": "Point",
    "coordinates": [-73.9654, 40.7829]
  },
  "boundary": null,
  "created_at": "2026-02-22T10:00:00Z"
}
```

Geometry columns in the response are always GeoJSON objects (or `null`).

### Supported GeoJSON types

AYB accepts any [GeoJSON geometry type](https://datatracker.ietf.org/doc/html/rfc7946):

```json
// Point
{ "type": "Point", "coordinates": [-73.9654, 40.7829] }

// LineString
{ "type": "LineString", "coordinates": [[-73.9, 40.7], [-73.8, 40.8], [-73.7, 40.9]] }

// Polygon
{ "type": "Polygon", "coordinates": [[[-73.9, 40.7], [-73.8, 40.7], [-73.8, 40.8], [-73.9, 40.8], [-73.9, 40.7]]] }

// MultiPoint, MultiLineString, MultiPolygon, GeometryCollection
```

### Reading records

Spatial columns are automatically serialized as GeoJSON in all read operations:

```bash
# Single record
curl http://localhost:8090/api/collections/places/550e8400-...

# List with filter
curl "http://localhost:8090/api/collections/places?filter=name='Central Park'"

# Expand foreign keys — related records with geometry also get GeoJSON
curl "http://localhost:8090/api/collections/checkins?expand=place"
```

### Updating records

```bash
curl -X PATCH http://localhost:8090/api/collections/places/550e8400-... \
  -H "Content-Type: application/json" \
  -d '{
    "location": {
      "type": "Point",
      "coordinates": [-73.9700, 40.7850]
    }
  }'
```

### Batch operations

Batch create and update operations handle geometry columns automatically:

```bash
curl -X POST http://localhost:8090/api/collections/places/batch \
  -H "Content-Type: application/json" \
  -d '{
    "operations": [
      {
        "method": "create",
        "body": {
          "name": "Times Square",
          "location": { "type": "Point", "coordinates": [-73.9855, 40.7580] }
        }
      },
      {
        "method": "create",
        "body": {
          "name": "Brooklyn Bridge",
          "location": { "type": "Point", "coordinates": [-73.9969, 40.7061] }
        }
      }
    ]
  }'
```

### Realtime SSE

SSE events for tables with geometry columns include GeoJSON in the record payload:

```bash
curl -N http://localhost:8090/api/realtime?tables=places
```

```
data: {"action":"INSERT","table":"places","record":{"id":"...","name":"Times Square","location":{"type":"Point","coordinates":[-73.9855,40.758]},"created_at":"..."}}
```

## Spatial queries

AYB does not include a custom spatial query DSL. Instead, use PostgreSQL RPC functions for spatial operations — the same pattern used by PostgREST and Supabase.

### Nearby search

```sql
CREATE OR REPLACE FUNCTION find_nearby(
  lat FLOAT8,
  lng FLOAT8,
  radius_m FLOAT8
)
RETURNS TABLE(
  id UUID,
  name TEXT,
  location JSONB,
  distance_m FLOAT8
) AS $$
  SELECT
    p.id,
    p.name,
    ST_AsGeoJSON(p.location)::jsonb AS location,
    ST_Distance(p.location::geography, ST_MakePoint(lng, lat)::geography) AS distance_m
  FROM places p
  WHERE ST_DWithin(p.location::geography, ST_MakePoint(lng, lat)::geography, radius_m)
  ORDER BY distance_m
$$ LANGUAGE sql;
```

Call via the RPC endpoint:

```bash
curl -X POST http://localhost:8090/api/rpc/find_nearby \
  -H "Content-Type: application/json" \
  -d '{"lat": 40.7829, "lng": -73.9654, "radius_m": 1000}'
```

::: tip RPC functions with geometry output
When writing RPC functions that return geometry columns, use `ST_AsGeoJSON(col)::jsonb` in your SELECT. AYB cannot auto-wrap RPC function results since it doesn't introspect arbitrary function return schemas.
:::

### Bounding box search

```sql
CREATE OR REPLACE FUNCTION places_in_bbox(
  min_lng FLOAT8, min_lat FLOAT8,
  max_lng FLOAT8, max_lat FLOAT8
)
RETURNS TABLE(id UUID, name TEXT, location JSONB) AS $$
  SELECT id, name, ST_AsGeoJSON(location)::jsonb
  FROM places
  WHERE location && ST_MakeEnvelope(min_lng, min_lat, max_lng, max_lat, 4326)
$$ LANGUAGE sql;
```

### Route overlap detection

```sql
CREATE OR REPLACE FUNCTION routes_overlapping(route_id UUID)
RETURNS TABLE(id UUID, name TEXT, overlap_pct FLOAT8) AS $$
  SELECT r2.id, r2.name,
    ST_Length(ST_Intersection(r1.path::geometry, r2.path::geometry)) /
      NULLIF(ST_Length(r1.path::geometry), 0) AS overlap_pct
  FROM routes r1
  JOIN routes r2 ON r1.id != r2.id
    AND ST_Intersects(r1.path::geometry, r2.path::geometry)
  WHERE r1.id = route_id
$$ LANGUAGE sql;
```

### Using `filter` with spatial expressions

The `filter` query parameter supports raw SQL expressions, including PostGIS functions:

```bash
curl "http://localhost:8090/api/collections/places?filter=ST_DWithin(location::geography, ST_MakePoint(-73.9654, 40.7829)::geography, 1000)"
```

For complex queries, prefer RPC functions — they're easier to test, optimize, and maintain.

## Schema introspection

AYB exposes PostGIS metadata in the schema API:

```bash
curl http://localhost:8090/api/schema
```

```json
{
  "hasPostGIS": true,
  "postGISVersion": "3.4.2",
  "tables": [
    {
      "name": "places",
      "columns": [
        {
          "name": "location",
          "type": "geometry(Point,4326)",
          "jsonType": "object",
          "isGeometry": true,
          "geometryType": "Point",
          "srid": 4326
        }
      ]
    }
  ]
}
```

When PostGIS is not installed, `hasPostGIS` is `false` and geometry columns appear as `jsonType: "string"` (the raw PostgreSQL type name, no special handling).

## Error handling

Invalid GeoJSON input returns a `400` error with a descriptive message:

```json
{
  "code": 400,
  "message": "invalid GeoJSON geometry",
  "doc_url": "/guide/api-reference#error-format"
}
```

Common causes:
- Missing `type` or `coordinates` fields
- Incorrect coordinate array structure (e.g., single number instead of `[lng, lat]`)
- Non-numeric coordinate values

## Runtime detection

AYB detects PostGIS at startup and on every schema reload. If you install PostGIS after AYB is running:

```sql
CREATE EXTENSION postgis;
```

The schema watcher detects the DDL change and reloads automatically. New geometry columns created via migrations are picked up the same way.

Similarly, if PostGIS is dropped (`DROP EXTENSION postgis CASCADE`), AYB degrades gracefully — columns that were previously geometry are treated as regular string columns.
