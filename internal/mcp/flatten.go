package mcp

import (
	"fmt"
	"strings"
)

// FlattenContent reduces a heterogeneous MCP content array to a single
// string suitable for returning as an LLM tool-call result. This is the
// bridge between MCP's typed multi-modal results and BRUV's existing
// single-string tool result contract.
//
// Rules (see protocol.go Content docs):
//
//   - text items contribute their Text verbatim.
//   - image and audio items contribute a placeholder describing the
//     media — we DO NOT embed the base64 data. Current LLMs don't see
//     image data through the tool-result channel, so including it
//     would only bloat context and risk token-budget breakage.
//   - resource_link items contribute "[resource: <uri>]".
//   - resource items with inline text contribute that text directly.
//   - resource items with only a blob contribute a placeholder.
//   - unknown item types contribute a conservative debug string so
//     the LLM can at least see that something was returned.
//
// Items are joined with newlines. Empty output is possible if every
// item was unknown or empty — callers should check.
func FlattenContent(items []Content) string {
	if len(items) == 0 {
		return ""
	}
	var parts []string
	for _, item := range items {
		switch item.Type {
		case "text":
			if item.Text != "" {
				parts = append(parts, item.Text)
			}
		case "image":
			parts = append(parts, fmt.Sprintf("[image: %s, %d bytes base64]", nonEmpty(item.MimeType, "unknown"), len(item.Data)))
		case "audio":
			parts = append(parts, fmt.Sprintf("[audio: %s, %d bytes base64]", nonEmpty(item.MimeType, "unknown"), len(item.Data)))
		case "resource_link":
			label := item.URI
			if item.Name != "" {
				label = item.Name + " (" + item.URI + ")"
			}
			parts = append(parts, "[resource: "+label+"]")
		case "resource":
			if item.Resource == nil {
				parts = append(parts, "[resource: (empty)]")
				continue
			}
			if item.Resource.Text != "" {
				parts = append(parts, item.Resource.Text)
			} else if item.Resource.Blob != "" {
				parts = append(parts, fmt.Sprintf("[resource blob: %s, %d bytes base64]", nonEmpty(item.Resource.MimeType, "unknown"), len(item.Resource.Blob)))
			} else {
				parts = append(parts, "[resource: "+item.Resource.URI+"]")
			}
		default:
			// Unknown content type — flatten whatever useful fields
			// are populated so the LLM can at least see something
			// happened. Preferring Text > URI > Type as a fallback.
			if item.Text != "" {
				parts = append(parts, item.Text)
			} else if item.URI != "" {
				parts = append(parts, "[unknown: "+item.URI+"]")
			} else {
				parts = append(parts, "[unknown content item: "+item.Type+"]")
			}
		}
	}
	return strings.Join(parts, "\n")
}

// nonEmpty returns fallback when s is empty. Tiny helper to keep the
// format strings above readable.
func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
