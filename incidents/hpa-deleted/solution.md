# Solution — hpa-deleted

## What happened

The `{{.WorkloadName}}` HPA object was deleted. Deployments do not scale
themselves — without the autoscaler, the replica count is whatever it was when
the object disappeared, no matter how much load arrives. Metrics keep flowing
and the dashboard keeps drawing them; only the replicas line goes flat, which
is easy to read as "no load" if you look at the wrong panel first.

## Diagnosis path

```bash
kubectl get hpa -n {{.WorkloadNamespace}}                        # empty  ← the tell
kubectl get deploy {{.WorkloadName}} -n {{.WorkloadNamespace}} -o jsonpath='{.metadata.annotations}'  # labfault-hpa-deleted marker
labctl traffic start --app {{.WorkloadName}} --profile steady --rps 50   # replicas still do not move
```

## Fix

Re-apply the workload's autoscaler — the drill manifest renders it for
whichever app is bound:

```bash
labctl scenario render autoscaling-basics manifests/hpa.yaml --app {{.WorkloadName}} | kubectl apply -f -
kubectl get hpa {{.WorkloadName}} -n {{.WorkloadNamespace}}   # TARGETS populates within a minute
```

`labctl incident resolve hpa-deleted` rebuilds it from the spec stashed on the
Deployment instead, if you want the escape hatch.

## Real-world parallel

Autoscalers get deleted more often than anyone admits: a namespace cleanup
takes "unused" objects, a GitOps prune removes what a refactor stopped
managing, or a Helm release that owned the HPA is uninstalled while the
workload stays. The app runs fine at first — the outage waits for the next
traffic peak. `kubectl get hpa` belongs in every "it stopped scaling" runbook
before anything about metrics pipelines.
