# Mock Oracle Middleware API Reference

A REST API built with **FastAPI + SQLite** that simulates an Oracle database provisioning middleware. The controller calls this API; it can also be called directly with `curl` or via the built-in Swagger UI.

---

## Base URL

```
http://localhost:8080
```

## Swagger UI

FastAPI generates interactive documentation automatically:

```
http://localhost:8080/docs
```

---

## Data Model

Every database record has the following fields:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string (UUID) | Assigned by the API on creation |
| `dbName` | string | Oracle database name (SID/CDB) |
| `owner` | string | DBA username |
| `version` | string | Oracle version, e.g. `19c` |
| `characterSet` | string | NLS character set, default `AL32UTF8` |
| `sizeGB` | integer | Allocated size in GB |
| `serviceName` | string \| null | Oracle Net service name |
| `pdbName` | string \| null | Pluggable database name |
| `k8sName` | string \| null | Name of the k8s OracleDatabase resource |
| `k8sNamespace` | string \| null | Namespace of the k8s OracleDatabase resource |
| `phase` | string | Lifecycle phase |
| `message` | string | Human-readable status message |
| `createdAt` | ISO 8601 datetime | Record creation time |
| `updatedAt` | ISO 8601 datetime | Last modification time |

### Phase lifecycle

```
Creating  ──(~8s background task)──►  Ready
Starting  ──(~8s background task)──►  Ready  (operator self-healing restart)
    │
    └──(API error)──► Failed

Ready  ──(Stop button / manual)──►  Stopped
Stopped  ──(operator detects, ~30s)──►  Starting  ──►  Ready
```

| Phase | Meaning |
|-------|---------|
| `Creating` | First-time provisioning, triggered by `POST /databases` |
| `Starting` | Operator-initiated restart after a Stopped or lost-record event |
| `Ready` | Database online and accepting connections |
| `Stopped` | Database explicitly stopped (operator will restart it) |
| `Failed` | API or provisioning error |
| `Pending` | Waiting for first reconcile |

---

## Endpoints

### `POST /databases` — Create

Creates a new database record. Phase is immediately set to `Creating`. A background task transitions it to `Ready` after ~8 seconds.

**Request body:**

```json
{
  "dbName": "PRODDB01",
  "owner": "dba",
  "version": "19c",
  "characterSet": "AL32UTF8",
  "sizeGB": 500,
  "serviceName": "proddb01.internal",
  "pdbName": "PRODPDB1"
}
```

Required fields: `dbName`, `owner`, `version`, `sizeGB`.  
Optional: `characterSet` (defaults to `AL32UTF8`), `serviceName`, `pdbName`.

**Response:** `201 Created`

```json
{
  "id": "1f2ecada-a4b5-4e20-b6d2-37f7c131d7d7",
  "dbName": "PRODDB01",
  "owner": "dba",
  "version": "19c",
  "characterSet": "AL32UTF8",
  "sizeGB": 500,
  "serviceName": "proddb01.internal",
  "pdbName": "PRODPDB1",
  "phase": "Creating",
  "message": "Provisioning started",
  "createdAt": "2026-06-29T14:59:41.579283",
  "updatedAt": "2026-06-29T14:59:41.591555"
}
```

**curl example:**

```bash
curl -s -X POST http://localhost:8080/databases \
  -H "Content-Type: application/json" \
  -d '{
    "dbName": "TESTDB",
    "owner": "dba",
    "version": "19c",
    "sizeGB": 100
  }' | python3 -m json.tool
```

---

### `GET /databases` — List

Returns all database records.

**Response:** `200 OK` — array of database objects.

```bash
curl -s http://localhost:8080/databases | python3 -m json.tool
```

---

### `GET /databases/{id}` — Get

Returns a single database record by its UUID.

**Response:** `200 OK` — database object, or `404 Not Found`.

```bash
curl -s http://localhost:8080/databases/1f2ecada-a4b5-4e20-b6d2-37f7c131d7d7 \
  | python3 -m json.tool
```

---

### `PUT /databases/{id}` — Full Update

Replaces all spec fields. Does **not** touch `phase` or `message` — those are managed separately via the status endpoint.

**Request body:** same shape as `POST /databases`.

**Response:** `200 OK` — updated database object, or `404 Not Found`.

