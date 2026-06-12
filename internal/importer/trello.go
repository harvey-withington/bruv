// Package importer converts external data (Trello exports, BRUV exports)
// into repository state. Keeping this separate from internal/repo keeps the
// mapping/parse logic testable without a live Wails context.
package importer

import (
	"bruv/internal/model"
	"bruv/internal/repo"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// --- Trello JSON shape (the subset we actually read) ---

// TrelloBoard is the top-level document produced by Trello's
// "Show Menu → More → Print and export → JSON" action.
type TrelloBoard struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Desc     string          `json:"desc"`
	Closed   bool            `json:"closed"`
	Lists    []TrelloList    `json:"lists"`
	Cards    []TrelloCard    `json:"cards"`
	Labels   []TrelloLabel   `json:"labels"`
	Actions  []TrelloAction  `json:"actions"`
	Members  []TrelloMember  `json:"members"`
}

type TrelloList struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Closed bool    `json:"closed"`
	Pos    float64 `json:"pos"`
}

type TrelloLabel struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type TrelloMember struct {
	ID       string `json:"id"`
	FullName string `json:"fullName"`
	Username string `json:"username"`
}

type TrelloCheckItem struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"` // "complete" | "incomplete"
	Pos   float64 `json:"pos"`
}

// TrelloChecklist is embedded in the top-level board as well as referenced by
// cards via idChecklists — we look them up by ID to build card blocks.
type TrelloChecklist struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	IDCard     string             `json:"idCard"`
	CheckItems []TrelloCheckItem  `json:"checkItems"`
	Pos        float64            `json:"pos"`
}

type TrelloAttachment struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	MimeType string `json:"mimeType,omitempty"`
	IsUpload bool   `json:"isUpload,omitempty"`
}

type TrelloCard struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Desc         string             `json:"desc"`
	Closed       bool               `json:"closed"`
	IDList       string             `json:"idList"`
	IDLabels     []string           `json:"idLabels"`
	IDChecklists []string           `json:"idChecklists"`
	IDMembers    []string           `json:"idMembers"`
	Due          *time.Time         `json:"due"`
	Pos          float64            `json:"pos"`
	Attachments  []TrelloAttachment `json:"attachments"`
}

// TrelloAction carries comment history and member-add actions. We only read
// comments (type="commentCard") here.
type TrelloAction struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Date time.Time `json:"date"`
	MemberCreator struct {
		FullName string `json:"fullName"`
		Username string `json:"username"`
	} `json:"memberCreator"`
	Data struct {
		Text string `json:"text"`
		Card struct {
			ID string `json:"id"`
		} `json:"card"`
	} `json:"data"`
}

// boardFile is the wire-level envelope because Trello embeds checklists at the
// top level of the board JSON under a `checklists` key.
type boardFile struct {
	TrelloBoard
	Checklists []TrelloChecklist `json:"checklists"`
}

// ParsedBoard bundles a parsed Trello board with its resolved checklists,
// keyed by ID for the importer's convenience.
type ParsedBoard struct {
	Board      TrelloBoard
	Checklists map[string]TrelloChecklist
}

// ParseTrelloJSON decodes a Trello board export. Unknown fields are ignored.
func ParseTrelloJSON(data []byte) (*ParsedBoard, error) {
	var bf boardFile
	if err := json.Unmarshal(data, &bf); err != nil {
		return nil, fmt.Errorf("parse trello JSON: %w", err)
	}
	if bf.Name == "" {
		return nil, fmt.Errorf("not a Trello board export: missing board name")
	}

	checklists := make(map[string]TrelloChecklist, len(bf.Checklists))
	for _, cl := range bf.Checklists {
		checklists[cl.ID] = cl
	}
	return &ParsedBoard{Board: bf.TrelloBoard, Checklists: checklists}, nil
}

// --- Import result + options ---

// ArchiveMode controls what happens to Trello's closed lists and cards.
type ArchiveMode string

const (
	// ArchiveSkip drops closed lists and cards entirely.
	ArchiveSkip ArchiveMode = "skip"
	// ArchiveSeparate places closed cards in a dedicated "Archive" category
	// at the end of the project. Closed lists become categories prefixed
	// with "Archive: ".
	ArchiveSeparate ArchiveMode = "archive"
	// ArchiveInline imports closed cards as normal, no distinction.
	ArchiveInline ArchiveMode = "inline"
)

// Options tunes an import run.
type Options struct {
	Archive  ArchiveMode
	APIKey   string
	APIToken string
}

