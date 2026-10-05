# Hints — hpa-target-inflated

## Hint 1
The autoscaler exists and its ceiling is fine, yet replicas barely stir under
load. It is not refusing to scale — it is waiting. Something tells it what
"busy enough to scale" means, and that bar is set near the top. Which field
holds the bar?

## Hint 2
`kubectl get hpa {{.WorkloadName}} -n {{.WorkloadNamespace}}` — read the
TARGETS column. Near-saturation before it acts means the CPU percentage it
holds is far above the drill's 60. Describe the HPA and find the utilization
number.

## Hint 3
`kubectl get hpa {{.WorkloadName}} -n {{.WorkloadNamespace}} -o jsonpath='{.spec.metrics}'`
— `averageUtilization` is 95. Lower it back toward 60 with `kubectl patch` or
`kubectl edit`; anything at or under 80 reacts before saturation.
