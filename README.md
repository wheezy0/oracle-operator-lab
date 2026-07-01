# Oracle Database Operator — k8s CRD Lab

A fully working Kubernetes operator that manages fictional Oracle database resources using a Custom Resource Definition (CRD). A mock REST API simulates the Oracle middleware layer. Built on a lightweight k3s cluster running on a single Debian machine.

---

## Architecture

```
  Developer / SysAdmin
         │
         │  kubectl apply -f database.yaml
         │
         ▼
┌──────────────────────────────────────────────────────────┐
│                      k3s Cluster                         │
│                                                          │
│   kubectl apply ──► Admission Webhook (HTTPS :9443)      │
│                          │                               │
│                   Duplicate dbName? → REJECT             │
│                          │ OK                            │
│                          ▼                               │
│   ┌────────────────────┐     ┌────────────────────────┐  │
│   │  OracleDatabase    │     │   Oracle Operator      │  │
│   │  CRD Resource      │◄───►│   (Go / kubebuilder)   │  │
│   │                    │     └──────────┬─────────────┘  │
│   │  spec:             │                │                │
│   │    dbName: PRODDB  │                │  HTTP :8080    │
│   │    version: 19c    │                │                │
│   │    sizeGB: 500     │                │                │
│   │  status:           │                │                │
│   │    phase: Ready    │                │                │
│   │    dbID: <uuid>    │                │                │
│   └────────────────────┘                │                │
└────────────────────────────────────────┼────────────────┘
                                         │
                                         ▼
                          ┌──────────────────────────────┐
                          │  Mock Oracle Middleware API   │
                          │  FastAPI + SQLite (Python)    │
                          │  Runs on host via systemd     │
                          │                              │
                          │  POST   /databases            │
                          │  GET    /databases            │
                          │  GET    /databases/{id}       │
                          │  PUT    /databases/{id}       │
                          │  PATCH  /databases/{id}       │
                          │  DELETE /databases/{id}       │
                          │  GET    /databases/{id}/status│
                          │  PUT    /databases/{id}/status│
                          │  GET    /databases/watch (SSE)│
                          └──────────────────────────────┘
```

---

## Components

| Component | Language | Location |
|-----------|----------|----------|
| k3s cluster | — | host systemd service |
| Oracle Operator + Webhook | Go (kubebuilder) | `oracle-operator/` |
| Webhook TLS certificate | — | `oracle-operator/certs/` |
| Mock Oracle API | Python (FastAPI + SQLite) | `mock-api/` |
| CRD manifest | YAML | `oracle-operator/config/crd/bases/` |
| Webhook configuration | YAML | `oracle-operator/config/webhook/` |
| Sample resources | YAML | `samples/` |

---

## Documentation

| Document | Description |
|----------|-------------|
| [IMPLEMENTATION.md](docs/IMPLEMENTATION.md) | Full setup guide — replicable from scratch |
| [CRD.md](docs/CRD.md) | CRD field reference, status lifecycle, webhook validation |
| [API.md](docs/API.md) | Mock API endpoint reference with curl examples |
| [DATABASE-TEMPLATE.md](docs/DATABASE-TEMPLATE.md) | Annotated YAML template for database resources |
| [PRESENTATION.md](docs/PRESENTATION.md) | Architecture overview and design decisions |
| [DEMO.md](docs/DEMO.md) | Step-by-step demo and test scenarios |
| [STARTUP.md](docs/STARTUP.md) | Post-reboot startup and troubleshooting guide |

---

## Quick Start

```bash
# Start services (all enabled at boot via systemd)
sudo systemctl start mock-oracle-api.service
sudo systemctl start oracle-operator.service

# Apply a database resource
kubectl apply -f samples/db-19c-small.yaml

# Watch it provision
kubectl get oracledatabases -w
```
