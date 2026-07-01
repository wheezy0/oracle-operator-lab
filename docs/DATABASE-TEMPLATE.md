# OracleDatabase YAML Template

Reference guide for writing `OracleDatabase` resource definitions.

---

## Full Annotated Template

```yaml
apiVersion: oracle.dboperator.io/v1alpha1   # Fixed — always this value
kind: OracleDatabase                         # Fixed — always this value

metadata:
  # Name of the Kubernetes resource. Must be unique within the namespace.
  # Use lowercase, hyphens allowed. This is NOT the Oracle database name.
  name: my-database

  # Namespace to deploy into. Defaults to "default" if omitted.
  namespace: default

  # Optional labels for grouping/filtering with kubectl
  labels:
    env: production
    team: dba

spec:
  # REQUIRED — Oracle database name (SID or CDB name).
  # Max 8 characters (Oracle limit). Uppercase convention.
  dbName: MYDB01

  # REQUIRED — Oracle DBA username that owns this database.
  owner: dba

  # REQUIRED — Oracle version string. Quote it to avoid YAML parsing issues.
  # Common values: "19c", "21c", "23c"
  version: "19c"

  # REQUIRED — Allocated storage size in gigabytes.
  # Must be between 1 and 65536.
  sizeGB: 500

  # OPTIONAL — Oracle NLS character set.
  # Defaults to AL32UTF8 if not specified.
  characterSet: AL32UTF8

  # OPTIONAL — Oracle Net service name used by clients to connect.
  # Typically a DNS FQDN. Informational in this mock setup.
  serviceName: mydb01.internal

  # OPTIONAL — Pluggable Database name inside a Container Database (CDB).
  # Only relevant for Oracle 12c and later with multitenant architecture.
  pdbName: MYPDB1

  # OPTIONAL — When true, the operator stops the database and keeps it stopped.
  # Self-healing is suppressed. Set to false (or remove) to restart.
  # Default: false
  suspended: false
```

> **Never set `.status` fields manually.** They are written by the operator.

---

## Minimal Template

Only the four required fields:

```yaml
apiVersion: oracle.dboperator.io/v1alpha1
kind: OracleDatabase
metadata:
  name: my-database
spec:
  dbName: MYDB01
  owner: dba
  version: "19c"
  sizeGB: 100
```

---

## Production Template (19c, CDB+PDB)

```yaml
apiVersion: oracle.dboperator.io/v1alpha1
kind: OracleDatabase
metadata:
  name: proddb01
  namespace: default
  labels:
    env: production
    tier: primary
spec:
  dbName: PRODDB01
  owner: proddba
  version: "19c"
  characterSet: AL32UTF8
  sizeGB: 2048
  serviceName: proddb01.example.com
  pdbName: PRODPDB1
```

---

## Development Template (21c, small)

```yaml
apiVersion: oracle.dboperator.io/v1alpha1
kind: OracleDatabase
metadata:
  name: devdb01
  namespace: default
  labels:
    env: development
spec:
  dbName: DEVDB01
  owner: devdba
  version: "21c"
  sizeGB: 50
```

---

## Data Warehouse Template (23c, large)

```yaml
apiVersion: oracle.dboperator.io/v1alpha1
kind: OracleDatabase
metadata:
  name: dwdb01
  namespace: default
  labels:
    env: production
    tier: analytics
spec:
  dbName: DWDB01
  owner: dwdba
  version: "23c"
  characterSet: AL32UTF8
  sizeGB: 16384
  serviceName: datawarehouse.internal
  pdbName: DWPDB1
```

---

## Field Quick Reference

| Field | Required | Type | Default | Constraints |
|-------|----------|------|---------|-------------|
| `dbName` | ✅ | string | — | 1–8 chars |
| `owner` | ✅ | string | — | min 1 char |
| `version` | ✅ | string | — | min 1 char, quote in YAML |
| `sizeGB` | ✅ | integer | — | 1–65536 |
| `characterSet` | ✗ | string | `AL32UTF8` | — |
| `serviceName` | ✗ | string | — | — |
| `pdbName` | ✗ | string | — | — |
| `suspended` | ✗ | boolean | `false` | stops DB and suppresses self-healing |

---

## Common Mistakes

**Wrong — unquoted version number:**
```yaml
spec:
  version: 19c     # YAML may parse this as a string, but it's ambiguous
```

**Right — always quote the version:**
```yaml
spec:
  version: "19c"
```

---

**Wrong — dbName too long:**
```yaml
spec:
  dbName: VERYLONGDBNAME   # Rejected — max 8 characters
```

**Right:**
```yaml
spec:
  dbName: MYDB01
```

---

**Wrong — sizeGB out of range:**
```yaml
spec:
  sizeGB: 0        # Rejected — minimum is 1
```

---

## Validation

The API server validates all fields against the OpenAPI schema embedded in the CRD. Invalid resources are rejected immediately with a descriptive error:

```bash
kubectl apply -f bad-database.yaml
# The OracleDatabase "bad" is invalid:
# * spec.dbName: Too long: may not be longer than 8
# * spec.sizeGB: Invalid value: 0: spec.sizeGB in body should be >= 1
```

---

## Useful kubectl Commands

```bash
# Apply a database definition
kubectl apply -f my-database.yaml

# List all databases with status
kubectl get oracledatabases

# Watch for phase changes in real time
kubectl get oracledatabases -w

# Show full resource including status
kubectl describe oracledatabase my-database

# Show just the status fields
kubectl get oracledatabase my-database -o jsonpath='{.status}'

# Delete a database (triggers API cleanup via finalizer)
kubectl delete oracledatabase my-database
```
