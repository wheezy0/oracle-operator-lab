# Startup Guide — After Reboot

Everything in this project runs as systemd services and is configured to start automatically on boot. This document covers what to expect, how to verify everything came up correctly, and how to recover if something did not.

---

## Normal Boot — What Should Happen Automatically

On a clean reboot, systemd starts the services in this order:

```
k3s.service
    └── mock-oracle-api.service
            └── oracle-operator.service
```

You should not need to do anything. Give the machine about 30 seconds after login before checking.

---

## Step 1 — Verify All Three Services

```bash
systemctl is-active k3s mock-oracle-api.service oracle-operator.service
```

Expected output:
```
active
active
active
```

If any shows `inactive` or `failed`, jump to the **Troubleshooting** section below.

---

## Step 2 — Verify the k3s Cluster

```bash
kubectl get nodes
```

Expected output:
```
NAME        STATUS   ROLES           AGE   VERSION
framework   Ready    control-plane   Xd    v1.36.2+k3s1
```

The node must show `Ready`. If it shows `NotReady`, k3s is still starting — wait 15 seconds and try again.

---

## Step 3 — Verify the CRD and Webhook Are Registered

```bash
kubectl get crd oracledatabases.oracle.dboperator.io
kubectl get validatingwebhookconfiguration oracle-operator-validating-webhook
```

Both are stored in k3s etcd and persist across reboots. If either is missing, re-install:

```bash
# CRD
kubectl apply -f ~/oracle-operator-lab/oracle-operator/config/crd/bases/oracle.dboperator.io_oracledatabases.yaml

# Webhook configuration
kubectl apply -f ~/oracle-operator-lab/oracle-operator/config/webhook/validating-webhook.yaml
```

---

## Step 4 — Verify the Mock API

```bash
curl -s http://localhost:8080/databases
```

Expected output: a JSON array (empty `[]` or with existing records).

If you get a connection refused error, the mock API service is not up — see troubleshooting below.

## Step 4b — Open the Web Dashboard

Open a browser and go to:

```
http://localhost:8080/ui
```

You should see the dashboard with a green **Live** indicator in the top right. Any existing databases will appear in the table.

---

## Step 5 — Check Existing Database Resources

```bash
kubectl get oracledatabases -A
```

Any databases that existed before the reboot will still be here — they are stored in k3s etcd. Their phase should be `Ready` (the operator reconciles them on startup and syncs with the API backend).

Namespace-scoped team databases can be checked with the team kubeconfigs:

```bash
kubectl --kubeconfig ~/oracle-operator-lab/rbac/finance-dba.kubeconfig get oracledatabases -n team-finance
kubectl --kubeconfig ~/oracle-operator-lab/rbac/devops-dba.kubeconfig  get oracledatabases -n team-devops
```

The RBAC Roles, RoleBindings, and namespaces persist in etcd — no need to re-apply after a normal reboot. See [RBAC.md](RBAC.md) for details.

---

## Everything Is Up — You Are Ready

At this point the full stack is running. Apply new databases, run demos, or use the API directly.

---

## TLS Certificate — Important Note

The webhook server uses a self-signed TLS certificate located at:

```
~/oracle-operator-lab/oracle-operator/certs/tls.crt
~/oracle-operator-lab/oracle-operator/certs/tls.key
```

The certificate is valid for **10 years** from the date it was generated. It does not need to be renewed under normal circumstances.

**If you ever rebuild the k3s cluster from scratch** (e.g. `k3s-uninstall.sh` followed by a fresh install), you must regenerate the certificate and re-apply the webhook configuration, because the `caBundle` embedded in the webhook configuration must match the certificate the operator is using:

```bash
# 1 — Regenerate the certificate
openssl req -x509 -newkey rsa:2048 \
  -keyout ~/oracle-operator-lab/oracle-operator/certs/tls.key \
  -out ~/oracle-operator-lab/oracle-operator/certs/tls.crt \
  -days 3650 -nodes \
  -subj "/CN=oracle-operator-webhook" \
  -addext "subjectAltName=IP:127.0.0.1,DNS:localhost"

# 2 — Rebuild the webhook configuration with the new CA bundle
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

# 3 — Re-apply everything
kubectl apply -f ~/oracle-operator-lab/oracle-operator/config/crd/bases/oracle.dboperator.io_oracledatabases.yaml
kubectl apply -f ~/oracle-operator-lab/oracle-operator/config/webhook/validating-webhook.yaml
sudo systemctl restart oracle-operator.service
```

---

## Troubleshooting

### k3s not starting

```bash
sudo systemctl status k3s
sudo journalctl -u k3s -n 50
```

Try restarting manually:

```bash
sudo systemctl restart k3s
```

---

### mock-oracle-api not starting

```bash
systemctl status mock-oracle-api.service
journalctl -u mock-oracle-api.service -n 30
```

Common causes:
- Port 8080 already in use: `ss -tlnp | grep 8080`
- Python venv missing: check `~/oracle-operator-lab/mock-api/venv/` exists
- Working directory missing: check `~/oracle-operator-lab/mock-api/main.py` exists

Restart manually:

```bash
sudo systemctl restart mock-oracle-api.service
```

---

### oracle-operator not starting

```bash
systemctl status oracle-operator.service
journalctl -u oracle-operator.service -n 30
```

Common causes:
- k3s not fully up yet: wait 15 seconds and restart the operator
- Binary missing: check `~/oracle-operator-lab/oracle-operator/bin/oracle-operator` exists
- Kubeconfig unreadable: check `~/.kube/config` exists and is owned by your user
- TLS cert missing: check `~/oracle-operator-lab/oracle-operator/certs/tls.crt` and `tls.key` exist

Restart manually:

```bash
sudo systemctl restart oracle-operator.service
```

---

### Webhook rejecting all requests unexpectedly

If `kubectl apply` fails with a webhook error even for valid resources:

```bash
# Check operator is running and webhook server is listening
journalctl -u oracle-operator.service -n 20
ss -tlnp | grep 9443
```

If port 9443 is not listening, the operator is not fully started. Restart it:

```bash
sudo systemctl restart oracle-operator.service
```

If the cert and webhook config are out of sync (e.g. after a cert regeneration), re-apply the webhook configuration as described in the **TLS Certificate** section above.

As a last resort, to temporarily disable the webhook while debugging:

```bash
kubectl delete validatingwebhookconfiguration oracle-operator-validating-webhook
```

Re-apply it once the issue is resolved.

---

## Manual Startup (If Autostart Is Broken)

If you need to start everything by hand in the correct order:

```bash
# 1 — Start k3s
sudo systemctl start k3s

# 2 — Wait for node to be Ready
kubectl get nodes
# (repeat until STATUS shows Ready)

# 3 — Start the mock API
sudo systemctl start mock-oracle-api.service

# 4 — Start the operator
sudo systemctl start oracle-operator.service

# 5 — Verify everything
systemctl is-active k3s mock-oracle-api.service oracle-operator.service
kubectl get nodes
kubectl get crd oracledatabases.oracle.dboperator.io
kubectl get validatingwebhookconfiguration oracle-operator-validating-webhook
kubectl get oracledatabases
curl -s http://localhost:8080/databases
# Open http://localhost:8080/ui in a browser for the web dashboard
```

---

## Re-enable Autostart (If Disabled)

If the services were manually disabled at some point:

```bash
sudo systemctl enable k3s
sudo systemctl enable mock-oracle-api.service
sudo systemctl enable oracle-operator.service
```

---

## Live Log Tailing

Once everything is up, use these to monitor activity:

```bash
# Operator activity (reconcile, webhook validation events)
journalctl -u oracle-operator.service -f

# API requests
journalctl -u mock-oracle-api.service -f

# Both at once
journalctl -u oracle-operator.service -u mock-oracle-api.service -f
```
