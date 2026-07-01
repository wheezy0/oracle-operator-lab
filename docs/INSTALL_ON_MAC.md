# Installing on macOS (Apple Silicon)

This guide covers running the Oracle Database Operator project on a Mac with Apple Silicon (M1/M2/M3). The approach uses **Rancher Desktop** (which includes k3s) and the **Helm** deployment path — no systemd.

---

## Prerequisites

| Tool | How to get it |
|------|--------------|
| Rancher Desktop | https://rancherdesktop.io — download the macOS `.dmg` |
| Homebrew | https://brew.sh |
| Helm | `brew install helm` |
| Docker | Included with Rancher Desktop |
| kubectl | Included with Rancher Desktop — added to `~/.rd/bin/` |

---

## Step 1 — Install and Configure Rancher Desktop

1. Download and install Rancher Desktop from https://rancherdesktop.io
2. Launch it and wait for it to fully start (the tray icon stops spinning — can take 1-2 minutes)
3. Open **Preferences → Container Engine** and select **dockerd (moby)**
4. Open **Preferences → Kubernetes** and ensure Kubernetes is enabled

Rancher Desktop automatically installs `kubectl` and `docker` and adds them to your PATH via `~/.rd/bin/`. Restart your terminal after installation.

Install Helm via Homebrew:

```bash
brew install helm
```

Verify everything is working:

```bash
kubectl get nodes    # should show one node in Ready state
helm version --short
docker version
```

> **Important:** Rancher Desktop creates a `rancher-desktop` kubectl context. If you have other clusters (e.g. kind), make sure the right context is active:
> ```bash
> kubectl config current-context
> # If it shows something other than rancher-desktop:
> kubectl config use-context rancher-desktop
> ```

---

## Step 2 — Copy the Project to Your Mac

Transfer the project from the Linux machine or copy it. The project root should be at `~/oracle-operator-lab/`.

```
~/oracle-operator-lab/
├── oracle-operator/
├── mock-api/
├── helm/
├── rbac/
├── samples/
└── docs/
```

---

## Step 3 — Build Container Images

With Rancher Desktop using dockerd, images built with `docker build` are automatically available to k3s — no separate import step needed.

```bash
cd ~/oracle-operator-lab

docker build -t oracle-operator:latest ./oracle-operator/
docker build -t mock-oracle-api:latest ./mock-api/
```

---

## Step 4 — Generate the Webhook TLS Certificate

```bash
bash ~/oracle-operator-lab/helm/generate-certs.sh
```

This creates `helm/certs.yaml` with the base64-encoded certificate values ready for Helm.

> On `helm upgrade`, always pass `--reuse-values` to keep the same certificate.

---

## Step 5 — Install with Helm

```bash
helm install oracle-operator ~/oracle-operator-lab/helm/oracle-operator \
  -f ~/oracle-operator-lab/helm/certs.yaml
```

Watch the pods come up:

```bash
kubectl get pods -n oracle-system -w
```

Expected output once ready:

```
NAME                               READY   STATUS    RESTARTS
mock-oracle-api-xxxxxxxxx-xxxxx    1/1     Running   0
oracle-operator-xxxxxxxxx-xxxxx    1/1     Running   0
```

---

## Step 6 — Verify

```bash
kubectl get crd oracledatabases.oracle.dboperator.io
kubectl get validatingwebhookconfiguration oracle-operator-validating-webhook
kubectl logs -n oracle-system deployment/oracle-operator
```

---

## Step 7 — Create a Test Database

Copy and edit a sample file:

```bash
cp ~/oracle-operator-lab/samples/db-19c-small.yaml /tmp/mydb.yaml
# edit /tmp/mydb.yaml — change name, dbName, owner as needed
kubectl apply -f /tmp/mydb.yaml
kubectl get oracledatabases -w
```

---

## Step 8 — Open the Web Dashboard

```
http://localhost:30080/ui
```

Rancher Desktop exposes NodePort services at `localhost:<port>` automatically.

---

## Step 9 — Browse the Documentation

Install grip to read the markdown docs in your browser:

```bash
pip3 install grip
```

If `grip` is not found after install, run it via Python directly or fix the PATH:

```bash
# Run directly
python3 -m grip ~/oracle-operator-lab/docs/

# Or add pip's bin directory to PATH permanently
echo 'export PATH="$PATH:$(python3 -m site --user-base)/bin"' >> ~/.zshrc
source ~/.zshrc
grip ~/oracle-operator-lab/docs/
```

Then open `http://localhost:6419` in your browser.

---

## Step 10 — RBAC Team Users (Optional)

The `rbac/create-users.sh` script needs access to the k3s CA key, which lives inside the Rancher Desktop VM. Use `rdctl shell` to run it from inside the VM:

```bash
rdctl shell -- mkdir -p /tmp/rbac
cat ~/oracle-operator-lab/rbac/create-users.sh | rdctl shell -- tee /tmp/rbac/create-users.sh > /dev/null
rdctl shell -- sudo bash /tmp/rbac/create-users.sh
```

Copy the generated kubeconfigs back to your Mac:

```bash
rdctl shell -- cat /tmp/rbac/finance-dba.kubeconfig > ~/oracle-operator-lab/rbac/finance-dba.kubeconfig
rdctl shell -- cat /tmp/rbac/devops-dba.kubeconfig  > ~/oracle-operator-lab/rbac/devops-dba.kubeconfig
```

The team namespaces and RBAC roles are already created by Helm (`teams.enabled: true` in values.yaml).

---

## Upgrading After Code Changes

```bash
cd ~/oracle-operator-lab

# Rebuild the changed image
docker build -t oracle-operator:latest ./oracle-operator/
# or
docker build -t mock-oracle-api:latest ./mock-api/

# Restart the pod to pick up the new image
kubectl rollout restart deployment/oracle-operator -n oracle-system
# or
kubectl rollout restart deployment/mock-oracle-api -n oracle-system
```

---

## Uninstall

```bash
helm uninstall oracle-operator

# Optionally remove the CRD (deletes all OracleDatabase resources)
kubectl delete crd oracledatabases.oracle.dboperator.io
```

---

## Differences from the Linux Setup

| | Linux | macOS (Rancher Desktop) |
|--|-------|------------------------|
| k3s runs | Directly on host | Inside Rancher Desktop VM |
| Operator runs | systemd or Helm pod | Helm pod only |
| Image tool | `docker` + `k3s ctr import` | `docker build` (no import needed) |
| Dashboard port | 8080 (systemd) or 30080 (Helm) | 30080 |
| RBAC cert script | `sudo bash create-users.sh` | `rdctl shell -- sudo bash ...` |
| SQLite storage | File on disk (systemd) or PVC | PVC |

---

## Troubleshooting

### `kubectl get nodes` returns connection refused

kubectl is probably pointing at the wrong context. Switch it:

```bash
kubectl config use-context rancher-desktop
```

### Pods stuck in `ImagePullBackOff`

The image wasn't built while Rancher Desktop was using dockerd. Rebuild:

```bash
docker build -t oracle-operator:latest ~/oracle-operator-lab/oracle-operator/
kubectl rollout restart deployment/oracle-operator -n oracle-system
```

### Webhook errors on `kubectl apply`

```bash
kubectl get pods -n oracle-system
kubectl logs -n oracle-system deployment/oracle-operator | tail -20
```

If the cert needs regenerating:

```bash
bash ~/oracle-operator-lab/helm/generate-certs.sh
helm upgrade oracle-operator ~/oracle-operator-lab/helm/oracle-operator -f ~/oracle-operator-lab/helm/certs.yaml
```

### `grip: command not found`

```bash
python3 -m grip ~/oracle-operator-lab/docs/
```

Or fix PATH permanently:

```bash
echo 'export PATH="$PATH:$(python3 -m site --user-base)/bin"' >> ~/.zshrc
source ~/.zshrc
```

### NodePort not accessible at localhost:30080

Wait 30 seconds after Rancher Desktop fully starts, then try again. Check the service:

```bash
kubectl get svc -n oracle-system mock-oracle-api
```
