# Demo Guide — Oracle Database Operator

Step-by-step scenarios for testing and demonstrating the full stack.

---

## Before You Start

Make sure all services are running:

```bash
systemctl is-active mock-oracle-api.service oracle-operator.service k3s.service
```

Expected output:
```
active
active
active
```

Check the mock API is responding:

```bash
curl -s http://localhost:8080/databases
# Expected: []  (empty array if no databases exist)
```

---

## Scenario 1 — Create a Database and Watch It Become Ready

**Open two terminals side by side.**

**Terminal 1 — watch k8s resources live:**

```bash
kubectl get oracledatabases -w
```

**Terminal 2 — apply a database:**

```bash
kubectl apply -f ~/oracle-operator-lab/samples/db-19c-small.yaml
```

**What to observe in Terminal 1:**

```
NAME      DBNAME    VERSION   SIZEGB   PHASE      AGE
devdb01   DEVDB01   19c       50       Creating   2s
devdb01   DEVDB01   19c       50       Creating   8s
devdb01   DEVDB01   19c       50       Ready      18s
```

The phase transitions from `Creating` → `Ready` automatically after ~10 seconds (8s mock provisioning + 10s controller poll interval).

---

## Scenario 2 — Inspect the Full Resource State

```bash
kubectl describe oracledatabase devdb01
```

Key sections to point out:

```yaml
Spec:
  Db Name:   DEVDB01
  Owner:     devdba
  Size GB:   50
  Version:   19c

Status:
  Db ID:    bbb303e8-943a-41d8-a4e7-8de9c1997938
  Message:  Database online and accepting connections
  Phase:    Ready

Metadata:
  Finalizers:
    oracle.dboperator.io/finalizer
```

The `Db ID` is the UUID assigned by the mock API — the link between the k8s resource and the backend record.

---

## Scenario 3 — Verify the Backend

Confirm the record exists in the mock API:

```bash
curl -s http://localhost:8080/databases | python3 -m json.tool
```

Cross-reference the `id` field with the `dbID` in the k8s status. They should match.

---

## Scenario 4 — Deploy All Sample Databases at Once

```bash
kubectl apply -f ~/oracle-operator-lab/samples/
```

Watch all three provision simultaneously:

```bash
kubectl get oracledatabases -w
```

Expected:

```
NAME        DBNAME    VERSION   SIZEGB   PHASE      AGE
devdb01     DEVDB01   19c       50       Creating   2s
dwdb01      DWDB01    23c       16384    Creating   2s
financedb   FINDB     21c       2048     Creating   2s
devdb01     DEVDB01   19c       50       Ready      18s
dwdb01      DWDB01    23c       16384    Ready      18s
financedb   FINDB     21c       2048     Ready      18s
```

---

## Scenario 5 — Update a Database Spec

Edit the size of an existing database:

```bash
kubectl patch oracledatabase devdb01 \
  --type=merge \
  -p '{"spec": {"sizeGB": 200}}'
```

The operator reconciles immediately and calls `PUT /databases/{id}` with the new spec.

Verify the update hit the API:

```bash
curl -s http://localhost:8080/databases | python3 -m json.tool | grep -A2 '"sizeGB"'
```

---

## Scenario 6 — Manually Set a Database to Failed

Use the status endpoint to simulate a failure:

```bash
# Get the dbID first
DBID=$(kubectl get oracledatabase devdb01 -o jsonpath='{.status.dbID}')
echo "dbID: $DBID"

# Set it to Failed via the API
curl -s -X PUT http://localhost:8080/databases/$DBID/status \
  -H "Content-Type: application/json" \
  -d '{"phase": "Failed", "message": "Tablespace corruption detected"}' \
  | python3 -m json.tool
```

Wait up to 10 seconds, then check the k8s status:

```bash
kubectl get oracledatabase devdb01
# PHASE should show: Failed
```