// Result summarises what the importer did.
type Result struct {
	ProjectSlug    string `json:"project_slug"`
	ProjectName    string `json:"project_name"`
	Categories     int    `json:"categories"`
	Cards          int    `json:"cards"`
	Labels         int    `json:"labels"`
	Comments       int    `json:"comments"`
	Archived       int    `json:"archived"`
	SkippedClosed  int    `json:"skipped_closed"`
}

// --- Trello color → BRUV palette ---

// trelloColorToBRUV maps Trello's fixed label palette to the closest hex
// colour from repo.TagPalette. Empty/unknown colours fall through to the
// first palette entry so the label is still visually distinct.
func trelloColorToBRUV(trelloColor string) string {
	switch strings.ToLower(strings.TrimSpace(trelloColor)) {
	case "green":
		return "#61bd4f"
	case "yellow":
		return "#f2d600"
	case "orange":
		return "#ff9f1a"
	case "red":
		return "#eb5a46"
	case "purple":
		return "#c377e0"
	case "blue":
		return "#0079bf"
	case "sky":
		return "#00c2e0"
	case "lime":
		return "#51e898"
	case "pink":
		return "#ff78cb"
	case "black":
		return "#344563"
	}
	return repo.TagPalette[0]
}

// --- Importer ---

// ImportTrello writes a parsed Trello board into the repository under the
// given brand/stream. A new project is created; existing projects are never
// modified.
func ImportTrello(r *repo.Repository, brandSlug, streamSlug string, parsed *ParsedBoard, opts Options) (*Result, error) {
	if r == nil {
		return nil, fmt.Errorf("nil repository")
	}
	if parsed == nil {
		return nil, fmt.Errorf("nil board")
	}
	board := parsed.Board

	// 1) Create the project (repo auto-creates a default category we don't want).
	project, err := r.CreateProject(brandSlug, streamSlug, board.Name)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	// Remove the auto-created default category so only Trello lists appear.
	if cats, _ := r.ListCategories(brandSlug, streamSlug, project.Slug); len(cats) > 0 {
		for _, c := range cats {
			_ = r.DeleteCategory(brandSlug, streamSlug, project.Slug, c.Slug)
		}
	}

	// Persist the board description (if any) as the project description.
	if board.Desc != "" {
		_, _ = r.UpdateProjectDescription(brandSlug, streamSlug, project.Slug, board.Desc)
	}

	// 1b) Write project-scoped members to members.json.
	projectMembers := make([]model.ProjectMember, 0, len(board.Members))
	for _, tm := range board.Members {
		projectMembers = append(projectMembers, model.ProjectMember{
			ID:       tm.ID,
			FullName: tm.FullName,
			Username: tm.Username,
		})
	}
	if err := r.SaveProjectMembers(brandSlug, streamSlug, project.Slug, projectMembers); err != nil {
		return nil, fmt.Errorf("save project members: %w", err)
	}

	result := &Result{
		ProjectSlug: project.Slug,
		ProjectName: project.Name,
	}

	// 2) Labels — one project Label per Trello label with a non-empty name.
	//    Trello labels with a colour but no name still get imported (using the
	//    colour as the label name) so they remain round-trippable.
	trelloLabelToBRUV := make(map[string]string, len(board.Labels))
	trelloLabelIDToName := make(map[string]string, len(board.Labels))
	for _, tl := range board.Labels {
		name := tl.Name
		if name == "" {
			if tl.Color == "" {
				continue
			}
			name = tl.Color
		}
		updated, err := r.AddProjectLabel(brandSlug, streamSlug, project.Slug, name, trelloColorToBRUV(tl.Color))
		if err != nil {
			return nil, fmt.Errorf("add project label %q: %w", name, err)
		}
		// AddProjectLabel returns the full list; the newly-added one is last.
		if len(updated) > 0 {
			trelloLabelToBRUV[tl.ID] = updated[len(updated)-1].ID
			trelloLabelIDToName[tl.ID] = name
			result.Labels++
		}
	}

	// 3) Lists → Categories. Sort by Trello `pos` so ordering is preserved.
	lists := make([]TrelloList, 0, len(board.Lists))
	closedLists := make([]TrelloList, 0)
	for _, l := range board.Lists {
		if l.Closed {
			closedLists = append(closedLists, l)
			continue
		}
		lists = append(lists, l)
	}
	sort.SliceStable(lists, func(i, j int) bool { return lists[i].Pos < lists[j].Pos })
	sort.SliceStable(closedLists, func(i, j int) bool { return closedLists[i].Pos < closedLists[j].Pos })

	listIDToCategoryID := make(map[string]string, len(lists))

	// Active lists
	position := 0
	for _, tl := range lists {
		cat, err := r.CreateCategory(brandSlug, streamSlug, project.Slug, tl.Name, position)
		if err != nil {
			return nil, fmt.Errorf("create category %q: %w", tl.Name, err)
		}
		listIDToCategoryID[tl.ID] = cat.ID
		position++
		result.Categories++
	}

	// Closed lists, per archive mode
	switch opts.Archive {
	case ArchiveSeparate:
		for _, tl := range closedLists {
			name := "Archive: " + tl.Name
			cat, err := r.CreateCategory(brandSlug, streamSlug, project.Slug, name, position)
			if err != nil {
				return nil, fmt.Errorf("create archive category %q: %w", name, err)
			}
			listIDToCategoryID[tl.ID] = cat.ID
			position++
			result.Categories++
		}
	case ArchiveInline:
		for _, tl := range closedLists {
			cat, err := r.CreateCategory(brandSlug, streamSlug, project.Slug, tl.Name, position)
			if err != nil {
				return nil, fmt.Errorf("create category %q: %w", tl.Name, err)
			}
			listIDToCategoryID[tl.ID] = cat.ID
			position++
			result.Categories++
		}
	case ArchiveSkip:
		// Count closed lists as skipped later when we see their cards.
	}

	// For ArchiveSeparate, closed cards whose list is *active* go into a single
	// catch-all "Archive" category. Lazily created on first use.
	var catchAllArchiveID string
	ensureCatchAllArchive := func() (string, error) {
		if catchAllArchiveID != "" {
			return catchAllArchiveID, nil
		}
		cat, err := r.CreateCategory(brandSlug, streamSlug, project.Slug, "Archive", position)
		if err != nil {
			return "", err
		}
		catchAllArchiveID = cat.ID
		position++
		result.Categories++
		return cat.ID, nil
	}

	// 4) Group comments by card for quick lookup.
	commentsByCard := make(map[string][]TrelloAction)
	for _, a := range board.Actions {
		if a.Type != "commentCard" {
			continue
		}
		commentsByCard[a.Data.Card.ID] = append(commentsByCard[a.Data.Card.ID], a)
	}

	// 5) Cards — sort by pos within each list, then emit.
	cardsSorted := make([]TrelloCard, len(board.Cards))
	copy(cardsSorted, board.Cards)
	sort.SliceStable(cardsSorted, func(i, j int) bool {
		if cardsSorted[i].IDList != cardsSorted[j].IDList {
			return cardsSorted[i].IDList < cardsSorted[j].IDList
		}
		return cardsSorted[i].Pos < cardsSorted[j].Pos
	})

	// Position counter per category so PinCardAt gets a monotonic sequence.
	catPositions := make(map[string]int)

	for _, tc := range cardsSorted {
		// Decide target category
		targetCatID := ""
		isArchived := false

		if tc.Closed {
			switch opts.Archive {
			case ArchiveSkip:
				result.SkippedClosed++
				continue
			case ArchiveSeparate:
				// If the list was active, route to catch-all archive.
				if _, listIsActive := findListActive(lists, tc.IDList); listIsActive {
					id, err := ensureCatchAllArchive()
					if err != nil {
						return nil, fmt.Errorf("create archive category: %w", err)
					}
					targetCatID = id
				} else {
					// List was closed, so it already became an "Archive: X"
					// category above.
					targetCatID = listIDToCategoryID[tc.IDList]
				}
				isArchived = true
			case ArchiveInline:
				targetCatID = listIDToCategoryID[tc.IDList]
				isArchived = true
			}
		} else {
			if cid, ok := listIDToCategoryID[tc.IDList]; ok {
				targetCatID = cid
			} else {
				// Card's list was closed and ArchiveMode=skip.
				result.SkippedClosed++
				continue
			}
		}

		if targetCatID == "" {
			result.SkippedClosed++
			continue
		}

		title := strings.TrimSpace(tc.Name)
		if title == "" {
			title = "(untitled)"
		}

		// Empty type — Trello has no notion of BRUV card types, and forcing one
		// (e.g. "note") would misclassify everything. The user can set types
		// post-import or let an agent suggest them.
		card, err := r.CreateCard("", title)
		if err != nil {
			return nil, fmt.Errorf("create card %q: %w", title, err)
		}

		// Download upload-type attachments into the repo's attachments
		// store; link-type attachments become URL/image blocks below.
		var fileAttachments []model.FileAttachment
		for _, att := range tc.Attachments {
			if att.URL == "" {
				continue
			}
			if !att.IsUpload {
				continue
			}
			fa, err := downloadAndSaveAttachment(r, card.ID, tc.ID, att, opts.APIKey, opts.APIToken)
			if err != nil {
				// Non-fatal: log warning and continue so the import succeeds.
				fmt.Printf("Warning: failed to download attachment %s: %v\n", att.Name, err)
				continue
			}
			fileAttachments = append(fileAttachments, *fa)
		}

		blocks := buildCardBlocks(tc, parsed.Checklists)

		// Due date
		var dueDate *time.Time
		if tc.Due != nil && !tc.Due.IsZero() {
			dueDate = tc.Due
		}

		// Labels: set BOTH
		//   - card.Tags[]   — the string slice that the UI renders as tag chips
		//   - card.Labels[] — internal per-project label IDs, kept in sync
		// Their colours were registered globally when the project labels were
		// created earlier in this import.
		tagNames := make([]string, 0, len(tc.IDLabels))
		labelIDs := make([]string, 0, len(tc.IDLabels))
		for _, tlid := range tc.IDLabels {
			if bid, ok := trelloLabelToBRUV[tlid]; ok {
				labelIDs = append(labelIDs, bid)
			}
			if name, ok := trelloLabelIDToName[tlid]; ok && name != "" {
				tagNames = append(tagNames, name)
			}
		}

		// Update card details directly, overriding timestamps
		card.Blocks = blocks
		card.DueDate = dueDate
		card.Labels = labelIDs
		card.Tags = tagNames
		card.Description = strings.TrimSpace(tc.Desc)
		card.Members = tc.IDMembers
		card.FileAttachments = fileAttachments

		// Extract creation and update dates
		card.CreatedAt = extractTimeFromObjectID(tc.ID)
		card.UpdatedAt = findLatestActionDate(board.Actions, tc.ID, card.CreatedAt)

		if err := r.UpdateCardDirect(card.ID, card); err != nil {
			return nil, fmt.Errorf("update imported card %q: %w", title, err)
		}

		// Pin to the target category.
		pos := catPositions[targetCatID]
		if err := r.PinCardAt(card.ID, targetCatID, pos); err != nil {
			return nil, fmt.Errorf("pin imported card %q: %w", title, err)
		}
		catPositions[targetCatID] = pos + 1
		result.Cards++
		if isArchived {
			result.Archived++
		}

		// Comments → first-class comments on the card, preserving original timestamps.
		for _, act := range commentsByCard[tc.ID] {
			author := act.MemberCreator.FullName
			if author == "" {
				author = act.MemberCreator.Username
			}
			if author == "" {
				author = "Trello"
			}
			if _, err := r.AddCardComment(card.ID, author, act.Data.Text, act.Date); err != nil {
				return nil, fmt.Errorf("add imported comment: %w", err)
			}
			result.Comments++
		}
	}

	return result, nil
}

