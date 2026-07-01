# Namespace Isolation and RBAC

This project includes namespace-scoped access control so that different teams can only manage databases in their own namespace. Each team gets a dedicated namespace, a scoped Role, and a personal kubeconfig with a client certificate.

---

## Namespaces and Users

| Namespace | User | Use case |
|-----------|------|----------|
| `team-finance` | `finance-dba` | Finance team databases |
| `team-devops` | `devops-dba` | DevOps/CI databases |

---

## Files

```
oracle-operator-lab/rbac/
├── setup.yaml               # Namespaces + Roles + RoleBindings
├── create-users.sh          # Generates client certs and kubeconfigs (requires sudo)
├── finance-dba.kubeconfig   # Generated kubeconfig for finance-dba
└── devops-dba.kubeconfig    # Generated kubeconfig for devops-dba
```

---

## What `setup.yaml` Creates

- Two namespaces: `team-finance` and `team-devops`
- A `Role` named `oracle-dba` in each namespace with these permissions:

| Resource | Verbs |
|----------|-------|
| `oracledatabases` | get, list, watch, create, update, patch, delete |
| `oracledatabases/status` | get |

- A `RoleBinding` in each namespace binding the role to the team's username

Apply it:

```bash
kubectl apply -f ~/oracle-operator-lab/rbac/setup.yaml
```

---

## Generating User Certificates and Kubeconfigs

User authentication uses **client certificates** signed by the k3s internal CA. The script generates a private key, signs a certificate (CN=username, O=namespace), and writes a ready-to-use kubeconfig file with everything embedded.

```bash
sudo bash ~/oracle-operator-lab/rbac/create-users.sh
```

This creates `finance-dba.kubeconfig` and `devops-dba.kubeconfig` in the `rbac/` directory.

> **Requires sudo** — the script needs read access to the k3s CA key at `/var/lib/rancher/k3s/server/tls/client-ca.key`.

The certificates are valid for **10 years**. If the k3s cluster is rebuilt from scratch (new CA), run the script again to re-sign the certificates against the new CA.

---

## Using the Team Kubeconfigs

```bash
# Finance team — can only access team-finance namespace
kubectl --kubeconfig ~/oracle-operator-lab/rbac/finance-dba.kubeconfig \
  get oracledatabases -n team-finance

# DevOps team — can only access team-devops namespace
kubectl --kubeconfig ~/oracle-operator-lab/rbac/devops-dba.kubeconfig \
  get oracledatabases -n team-devops
```

**Isolation is enforced — cross-namespace access is denied:**

```bash
kubectl --kubeconfig ~/oracle-operator-lab/rbac/finance-dba.kubeconfig \
  get oracledatabases -n team-devops
# Error from server (Forbidden): ...
```

---

## Sample Databases in Namespaced Resources

Two sample YAML files are provided for the namespaced teams:

```bash
# Apply as admin
kubectl apply -f ~/oracle-operator-lab/samples/ns-finance-db.yaml   # team-finance
kubectl apply -f ~/oracle-operator-lab/samples/ns-devops-db.yaml    # team-devops

# View as the team user
kubectl --kubeconfig ~/oracle-operator-lab/rbac/finance-dba.kubeconfig \
  get oracledatabases -n team-finance

kubectl --kubeconfig ~/oracle-operator-lab/rbac/devops-dba.kubeconfig \
  get oracledatabases -n team-devops
```

---

## How Authentication Works

k8s authenticates users by the **Common Name (CN)** of the client certificate. The **Organization (O)** field maps to a k8s group, but in this setup it is set to the namespace name for clarity.

```
CN=finance-dba  →  username in k8s RBAC
O=team-finance  →  group (not used for binding in this setup)
```

The `RoleBinding` binds the `oracle-dba` Role to the subject `finance-dba` (by username) in the `team-finance` namespace. Only requests presenting a certificate with `CN=finance-dba` are allowed.

---

## Admin vs Team Access

The admin kubeconfig (`~/.kube/config`) has full cluster-admin access and can manage resources in all namespaces. Team kubeconfigs are namespace-scoped and limited to `OracleDatabase` resources only.

```bash
# Admin can see everything
kubectl get oracledatabases -A

# Team user can only see their namespace
kubectl --kubeconfig ~/oracle-operator-lab/rbac/finance-dba.kubeconfig \
  get oracledatabases -n team-finance
```

---

## After a Reboot

The namespaces, Roles, and RoleBindings are stored in k3s etcd and persist across reboots — no need to re-apply `setup.yaml`. The kubeconfig files are static files on disk and also persist.

The only case requiring action is a full k3s cluster rebuild — in that case, re-apply `setup.yaml` and re-run `create-users.sh` to sign new certificates against the new CA.