The controller will pick up the Failed phase on the next reconcile via `PUT /databases/{id}` and reflect it in status.

> To recover: set phase back to `Ready` manually via the API, or delete and recreate the resource.

---

## Scenario 7 — Watch the SSE Stream

Open a terminal and start watching the event stream:

```bash
curl -s -N http://localhost:8080/databases/watch
```

In another terminal, create or delete a database:

```bash
kubectl apply -f ~/oracle-operator-lab/samples/db-19c-small.yaml
```

Back in the watch terminal you should see:

```
data: {"type": "ADDED", "object": {"id": "...", "dbName": "DEVDB01", "phase": "Creating", ...}}

data: {"type": "MODIFIED", "object": {"id": "...", "dbName": "DEVDB01", "phase": "Ready", ...}}
```

---

## Scenario 8 — Delete a Database

```bash
kubectl delete oracledatabase devdb01
```

**What happens:**

1. k8s sets `DeletionTimestamp` on the resource
2. The operator reconciles, sees the timestamp
3. Operator calls `DELETE /databases/{id}` on the mock API
4. Operator removes the finalizer from the resource
5. k8s completes the deletion

Verify the record is gone from the API:

```bash
curl -s http://localhost:8080/databases | python3 -m json.tool
# devdb01 should no longer appear
```

---

## Scenario 9 — Test Webhook: Duplicate dbName (Expected Failure)

The admission webhook rejects any resource whose `dbName` is already in use, before it is ever created in k8s.

First confirm `DEVDB01` already exists:

```bash
kubectl get oracledatabases
# devdb01 should be listed with dbName DEVDB01
```

Now try to create a second resource with the same `dbName`:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: oracle.dboperator.io/v1alpha1
kind: OracleDatabase
metadata:
  name: duplicate-test
  namespace: default
spec:
  dbName: DEVDB01
  owner: dba
  version: "19c"
  sizeGB: 100
EOF
```

Expected immediate rejection:

```
Error from server (Forbidden): admission webhook "voracledatabase-v1alpha1.kb.io" denied the request:
  spec.dbName: Invalid value: "DEVDB01": already in use by OracleDatabase default/devdb01
```

Note: no resource was created — `kubectl get oracledatabase duplicate-test` will return not found.

---

## Scenario 10 — Test Schema Validation (Expected Failure)

Try to create a resource with an invalid `dbName` (too long):

```bash
cat <<EOF | kubectl apply -f -
apiVersion: oracle.dboperator.io/v1alpha1
kind: OracleDatabase
metadata:
  name: bad-db
spec:
  dbName: TOOLONGNAME
  owner: dba
  version: "19c"
  sizeGB: 100
EOF
```

Expected error from the API server:

```
The OracleDatabase "bad-db" is invalid:
  spec.dbName: Too long: may not be longer than 8
