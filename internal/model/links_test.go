package model

import "testing"

func TestDetectJira(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantCode string
		wantOK   bool
	}{
		{"basic", "see https://meetdandy.atlassian.net/browse/EPDCHAIR-5713 now", "EPDCHAIR-5713", true},
		{"http", "http://jira.example.com/browse/AB1-42", "AB1-42", true},
		{"none", "just some text", "", false},
		{"not a browse url", "https://meetdandy.atlassian.net/projects/EPDCHAIR", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, ok := DetectJira(tt.text)
			if ok != tt.wantOK || code != tt.wantCode {
				t.Errorf("DetectJira(%q) = (%q, %v), want (%q, %v)", tt.text, code, ok, tt.wantCode, tt.wantOK)
			}
		})
	}
}

func TestDetectPR(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantRepo string
		wantNum  string
		wantOK   bool
	}{
		{"basic", "PR: https://github.com/orthly/orthlyweb/pull/47477 done", "orthly/orthlyweb", "47477", true},
		{"none", "no link here", "", "", false},
		{"not a pull", "https://github.com/orthly/orthlyweb/issues/1", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, num, ok := DetectPR(tt.text)
			if ok != tt.wantOK || repo != tt.wantRepo || num != tt.wantNum {
				t.Errorf("DetectPR(%q) = (%q, %q, %v), want (%q, %q, %v)", tt.text, repo, num, ok, tt.wantRepo, tt.wantNum, tt.wantOK)
			}
		})
	}
}

func TestDetectPRs(t *testing.T) {
	t.Run("none", func(t *testing.T) {
		if refs := DetectPRs("no links"); refs != nil {
			t.Errorf("DetectPRs(none) = %+v, want nil", refs)
		}
	})
	t.Run("multiple in order", func(t *testing.T) {
		text := "https://github.com/orthly/orthlyweb/pull/47477 then https://github.com/orthly/orthlyweb/pull/47480"
		refs := DetectPRs(text)
		if len(refs) != 2 {
			t.Fatalf("got %d refs, want 2", len(refs))
		}
		if refs[0].Code != "#47477" || refs[0].Repo != "orthly/orthlyweb" {
			t.Errorf("refs[0] = %+v", refs[0])
		}
		if refs[1].Code != "#47480" || refs[1].Repo != "orthly/orthlyweb" {
			t.Errorf("refs[1] = %+v", refs[1])
		}
	})
}

func TestDetectJiras(t *testing.T) {
	t.Run("none", func(t *testing.T) {
		if codes := DetectJiras("no links"); codes != nil {
			t.Errorf("DetectJiras(none) = %+v, want nil", codes)
		}
	})
	t.Run("multiple in order", func(t *testing.T) {
		text := "https://meetdandy.atlassian.net/browse/EPDCHAIR-5713 and https://x/browse/ES-8858"
		codes := DetectJiras(text)
		if len(codes) != 2 || codes[0] != "EPDCHAIR-5713" || codes[1] != "ES-8858" {
			t.Fatalf("got %+v, want [EPDCHAIR-5713 ES-8858]", codes)
		}
	})
}

func TestStripLinks(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"pr mid-line", "see https://github.com/orthly/orthlyweb/pull/47477 for details", "see for details"},
		{"jira trailing", "ticket https://meetdandy.atlassian.net/browse/EPDCHAIR-5713", "ticket"},
		{"both", "a https://github.com/a/b/pull/9 b https://x.atlassian.net/browse/AB-1 c", "a b c"},
		{"no link untouched", "plain text", "plain text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripLinks(tt.in); got != tt.want {
				t.Errorf("StripLinks(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestShortenLinks(t *testing.T) {
	t.Run("no links", func(t *testing.T) {
		got, refs := ShortenLinks("plain text")
		if got != "plain text" || refs != nil {
			t.Errorf("ShortenLinks(plain) = (%q, %v), want unchanged and nil refs", got, refs)
		}
	})

	t.Run("both kinds in reading order", func(t *testing.T) {
		text := "jira https://meetdandy.atlassian.net/browse/EPDCHAIR-5713 then pr https://github.com/orthly/orthlyweb/pull/47477 end"
		got, refs := ShortenLinks(text)
		want := "jira EPDCHAIR-5713 then pr #47477 end"
		if got != want {
			t.Errorf("ShortenLinks shortened = %q, want %q", got, want)
		}
		if len(refs) != 2 {
			t.Fatalf("got %d refs, want 2", len(refs))
		}
		if refs[0].Code != "EPDCHAIR-5713" || refs[0].URL != "https://meetdandy.atlassian.net/browse/EPDCHAIR-5713" {
			t.Errorf("refs[0] = %+v", refs[0])
		}
		if refs[1].Code != "#47477" || refs[1].URL != "https://github.com/orthly/orthlyweb/pull/47477" {
			t.Errorf("refs[1] = %+v", refs[1])
		}
	})

	t.Run("pr before jira preserves order", func(t *testing.T) {
		text := "https://github.com/a/b/pull/9 and https://x.atlassian.net/browse/AB-1"
		got, refs := ShortenLinks(text)
		want := "#9 and AB-1"
		if got != want {
			t.Errorf("shortened = %q, want %q", got, want)
		}
		if len(refs) != 2 || refs[0].Code != "#9" || refs[1].Code != "AB-1" {
			t.Errorf("refs out of order: %+v", refs)
		}
	})
}
