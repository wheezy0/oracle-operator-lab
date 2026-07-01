# CRD Reference — OracleDatabase

The `OracleDatabase` Custom Resource Definition (CRD) represents a request to provision an Oracle database through the middleware API. It follows standard Kubernetes API conventions with `spec` (desired state) and `status` (observed state) subresources.

---

## Resource Identity

| Property | Value |
|----------|-------|
| API Group | `oracle.dboperator.io` |
| Version | `v1alpha1` |
| Kind | `OracleDatabase` |
| Plural | `oracledatabases` |
| Scope | Namespaced |
| Short name | — |

```bash
# These are all equivalent
kubectl get oracledatabases
kubectl get oracledatabase
kubectl get OracleDatabase
```

---

## Spec Fields

All fields live under `.spec`. Fields marked **required** must be present or the API server will reject the resource.

### `dbName` — required

The Oracle database name (SID or CDB name).

| Constraint | Value |
|------------|-------|
| Type | `string` |
| Min length | 1 |
| Max length | 8 (Oracle SID limit) |

```yaml
spec:
  dbName: PRODDB01
```

---

### `owner` — required

The Oracle DBA username that owns this database.

| Constraint | Value |
|------------|-------|
| Type | `string` |
| Min length | 1 |

```yaml
spec:
  owner: dba
```

---

### `version` — required

The Oracle database version string.

| Constraint | Value |
|------------|-------|
| Type | `string` |
| Min length | 1 |
| Examples | `19c`, `21c`, `23c` |

```yaml
spec:
  version: "19c"
```

> **Note:** Quote the value in YAML — unquoted `19c` may be misinterpreted.

---

### `sizeGB` — required

Initial allocated storage size in gigabytes.

| Constraint | Value |
|------------|-------|
| Type | `integer` (int32) |
| Minimum | 1 |
| Maximum | 65536 |

```yaml
spec:
  sizeGB: 500
```

---

### `characterSet` — optional

The Oracle National Language Support (NLS) character set.

| Constraint | Value |
|------------|-------|
| Type | `string` |
| Default | `AL32UTF8` |

```yaml
spec:
  characterSet: AL32UTF8
```

---

### `serviceName` — optional

The Oracle Net service name used by clients to connect. Typically a DNS name.

| Constraint | Value |
|------------|-------|
| Type | `string` |

```yaml
spec:
  serviceName: proddb01.internal
```

---

### `pdbName` — optional

The Pluggable Database (PDB) name inside a Container Database (CDB). Only relevant for Oracle 12c+.

| Constraint | Value |
|------------|-------|
| Type | `string` |

```yaml
spec:
  pdbName: PRODPDB1
```

---

### `suspended` — optional

When `true`, the operator stops the database and keeps it stopped. Self-healing is suppressed — the operator will not restart a suspended database. Set back to `false` (or remove the field) to start it again.

| Constraint | Value |
|------------|-------|
| Type | `boolean` |
| Default | `false` |

```yaml
spec:
  suspended: true
```

Stop a running database via kubectl:

```bash
kubectl patch oracledatabase devdb01 --type=merge -p '{"spec": {"suspended": true}}'
```

Start it again:

```bash
kubectl patch oracledatabase devdb01 --type=merge -p '{"spec": {"suspended": false}}'
```

> **Note:** This is different from the dashboard Stop button. The dashboard Stop simulates an unexpected outage — the operator self-heals it automatically. `suspended: true` is an intentional, persistent stop that the operator respects.

---

## Status Fields

Status is written by the operator — never set these manually. They live under `.status`.

### `phase`

Lifecycle phase of the database.

| Value | Meaning |
|-------|---------|
| `Pending` | Resource created, controller not yet reconciled |
| `Creating` | First-time provisioning in progress (~8s) |
| `Starting` | Operator-initiated restart (self-healing or unsuspend) |
| `Ready` | Database online and accepting connections |
| `Stopped` | Database stopped — operator will self-heal unless `suspended: true` |
| `Failed` | API call failed — see `message` for details |

