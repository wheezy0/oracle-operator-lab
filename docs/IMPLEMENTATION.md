# Implementation Guide — Oracle Database Operator

Complete step-by-step guide to replicate this project from scratch on a Debian machine.

---

## Prerequisites

| Requirement | Notes |
|-------------|-------|
| Debian 12+ | Any modern Debian/Ubuntu works |
| `curl` | `sudo apt install -y curl` |
| `git` | Usually pre-installed |
| `make` | Usually pre-installed |
| Sudo access | Needed for k3s and systemd |

---

## Step 1 — Install k3s

k3s is a lightweight single-binary Kubernetes distribution. It installs everything (API server, scheduler, etcd, kubectl) as one systemd service.

```bash
curl -sfL https://get.k3s.io | sh -
```

Verify it started:

```bash
sudo systemctl status k3s
```

### Configure kubectl for your user

k3s writes its kubeconfig to `/etc/rancher/k3s/k3s.yaml` (root-owned). Copy it to your home directory:

```bash
mkdir -p ~/.kube
sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
sudo chown $USER:$USER ~/.kube/config
echo 'export KUBECONFIG=~/.kube/config' >> ~/.bashrc
```

Verify the cluster is up:

```bash
KUBECONFIG=~/.kube/config kubectl get nodes
```

Expected output: one node with status `Ready`.

---

## Step 2 — Install Go and kubebuilder

### Go

Check if Go is already installed:

```bash
go version
```