func findListActive(lists []TrelloList, id string) (TrelloList, bool) {
	for _, l := range lists {
		if l.ID == id {
			return l, true
		}
	}
	return TrelloList{}, false
}

// buildCardBlocks converts a Trello card's payload into BRUV blocks.
// The Trello card's description is NOT included here — it lands on
// card.Description (the intrinsic field) via the caller, not as a
// block. The block list is for structured content only.
func buildCardBlocks(tc TrelloCard, checklists map[string]TrelloChecklist) []model.Block {
	blocks := make([]model.Block, 0, 3)

	// Checklists → one checklist block each
	for _, clID := range tc.IDChecklists {
		cl, ok := checklists[clID]
		if !ok {
			continue
		}
		items := make([]map[string]any, 0, len(cl.CheckItems))
		sortedItems := make([]TrelloCheckItem, len(cl.CheckItems))
		copy(sortedItems, cl.CheckItems)
		sort.SliceStable(sortedItems, func(i, j int) bool { return sortedItems[i].Pos < sortedItems[j].Pos })
		for _, ci := range sortedItems {
			items = append(items, map[string]any{
				"id":   fmt.Sprintf("ck-%s", uuid.New().String()[:8]),
				"text": ci.Name,
				"done": strings.EqualFold(ci.State, "complete"),
			})
		}
		label := cl.Name
		if label == "" {
			label = "Checklist"
		}
		blocks = append(blocks, model.Block{
			ID:    newBlockID(),
			Type:  model.BlockChecklist,
			Label: label,
			Key:   repo.Slugify(label),
			Value: items,
		})
	}

	// Attachments → URL block for link attachments. Upload attachments are skipped
	// here since they land in card.FileAttachments and are rendered in the list.
	for _, att := range tc.Attachments {
		if att.URL == "" {
			continue
		}
		if att.IsUpload {
			continue
		}
		if isImageAttachment(att) {
			blocks = append(blocks, model.Block{
				ID:    newBlockID(),
				Type:  model.BlockImage,
				Label: fallbackAttachmentName(att),
				Key:   "image",
				Value: map[string]any{"url": att.URL, "caption": att.Name},
			})
			continue
		}
		blocks = append(blocks, model.Block{
			ID:    newBlockID(),
			Type:  model.BlockURL,
			Label: fallbackAttachmentName(att),
			Key:   "url",
			Value: att.URL,
		})
	}

	return blocks
}

