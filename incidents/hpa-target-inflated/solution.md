# Solution — hpa-target-inflated

## What happened

The `{{.WorkloadName}}` HPA's CPU `averageUtilization` was raised to 95. The
autoscaler still works — it just waits until pods are nearly saturated before
adding replicas. Under load the service queues behind too few pods first and
scales second, so tail latency pulls away from the typical request while the
replica count looks merely slow rather than stuck.

This is the hardest of the three HPA faults to notice: everything exists,
every number is finite, and `kubectl get hpa` shows a TARGETS column that
reads as healthy until you compare the target against what it should be.

## Diagnosis path

```bash
kubectl get hpa {{.WorkloadName}} -n {{.WorkloadNamespace}}   # TARGETS: 45%/95%  ← the tell
kubectl get hpa {{.WorkloadName}} -n {{.WorkloadNamespace}} -o jsonpath='{.spec.metrics}'
```

## Fix

```bash
kubectl -n {{.WorkloadNamespace}} patch hpa {{.WorkloadName}} \
  -p '{"spec":{"metrics":[{"type":"Resource","resource":{"name":"cpu","target":{"type":"Utilization","averageUtilization":60}}}]}}'
```

Any target at or under 80 resolves it — the detection check grades sanity,
not one particular number, so your own tuned value counts too.

## Real-world parallel

Inflated targets are how cost anxiety becomes latency: someone raises the
target to "use what we pay for", a copied HPA ships with the source
environment's headroom assumptions, or a well-meaning tune chases fewer
replicas without watching the tail. CPU past ~80% is queueing territory, not
headroom — the autoscaling-basics dashboard's p99/p50 panel shows exactly when
the tail pulls away.
