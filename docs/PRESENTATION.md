# Oracle Database Operator — Project Presentation

---

## What Is This?

A fully functional **Kubernetes operator** that manages Oracle database resources on a lightweight local cluster. It demonstrates the complete operator pattern:

- A **Custom Resource Definition (CRD)** that extends the Kubernetes API with a new resource type: `OracleDatabase`
- A **controller** (operator) written in Go that watches for these resources and reacts to changes
- A **mock REST API** written in Python that simulates the Oracle database middleware layer
- Runs on any standard Kubernetes cluster — tested on **k3s** (Linux) and **Rancher Desktop** (macOS Apple Silicon)

---

## The Problem It Solves

In a real enterprise environment, provisioning an Oracle database involves:

1. Submitting a request to a middleware API (authentication, validation, routing)
2. The middleware calling Oracle's provisioning layer
3. Waiting for async provisioning to complete
4. Tracking state and surfacing it back to the requester

This project simulates that entire chain using Kubernetes as the control plane.

**What this lab covers:**

| Step | In this lab |
|------|------------|
| 1. Middleware API | ✅ Fully implemented — FastAPI mock with validation, CRUD, and SSE streaming |
| 2. Oracle provisioning layer | ⚡ Simulated — an 8-second background task stands in for a real Oracle cluster |
| 3. Async provisioning | ✅ Implemented — Creating → Ready transition with phase tracking |
| 4. State tracking | ✅ Implemented — status written back to the CRD, visible via `kubectl get oracledatabases` |

**Not included:** real Oracle connectivity, enterprise authentication (LDAP/SSO), high availability, multi-region routing.

---

## Architecture

```
  Developer / SysAdmin
         │
         │  kubectl apply -f database.yaml
         ▼
┌──────────────────────────────────────────────────────┐
│                 Kubernetes Cluster                   │
│                                                      │
│   ┌──────────────────┐     ┌────────────────────┐   │
│   │  OracleDatabase  │     │  Oracle Operator   │   │
│   │  (CRD Resource)  │◄───►│  (Go controller)   │   │
│   │                  │     └────────┬───────────┘   │
│   │  spec:           │              │               │
│   │    dbName: PROD  │              │ HTTP          │
│   │    version: 19c  │              │               │
│   │    sizeGB: 500   │              │               │
│   │                  │              │               │
│   │  status:         │              │               │
│   │    phase: Ready  │              │               │
│   │    dbID: <uuid>  │              │               │
│   └──────────────────┘              │               │
└─────────────────────────────────────┼───────────────┘
                                      │
                                      ▼
                       ┌──────────────────────────┐
                       │  Mock Oracle Middleware   │
                       │  API (FastAPI + SQLite)   │
                       │                          │
                       │  Simulates:              │
                       │  • Async provisioning    │
                       │  • State persistence     │
                       │  • REST CRUD             │
                       │  • SSE Watch stream      │
                       └──────────────────────────┘
```

---

## Technology Stack

| Layer | Technology | Why |
|-------|-----------|-----|
| Kubernetes | k3s / Rancher Desktop | Lightweight local clusters — k3s on Linux, Rancher Desktop on macOS |
| Operator framework | kubebuilder v4 | Industry standard for Go operators, generates CRD manifests |
| Controller language | Go | Native k8s client libraries, compiled binary, ideal for controllers |
| Mock API language | Python | FastAPI is the fastest way to build a well-documented REST API |
| Mock API framework | FastAPI + uvicorn | Async support, built-in OpenAPI docs, SSE streaming |
| Mock API storage | SQLite via SQLAlchemy | Zero-config, file-based, perfect for simulation |
| Deployment packaging | Helm | Packages all resources into a single installable chart |

---

## How the Operator Pattern Works

The operator continuously runs a **reconcile loop**. Every time a resource is created, updated, or deleted — or every time a periodic requeue fires — the loop runs:

```
┌─────────────────────────────────────────────────────┐
│                  Reconcile Loop                     │
│                                                     │
│  1. Fetch OracleDatabase resource from k8s          │
│         │                                           │
│         ▼                                           │
│  2. Is it being deleted?                            │
│     YES → call DELETE /databases/{id}               │
│          → remove finalizer → done                  │
│         │                                           │
│         ▼                                           │
│  3. Has our finalizer?                              │
│     NO  → add finalizer → done (will re-run)        │
│         │                                           │
│         ▼                                           │
│  4. Does status.dbID exist?                         │
│     NO  → call POST /databases                      │
│          → save returned ID to status               │
│          → if phase=Creating/Starting: requeue 10s  │
│     YES → call PUT /databases/{id}  (sync spec)     │
│          → if 404: clear dbID → requeue (→ step 4)  │
│          → update status with returned phase        │
│          → if phase=Creating/Starting: requeue 10s  │
│          → if phase=Stopped: call UpdateStatus      │
│               with "Starting" → requeue 10s         │
│          → if phase=Ready: requeue in 30s           │
└─────────────────────────────────────────────────────┘
```