func fallbackAttachmentName(att TrelloAttachment) string {
	if att.Name != "" {
		return att.Name
	}
	return "Attachment"
}

func isImageAttachment(att TrelloAttachment) bool {
	if strings.HasPrefix(strings.ToLower(att.MimeType), "image/") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(att.URL))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
		return true
	}
	// Fall back on the attachment name's extension if the URL lacks one.
	ext = strings.ToLower(filepath.Ext(att.Name))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
		return true
	}
	return false
}

func newBlockID() string {
	return fmt.Sprintf("blk-%s", uuid.New().String()[:8])
}

// extractTimeFromObjectID extracts the creation time from a 24-character hex MongoDB ObjectID.
// The first 8 hex characters represent the seconds since epoch in big-endian.
func extractTimeFromObjectID(id string) time.Time {
	if len(id) != 24 {
		return time.Now().UTC()
	}
	var sec int64
	_, err := fmt.Sscanf(id[:8], "%x", &sec)
	if err != nil {
		return time.Now().UTC()
	}
	return time.Unix(sec, 0).UTC()
}

// findLatestActionDate returns the latest timestamp of an action on a card, or falls back to defaultTime.
func findLatestActionDate(actions []TrelloAction, cardID string, defaultTime time.Time) time.Time {
	latest := defaultTime
	for _, act := range actions {
		if act.Data.Card.ID == cardID {
			if act.Date.After(latest) {
				latest = act.Date
			}
		}
	}
	return latest
}

