package usage

// FleetCap is the fleet spend ceiling for the active window: the working ceiling
// minus the reserved interactive headroom (reservePct of the ceiling). Returns 0
// when there is no working ceiling (CeilingEst <= 0), which the caller reads as
// "no cap — fail open". reservePct is assumed already validated to [0,1].
func (e Estimate) FleetCap(reservePct float64) int64 {
	if e.CeilingEst <= 0 {
		return 0
	}
	c := float64(e.CeilingEst) * (1 - reservePct)
	if c < 0 {
		c = 0
	}
	return int64(c)
}

// AllowFleet reports whether a new fleet call is within budget. With no working
// ceiling (uncalibrated AND no PublishedPrior, so CeilingEst == 0) it fails open.
// This is exactly the locked "prior -> enforce, no prior -> fail open" rule: the
// Estimator folds a configured PublishedPrior into CeilingEst, so a present prior
// yields a non-zero ceiling and enforces here.
func (e Estimate) AllowFleet(reservePct float64) bool {
	if e.CeilingEst <= 0 {
		return true
	}
	return e.Used < e.FleetCap(reservePct)
}