```

The CRD schema validation rejects it before it ever reaches the controller.

---

## Scenario 11 — Operator Recovers a Lost Record

This demonstrates what happens when the mock API loses a record (e.g. the SQLite database is wiped) while the k8s resources still exist.

```bash
# Stop the API, wipe the database, restart
sudo systemctl stop mock-oracle-api.service
rm ~/oracle-operator-lab/mock-api/mock_oracle.db
sudo systemctl start mock-oracle-api.service
```

The k8s resources still exist but now point to IDs that no longer exist in the API. The operator detects the 404 on the next reconcile, clears the stale `dbID` from the status, and re-creates the record:

```bash
kubectl get oracledatabases -w
# Resources will briefly show Pending → Creating → Ready
```

---

## Scenario 12 — Web Dashboard: Stop and Self-Healing

Open the dashboard in a browser:

```
http://localhost:8080/ui
```

**Stop a database:**
1. Find a Ready database in the table
2. Click **⏹ Stop** — the row immediately shows Stopped (red badge)
3. Wait ~30 seconds — the operator detects the Stopped phase on its next periodic check
4. The row transitions to **Starting** (blue, pulsing) — operator is restarting it
5. After ~8 seconds — the row transitions back to **Ready** (green)

This demonstrates the operator's self-healing loop: it ensures that no database stays Stopped unless it is intentionally deleted.

---

## Scenario 13 — Web Dashboard: Remove and Re-Create

**Remove a database from the API (not from k8s):**
1. Find any database in the dashboard
2. Click **✕ Remove** — the row disappears immediately from the table (SSE DELETED event)
3. Wait up to 30 seconds — the operator reconciles on its periodic requeue
4. The operator calls `PUT /databases/{id}`, gets a 404, clears the `dbID`, and re-creates the record
5. The row reappears in the dashboard as **Creating** → then **Ready**

**Permanently removing a database (from k8s AND the API):**

```bash
kubectl delete oracledatabase devdb01
```

This goes through the finalizer: the operator calls `DELETE /databases/{id}`, then removes the finalizer, and k8s deletes the resource. The row disappears from the dashboard and does **not** come back.

---

## Scenario 14 — Suspend a Database via kubectl

This demonstrates the `spec.suspended` field — a kubectl-native way to intentionally stop a database and keep it stopped (unlike the dashboard Stop button, which triggers self-healing).

**Stop a running database:**

```bash
kubectl patch oracledatabase devdb01 --type=merge -p '{"spec": {"suspended": true}}'
```

Watch the phase change:

```bash
kubectl get oracledatabases -w
# devdb01 transitions to Stopped and stays there
```

Confirm the operator is not restarting it — wait 60 seconds and check again:

```bash
kubectl get oracledatabase devdb01
# PHASE should still be Stopped
```

**Start it again:**

```bash
kubectl patch oracledatabase devdb01 --type=merge -p '{"spec": {"suspended": false}}'
```

The operator detects `suspended=false`, sees the Stopped phase, and self-heals it back to `Starting` → `Ready`.

**Suspend via a YAML file** (preferred for version-controlled infrastructure):

```yaml
spec:
  suspended: true
```

```bash
kubectl apply -f my-database.yaml
```

---

## Scenario 15 — Restart Services and Verify Recovery

Stop both services:

```bash
sudo systemctl stop oracle-operator.service mock-oracle-api.service
```

Check that existing k8s resources still exist (they live in k3s etcd, not in the services):

```bash
kubectl get oracledatabases
```

Restart services:

```bash
sudo systemctl start mock-oracle-api.service oracle-operator.service
```

The operator will reconcile all existing resources on startup. Databases that exist in k8s but not in the API backend will be re-created.

---

## Useful Commands Reference

```bash
# --- kubectl ---
kubectl get oracledatabases                          # list all
kubectl get oracledatabases -w                       # watch live
kubectl describe oracledatabase <name>               # full details
kubectl get oracledatabase <name> -o yaml            # raw YAML
kubectl delete oracledatabase <name>                 # delete (triggers cleanup)

# --- Logs ---
journalctl -u oracle-operator.service -f             # operator logs (live)
journalctl -u mock-oracle-api.service -f             # API logs (live)
journalctl -u oracle-operator.service -n 50          # last 50 lines

# --- API ---
curl -s http://localhost:8080/databases              # list all records
curl -s http://localhost:8080/docs                   # Swagger UI (open in browser)
curl -s -N http://localhost:8080/databases/watch     # SSE stream
# http://localhost:8080/ui                           # Web dashboard (open in browser)

# --- Services ---
sudo systemctl status mock-oracle-api.service        # check status
sudo systemctl status oracle-operator.service
sudo systemctl restart oracle-operator.service       # restart after binary rebuild

# --- Reset API backend ---
sudo systemctl stop mock-oracle-api.service
rm ~/oracle-operator-lab/mock-api/mock_oracle.db
sudo systemctl start mock-oracle-api.service
```
