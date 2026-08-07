// Package attention builds a deliberately narrow human attention brief over queue candidates.
package attention

import (
	"fmt"
	"sort"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

type Source interface {
	List() ([]queue.Directive, error)
}
type Options struct {
	CommitmentFloorComplete bool
	ContextKey              string
}
type Selection struct {
	Directive queue.Directive `json:"directive"`
	Reason    string          `json:"reason"`
}
type Brief struct {
	Selections []Selection `json:"selections"`
}

// Build uses eligibility then lexicographic policy stages, never a global weighted score.
func Build(src Source, now time.Time, opts Options) (Brief, error) {
	ds, err := src.List()
	if err != nil {
		return Brief{}, fmt.Errorf("list attention candidates: %w", err)
	}
	eligible := make([]queue.Directive, 0, len(ds))
	for _, d := range ds {
		if !d.NotBefore.IsZero() && d.NotBefore.After(now) {
			continue
		}
		if d.Attention == nil || (d.Attention.Actor != "human" && d.Attention.Actor != "human+agent" && d.Attention.Actor != "either") {
			continue
		}
		eligible = append(eligible, d)
	}
	var b Brief
	if !opts.CommitmentFloorComplete {
		cs := filter(eligible, "commitment")
		sort.SliceStable(cs, func(i, j int) bool { return commitmentLess(cs[i], cs[j]) })
		if len(cs) > 0 {
			a := cs[0].Attention
			reason := "protected commitment: smallest eligible human action"
			if a.ExternalWaiting {
				reason += "; external party is waiting"
			}
			if a.CompletionGate != "" {
				reason += "; done when " + a.CompletionGate
			}
			b.Selections = append(b.Selections, Selection{cs[0], reason})
		}
	}
	ms := filter(eligible, "maker")
	sort.SliceStable(ms, func(i, j int) bool {
		ai, aj := ms[i].Attention, ms[j].Attention
		im := ai.ContextKey == opts.ContextKey
		jm := aj.ContextKey == opts.ContextKey
		if im != jm {
			return im
		}
		return ms[i].ID < ms[j].ID
	})
	if len(ms) > 0 {
		reason := "maker continuation"
		if ms[0].Attention.ContextKey == opts.ContextKey && opts.ContextKey != "" {
			reason += ": preserves warm context " + opts.ContextKey
		}
		if ms[0].Attention.ResumeRef != "" {
			reason += "; resume from " + ms[0].Attention.ResumeRef
		}
		b.Selections = append(b.Selections, Selection{ms[0], reason})
	}
	return b, nil
}
func filter(ds []queue.Directive, kind string) []queue.Directive {
	var x []queue.Directive
	for _, d := range ds {
		if d.Attention.Kind == kind {
			x = append(x, d)
		}
	}
	return x
}
func commitmentLess(a, b queue.Directive) bool {
	am, bm := a.Attention.EstimatedHumanMinutes, b.Attention.EstimatedHumanMinutes
	if am == 0 {
		am = 1 << 30
	}
	if bm == 0 {
		bm = 1 << 30
	}
	if am != bm {
		return am < bm
	}
	if a.Attention.ExternalWaiting != b.Attention.ExternalWaiting {
		return a.Attention.ExternalWaiting
	}
	return a.ID < b.ID
}
