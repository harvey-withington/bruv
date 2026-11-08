package importer

import (
	"bruv/internal/repo"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// --- Single-card import ---
//
// Trello's card menu (Share → Export JSON) exports one card: the card's own
// fields at the top level, with its labels, checklists and recent actions
// embedded. Unlike a board export it carries no attachment list — the
// attachments are rebuilt from the attachment actions, or fetched from
// Trello's API when the user has saved a key + token.

// trelloCardFile is the wire shape of a single-card export.
type trelloCardFile struct {
	TrelloCard
	IDBoard    string            `json:"idBoard"`
	Labels     []TrelloLabel     `json:"labels"`
	Checklists []TrelloChecklist `json:"checklists"`
	Actions    []TrelloAction    `json:"actions"`
	// Cards is present only on a board export.
	Cards json.RawMessage `json:"cards"`
}

// ParsedCard is a parsed single-card export.
type ParsedCard struct {
	Card       TrelloCard
	Labels     []TrelloLabel
	Checklists map[string]TrelloChecklist
	Actions    []TrelloAction
}

// CardResult summarises a single-card import.
type CardResult struct {
	CardID      string   `json:"card_id"`
	Title       string   `json:"title"`
	Tags        []string `json:"tags"`
	Checklists  int      `json:"checklists"`
	Attachments int      `json:"attachments"`
	// LinkedAttachments names the uploads that couldn't be downloaded
	// (usually: no Trello key + token saved) and were kept as links.
	LinkedAttachments []string `json:"linked_attachments"`
	Comments          int      `json:"comments"`
}

// ParseTrelloCardJSON decodes a Trello single-card export. A board export
// is refused — it goes through ParseTrelloJSON.
func ParseTrelloCardJSON(data []byte) (*ParsedCard, error) {
	var cf trelloCardFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parse trello card JSON: %w", err)
	}
	if len(cf.Cards) > 0 {
		return nil, fmt.Errorf("this is a Trello board export, not a single card — import it from the stream's Import from Trello")
	}
	if cf.ID == "" || cf.IDBoard == "" || cf.IDList == "" {
		return nil, fmt.Errorf("not a Trello card export: missing card, board or list id")
	}
	card := cf.TrelloCard
	checklists := make(map[string]TrelloChecklist, len(cf.Checklists))
	for _, cl := range cf.Checklists {
		checklists[cl.ID] = cl
	}
	// Without idChecklists, every embedded checklist is this card's.
	if len(card.IDChecklists) == 0 {
		for _, cl := range cf.Checklists {
			card.IDChecklists = append(card.IDChecklists, cl.ID)
		}
	}
	return &ParsedCard{Card: card, Labels: cf.Labels, Checklists: checklists, Actions: cf.Actions}, nil
}

// ImportTrelloCard writes a parsed single-card export into the repository
// as a new card at the end of categoryID. Tags are set on the card, but
// adding them to the project's tag list is left to the caller (the card
// service does that for any tag edit).
func ImportTrelloCard(r *repo.Repository, categoryID string, parsed *ParsedCard, opts Options) (*CardResult, error) {
	if r == nil {
		return nil, fmt.Errorf("nil repository")
	}
	if parsed == nil {
		return nil, fmt.Errorf("nil card")
	}
	tc := parsed.Card
	tc.Attachments = resolveCardAttachments(parsed, opts)

	tags := make([]string, 0, len(parsed.Labels))
	for _, l := range parsed.Labels {
		if name := trelloLabelName(l); name != "" {
			tags = append(tags, name)
		}
	}
	carryTagColors(r, parsed.Labels)

	var comments []TrelloAction
	for _, a := range parsed.Actions {
		if a.Type == "commentCard" {
			comments = append(comments, a)
		}
	}

	written, err := writeTrelloCard(r, trelloCardWrite{
		Card:       tc,
		Checklists: parsed.Checklists,
		Comments:   comments,
		Actions:    parsed.Actions,
		Tags:       tags,
		CategoryID: categoryID,
		Position:   -1,
	}, opts)
	if err != nil {
		return nil, err
	}

	checklistCount := 0
	for _, id := range tc.IDChecklists {
		if _, ok := parsed.Checklists[id]; ok {
			checklistCount++
		}
	}
	return &CardResult{
		CardID:            written.CardID,
		Title:             strings.TrimSpace(tc.Name),
		Tags:              tags,
		Checklists:        checklistCount,
		Attachments:       written.Attachments,
		LinkedAttachments: written.LinkedAttachments,
		Comments:          written.Comments,
	}, nil
}

