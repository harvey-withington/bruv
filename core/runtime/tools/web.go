package tools

// RunWebTool is the one implementation of the read-only web tools
// (web_fetch, web_search) behind card chat, project chat and agents. They
// touch nothing on the board, so Suggest mode runs them straight away too.

import (
	"bruv/internal/agent"
	"bruv/internal/llm"
	"bruv/internal/model"
)

// IsWebTool reports whether name is one of the web tools.
func IsWebTool(name string) bool { return name == "web_fetch" || name == "web_search" }

// RunWebTool runs a web tool and returns the text for the model plus the
// action record for the transcript. ok is false for any other tool.
func RunWebTool(tc llm.ToolCall) (result string, action *model.ToolAction, ok bool) {
	var err error
	var summary string
	switch tc.Name {
	case "web_fetch":
		url, _ := tc.Arguments["url"].(string)
		result, err = agent.WebFetch(url)
		summary = "fetched " + url
	case "web_search":
		query, _ := tc.Arguments["query"].(string)
		result, err = agent.WebSearch(query)
		summary = "searched: " + query
	default:
		return "", nil, false
	}
	if err != nil {
		result = "error: " + err.Error()
		summary = result
	}
	return result, &model.ToolAction{Tool: tc.Name, Input: tc.Arguments, Result: summary}, true
}
