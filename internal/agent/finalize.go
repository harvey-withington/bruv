package agent

// ShouldNotifyForStatus returns true when a run with the given final
// status matches at least one of the triggers the agent is configured
// to notify on. Valid triggers are "success" and "failure"; any other
// string is ignored silently so a corrupt config doesn't nuke notifs.
//
// Pure lookup — extracted from executeAgent's defer so the trigger
// matrix is unit-testable.
func ShouldNotifyForStatus(status string, notifyOn []string) bool {
	for _, trigger := range notifyOn {
		if (trigger == "success" && status == "success") ||
			(trigger == "failure" && status == "failure") {
			return true
		}
	}
	return false
}

// BudgetExceeded reports whether a cost-spent value has reached or
// exceeded the configured budget. Returns false when budget is 0
// (unlimited, the default). Kept as a one-liner function rather than
// inlined so the "0 means unlimited" sentinel has an enforced
// contract + test coverage.
func BudgetExceeded(costSpentUSD, costBudgetUSD float64) bool {
	if costBudgetUSD <= 0 {
		return false
	}
	return costSpentUSD >= costBudgetUSD
}
