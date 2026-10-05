# Solution — hpa-max-pinned

## What happened

The `{{.WorkloadName}}` HPA's `spec.maxReplicas` was pinned to the value of
`spec.minReplicas`. The autoscaler still computes desired replicas from CPU,
but the result is clamped to the ceiling before anything scales — so the
replica count sits at the floor while load climbs, latency degrades, and every
pod-level signal stays green.

Nothing reports this. The HPA is not failing; it is doing exactly what it was
told. The only tell is the flat replicas line under a climbing load line on
the autoscaling dashboard, and `MAXPODS` equalling `MINPODS` in
`kubectl get hpa`.

## Diagnosis path

```bash
kubectl get hpa {{.WorkloadName}} -n {{.WorkloadNamespace}}   # MAXPODS == MINPODS  ← the tell
kubectl get hpa {{.WorkloadName}} -n {{.WorkloadNamespace}} -o jsonpath='{.spec}'
labctl traffic start --app {{.WorkloadName}} --profile steady --rps 50   # replicas still do not move
```

## Fix

```bash
kubectl -n {{.WorkloadNamespace}} patch hpa {{.WorkloadName}} \
  -p '{"spec":{"maxReplicas":4}}'
kubectl get hpa {{.WorkloadName}} -n {{.WorkloadNamespace}}   # MAXPODS above MINPODS
```

Any ceiling above the floor resolves it — the detection check grades the
relationship between the two fields, not one particular number, so restoring
your own original value counts too.

## Real-world parallel

A capped maxReplicas is how autoscaling silently stops: a cost-control PR
lowers the ceiling "temporarily", a copied HPA ships with `maxReplicas: 1`
from a dev overlay, or a GitOps default clamps what a team tuned by hand.
`MAXPODS == MINPODS` in `kubectl get hpa` is the fingerprint — check the
autoscaler's bounds before blaming the metrics pipeline or the app.
