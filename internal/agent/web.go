package agent

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	maxResponseBytes = 1 * 1024 * 1024 // 1MB
	maxTextLen       = 4000            // chars returned to LLM
	userAgent        = "BRUV/1.0 (Desktop App)"
)

var (
	htmlTagRe    = regexp.MustCompile(`<[^>]*>`)
	whitespaceRe = regexp.MustCompile(`\s{2,}`)
	titleRe      = regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)
)

// WebFetch fetches a URL and returns extracted text content.
func WebFetch(fetchURL string) (string, error) {
	if fetchURL == "" {
		return "", fmt.Errorf("url is required")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", fetchURL, nil)
	if err != nil {
		return "", fmt.Errorf("invalid url: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	content := string(body)
	contentType := resp.Header.Get("Content-Type")

	if strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml") {
		content = extractTextFromHTML(content)
	}

	if len(content) > maxTextLen {
		content = content[:maxTextLen] + "\n...(truncated)"
	}

	return content, nil
}

// WebSearch searches DuckDuckGo and returns top results.
func WebSearch(query string) (string, error) {
	if query == "" {
		return "", fmt.Errorf("query is required")
	}

	client := &http.Client{Timeout: 30 * time.Second}

	formData := url.Values{"q": {query}}
	req, err := http.NewRequest("POST", "https://html.duckduckgo.com/html/", strings.NewReader(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("read search response: %w", err)
	}

	results := parseDDGResults(string(body))
	if len(results) == 0 {
		return "No results found. Try using web_fetch with a specific URL instead.", nil
	}

	var sb strings.Builder
	for i, r := range results {
		if i >= 5 {
			break
		}
		sb.WriteString(fmt.Sprintf("%d. %s\n   URL: %s\n   %s\n\n", i+1, r.title, r.url, r.snippet))
	}
	return sb.String(), nil
}

// HTTPRequest makes a generic HTTP request and returns the response body.
func HTTPRequest(method, reqURL, body string) (string, error) {
	if reqURL == "" {
		return "", fmt.Errorf("url is required")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, reqURL, bodyReader)
	if err != nil {
		return "", fmt.Errorf("invalid request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	content := string(respBody)
	if len(content) > maxTextLen {
		content = content[:maxTextLen] + "\n...(truncated)"
	}

	return fmt.Sprintf("HTTP %d\n%s", resp.StatusCode, content), nil
}

func extractTextFromHTML(html string) string {
	// Extract title
	title := ""
	if m := titleRe.FindStringSubmatch(html); len(m) > 1 {
		title = strings.TrimSpace(m[1])
	}

	// Remove script, style, and noscript elements
	for _, tag := range []string{"script", "style", "noscript"} {
		re := regexp.MustCompile(`(?is)<` + tag + `[^>]*>.*?</` + tag + `>`)
		html = re.ReplaceAllString(html, "")
	}

	// Strip HTML tags
	text := htmlTagRe.ReplaceAllString(html, " ")

	// Normalize whitespace
	text = whitespaceRe.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	if title != "" {
		text = "Title: " + title + "\n\n" + text
	}

	return text
}

type searchResult struct {
	title   string
	url     string
	snippet string
}

func parseDDGResults(html string) []searchResult {
	var results []searchResult

	// Parse result links: <a class="result__a" href="...">title</a>
	linkRe := regexp.MustCompile(`(?i)<a[^>]*class="result__a"[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	snippetRe := regexp.MustCompile(`(?i)<a[^>]*class="result__snippet"[^>]*>(.*?)</a>`)

	links := linkRe.FindAllStringSubmatch(html, 10)
	snippets := snippetRe.FindAllStringSubmatch(html, 10)

	for i, link := range links {
		if len(link) < 3 {
			continue
		}
		r := searchResult{
			title: stripTags(link[2]),
			url:   link[1],
		}
		if i < len(snippets) && len(snippets[i]) > 1 {
			r.snippet = stripTags(snippets[i][1])
		}
		// DDG wraps URLs in a redirect; extract the actual URL
		if strings.Contains(r.url, "uddg=") {
			if u, err := url.Parse(r.url); err == nil {
				if actual := u.Query().Get("uddg"); actual != "" {
					r.url = actual
				}
			}
		}
		results = append(results, r)
	}

	return results
}

func stripTags(s string) string {
	return strings.TrimSpace(htmlTagRe.ReplaceAllString(s, ""))
}
