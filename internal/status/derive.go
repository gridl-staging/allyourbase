package status

import "time"

const (
	// SlowProbeThreshold marks an otherwise-healthy probe as degraded due to latency.
	SlowProbeThreshold = time.Second
)

// DeriveStatus rolls up per-service probe results into one overall service status.
func DeriveStatus(results []ProbeResult) ServiceStatus {
	if len(results) == 0 {
		return Operational
	}

	unhealthy := 0
	allHealthy := true
	hasSlow := false
	for _, r := range results {
		if !r.Healthy {
			unhealthy++
			allHealthy = false
		}
		if r.Healthy && r.Latency > SlowProbeThreshold {
			hasSlow = true
		}
	}

	if allHealthy {
		if hasSlow {
			return Degraded
		}
		return Operational
	}

	healthy := len(results) - unhealthy
	if unhealthy > healthy {
		return MajorOutage
	}
	return PartialOutage
}
