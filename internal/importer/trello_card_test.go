package importer

import (
	"bruv/internal/model"
	"bruv/internal/repo"
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// A trimmed single-card export in the shape Trello's card menu produces:
// no attachments array (attachments only appear as actions, one of them
// later deleted), checklists whose idChecklists order differs from their
// display order, labels embedded, and a comment action.
const fixtureCard = `{
  "id": "69b1160f42548362c382874c",
  "idBoard": "6911465e5a0ee615e42a65ac",
  "idList": "680c8ca98d24e6e53f22b803",
  "name": "Trip Planning Phnom Penh Mar 2026",
  "desc": "",
  "closed": false,
  "due": null,
  "idChecklists": ["chk-bag", "chk-leave"],
  "labels": [
    {"id": "lbl-1", "name": "Type: Event / Trip", "color": "purple"},
    {"id": "lbl-2", "name": "", "color": "green"}
  ],
  "checklists": [
    {"id": "chk-bag", "name": "Packing - Shoulder Bag", "pos": 33248, "checkItems": [
      {"id": "ci-2", "name": "Pens", "state": "complete", "pos": 2760},
      {"id": "ci-1", "name": "PASSPORT!", "state": "complete", "pos": 2048}
    ]},
    {"id": "chk-leave", "name": "Preparation to Leave", "pos": 320, "checkItems": [
      {"id": "ci-3", "name": "SIM cards / roaming", "state": "incomplete", "pos": 1}
    ]}
  ],
  "actions": [
    {"id": "a4", "type": "commentCard", "date": "2026-03-30T05:18:30.919Z",
     "memberCreator": {"fullName": "Harv", "username": "harveyathome"},
     "data": {"text": "Flight moved.", "card": {"id": "69b1160f42548362c382874c"}}},
    {"id": "a3", "type": "deleteAttachmentFromCard", "date": "2026-03-23T19:11:29.006Z",
     "data": {"attachment": {"id": "att-lease", "name": "Lease"}, "card": {"id": "69b1160f42548362c382874c"}}},
    {"id": "a2", "type": "addAttachmentToCard", "date": "2026-03-14T16:01:08.061Z",
     "data": {"attachment": {"id": "att-pdf", "name": "booking-details.pdf",
       "url": "https://trello.com/1/cards/69b1160f42548362c382874c/attachments/att-pdf/download/booking-details.pdf"},
       "card": {"id": "69b1160f42548362c382874c"}}},
    {"id": "a1", "type": "addAttachmentToCard", "date": "2026-03-11T13:52:04.911Z",
     "data": {"attachment": {"id": "att-lease", "name": "Lease",
       "url": "https://trello.com/1/cards/69b1160f42548362c382874c/attachments/att-lease/download/lease.jpg"},
       "card": {"id": "69b1160f42548362c382874c"}}}
  ]
}`

// failingTransport answers every request with 401, as Trello does for an
// upload fetched without credentials.
type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("unauthorized")), Header: make(http.Header)}, nil
}

// apiTransport serves Trello's attachment list for the card and a body
// for every download, recording the URLs asked for.
type apiTransport struct{ requested []string }

func (t *apiTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.requested = append(t.requested, req.URL.String())
	body := "file bytes"
	if strings.HasSuffix(req.URL.Path, "/attachments") {
		body = `[
		  {"id": "att-cover", "name": "cover.png", "url": "https://trello.com/1/cards/69b1160f42548362c382874c/attachments/att-cover/download/cover.png", "mimeType": "image/png", "isUpload": true},
		  {"id": "att-pdf", "name": "booking-details.pdf", "url": "https://trello.com/1/cards/69b1160f42548362c382874c/attachments/att-pdf/download/booking-details.pdf", "mimeType": "application/pdf", "isUpload": true},
		  {"id": "att-link", "name": "Map", "url": "https://maps.example.com/pp", "isUpload": false}
		]`
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader([]byte(body))), Header: make(http.Header)}, nil
}

func useTransport(t *testing.T, rt http.RoundTripper) {
	t.Helper()
	old := http.DefaultClient.Transport
	http.DefaultClient.Transport = rt
	t.Cleanup(func() { http.DefaultClient.Transport = old })
}

// importTarget makes a project with one category and returns its id.
func importTarget(t *testing.T, r *repo.Repository) string {
	t.Helper()
	p, err := r.CreateProject("test-brand", "main", "Travel")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	cat, err := r.CreateCategory("test-brand", "main", p.Slug, "Trips - Done", 0)
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	return cat.ID
}

func importFixtureCard(t *testing.T, r *repo.Repository, catID string, opts Options) (*CardResult, *model.Card) {
	t.Helper()
	parsed, err := ParseTrelloCardJSON([]byte(fixtureCard))
	if err != nil {
		t.Fatalf("ParseTrelloCardJSON: %v", err)
	}
	result, err := ImportTrelloCard(r, catID, parsed, opts)
	if err != nil {
		t.Fatalf("ImportTrelloCard: %v", err)
	}
	card, err := r.GetCard(result.CardID)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	return result, card
}

func blocksOfType(card *model.Card, typ string) []model.Block {
	var out []model.Block
	for _, b := range card.Blocks {
		if b.Type == typ {
			out = append(out, b)
		}
	}
	return out
}