// carryTagColors gives each label's tag its Trello colour, unless the tag
// already has a colour in this repo.
func carryTagColors(r *repo.Repository, labels []TrelloLabel) {
	colors, err := r.GetTagColors()
	if err != nil {
		slog.Warn("trello card import: read tag colours", "err", err)
		return
	}
	for _, l := range labels {
		name := trelloLabelName(l)
		if name == "" || colors[name] != "" {
			continue
		}
		if _, err := r.SetTagColor(name, trelloColorToBRUV(l.Color)); err != nil {
			slog.Warn("trello card import: set tag colour", "tag", name, "err", err)
		}
	}
}

// resolveCardAttachments picks the most complete attachment list available:
// Trello's API (with credentials — it also lists attachments the export's
// actions don't mention, such as ones carried over from a copied card), an
// attachments array if the export has one, else the attachment actions.
func resolveCardAttachments(p *ParsedCard, opts Options) []TrelloAttachment {
	if opts.APIKey != "" && opts.APIToken != "" {
		list, err := fetchCardAttachments(p.Card.ID, opts.APIKey, opts.APIToken)
		if err == nil {
			return list
		}
		slog.Warn("trello card import: list attachments via API", "err", err)
	}
	if p.Card.Attachments != nil {
		return p.Card.Attachments
	}
	return attachmentsFromActions(p.Actions)
}

// fetchCardAttachments lists a card's attachments from Trello's API.
func fetchCardAttachments(trelloCardID, apiKey, apiToken string) ([]TrelloAttachment, error) {
	endpoint := fmt.Sprintf("https://api.trello.com/1/cards/%s/attachments?fields=id,name,url,mimeType,isUpload", url.PathEscape(trelloCardID))
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf(`OAuth oauth_consumer_key="%s", oauth_token="%s"`, apiKey, apiToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}
	var list []TrelloAttachment
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode attachments: %w", err)
	}
	return list, nil
}

// attachmentsFromActions rebuilds the attachment list from the export's
// add/delete actions, oldest first. Trello lists actions newest first.
func attachmentsFromActions(actions []TrelloAction) []TrelloAttachment {
	var list []TrelloAttachment
	for i := len(actions) - 1; i >= 0; i-- {
		a := actions[i]
		id := a.Data.Attachment.ID
		if id == "" {
			continue
		}
		switch a.Type {
		case "addAttachmentToCard":
			if a.Data.Attachment.URL == "" {
				continue
			}
			list = append(list, TrelloAttachment{
				ID:       id,
				Name:     a.Data.Attachment.Name,
				URL:      a.Data.Attachment.URL,
				MimeType: repo.DetectMime(a.Data.Attachment.Name),
				IsUpload: isTrelloUploadURL(a.Data.Attachment.URL),
			})
		case "deleteAttachmentFromCard":
			list = removeAttachment(list, id)
		}
	}
	return list
}

func removeAttachment(list []TrelloAttachment, id string) []TrelloAttachment {
	out := list[:0]
	for _, att := range list {
		if att.ID != id {
			out = append(out, att)
		}
	}
	return out
}

// isTrelloUploadURL reports whether an attachment URL is a file stored by
// Trello (an upload) rather than a link to somewhere else.
func isTrelloUploadURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "trello.com", "api.trello.com":
		return strings.HasPrefix(u.Path, "/1/cards/") && strings.Contains(u.Path, "/attachments/")
	case "trello-attachments.s3.amazonaws.com":
		return true
	}
	return false
}
