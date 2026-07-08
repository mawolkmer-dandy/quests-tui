package app

import (
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
)

// captureLinks scans a quest's body lines in order and fills in its
// integration links: the FIRST Jira URL sets JiraCode (if empty), the FIRST
// GitHub PR URL sets PRCode/PRRepo (if empty). "One of each" — once set, it's
// left alone; extra links stay in the body and just render shortened. Called
// from commitBodyLine (ModalQuestDetail only), so it's idempotent — re-running
// it never overwrites an already-captured code.
func (m *Model) captureLinks(q *model.Quest) {
	for _, l := range q.Body {
		if q.JiraCode == "" {
			if code, ok := model.DetectJira(l.Text); ok {
				q.JiraCode = code
			}
		}
		if q.PRCode == "" {
			if repo, num, ok := model.DetectPR(l.Text); ok {
				q.PRCode = "#" + num
				q.PRRepo = repo
			}
		}
		if q.JiraCode != "" && q.PRCode != "" {
			return
		}
	}
}

// openURL opens url in the system browser, fire-and-forget — a failed launch
// is silently ignored (there's nothing useful to surface for it in the TUI).
func openURL(url string) tea.Cmd {
	return func() tea.Msg {
		_ = exec.Command("open", url).Start()
		return nil
	}
}

// jiraURL builds the browse URL for a Jira issue key against base (the
// configured Jira base, e.g. "https://meetdandy.atlassian.net").
func jiraURL(code, base string) string {
	return strings.TrimRight(base, "/") + "/browse/" + code
}

// prURL builds the pull-request URL from "owner/repo" and a PR number (with
// any leading "#" stripped).
func prURL(repo, num string) string {
	return "https://github.com/" + repo + "/pull/" + strings.TrimPrefix(num, "#")
}
