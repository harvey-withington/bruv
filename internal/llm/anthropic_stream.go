package llm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// anthropicStreamEvent is the union of the Messages API stream events
// readAnthropicStream acts on; each event uses the fields its type needs.
type anthropicStreamEvent struct {
	Type         string           `json:"type"`
	Index        int              `json:"index"`
	Message      anthropicMessage `json:"message"`       // message_start
	ContentBlock anthropicBlock   `json:"content_block"` // content_block_start
	Delta        struct {
		Type        string `json:"type"`
		Text        string `json:"text"`         // text_delta
		PartialJSON string `json:"partial_json"` // input_json_delta
		StopReason  string `json:"stop_reason"`  // message_delta
	} `json:"delta"`
	Usage anthropicUsage `json:"usage"` // message_delta (cumulative)
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// streamBlock is a content block being assembled from deltas. Builders,
// not string concatenation: a long reply arrives in thousands of deltas.
type streamBlock struct {
	block anthropicBlock
	text  strings.Builder
	input strings.Builder // tool_use input JSON, streamed in fragments
}

// readAnthropicStream assembles a Messages API server-sent-event stream
// into the message a non-streaming call would have returned. onOutput,
// when set, receives a running estimate of output tokens (~4 characters
// a token) of the visible text and tool input streamed so far.
func readAnthropicStream(r io.Reader, onOutput func(int)) (*anthropicMessage, error) {
	msg := &anthropicMessage{}
	var blocks []*streamBlock
	var streamedChars int
	finished := false

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		// "event:" lines repeat the type the data carries; blank lines
		// and ":" comments (keep-alives) carry nothing.
		data, ok := strings.CutPrefix(sc.Text(), "data:")
		if !ok {
			continue
		}
		var ev anthropicStreamEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &ev); err != nil {
			return nil, fmt.Errorf("parse response: %w", err)
		}
		switch ev.Type {
		case "message_start":
			msg.Model = ev.Message.Model
			msg.Usage = ev.Message.Usage
		case "content_block_start":
			for len(blocks) <= ev.Index {
				blocks = append(blocks, &streamBlock{})
			}
			blocks[ev.Index].block = ev.ContentBlock
		case "content_block_delta":
			if ev.Index < 0 || ev.Index >= len(blocks) {
				return nil, fmt.Errorf("parse response: delta for unknown content block %d", ev.Index)
			}
			switch ev.Delta.Type {
			case "text_delta":
				blocks[ev.Index].text.WriteString(ev.Delta.Text)
				streamedChars += len(ev.Delta.Text)
			case "input_json_delta":
				blocks[ev.Index].input.WriteString(ev.Delta.PartialJSON)
				streamedChars += len(ev.Delta.PartialJSON)
			default:
				continue // thinking / signature deltas: hidden, not counted
			}
			if onOutput != nil {
				onOutput(streamedChars / 4)
			}
		case "message_delta":
			msg.StopReason = ev.Delta.StopReason
			if ev.Usage.OutputTokens > 0 {
				msg.Usage.OutputTokens = ev.Usage.OutputTokens
			}
			if ev.Usage.InputTokens > 0 {
				msg.Usage.InputTokens = ev.Usage.InputTokens
			}
		case "message_stop":
			finished = true
		case "error":
			if ev.Error.Type == "overloaded_error" {
				return nil, &RateLimitError{Provider: "anthropic", StatusCode: 529, Body: truncate(ev.Error.Message, 200)}
			}
			return nil, fmt.Errorf("Anthropic stream error (%s): %s", ev.Error.Type, truncate(ev.Error.Message, 200))
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if !finished {
		return nil, fmt.Errorf("read response: stream ended before the message finished")
	}

	for _, b := range blocks {
		block := b.block
		switch block.Type {
		case "text":
			block.Text += b.text.String()
		case "tool_use":
			if b.input.Len() > 0 {
				var input map[string]any
				if err := json.Unmarshal([]byte(b.input.String()), &input); err != nil {
					// Cut off mid-arguments by the output cap: the call
					// never arrived whole, so it is dropped rather than
					// run with half its input. Anywhere else it's a
					// malformed response.
					if anthropicStopReason(msg.StopReason) == StopMaxTokens {
						continue
					}
					return nil, fmt.Errorf("anthropic: tool call %q has unparseable arguments: %w", block.Name, err)
				}
				block.Input = input
			}
			if block.Input == nil {
				block.Input = map[string]any{}
			}
		}
		msg.Content = append(msg.Content, block)
	}
	return msg, nil
}