---

### `dbID`

The UUID assigned by the mock API backend on creation. Used by the controller for all subsequent API calls (update, delete, status).

```yaml
status:
  dbID: 1f2ecada-a4b5-4e20-b6d2-37f7c131d7d7
```

---

### `message`

Human-readable description of the current status or last error.

```yaml
status:
  message: "Database online and accepting connections"
```

---

### `conditions`

Standard Kubernetes condition array. Each condition has `type`, `status` (`True`/`False`/`Unknown`), `reason`, `message`, and `lastTransitionTime`. Currently populated by controller-runtime internals.

---

## kubectl Output Columns

When running `kubectl get oracledatabases`, the following columns are shown:

```
NAME        DBNAME    VERSION   SIZEGB   PHASE   AGE
proddb01    PRODDB01  19c       500      Ready   5m
```

| Column | Source |
|--------|--------|
| NAME | `.metadata.name` |
| DBNAME | `.spec.dbName` |
| VERSION | `.spec.version` |
| SIZEGB | `.spec.sizeGB` |
| PHASE | `.status.phase` |
| AGE | `.metadata.creationTimestamp` |

---

## Finalizer

The controller adds a finalizer `oracle.dboperator.io/finalizer` to every resource it manages. This prevents Kubernetes from deleting the resource until the controller has had a chance to call `DELETE /databases/{id}` on the mock API. Only then is the finalizer removed and the resource fully deleted.

```yaml
metadata:
  finalizers:
    - oracle.dboperator.io/finalizer
```

---

## RBAC Markers

The following RBAC permissions are generated from markers in the controller source and applied when running `make manifests`:

```
get;list;watch;create;update;patch;delete  →  oracledatabases
get;update;patch                           →  oracledatabases/status
update                                     →  oracledatabases/finalizers
```

---

## Admission Webhook Validation

An **Validating Admission Webhook** runs inside the operator and intercepts every `CREATE` and `UPDATE` request before k8s accepts it. If validation fails, `kubectl apply` is rejected immediately with a clear error — the resource is never created.

### What is validated

**On create:** the `spec.dbName` value is checked against all existing `OracleDatabase` resources across all namespaces. If any existing resource already uses the same `dbName`, the request is denied.

**On update:** the same check applies only if `spec.dbName` is being changed. If the name stays the same, the update is allowed through without a lookup.

**On delete:** no validation — deletion is always allowed.

### Example rejection

```bash
$ kubectl apply -f database.yaml
Error from server (Forbidden): admission webhook "voracledatabase-v1alpha1.kb.io" denied the request:
  spec.dbName: Invalid value: "PRODDB01": already in use by OracleDatabase default/proddb01
```

### Webhook endpoint

The webhook server runs on port `9443` (HTTPS) inside the operator process. The path is:

```
/validate-oracle-dboperator-io-v1alpha1-oracledatabase
```

### TLS certificate

The webhook requires HTTPS. A self-signed certificate is used, stored at:

```
oracle-operator/certs/tls.crt
oracle-operator/certs/tls.key
```

The certificate covers `127.0.0.1` and `localhost` as Subject Alternative Names and is valid for 10 years. The CA bundle is embedded in the `ValidatingWebhookConfiguration` so the k8s API server can verify the webhook's identity.

See [STARTUP.md](STARTUP.md) for instructions on regenerating the certificate if the cluster is ever rebuilt from scratch.

### Webhook configuration location

```
oracle-operator/config/webhook/validating-webhook.yaml
```

Install or update it in the cluster with:

```bash
kubectl apply -f oracle-operator/config/webhook/validating-webhook.yaml
```

---

## CRD Manifest Location

```
oracle-operator/config/crd/bases/oracle.dboperator.io_oracledatabases.yaml
```

Install or update it in the cluster with:

```bash
kubectl apply -f oracle-operator/config/crd/bases/oracle.dboperator.io_oracledatabases.yaml
```
