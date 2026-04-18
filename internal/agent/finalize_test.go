package agent

import "testing"

func TestShouldNotifyForStatus(t *testing.T) {
	cases := []struct {
		name     string
		status   string
		notifyOn []string
		want     bool
	}{
		{"success trigger, success run", "success", []string{"success"}, true},
		{"failure trigger, failure run", "failure", []string{"failure"}, true},
		{"both triggers, success run", "success", []string{"success", "failure"}, true},
		{"both triggers, failure run", "failure", []string{"success", "failure"}, true},
		{"success trigger, failure run", "failure", []string{"success"}, false},
		{"failure trigger, success run", "success", []string{"failure"}, false},
		{"no triggers", "success", nil, false},
		{"cancelled status, no match", "cancelled", []string{"success", "failure"}, false},
		{"garbage triggers ignored", "success", []string{"whenever", "weekdays"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ShouldNotifyForStatus(c.status, c.notifyOn); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestBudgetExceeded(t *testing.T) {
	cases := []struct {
		name          string
		spent, budget float64
		want          bool
	}{
		{"unlimited (budget=0) never exceeds", 9999, 0, false},
		{"negative budget treated as unlimited", 100, -1, false},
		{"spent below budget", 4.99, 5.00, false},
		{"spent exactly at budget — exceeded", 5.00, 5.00, true},
		{"spent over budget", 5.01, 5.00, true},
		{"zero spend under any positive budget", 0, 5.00, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := BudgetExceeded(c.spent, c.budget); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}
