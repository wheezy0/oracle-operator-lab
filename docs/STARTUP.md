# Startup Guide — After Reboot

The operator and mock API run as pods inside the Kubernetes cluster, managed by Helm. On reboot, k3s starts automatically and Kubernetes restarts the pods. You should not need to do anything manually.

---

## Step 1 — Verify k3s Is Running

```bash
kubectl get nodes
```

Expected output:
```
NAME        STATUS   ROLES           AGE   VERSION
framework   Ready    control-plane   Xd    v1.36.2+k3s1
```

The node must show `Ready`. If it shows `NotReady`, k3s is still starting — wait 15 seconds and try again.

If kubectl fails with connection refused:
```bash
sudo systemctl status k3s
sudo systemctl start k3s
```

---

## Step 2 — Verify the Pods Are Running

```bash
kubectl get pods -n oracle-system
```

Expected output:
```
NAME                               READY   STATUS    RESTARTS
mock-oracle-api-xxxxxxxxx-xxxxx    1/1     Running   0
oracle-operator-xxxxxxxxx-xxxxx    1/1     Running   0
```

If a pod shows `Pending` or `CrashLoopBackOff`:
```bash
kubectl describe pod -n oracle-system <pod-name>
kubectl logs -n oracle-system <pod-name>
```

---

## Step 3 — Verify the CRD and Webhook Are Registered

```bash
kubectl get crd oracledatabases.oracle.dboperator.io
kubectl get validatingwebhookconfiguration oracle-operator-validating-webhook
```

Both are stored in k3s etcd and persist across reboots. If either is missing, re-install the Helm chart:

```bash
helm install oracle-operator ~/oracle-operator-lab/helm/oracle-operator \
  -f ~/oracle-operator-lab/helm/certs.yaml
```

---

## Step 4 — Verify the Mock API

```bash
curl -s http://localhost:30080/databases
```

Expected output: a JSON array (empty `[]` or with existing records).

---

## Step 5 — Open the Web Dashboard

```
http://localhost:30080/ui
```

You should see the dashboard with a green **Live** indicator in the top right.

---

## Step 6 — Check Existing Database Resources

```bash
kubectl get oracledatabases -A
```

Databases that existed before the reboot will still be here — they are stored in k3s etcd. Their phase should be `Ready` once the operator reconciles on startup.

Team databases can be checked with the team kubeconfigs:

```bash
kubectl --kubeconfig ~/oracle-operator-lab/rbac/finance-dba.kubeconfig get oracledatabases -n team-finance
kubectl --kubeconfig ~/oracle-operator-lab/rbac/devops-dba.kubeconfig  get oracledatabases -n team-devops
```

---

## Everything Is Up — You Are Ready

At this point the full stack is running. Apply new databases, run demos, or use the API directly.

---

## Live Log Tailing

```bash
# Operator logs (reconcile events, webhook validation)
kubectl logs -n oracle-system deployment/oracle-operator -f

# Mock API logs (HTTP requests)
kubectl logs -n oracle-system deployment/mock-oracle-api -f

# Both at once (requires two terminals, or use a multiplexer)
kubectl logs -n oracle-system deployment/oracle-operator -f &
kubectl logs -n oracle-system deployment/mock-oracle-api -f
```

---

## Troubleshooting

### k3s not starting

```bash
sudo systemctl status k3s
sudo journalctl -u k3s -n 50
sudo systemctl restart k3s
```

### Pod stuck in CrashLoopBackOff

```bash
kubectl describe pod -n oracle-system <pod-name>
kubectl logs -n oracle-system <pod-name> --previous
```

### Webhook rejecting all requests unexpectedly

Check the operator pod is healthy:

```bash
kubectl get pods -n oracle-system
kubectl logs -n oracle-system deployment/oracle-operator | tail -20
```

If the certificate needs regenerating:

```bash
bash ~/oracle-operator-lab/helm/generate-certs.sh
helm upgrade oracle-operator ~/oracle-operator-lab/helm/oracle-operator \
  -f ~/oracle-operator-lab/helm/certs.yaml
```

### Helm chart not installed

If the pods don't exist at all after reboot, the Helm chart was never installed or was uninstalled. Re-install:

```bash
bash ~/oracle-operator-lab/helm/generate-certs.sh
helm install oracle-operator ~/oracle-operator-lab/helm/oracle-operator \
  -f ~/oracle-operator-lab/helm/certs.yaml
```

See [HELM.md](HELM.md) for the full install procedure.
