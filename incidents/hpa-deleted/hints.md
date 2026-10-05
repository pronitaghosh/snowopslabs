# Hints — hpa-deleted

## Hint 1
The app is healthy, the metrics flow, and load climbs while replicas sit
still. The autoscaling-basics drill taught you which object turns metrics into
replicas. Does that object still exist for `{{.WorkloadName}}`?

## Hint 2
`kubectl get hpa -n {{.WorkloadNamespace}}` — empty. No autoscaler means no
scaling, however loud the metrics get. Something deleted it; the Deployment
carries a `labfault-hpa-deleted` annotation saying what used to be there.

## Hint 3
Recreate the HPA for `{{.WorkloadName}}` — re-render the drill manifest and
apply it:
`labctl scenario render autoscaling-basics manifests/hpa.yaml --app {{.WorkloadName}} | kubectl apply -f -`.
Then drive load and watch replicas move again.
