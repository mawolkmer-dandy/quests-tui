package model

import (
	"regexp"
	"strings"
)

// Link detection for integration capture (see internal/app/links.go). These
// are deliberately pure and self-contained so they can be unit-tested and
// reused from both the capture path (scanning body lines) and the render path
// (shortening any remaining URLs to their code inline).
var (
	// jiraURLRE matches a Jira browse URL on any host, capturing the issue
	// key (e.g. "EPDCHAIR-5713").
	jiraURLRE = regexp.MustCompile(`https?://[^/\s]+/browse/([A-Z][A-Z0-9]+-\d+)`)
	// prURLRE matches a GitHub pull-request URL, capturing "owner/repo" and
	// the PR number.
	prURLRE = regexp.MustCompile(`https?://github\.com/([\w.-]+/[\w.-]+)/pull/(\d+)`)
)

// LinkRef is one detected link and its short code — the code being what the
// URL is replaced with inline ("#47477" / "EPDCHAIR-5713"), and the URL being
// what a click on that code should open.
type LinkRef struct {
	Code string
	URL  string
}

// DetectJira returns the first Jira issue key in text, if any.
func DetectJira(text string) (code string, ok bool) {
	m := jiraURLRE.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// DetectPR returns the first GitHub PR's "owner/repo" and number in text, if
// any.
func DetectPR(text string) (repo, num string, ok bool) {
	m := prURLRE.FindStringSubmatch(text)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// PRRef is one detected GitHub PR: its short code ("#47477") and "owner/repo".
type PRRef struct {
	Code string
	Repo string
}

// DetectPRs returns every GitHub PR in text, in reading order, as PRRefs. Used
// by the capture path to link all PRs pasted into a quest's body (not just the
// first).
func DetectPRs(text string) []PRRef {
	var refs []PRRef
	for _, m := range prURLRE.FindAllStringSubmatch(text, -1) {
		refs = append(refs, PRRef{Code: "#" + m[2], Repo: m[1]})
	}
	return refs
}

// StripLinks removes every Jira browse URL and GitHub PR URL from text and
// tidies the doubled / edge whitespace their removal leaves behind, returning
// the cleaned text. Used by the paste-time capture path to pull a pasted URL
// out of the line once its code has been captured.
func StripLinks(text string) string {
	text = jiraURLRE.ReplaceAllString(text, "")
	text = prURLRE.ReplaceAllString(text, "")
	text = collapseSpacesRE.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

var collapseSpacesRE = regexp.MustCompile(`[ \t]{2,}`)

// ShortenLinks replaces every Jira browse URL and GitHub PR URL in text with
// its short code ("EPDCHAIR-5713" / "#47477"), returning the rewritten text
// and the ordered list of links found (for clickable rendering). URLs are
// replaced left to right in the order they appear.
func ShortenLinks(text string) (shortened string, refs []LinkRef) {
	type match struct {
		start, end int
		code, url  string
	}
	var matches []match

	for _, loc := range jiraURLRE.FindAllStringSubmatchIndex(text, -1) {
		url := text[loc[0]:loc[1]]
		code := text[loc[2]:loc[3]]
		matches = append(matches, match{start: loc[0], end: loc[1], code: code, url: url})
	}
	for _, loc := range prURLRE.FindAllStringSubmatchIndex(text, -1) {
		url := text[loc[0]:loc[1]]
		num := text[loc[4]:loc[5]]
		matches = append(matches, match{start: loc[0], end: loc[1], code: "#" + num, url: url})
	}

	if len(matches) == 0 {
		return text, nil
	}

	// Sort by start offset so replacements and refs come out in reading order.
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j].start < matches[j-1].start; j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
	}

	var b []byte
	last := 0
	for _, mt := range matches {
		b = append(b, text[last:mt.start]...)
		b = append(b, mt.code...)
		last = mt.end
		refs = append(refs, LinkRef{Code: mt.code, URL: mt.url})
	}
	b = append(b, text[last:]...)
	return string(b), refs
}
