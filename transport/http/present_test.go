package http

import (
	"encoding/hex"
	nethttp "net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestSignPresentMAC_Deterministic(t *testing.T) {
	secret := []byte("test-secret")
	a := hex.EncodeToString(SignPresentMAC(secret, "repo1", "card1", 1000))
	b := hex.EncodeToString(SignPresentMAC(secret, "repo1", "card1", 1000))
	if a != b {
		t.Fatal("signature not deterministic")
	}
	if hex.EncodeToString(SignPresentMAC(secret, "repo1", "card2", 1000)) == a {
		t.Fatal("a different card must produce a different signature")
	}
	if hex.EncodeToString(SignPresentMAC([]byte("other"), "repo1", "card1", 1000)) == a {
		t.Fatal("a different secret must produce a different signature")
	}
}

func TestPresentDataHandler(t *testing.T) {
	secret := []byte("s3cr3t")
	cfg := &PresentConfig{
		Secret: secret,
		ResolveCardJSON: func(repoID, cardID string) ([]byte, bool) {
			if repoID == "repo1" && cardID == "card1" {
				return []byte(`{"id":"card1"}`), true
			}
			return nil, false
		},
	}
	h := presentDataHandler(cfg)
	sign := func(repo, card string, exp int64) string {
		return hex.EncodeToString(SignPresentMAC(secret, repo, card, exp))
	}
	itoa := func(n int64) string { return strconv.FormatInt(n, 10) }
	future := time.Now().Add(time.Hour).Unix()

	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(nethttp.MethodGet, path, nil))
		return rec
	}

	t.Run("valid signature returns card JSON", func(t *testing.T) {
		rec := get("/present-data/repo1/card1?exp=" + itoa(future) + "&sig=" + sign("repo1", "card1", future))
		if rec.Code != nethttp.StatusOK {
			t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
		}
		if rec.Body.String() != `{"id":"card1"}` {
			t.Fatalf("unexpected body: %s", rec.Body.String())
		}
	})

	t.Run("bad signature is rejected", func(t *testing.T) {
		if code := get("/present-data/repo1/card1?exp=" + itoa(future) + "&sig=deadbeef").Code; code != nethttp.StatusUnauthorized {
			t.Fatalf("want 401, got %d", code)
		}
	})

	t.Run("expired url is rejected", func(t *testing.T) {
		past := time.Now().Add(-time.Hour).Unix()
		if code := get("/present-data/repo1/card1?exp=" + itoa(past) + "&sig=" + sign("repo1", "card1", past)).Code; code != nethttp.StatusUnauthorized {
			t.Fatalf("want 401, got %d", code)
		}
	})

	t.Run("valid signature for an unknown card is 404", func(t *testing.T) {
		if code := get("/present-data/repo1/nope?exp=" + itoa(future) + "&sig=" + sign("repo1", "nope", future)).Code; code != nethttp.StatusNotFound {
			t.Fatalf("want 404, got %d", code)
		}
	})

	t.Run("malformed path is 400", func(t *testing.T) {
		if code := get("/present-data/repo1?exp=" + itoa(future) + "&sig=abc").Code; code != nethttp.StatusBadRequest {
			t.Fatalf("want 400, got %d", code)
		}
	})
}
