# Cluster access configuration examples

Thule project config (`thule.conf`) intentionally does **not** carry secret material.
Use `clusterRef` in `thule.conf` and resolve credentials out-of-band in Thule control-plane config.

## 1) Per-project `thule.conf` (GKE target)

```yaml
version: v1
project: payments
clusterRef: gke-prod-eu1
namespace: payments
render:
  mode: flux
  path: manifests
policy:
  profile: strict
```

## 2) Per-project `thule.conf` (bare-metal target)

```yaml
version: v1
project: observability
clusterRef: bm-prod-west
namespace: observability
render:
  mode: kustomize
  path: .
policy:
  profile: baseline
```

## 3) Example control-plane cluster credential catalog (outside `thule.conf`)

Example-only format showing how one Thule deployment can resolve both GKE and bare-metal credentials by `clusterRef`.

```yaml
# .thule/clusters.yaml (example operational config, not project config)
clusters:
  - clusterRef: gke-prod-eu1
    type: gke
    endpoint: https://34.120.10.20
    auth:
      mode: workloadIdentity
      gcpServiceAccount: thule-reader-prod@my-project.iam.gserviceaccount.com
      workloadIdentityProvider: projects/123456789/locations/global/workloadIdentityPools/thule/providers/gitlab
    authorization:
      allowedRepos:
        - platform/gitops
      allowedNamespaces:
        - payments
        - billing

  - clusterRef: bm-prod-west
    type: kubeconfig
    endpoint: https://k8s-api.prod-west.example.net:6443
    auth:
      mode: staticSecretRef
      # Secret manager path that stores read-only kubeconfig/service account token
      secretRef: secret/data/thule/clusters/bm-prod-west
    authorization:
      allowedRepos:
        - platform/gitops
      allowedNamespaces:
        - observability
        - monitoring
```

## Notes

- Keep cluster credentials in a secret manager (Vault / cloud secret service), not in Git.
- For GKE, prefer Workload Identity/OIDC over long-lived key files.
- For bare metal, use a read-only Kubernetes service account token or client cert with namespace-scoped RBAC.
