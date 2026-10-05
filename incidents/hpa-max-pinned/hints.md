# Hints — hpa-max-pinned

## Hint 1
The app is healthy and the metrics flow, so this is not a workload problem —
it is a scaling problem. Load climbs, replicas do not. Something decides how
many replicas the load deserves, and it has stopped deciding. What owns that
decision for `{{.WorkloadName}}`?

## Hint 2
`kubectl get hpa {{.WorkloadName}} -n {{.WorkloadNamespace}}` — compare MINPODS
and MAXPODS. An autoscaler whose ceiling equals its floor cannot scale no
matter what the metrics say. Read the full spec and find which field holds the
ceiling.

## Hint 3
`kubectl get hpa {{.WorkloadName}} -n {{.WorkloadNamespace}} -o yaml` —
`spec.maxReplicas` equals `spec.minReplicas`. Raise the ceiling with
`kubectl patch` or `kubectl edit`; the floor stays where it is.