---

## Phase Lifecycle

```
    kubectl apply                  ~8s background task
         │                              │
         ▼                              ▼
    [ Pending ]  ──►  [ Creating ]  ──►  [ Ready ]  ◄──┐
                            │               │           │
                            └──► [ Failed ] │           │
                                            │ Stop       │
                                            ▼           │ ~8s
                                       [ Stopped ]      │
                                            │           │
                          operator detects  │           │
                                            ▼           │
                                       [ Starting ] ────┘
```

| Phase | Triggered by |
|-------|-------------|
| `Creating` | First `kubectl apply` |
| `Starting` | Operator self-healing after Stopped or lost record |
| `Ready` | Provisioning simulation completes |
| `Stopped` | Stop button in dashboard, or manual API call |
| `Failed` | API error during create/update |

---

## What "Mock" Means Here

The mock API simulates three things a real Oracle middleware would do:

1. **Async provisioning** — returns `Creating` immediately, then transitions to `Ready` after 8 seconds via a background task. A real system would take minutes or hours.

2. **State persistence** — stores all database records in SQLite. A real system would store state in the Oracle cluster itself.

3. **Event streaming** — the `/databases/watch` SSE endpoint broadcasts `ADDED`, `MODIFIED`, `DELETED` events. A real system might use Kafka, webhooks, or a proprietary event bus.

---

## Key Design Decisions

### Admission webhook for duplicate prevention
The operator runs a Validating Admission Webhook on port 9443 (HTTPS). When `kubectl apply` is called, the k8s API server pauses and asks the webhook "is this allowed?" before accepting the resource. The webhook lists all existing `OracleDatabase` resources and rejects the request if `spec.dbName` is already in use anywhere in the cluster. This catches the error at the earliest possible point — before the resource is created in k8s and before the controller ever runs.

The webhook uses a self-signed TLS certificate valid for 10 years. In the Helm deployment the operator runs inside the cluster as a pod, so the webhook configuration uses a Kubernetes `Service` reference rather than a URL. The certificate is generated with the in-cluster service DNS name as SAN (`oracle-operator-webhook.oracle-system.svc`) and its CA bundle is embedded directly in the `ValidatingWebhookConfiguration` so the API server can verify the webhook's identity.

### Finalizer pattern
The operator registers a finalizer on every resource it manages. This ensures the controller always gets a chance to clean up the external resource (the mock API record) before Kubernetes removes the CRD object. Without this, deleting a resource would leave orphaned records in the mock API.

### Status as a subresource
`status` is a separate Kubernetes subresource. This means:
- Only the operator can update status (users cannot accidentally overwrite it)
- `kubectl edit` on the resource shows spec, not status
- Requires a separate `r.Status().Update()` call in the controller

### MOCK_API_URL as an environment variable
The API URL is not hardcoded. Set `MOCK_API_URL` to point the operator at any endpoint — useful if the mock API moves to a different host or port.

### Requeue polling
Rather than using the Watch SSE stream from inside the operator, the controller polls the API by requeueing. This is simpler and more robust — SSE connections can drop and require reconnection logic.

---

## Project Stats

| Item | Count |
|------|-------|
| Go source files | 6 |
| Python source lines | ~280 |
| HTML dashboard | 1 (dark theme, SSE live updates) |
| API endpoints | 10 (9 JSON + 1 HTML dashboard) |
| CRD fields (spec) | 8 |
| Admission webhooks | 1 (ValidateCreate + ValidateUpdate) |
| Deployment | Helm chart |
| Helm chart templates | 12 |
| Sample YAML files | 6 |
| RBAC namespaces | 2 (team-finance, team-devops) |

---

## What Could Be Extended

- ~~**Webhook validation** — reject resources with duplicate `dbName` values at admission time~~ ✅ **Implemented**
- ~~**Multiple namespaces** — scope databases by namespace/team with RBAC isolation~~ ✅ **Implemented**
- ~~**Web GUI dashboard** — real-time view of all databases with Stop/Remove actions~~ ✅ **Implemented**
- ~~**Helm chart** — package the CRD and operator for easy deployment~~ ✅ **Implemented**
- **Status conditions** — populate the `conditions` array with structured machine-readable state
- **Prometheus metrics** — expose provisioning duration, error rates
- **Real Oracle connectivity** — replace the mock API with a real Oracle REST Data Services (ORDS) endpoint