```bash
curl -s -X PUT http://localhost:8080/databases/1f2ecada-a4b5-4e20-b6d2-37f7c131d7d7 \
  -H "Content-Type: application/json" \
  -d '{
    "dbName": "PRODDB01",
    "owner": "dba",
    "version": "19c",
    "sizeGB": 1000
  }' | python3 -m json.tool
```

---

### `PATCH /databases/{id}` — Partial Update

Updates only the fields provided. Omitted fields keep their current values.

**Request body:** all fields optional.

```json
{
  "sizeGB": 2000
}
```

**Response:** `200 OK` — updated database object, or `404 Not Found`.

```bash
curl -s -X PATCH http://localhost:8080/databases/1f2ecada-a4b5-4e20-b6d2-37f7c131d7d7 \
  -H "Content-Type: application/json" \
  -d '{"sizeGB": 2000}' | python3 -m json.tool
```

---

### `DELETE /databases/{id}` — Delete

Removes the database record permanently.

**Response:** `204 No Content`, or `404 Not Found`.

```bash
curl -s -X DELETE http://localhost:8080/databases/1f2ecada-a4b5-4e20-b6d2-37f7c131d7d7
```

---

### `GET /databases/{id}/status` — Get Status

Returns only the status fields of a database record. Lightweight — useful for polling.

**Response:** `200 OK`

```json
{
  "id": "1f2ecada-a4b5-4e20-b6d2-37f7c131d7d7",
  "phase": "Ready",
  "message": "Database online and accepting connections"
}
```

```bash
curl -s http://localhost:8080/databases/1f2ecada-a4b5-4e20-b6d2-37f7c131d7d7/status
```

---

### `PUT /databases/{id}/status` — Update Status

Updates only `phase` and `message`. Used by the operator and for manual testing.

Setting phase to `Creating` or `Starting` automatically triggers the 8-second provisioning simulation that transitions the record to `Ready`.

**Request body:**

```json
{
  "phase": "Ready",
  "message": "Database online and accepting connections"
}
```

**Response:** `200 OK` — status object.

```bash
curl -s -X PUT http://localhost:8080/databases/1f2ecada-a4b5-4e20-b6d2-37f7c131d7d7/status \
  -H "Content-Type: application/json" \
  -d '{"phase": "Ready", "message": "Manually set to Ready"}' \
  | python3 -m json.tool
```

---

### `GET /ui` — Web Dashboard

Serves the web dashboard as an HTML page. Open in a browser:

```
http://localhost:8080/ui
```

The dashboard shows all databases in a table with real-time updates via SSE. It includes:
- Stats row: Ready / Creating+Starting / Stopped / Total counts
- Per-row phase badges with pulse animation for active phases
- **Stop** button (Ready databases) — sets phase to Stopped; the operator self-heals within ~30s
- **Remove** button (Ready or Stopped) — deletes the mock API record; the operator re-creates it within ~30s since the k8s resource still exists
- Live SSE connection indicator (green dot = connected)

---

### `GET /databases/watch` — Watch (SSE)

A long-lived HTTP connection that streams change events as Server-Sent Events (SSE). Each event is a JSON object with a `type` and the full database `object`.

**Event types:** `ADDED`, `MODIFIED`, `DELETED`

**Event format:**

```
data: {"type": "ADDED", "object": {"id": "...", "dbName": "...", ...}}

data: {"type": "MODIFIED", "object": {"id": "...", "phase": "Ready", ...}}

data: {"type": "DELETED", "object": {"id": "...", ...}}
```

A `keepalive` comment is sent every 15 seconds to keep the connection alive.

```bash
# Stream events in real time — open in a separate terminal
curl -s -N http://localhost:8080/databases/watch
```

---

## Backend

The API uses **SQLite** via SQLAlchemy. The database file is created automatically at startup:

```
mock-api/mock_oracle.db
```

To reset all data (wipe the database):

```bash
sudo systemctl stop mock-oracle-api.service
rm ~/oracle-operator-lab/mock-api/mock_oracle.db
sudo systemctl start mock-oracle-api.service
```

---

## Starting the API

### Via systemd (recommended)

```bash
sudo systemctl start mock-oracle-api.service
sudo systemctl status mock-oracle-api.service
journalctl -u mock-oracle-api.service -f
```

### Manually (for development)

```bash
cd ~/oracle-operator-lab/mock-api
./run.sh
# or with auto-reload:
venv/bin/uvicorn main:app --host 0.0.0.0 --port 8080 --reload
```
