package usage

import "time"

// Confidence labels how trustworthy a remaining estimate is.
type Confidence string

const (
	ConfUncalibrated Confidence = "uncalibrated" // no exhaustion observed yet
	ConfLow          Confidence = "low"
	ConfMed          Confidence = "med"
	ConfHigh         Confidence = "high"
)

// Estimate is the per-(provider,window) usage picture at a point in time.
type Estimate struct {
	Provider     string
	WindowClass  string
	Used         int64 // tokens used in the active window (0 if no active window)
	CeilingEst   int64 // learned effective ceiling; 0 when uncalibrated
	RemainingEst int64 // max(0, CeilingEst-Used) when calibrated; 0 otherwise
	Confidence   Confidence
	WindowAnchor time.Time
	WindowReset  time.Time
	WindowActive bool
}

// Estimator turns ledger snapshots into per-provider estimates. PublishedPrior is an
// optional weak per-provider ceiling prior (0 = unknown); it does not by itself calibrate.
type Estimator struct {
	PublishedPrior map[string]int64
}

// Estimate computes the active window's usage and (Task 5) the learned ceiling.
func (e Estimator) Estimate(events []UsageEvent, limits []LimitEvent, provider string, wc WindowClass, now time.Time) Estimate {
	// Filter this provider's events.
	var pev []UsageEvent
	for _, ev := range events {
		if ev.Provider == provider {
			pev = append(pev, ev)
		}
	}
	anchor, reset, active := CurrentWindow(pev, wc, now)

	var used int64
	if active {
		for _, ev := range pev {
			if !ev.Ts.Before(anchor) && ev.Ts.Before(reset) {
				used += ev.Total()
			}
		}
	}

	return Estimate{
		Provider:     provider,
		WindowClass:  wc.Name,
		Used:         used,
		Confidence:   ConfUncalibrated,
		WindowAnchor: anchor,
		WindowReset:  reset,
		WindowActive: active,
	}
}