If not, install from [https://go.dev/dl/](https://go.dev/dl/) — download the Linux amd64 tarball, extract to `/usr/local`, and add to PATH:

```bash
sudo tar -C /usr/local -xzf go*.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
```

This project was built with **Go 1.24.4**.

### kubebuilder

kubebuilder scaffolds Kubernetes operators. It is a single binary:

```bash
curl -sfL "https://github.com/kubernetes-sigs/kubebuilder/releases/latest/download/kubebuilder_linux_amd64" \
  -o /tmp/kubebuilder
chmod +x /tmp/kubebuilder
mkdir -p ~/bin
mv /tmp/kubebuilder ~/bin/kubebuilder
echo 'export PATH=$PATH:$HOME/bin' >> ~/.bashrc
```

Verify:

```bash
~/bin/kubebuilder version
```

This project was built with **kubebuilder v4.15.0**.

---

## Step 3 — Scaffold the Go Operator

### 3.1 Create project directories

```bash
mkdir -p ~/oracle-operator-lab/oracle-operator ~/oracle-operator-lab/mock-api
cd ~/oracle-operator-lab/oracle-operator
```

### 3.2 Initialize the kubebuilder project

```bash
kubebuilder init --domain dboperator.io --repo dboperator.io/oracle-operator
```

This creates the Go module, `Makefile`, `cmd/main.go`, and all scaffolding.

### 3.3 Generate the CRD and controller skeleton

```bash
kubebuilder create api --group oracle --version v1alpha1 --kind OracleDatabase \
  --resource --controller
```

This creates:
- `api/v1alpha1/oracledatabase_types.go` — CRD type definitions
- `internal/controller/oracledatabase_controller.go` — controller logic

### 3.4 Define the OracleDatabase spec

Edit `api/v1alpha1/oracledatabase_types.go` to replace the placeholder `Foo` field with the real database fields. See [CRD.md](CRD.md) for the full field reference.

Key types:

```go
type OracleDatabaseSpec struct {
    DbName       string `json:"dbName"`
    Owner        string `json:"owner"`
    Version      string `json:"version"`
    CharacterSet string `json:"characterSet,omitempty"`
    SizeGB       int32  `json:"sizeGB"`
    ServiceName  string `json:"serviceName,omitempty"`
    PdbName      string `json:"pdbName,omitempty"`
}

type OracleDatabaseStatus struct {
    Phase      string             `json:"phase,omitempty"`
    DbID       string             `json:"dbID,omitempty"`
    Message    string             `json:"message,omitempty"`
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

After editing, regenerate the deep copy code and CRD manifest:

```bash
make generate
make manifests
```

### 3.5 Write the API client

Create `internal/controller/apiclient.go` — an HTTP client that calls the mock API with four methods:

| Method | HTTP call |
|--------|-----------|
| `Create(req)` | `POST /databases` |
| `Update(id, req)` | `PUT /databases/{id}` |
| `Delete(id)` | `DELETE /databases/{id}` |
| `UpdateStatus(id, phase, msg)` | `PUT /databases/{id}/status` |

The `DBRequest` struct includes `K8sName` and `K8sNamespace` fields so the mock API knows which k8s resource each database record belongs to (displayed in the web dashboard).

The client also defines a sentinel error `ErrNotFound` which is returned when the API responds with `404`. The controller uses this to detect a lost record and recover gracefully.

### 3.6 Write the controller

The reconcile loop in `internal/controller/oracledatabase_controller.go` follows this logic:

```
1. Fetch OracleDatabase resource — not found? return
2. DeletionTimestamp set?
   → Call Delete on API
   → Remove finalizer
   → return
3. No finalizer?
   → Add finalizer
   → return (will reconcile again)
4. status.dbID empty?
   → Call Create on API
   → Write returned ID + phase to status
   → If phase == "Creating" or "Starting": requeue in 10s
5. status.dbID set?
   → Call Update on API (syncs spec, returns current phase)
   → If API returns 404: clear dbID, set phase Pending → return (will fall to step 4)
   → Write phase to status
   → If phase == "Creating" or "Starting": requeue in 10s
   → If phase == "Stopped": call UpdateStatus with "Starting" → requeue in 10s
   → If phase == "Ready": requeue in 30s (periodic check for self-healing)
```

### 3.7 Pass the API URL from main

In `cmd/main.go`, read `MOCK_API_URL` from the environment and pass it to the reconciler:

```go
apiURL := os.Getenv("MOCK_API_URL")
if apiURL == "" {
    apiURL = "http://localhost:8080"
}

(&controller.OracleDatabaseReconciler{
    Client: mgr.GetClient(),
    Scheme: mgr.GetScheme(),
    APIURL: apiURL,
}).SetupWithManager(mgr)
```

### 3.8 Build the binary

```bash
cd ~/oracle-operator-lab/oracle-operator
go build -o bin/oracle-operator ./cmd/main.go
```

---

## Step 4 — Build the Python Mock API

### 4.1 Install Python venv support

```bash
sudo apt install -y python3.13-venv
```

### 4.2 Create virtual environment and install packages

```bash
cd ~/oracle-operator-lab/mock-api
python3 -m venv venv
venv/bin/pip install fastapi uvicorn sqlalchemy sse-starlette
```

### 4.3 Write the API

Create `main.py` with:

- SQLAlchemy + SQLite backend (`mock_oracle.db` created automatically on startup)
- Pydantic models for request/response validation
- All CRUD endpoints + Watch (SSE) + a `/ui` route serving the web dashboard
- `k8sName` and `k8sNamespace` fields on every database record (populated by the operator, displayed in the dashboard)
- A `_simulate_provisioning()` background task that moves phase from `Creating` or `Starting` → `Ready` after 8 seconds
- The `PUT /databases/{id}/status` endpoint triggers the provisioning simulation when phase is set to `Creating` or `Starting`

Create `static/index.html` — the web dashboard (served at `http://localhost:8080/ui`):

- Dark theme, real-time updates via SSE
- Stats row: Ready (green), Creating/Starting (yellow), Stopped (red), Total (blue)
- Per-database actions: **Stop** (Ready only), **Remove** (Ready or Stopped)
- Phase badges with pulse animation for Creating/Starting

Full endpoint list in [API.md](API.md).

### 4.4 Create the start script

```bash
cat > run.sh << 'EOF'
#!/bin/bash
cd "$(dirname "$0")"
exec venv/bin/uvicorn main:app --host 0.0.0.0 --port 8080 --reload
EOF
chmod +x run.sh
```

---

## Step 5 — Install the CRD into k3s

```bash
kubectl apply -f ~/oracle-operator-lab/oracle-operator/config/crd/bases/oracle.dboperator.io_oracledatabases.yaml
```

Verify:

```bash
kubectl get crd oracledatabases.oracle.dboperator.io
```

> **Note:** The CRD is stored in k3s's embedded etcd and persists across reboots. You only need to apply it once (or after re-creating the cluster).

---

## Step 6 — Install systemd Services

Two service files are provided in the project root.

```bash
sudo cp ~/oracle-operator-lab/mock-oracle-api.service /etc/systemd/system/
sudo cp ~/oracle-operator-lab/oracle-operator.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now mock-oracle-api.service
sudo systemctl enable --now oracle-operator.service
```

### Service: `mock-oracle-api.service`

| Property | Value |
|----------|-------|
| Binary | `venv/bin/uvicorn main:app` |
| Listen | `0.0.0.0:8080` |
| Starts after | `network.target` |
| Restart | on-failure, 5s delay |

### Service: `oracle-operator.service`

| Property | Value |
|----------|-------|
| Binary | `bin/oracle-operator` |
| Env vars | `KUBECONFIG`, `MOCK_API_URL` |
| Starts after | `k3s.service`, `mock-oracle-api.service` |
| Restart | on-failure, 5s delay |

---

## Step 7 — Add the Validating Admission Webhook

The webhook intercepts `CREATE` and `UPDATE` requests before k8s accepts them, allowing the operator to reject resources with a duplicate `dbName`.

### 7.1 Scaffold the webhook

```bash
cd ~/oracle-operator-lab/oracle-operator
kubebuilder create webhook --group oracle --version v1alpha1 --kind OracleDatabase --programmatic-validation
```

This creates `internal/webhook/v1alpha1/oracledatabase_webhook.go` and updates `cmd/main.go` to register the webhook server.

### 7.2 Write the validation logic

Edit `internal/webhook/v1alpha1/oracledatabase_webhook.go`. The validator struct needs a `client.Client` so it can list existing resources:

```go
type OracleDatabaseCustomValidator struct {
    client.Client
}

func SetupOracleDatabaseWebhookWithManager(mgr ctrl.Manager) error {
    return ctrl.NewWebhookManagedBy(mgr, &oraclev1alpha1.OracleDatabase{}).
        WithValidator(&OracleDatabaseCustomValidator{Client: mgr.GetClient()}).
        Complete()
}
```

`ValidateCreate` and `ValidateUpdate` call a shared helper that lists all `OracleDatabase` resources across all namespaces and rejects the request if any existing resource (other than the one being validated) has the same `spec.dbName`.

### 7.3 Generate the TLS certificate

Webhooks require HTTPS. Generate a self-signed certificate covering `127.0.0.1` and `localhost`:

```bash
mkdir -p ~/oracle-operator-lab/oracle-operator/certs
openssl req -x509 -newkey rsa:2048 \
  -keyout ~/oracle-operator-lab/oracle-operator/certs/tls.key \
  -out ~/oracle-operator-lab/oracle-operator/certs/tls.crt \
  -days 3650 -nodes \
  -subj "/CN=oracle-operator-webhook" \
  -addext "subjectAltName=IP:127.0.0.1,DNS:localhost"
```

> **Important:** The certificate is valid for 10 years. If you ever rebuild the k3s cluster from scratch, you must regenerate this certificate and re-apply the webhook configuration. The `caBundle` in the webhook YAML must always match the certificate the operator is using. See [STARTUP.md](STARTUP.md) for the full procedure.

### 7.4 Create the ValidatingWebhookConfiguration

Since the operator runs outside the cluster (on the host), use a `url`-based webhook configuration rather than a service reference. Embed the certificate as the `caBundle`:

```bash
CA_BUNDLE=$(base64 -w0 ~/oracle-operator-lab/oracle-operator/certs/tls.crt)
cat > ~/oracle-operator-lab/oracle-operator/config/webhook/validating-webhook.yaml << EOF
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: oracle-operator-validating-webhook
webhooks:
  - name: voracledatabase-v1alpha1.kb.io
    admissionReviewVersions: ["v1"]
    clientConfig:
      url: "https://127.0.0.1:9443/validate-oracle-dboperator-io-v1alpha1-oracledatabase"
      caBundle: ${CA_BUNDLE}
    rules:
      - apiGroups: ["oracle.dboperator.io"]
        apiVersions: ["v1alpha1"]
        operations: ["CREATE", "UPDATE"]
        resources: ["oracledatabases"]
    sideEffects: None
    failurePolicy: Fail
EOF
```

### 7.5 Update the systemd service

Add the `--webhook-cert-path` flag to the operator's `ExecStart` line in `oracle-operator.service`:

```ini
ExecStart=/home/stefanb/oracle-operator-lab/oracle-operator/bin/oracle-operator \
  --webhook-cert-path=/home/stefanb/oracle-operator-lab/oracle-operator/certs
```

### 7.6 Build, install, and apply

```bash
# Rebuild the binary
go build -o bin/oracle-operator ./cmd/main.go

# Install updated service file and restart
sudo cp ~/oracle-operator-lab/oracle-operator.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl restart oracle-operator.service

# Apply the webhook configuration
kubectl apply -f ~/oracle-operator-lab/oracle-operator/config/webhook/validating-webhook.yaml
```

---

## Directory Layout

```
oracle-operator-lab/
├── README.md
├── mock-oracle-api.service       # systemd unit
├── oracle-operator.service       # systemd unit
├── sample-database.yaml          # original test resource
├── samples/
│   ├── db-19c-small.yaml
│   ├── db-21c-pdb.yaml
│   ├── db-23c-large.yaml
│   ├── ns-finance-db.yaml            # team-finance namespace database
│   └── ns-devops-db.yaml             # team-devops namespace database
├── rbac/
│   ├── setup.yaml                    # Namespaces + Roles + RoleBindings
│   ├── create-users.sh               # Generates client certs and kubeconfigs
│   ├── finance-dba.kubeconfig        # Generated — finance team access
│   └── devops-dba.kubeconfig         # Generated — devops team access
├── docs/
│   ├── IMPLEMENTATION.md         # this file
│   ├── CRD.md
│   ├── API.md
│   ├── DATABASE-TEMPLATE.md
│   ├── PRESENTATION.md
│   └── DEMO.md
├── mock-api/
│   ├── main.py
│   ├── requirements.txt          # pip dependencies (used by Dockerfile)
│   ├── Dockerfile
│   ├── run.sh
│   ├── static/
│   │   └── index.html            # Web dashboard (served at /ui)
│   └── venv/                     # Python virtual environment (systemd only)
└── oracle-operator/
    ├── api/v1alpha1/
    │   ├── oracledatabase_types.go
    │   └── zz_generated.deepcopy.go
    ├── internal/controller/
    │   ├── oracledatabase_controller.go
    │   └── apiclient.go
    ├── internal/webhook/v1alpha1/
    │   └── oracledatabase_webhook.go
    ├── cmd/main.go
    ├── certs/
    │   ├── tls.crt                # self-signed webhook certificate (10yr)
    │   └── tls.key
    ├── config/crd/bases/
    │   └── oracle.dboperator.io_oracledatabases.yaml
    ├── config/webhook/
    │   └── validating-webhook.yaml
    ├── Dockerfile
    ├── bin/oracle-operator        # compiled binary
    └── go.mod
├── helm/
│   ├── generate-certs.sh         # Generates webhook TLS cert + certs.yaml
│   ├── certs.yaml                # Generated — TLS values for helm install
│   └── oracle-operator/          # Helm chart
│       ├── Chart.yaml
│       ├── values.yaml
│       ├── crds/
│       │   └── oracle.dboperator.io_oracledatabases.yaml
│       └── templates/
│           ├── namespace.yaml
│           ├── serviceaccount.yaml
│           ├── clusterrole.yaml
│           ├── clusterrolebinding.yaml
│           ├── webhook-cert-secret.yaml
│           ├── operator-deployment.yaml
│           ├── operator-service.yaml
│           ├── mock-api-deployment.yaml
│           ├── mock-api-service.yaml
│           ├── mock-api-pvc.yaml
│           ├── validating-webhook.yaml
│           └── team-rbac.yaml
```