func TestImportTrelloCard_FromActions(t *testing.T) {
	r := setupImporterRepo(t) // mock transport: every download succeeds
	catID := importTarget(t, r)
	result, card := importFixtureCard(t, r, catID, Options{})

	if card.Title != "Trip Planning Phnom Penh Mar 2026" {
		t.Errorf("title = %q", card.Title)
	}
	pins, err := r.ListCardsInCategory(catID)
	if err != nil || len(pins) != 1 || pins[0].CardID != card.ID {
		t.Errorf("card not pinned to the category: %v %v", pins, err)
	}

	// Tags: named label by name, colour-only label by colour; Trello
	// colours carried into the repo's tag colours.
	if strings.Join(card.Tags, ",") != "Type: Event / Trip,green" {
		t.Errorf("tags = %v", card.Tags)
	}
	colors, _ := r.GetTagColors()
	if colors["Type: Event / Trip"] != "#c377e0" {
		t.Errorf("tag colour = %q, want Trello purple", colors["Type: Event / Trip"])
	}

	// Checklists in Trello's display order (by pos), items by pos too.
	cls := blocksOfType(card, model.BlockChecklist)
	if len(cls) != 2 || cls[0].Label != "Preparation to Leave" || cls[1].Label != "Packing - Shoulder Bag" {
		t.Fatalf("checklists = %+v", cls)
	}
	items, _ := cls[1].Value.([]any)
	if len(items) != 2 {
		t.Fatalf("shoulder bag items = %#v", cls[1].Value)
	}
	if first, _ := items[0].(map[string]any); first["text"] != "PASSPORT!" || first["done"] != true {
		t.Errorf("first item = %#v", items[0])
	}
	if result.Checklists != 2 {
		t.Errorf("result.Checklists = %d", result.Checklists)
	}

	// The deleted attachment is gone; the remaining upload is downloaded.
	if len(card.FileAttachments) != 1 || card.FileAttachments[0].Name != "booking-details.pdf" {
		t.Errorf("file attachments = %+v", card.FileAttachments)
	}
	if len(result.LinkedAttachments) != 0 {
		t.Errorf("linked = %v", result.LinkedAttachments)
	}

	cf, err := r.LoadComments(card.ID)
	if err != nil || len(cf.Comments) != 1 || cf.Comments[0].Author != "Harv" || cf.Comments[0].Text != "Flight moved." {
		t.Errorf("comments = %+v %v", cf, err)
	}
	if card.CreatedAt.Year() != 2026 || card.UpdatedAt.Month() != 3 || card.UpdatedAt.Day() != 30 {
		t.Errorf("dates: created %v updated %v", card.CreatedAt, card.UpdatedAt)
	}
}

// Without credentials Trello refuses the download; the upload is kept as
// a link block instead of being lost, and the result names it.
func TestImportTrelloCard_UndownloadableUploadBecomesLink(t *testing.T) {
	r := setupImporterRepo(t)
	useTransport(t, failingTransport{})
	catID := importTarget(t, r)
	result, card := importFixtureCard(t, r, catID, Options{})

	if len(card.FileAttachments) != 0 {
		t.Errorf("file attachments = %+v", card.FileAttachments)
	}
	if len(result.LinkedAttachments) != 1 || result.LinkedAttachments[0] != "booking-details.pdf" {
		t.Errorf("linked = %v", result.LinkedAttachments)
	}
	urls := blocksOfType(card, model.BlockURL)
	if len(urls) != 1 || !strings.Contains(urls[0].Value.(string), "/att-pdf/download/") {
		t.Errorf("url blocks = %+v", urls)
	}
}

// With credentials the attachment list comes from Trello's API — it
// includes attachments the export's actions never mention — and uploads
// are fetched from api.trello.com.
func TestImportTrelloCard_WithCredentialsUsesAPIList(t *testing.T) {
	r := setupImporterRepo(t)
	rt := &apiTransport{}
	useTransport(t, rt)
	catID := importTarget(t, r)
	_, card := importFixtureCard(t, r, catID, Options{APIKey: "k", APIToken: "tok"})

	if len(card.FileAttachments) != 2 {
		t.Errorf("file attachments = %+v", card.FileAttachments)
	}
	if urls := blocksOfType(card, model.BlockURL); len(urls) != 1 || urls[0].Value != "https://maps.example.com/pp" {
		t.Errorf("link blocks = %+v", urls)
	}
	for _, u := range rt.requested {
		if !strings.HasPrefix(u, "https://api.trello.com/1/cards/69b1160f42548362c382874c/") {
			t.Errorf("request outside Trello's API: %s", u)
		}
	}
}

func TestParseTrello_RefusesTheOtherExportShape(t *testing.T) {
	if _, err := ParseTrelloJSON([]byte(fixtureCard)); !errors.Is(err, ErrTrelloCardExport) {
		t.Errorf("board parser on a card export: err = %v, want ErrTrelloCardExport", err)
	}
	if _, err := ParseTrelloCardJSON([]byte(fixtureBoard)); err == nil {
		t.Error("card parser accepted a board export")
	}
	if _, err := ParseTrelloCardJSON([]byte(`{"name": "x"}`)); err == nil {
		t.Error("card parser accepted JSON without card ids")
	}
}

func TestTrelloDownloadURL(t *testing.T) {
	cases := map[string]string{
		"https://trello.com/1/cards/c1/attachments/a1/download/f.pdf": "https://api.trello.com/1/cards/c1/attachments/a1/download/f.pdf",
		"https://trello-attachments.s3.amazonaws.com/x/y/f.pdf":       "https://api.trello.com/1/cards/c1/attachments/a1/download/f.pdf",
		"https://example.com/f.pdf":                                   "https://example.com/f.pdf",
		"https://trello.com/b/board":                                  "https://trello.com/b/board",
	}
	for in, want := range cases {
		if got := trelloDownloadURL(TrelloAttachment{ID: "a1", Name: "f.pdf", URL: in}, "c1"); got != want {
			t.Errorf("trelloDownloadURL(%q) = %q, want %q", in, got, want)
		}
	}
}
