# Helm Deployment

This document covers deploying the Oracle Database Operator using Helm — the production-style alternative to the systemd services described in [STARTUP.md](STARTUP.md).

With Helm, everything runs **inside the k3s cluster** as pods:
- The operator runs as a `Deployment` in the `oracle-system` namespace
- The mock API runs as a `Deployment` with a `PersistentVolumeClaim` for SQLite storage
- The webhook uses a Kubernetes `Service` instead of a host port

---

## Prerequisites

| Tool | Install |
|------|---------|
| Docker | Already installed |
| Helm | Already installed at `~/bin/helm` |
| k3s | Running |

---

## Step 1 — Build the Container Images

```bash
cd ~/oracle-operator-lab

# Build operator image (multi-stage Go build — requires Go 1.26 in the builder image)
docker build -t oracle-operator:latest ./oracle-operator/

# Build mock API image
docker build -t mock-oracle-api:latest ./mock-api/
```

---

## Step 2 — Import Images into k3s

k3s uses its own containerd runtime — Docker images must be imported explicitly.

```bash
docker save oracle-operator:latest  | sudo k3s ctr images import -
docker save mock-oracle-api:latest  | sudo k3s ctr images import -
```

Verify:

```bash
sudo k3s ctr images ls | grep -E "oracle-operator|mock-oracle"
```

---

## Step 3 — Stop the Systemd Services

The systemd services use the same ports and the same `ValidatingWebhookConfiguration`. Stop them before installing the Helm chart.

```bash
sudo systemctl stop oracle-operator.service mock-oracle-api.service

# Remove the old URL-based webhook config
kubectl delete validatingwebhookconfiguration oracle-operator-validating-webhook
```

---

## Step 4 — Generate the Webhook TLS Certificate

The webhook needs a new certificate with the in-cluster service DNS name as SAN
(the old cert covered `127.0.0.1`, which no longer applies).

```bash
bash ~/oracle-operator-lab/helm/generate-certs.sh
```

This creates `helm/certs/` with the raw cert files and `helm/certs.yaml` with the base64 values ready for Helm.

> On `helm upgrade`, always pass `--reuse-values` to keep the same certificate. Regenerating it would break the webhook.

---

## Step 5 — Install the Chart

```bash
helm install oracle-operator ~/oracle-operator-lab/helm/oracle-operator \
  -f ~/oracle-operator-lab/helm/certs.yaml
```

Helm creates in order:
1. `oracle-system` namespace
2. Team namespaces (`team-finance`, `team-devops`) and their RBAC
3. ServiceAccount, ClusterRole, ClusterRoleBinding for the operator
4. TLS Secret for the webhook
5. Mock API Deployment + PVC + NodePort Service (port **30080**)
6. Operator Deployment + webhook Service
7. `ValidatingWebhookConfiguration` (service-based)

---

## Step 6 — Verify

```bash
# Check pods are running
kubectl get pods -n oracle-system

# Check the operator logs
kubectl logs -n oracle-system deployment/oracle-operator -f

# Check the webhook is registered
kubectl get validatingwebhookconfiguration oracle-operator-validating-webhook
```

Expected pod output:

```
NAME                              READY   STATUS    RESTARTS
mock-oracle-api-xxxxxxxxx-xxxxx   1/1     Running   0
oracle-operator-xxxxxxxxx-xxxxx   1/1     Running   0
```

The web dashboard is now available at:

```
http://localhost:30080/ui
```

---

## Applying Databases

Works exactly the same as before — the operator watches across all namespaces:

```bash
kubectl apply -f ~/oracle-operator-lab/samples/db-19c-small.yaml
kubectl get oracledatabases -w
```

---

## Upgrading

After changing Go or Python code:

1. Rebuild and re-import the image:
   ```bash
   docker build -t oracle-operator:latest ./oracle-operator/
   docker save oracle-operator:latest | sudo k3s ctr images import -
   ```

2. Restart the pod to pick up the new image:
   ```bash
   kubectl rollout restart deployment/oracle-operator -n oracle-system
   ```

3. For Helm values changes only (no image rebuild):
   ```bash
   helm upgrade oracle-operator ~/oracle-operator-lab/helm/oracle-operator \
     --reuse-values
   ```

---

## Uninstall

```bash
helm uninstall oracle-operator
```

This removes all resources except the CRD (Helm never deletes CRDs on uninstall to avoid data loss). To also remove the CRD:

```bash
kubectl delete crd oracledatabases.oracle.dboperator.io
```

---

## Reverting to Systemd

To go back to the systemd-based setup:

```bash
# Uninstall Helm chart
helm uninstall oracle-operator
kubectl delete crd oracledatabases.oracle.dboperator.io

# Re-apply the CRD and URL-based webhook
kubectl apply -f ~/oracle-operator-lab/oracle-operator/config/crd/bases/oracle.dboperator.io_oracledatabases.yaml
kubectl apply -f ~/oracle-operator-lab/oracle-operator/config/webhook/validating-webhook.yaml
kubectl apply -f ~/oracle-operator-lab/rbac/setup.yaml

# Start systemd services
sudo systemctl start mock-oracle-api.service oracle-operator.service
```

---

## Key Differences vs Systemd Deployment

| | Systemd | Helm |
|--|---------|------|
| Operator runs | On host machine | Inside k3s as a pod |
| Auth to k8s | `~/.kube/config` | ServiceAccount token |
| Webhook endpoint | `https://127.0.0.1:9443` | k8s Service (in-cluster) |
| Mock API port | 8080 | NodePort 30080 |
| SQLite storage | File on disk | PersistentVolumeClaim |
| Deploy update | Rebuild binary + restart service | Rebuild image + rollout restart |
| Packaging | Binary | Container image |
