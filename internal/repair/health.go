package repair

// HealthReport summarizes the state of a collection of stripes.
type HealthReport struct {
	Total        int     // total stripes examined
	Healthy      int     // stripes with all shards present
	Degraded     int     // stripes with some degraded shards but recoverable
	AtRisk       int     // stripes missing shards but still recoverable
	Lost         int     // stripes that are unrecoverable
	AvgRedundancy float64 // average remaining redundancy ratio
}

// Assess evaluates a set of stripes and produces a HealthReport.
func Assess(stripes []*Stripe) *HealthReport {
	report := &HealthReport{Total: len(stripes)}
	if len(stripes) == 0 {
		return report
	}
	totalRedundancy := 0.0
	for _, s := range stripes {
		if s.Validate() != nil {
			continue
		}
		if s.IsHealthy() {
			report.Healthy++
			totalRedundancy += float64(s.ParityShards)
			continue
		}
		avail := s.AvailableCount()
		if avail < s.DataShards {
			report.Lost++
			totalRedundancy += 0
			continue
		}
		missing := len(s.MissingIndices())
		degraded := len(s.DegradedIndices())
		remaining := avail - s.DataShards
		totalRedundancy += float64(remaining)
		if missing > 0 {
			report.AtRisk++
		} else if degraded > 0 {
			report.Degraded++
		}
	}
	if report.Total > 0 {
		report.AvgRedundancy = totalRedundancy / float64(report.Total)
	}
	return report
}

// NeedsUrgentRepair returns true if the health report indicates immediate action
// is needed (any stripe lost or average redundancy below 1.0).
func (r *HealthReport) NeedsUrgentRepair() bool {
	if r.Lost > 0 {
		return true
	}
	return r.AvgRedundancy < 1.0
}

// HealthScore returns a score in [0, 100] representing overall storage health.
// 100 means all stripes are fully healthy; 0 means all data is lost.
func (r *HealthReport) HealthScore() int {
	if r.Total == 0 {
		return 100
	}
	// Weighted: healthy=100, degraded=80, at-risk=50, lost=0.
	score := float64(r.Healthy)*100 + float64(r.Degraded)*80 +
		float64(r.AtRisk)*50 + float64(r.Lost)*0
	avg := score / float64(r.Total)
	if avg > 100 {
		avg = 100
	}
	if avg < 0 {
		avg = 0
	}
	return int(avg)
}

// ClassifyStripe returns a human-readable classification of a single stripe.
func ClassifyStripe(s *Stripe) string {
	if err := s.Validate(); err != nil {
		return "invalid"
	}
	if s.IsHealthy() {
		return "healthy"
	}
	avail := s.AvailableCount()
	if avail < s.DataShards {
		return "lost"
	}
	missing := len(s.MissingIndices())
	if missing > 0 {
		return "at-risk"
	}
	return "degraded"
}
