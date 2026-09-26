package routing

import (
	"regexp"
	"strings"

	"bruv/internal/config"
)

// Default heuristic keyword lists (spec §Complexity heuristic). A
// router's own lists replace these when set.
var (
	DefaultReasoningKeywords = []string{
		"plan", "design", "why", "compare", "analyse", "analyze", "strategy",
		"refactor", "debug", "architect", "evaluate", "trade-off", "tradeoff",
	}
	DefaultLightKeywords = []string{
		"rename", "typo", "tag", "title", "shorten", "one line", "translate",
	}
)

// Default band thresholds.
const (
	DefaultMediumThreshold = 30
	DefaultHighThreshold   = 60
)

// Signal is one contribution to a complexity score, reported so the
// user can see why a request scored what it did.
type Signal struct {
	Key    string `json:"key"`              // stable id, localized by the UI
	Points int    `json:"points"`           // may be negative
	Detail string `json:"detail,omitempty"` // e.g. the keyword that matched
}

// Assessment is the heuristic's verdict on one request.
type Assessment struct {
	Score         int                   `json:"score"`
	Band          config.ComplexityBand `json:"band"`
	Signals       []Signal              `json:"signals"`
	MessageTokens int                   `json:"message_tokens"`
}

// numberedList matches two or more lines that start "1." / "2)" etc.
var numberedList = regexp.MustCompile(`(?m)^\s*\d+[.)]\s+\S`)

// Assess scores a request 0–100 with the router's keyword lists and
// thresholds. Deterministic: the same request always scores the same.
func Assess(req Request, r config.LLMRouter) Assessment {
	msgTokens := EstimateTokens(req.UserMessage)
	lower := strings.ToLower(req.UserMessage)
	var signals []Signal
	add := func(key string, points int, detail string) {
		signals = append(signals, Signal{Key: key, Points: points, Detail: detail})
	}

	switch {
	case msgTokens > 1500:
		add("message_long", 30, "")
	case msgTokens >= 150:
		add("message_medium", 15, "")
	}

	switch {
	case req.ContextTokens > 32000:
		add("context_large", 20, "")
	case req.ContextTokens > 8000:
		add("context_medium", 10, "")
	}

	switch req.Task {
	case "agent_run":
		add("task_agent", 20, "")
	case "project_chat":
		add("task_project", 10, "")
	}

	if req.ToolsOffered {
		add("tools", 10, "")
	}

	reasoning := r.ReasoningKeywords
	if reasoning == nil {
		reasoning = DefaultReasoningKeywords
	}
	// Each distinct reasoning word adds weight: one "why" is a question,
	// "plan … design … trade-offs" is a planning job.
	if kws := matchedKeywords(lower, reasoning, 2); len(kws) > 0 {
		add("reasoning_keyword", 15*len(kws), strings.Join(kws, ", "))
	}

	if strings.Count(req.UserMessage, "?") >= 2 || len(numberedList.FindAllString(req.UserMessage, 2)) >= 2 {
		add("several_asks", 10, "")
	}

	light := r.LightKeywords
	if light == nil {
		light = DefaultLightKeywords
	}
	if kw := firstKeyword(lower, light); kw != "" {
		add("light_keyword", -10, kw)
	}

	score := 0
	for _, s := range signals {
		score += s.Points
	}
	score = min(max(score, 0), 100)
	if signals == nil {
		signals = []Signal{}
	}
	return Assessment{
		Score:         score,
		Band:          bandFor(score, r.Thresholds),
		Signals:       signals,
		MessageTokens: msgTokens,
	}
}

func bandFor(score int, t config.ComplexityThresholds) config.ComplexityBand {
	medium, high := t.Medium, t.High
	if medium <= 0 {
		medium = DefaultMediumThreshold
	}
	if high <= 0 {
		high = DefaultHighThreshold
	}
	switch {
	case score >= high:
		return config.BandHigh
	case score >= medium:
		return config.BandMedium
	default:
		return config.BandLow
	}
}

// firstKeyword returns the first keyword found in text as a word (or
// phrase), or "".
func firstKeyword(text string, keywords []string) string {
	if kws := matchedKeywords(text, keywords, 1); len(kws) > 0 {
		return kws[0]
	}
	return ""
}

// matchedKeywords returns up to limit keywords found in text as words.
// A keyword must start at a word boundary ("tag" doesn't fire on
// "stage") and end at one, allowing common inflections ("plans",
// "planning", "trade-offs").
func matchedKeywords(text string, keywords []string, limit int) []string {
	var out []string
	for _, kw := range keywords {
		k := strings.ToLower(strings.TrimSpace(kw))
		if k != "" && containsWord(text, k) {
			out = append(out, kw)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}

var inflections = []string{"", "s", "es", "d", "ed", "ing", "ning", "ned", "ging", "ged", "ting", "ted"}

func containsWord(text, word string) bool {
	for i := 0; ; {
		j := strings.Index(text[i:], word)
		if j < 0 {
			return false
		}
		start := i + j
		end := start + len(word)
		if isBoundary(text, start-1) {
			for _, suffix := range inflections {
				if strings.HasPrefix(text[end:], suffix) && isBoundary(text, end+len(suffix)) {
					return true
				}
			}
		}
		i = start + 1
	}
}

func isBoundary(text string, i int) bool {
	if i < 0 || i >= len(text) {
		return true
	}
	c := text[i]
	return !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_')
}
