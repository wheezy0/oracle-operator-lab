# TL;DR — Oracle Database Operator

A Kubernetes operator that manages fictional Oracle databases as custom resources. You define a database in YAML, apply it with kubectl, and the operator takes care of the rest — creating it, tracking its state, and keeping it running.

---

## Components

**OracleDatabase CRD**
A custom Kubernetes resource type. You create one to request a database. It has a `spec` (what you want) and a `status` (what actually exists). Kubernetes stores it in etcd like any other resource.

**Oracle Operator (Go)**
A controller that runs as a pod inside the cluster. It watches for `OracleDatabase` resources and reacts to changes. Every 30 seconds it also wakes up and checks that everything is still in order. It talks to the mock API and writes results back to the resource status.

**Mock Oracle Middleware API (Python/FastAPI)**
Simulates the backend Oracle provisioning layer. Stores database records in SQLite. Exposes a REST API that the operator calls to create, update, and delete databases. Also simulates async provisioning — when a database is created it starts in `Creating` phase and transitions to `Ready` after 8 seconds.

**Validating Admission Webhook**
Intercepts every `kubectl apply` before Kubernetes accepts it. Rejects the request if another database with the same `dbName` already exists anywhere in the cluster. This runs inside the operator pod on port 9443 over HTTPS.

**Web Dashboard**
A dark-theme single-page app served by the mock API at `/ui`. Shows all databases in a live table with phase badges and action buttons. Updates in real time via a Server-Sent Events stream.

**Helm Chart**
Packages everything into a single installable unit. One `helm install` command creates the namespace, RBAC, secrets, deployments, and webhook configuration.

---

## Workflow — What Happens When You Apply a Database

1. You run `kubectl apply -f mydb.yaml`
2. The Kubernetes API server calls the **admission webhook** — the operator checks for duplicate `dbName`. If a duplicate exists, the request is rejected immediately and nothing is created.
3. Kubernetes stores the `OracleDatabase` object in etcd and notifies the operator.
4. The operator's **reconcile loop** runs. It sees no `dbID` in the status, so it calls `POST /databases` on the mock API.
5. The mock API creates a record, sets phase to `Creating`, starts an 8-second background task, and returns the new database ID.
6. The operator saves the ID and phase to the resource status. It requeues itself to check again in 10 seconds.
7. After 8 seconds the mock API background task sets the phase to `Ready`.
8. The operator's next reconcile runs, calls `PUT /databases/{id}` (syncing the spec), gets back `Ready`, and updates the status. It then requeues for 30 seconds.
9. The database is now `Ready`. The operator keeps polling every 30 seconds to catch any unexpected state changes.

---

## Workflow — What Happens When You Delete a Database

1. You run `kubectl delete oracledatabase mydb`
2. Kubernetes marks the resource with a deletion timestamp but does **not** remove it — because the operator registered a **finalizer** on it.
3. The reconcile loop runs, sees the deletion timestamp, and calls `DELETE /databases/{id}` on the mock API.
4. The operator removes the finalizer from the resource.
5. Kubernetes sees the finalizer is gone and deletes the object from etcd.

---

## Self-Healing

The operator actively monitors database state. If the mock API reports a database as `Stopped` (e.g. because the Stop button was clicked in the dashboard), the operator calls `PUT /databases/{id}/status` to set it back to `Starting`, which triggers the mock API to provision it again.

To intentionally stop a database without the operator restarting it, set `spec.suspended: true`:

```bash
kubectl patch oracledatabase mydb --type=merge -p '{"spec": {"suspended": true}}'
```

The operator will stop the database and leave it stopped until you remove the flag.

---

## Recovery — Lost Records

If the mock API database is wiped (all records deleted), the operator detects a `404` on the next `PUT /databases/{id}` call. It clears the stored `dbID` from the resource status and falls back into the create path on the next reconcile. All databases re-provision themselves automatically without any manual intervention.

---

## Authentication

All `/databases` API endpoints require an `X-API-Key` header. The key is stored in a Kubernetes Secret and mounted as an environment variable into both the operator and mock API pods. The dashboard receives the key server-side and includes it automatically in all API calls from the browser.

The SSE watch stream (`/databases/watch`) is exempt — browsers cannot send custom headers with `EventSource`.

---

## RBAC

Two team namespaces exist: `team-finance` and `team-devops`. Each has a dedicated `oracle-dba` Role that allows creating and managing `OracleDatabase` resources only within that namespace. Team members use their own kubeconfig files and cannot see or touch resources in other namespaces.

---

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Kubernetes | k3s (Linux) / Rancher Desktop (macOS) |
| Operator | Go, kubebuilder v4 |
| Mock API | Python, FastAPI, SQLite |
| Dashboard | Vanilla JS, SSE |
| Packaging | Helm |
