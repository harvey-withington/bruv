package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListModelsShapes(t *testing.T) {
	cases := []struct {
		provider, path, body  string
		wantHeader, wantValue string
		want                  []DiscoveredModel
	}{
		{"openai_compatible", "/v1/models", `{"data":[{"id":"z-model","name":"Zed"},{"id":"a-model"}]}`,
			"Authorization", "Bearer k", []DiscoveredModel{{ID: "a-model"}, {ID: "z-model", Label: "Zed"}}},
		{"anthropic", "/v1/models", `{"data":[{"id":"claude-sonnet-5","display_name":"Claude Sonnet 5"}]}`,
			"x-api-key", "k", []DiscoveredModel{{ID: "claude-sonnet-5", Label: "Claude Sonnet 5"}}},
		{"ollama", "/api/tags", `{"models":[{"name":"llama3.1:8b"}]}`,
			"", "", []DiscoveredModel{{ID: "llama3.1:8b"}}},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != c.path {
				t.Errorf("%s: path %q, want %q", c.provider, r.URL.Path, c.path)
			}
			if c.wantHeader != "" && r.Header.Get(c.wantHeader) != c.wantValue {
				t.Errorf("%s: header %s = %q", c.provider, c.wantHeader, r.Header.Get(c.wantHeader))
			}
			w.Write([]byte(c.body))
		}))
		base := srv.URL
		if c.provider == "openai_compatible" {
			base += "/v1"
		}
		got, err := ListModels(context.Background(), c.provider, "k", base)
		srv.Close()
		if err != nil {
			t.Fatalf("%s: %v", c.provider, err)
		}
		if len(got) != len(c.want) {
			t.Fatalf("%s: got %+v", c.provider, got)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s[%d] = %+v, want %+v", c.provider, i, got[i], c.want[i])
			}
		}
	}
}

func TestListModelsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()
	if _, err := ListModels(context.Background(), "openai", "bad", srv.URL); err == nil {
		t.Error("expected error on 401")
	}
}
