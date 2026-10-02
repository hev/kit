package trace

// SessionOutcomes uses explicit states: empty/unknown never implies success.
type SessionOutcomes struct {
	PRState        string     `json:"pr_state"`
	PRMerged       string     `json:"pr_merged"`
	PRClosed       string     `json:"pr_closed"`
	PRURL          string     `json:"pr_url"`
	CIState        string     `json:"ci_state"`
	CIConclusions  StringList `json:"ci_conclusions"`
	CIRuns         StringList `json:"ci_runs"`
	Reverted       string     `json:"reverted"`
	RevertState    string     `json:"revert_state"`
	RevertCommits  StringList `json:"revert_commits"`
	RevertUntil    int64      `json:"revert_until"`
	OutcomeChecked int64      `json:"outcome_checked"`
}