// downloadAndSaveAttachment fetches the attachment from the URL and saves it to the repo attachments path.
func downloadAndSaveAttachment(r *repo.Repository, bruvCardID, trelloCardID string, att TrelloAttachment, apiKey, apiToken string) (*model.FileAttachment, error) {
	targetURL := att.URL
	if apiKey != "" && apiToken != "" {
		// If it's a Trello attachment, download it via the Trello API endpoint which redirects to a fresh S3 URL.
		// Appending key/token directly to an expired S3 URL will fail signature checks on Amazon S3.
		if strings.Contains(targetURL, "trello-attachments.s3.amazonaws.com") || strings.Contains(targetURL, "api.trello.com") {
			targetURL = fmt.Sprintf("https://api.trello.com/1/cards/%s/attachments/%s/download/%s", trelloCardID, att.ID, url.PathEscape(att.Name))
		}
	}

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, err
	}

	if apiKey != "" && apiToken != "" {
		// Trello's attachment download endpoint does not support key/token query parameters.
		// It requires authenticating via the Authorization OAuth header.
		// Go's http.Client automatically strips this header on redirecting to S3, which prevents S3 from rejecting the request.
		req.Header.Set("Authorization", fmt.Sprintf(`OAuth oauth_consumer_key="%s", oauth_token="%s"`, apiKey, apiToken))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		// Mask token in error URL if printing targetURL to prevent leaking in logs, but printing body is safe and helpful
		return nil, fmt.Errorf("HTTP status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	dir := filepath.Join(r.Root, "attachments", bruvCardID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	id := fmt.Sprintf("att-%s", uuid.New().String())
	filePath := filepath.Join(dir, id)
	out, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}
	defer out.Close()

	n, err := io.Copy(out, resp.Body)
	if err != nil {
		return nil, err
	}

	mime := att.MimeType
	if mime == "" {
		mime = resp.Header.Get("Content-Type")
	}
	if mime == "" {
		mime = repo.DetectMime(att.Name)
	}

	return &model.FileAttachment{
		ID:      id,
		Name:    fallbackAttachmentName(att),
		Mime:    mime,
		Size:    n,
		AddedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}
