package main

// ResumeAudit is the authoritative context reconstructed before a new run continues a thread
// (STORY-0029 AC-4a). Branch/Workspace/OpenQuestions/ResumeSummary are SOFT state from the Thread;
// LastDiff/LastGrade are AUTHORITATIVE, taken from the last run's Result (never from lean-ctx).
type ResumeAudit struct {
	ThreadID      string
	Branch        string
	Workspace     string
	ResumeSummary ResumeSummary
	LastVerified  string // the thread's LastVerifiedState
	OpenQuestions []string
	LastDiff      []byte // from Result.PatchData (nil if no prior Result)
	LastGrade     string // "pass" | "fail" | "" (empty when last == nil)
}

// ReconstructResumeAudit assembles the resume context for threadID before resuming it
// (STORY-0029 AC-4a). Soft fields come from the ThreadStore; LastDiff/LastGrade come from the
// last run's Result (authoritative). `last` may be nil (no prior run → LastDiff nil, LastGrade "").
// Returns ok=false when the thread is unknown.
// LastGrade is derived via the existing daemon `passed(last, nil)` helper: ""=no result,
// "pass"=passed, "fail"=otherwise.
func ReconstructResumeAudit(store *ThreadStore, threadID string, last *Result) (ResumeAudit, bool) {
	th, ok := store.Get(threadID)
	if !ok {
		return ResumeAudit{}, false
	}

	audit := ResumeAudit{
		ThreadID:      th.ID,
		Branch:        th.CurrentBranch,
		Workspace:     th.CurrentWorkspace,
		ResumeSummary: th.ResumeSummary,
		LastVerified:  th.LastVerifiedState,
		OpenQuestions: th.OpenQuestions,
	}

	if last != nil {
		audit.LastDiff = last.PatchData
		if passed(last, nil) {
			audit.LastGrade = "pass"
		} else {
			audit.LastGrade = "fail"
		}
	}

	return audit, true
}

// ContinueRun returns a value-copy of base whose checkout Ref is set to the thread's current branch,
// so a resumed run CONTINUES the existing branch/workspace by default instead of treating it as a
// blank slate (STORY-0029 AC-3). If the audit has no Branch, base is returned unchanged. NOTE: the
// copy is shallow — base.Cmd/base.Env (slice/map) are aliased with the returned Task; callers must
// not mutate them in place. Only Ref is changed.
func (a ResumeAudit) ContinueRun(base Task) Task {
	if a.Branch == "" {
		return base
	}
	base.Ref = a.Branch
	return base
}
