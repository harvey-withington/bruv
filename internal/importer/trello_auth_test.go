package importer

import "testing"

// The Trello key + token ride in an Authorization header on attachment
// downloads. Link attachments point anywhere, so the header may only go
// to Trello's own API over https.
func TestIsTrelloAPIHost(t *testing.T) {
	cases := map[string]bool{
		"https://api.trello.com/1/cards/c/attachments/a/download/f.png": true,
		"https://API.Trello.com/1/x":                                     true,
		"http://api.trello.com/1/x":                                      false, // never over plain http
		"https://trello-attachments.s3.amazonaws.com/x":                  false,
		"https://example.com/api.trello.com/x":                           false,
		"https://api.trello.com.evil.example/x":                          false,
		"not a url":                                                      false,
	}
	for in, want := range cases {
		if got := isTrelloAPIHost(in); got != want {
			t.Errorf("isTrelloAPIHost(%q) = %v, want %v", in, got, want)
		}
	}
}
