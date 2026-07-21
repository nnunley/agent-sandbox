package usage

import (
	"fmt"
	"io"
	"sort"
	"time"
)

// Report prints a per-provider usage line for every provider present in the ledger,
// computed for the 5h window at now. Visibility-sooner: window facts + used are always
// shown; remaining is a number when calibrated, else an uncalibrated label.
func Report(l *Ledger, est Estimator, now time.Time, w io.Writer) {
	events := l.Events()
	limits := l.Limits()
	seen := map[string]bool{}
	var providers []string
	for _, e := range events {
		if !seen[e.Provider] {
			seen[e.Provider] = true
			providers = append(providers, e.Provider)
		}
	}
	sort.Strings(providers)
	if len(providers) == 0 {
		fmt.Fprintln(w, "no usage recorded yet")
		return
	}
	for _, p := range providers {
		e := est.Estimate(events, limits, p, Window5h, now)
		left := "expired/idle"
		if e.WindowActive {
			left = e.WindowReset.Sub(now).Round(time.Minute).String() + " left"
		}
		var remaining string
		if e.CeilingEst > 0 {
			remaining = fmt.Sprintf("est ~%d remaining (%s)", e.RemainingEst, e.Confidence)
		} else {
			remaining = "est remaining: uncalibrated (learns ceiling after first limit-hit)"
		}
		fmt.Fprintf(w, "%s: %d used this window (resets %s, %s) · %s\n",
			p, e.Used, e.WindowReset.Format("15:04"), left, remaining)
	}
}
