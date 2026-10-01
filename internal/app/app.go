package app

import (
	"bytes"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fsnotify/fsnotify"

	"github.com/mawolkmer-dandy/quests-tui/internal/config"
	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/quickadd"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// viewVPad is the blank-row breathing room kept at the top and bottom of the
// main outline and the focus views (clamped down on very short terminals).
const viewVPad = 8

// mouseLeakChars are the only characters an SGR mouse report ("ESC[<b;x;yM",
// or fragments of one) is built from. Some terminals mis-deliver these as
// text — especially bursts of "[" when the wheel over-scrolls a boundary.
const mouseLeakChars = "\x1b[<;0123456789Mm"

// isMouseLeak flags a multi-rune key event whose runes are ALL from the
// mouse-report alphabet and include a "[" or "<" introducer — that can only
// be a leaked (partial) mouse sequence, never real typing. A lone "[" (or
// digits like "12", "5M") is left alone so ordinary input still works.
func isMouseLeak(k tea.Key) bool {
	if k.Text == "" || len(k.Text) < 2 {
		return false
	}
	marker := false
	for _, r := range k.Text {
		if !strings.ContainsRune(mouseLeakChars, r) {
			return false
		}
		if r == '[' || r == '<' {
			marker = true
		}
	}
	return marker
}

// mouseAlphabetKey reports whether every rune of a key event is drawn from
// the mouse-report alphabet — used only inside the brief post-wheel window
// (see lastWheelAt), where such a keystroke can only be a leaked fragment,
// never intentional typing.
func mouseAlphabetKey(k tea.Key) bool {
	if k.Text == "" {
		return false
	}
	for _, r := range k.Text {
		if !strings.ContainsRune(mouseLeakChars, r) {
			return false
		}
	}
	return true
}

// cursorTarget identifies the row the cursor is on by identity (project ID /
// quest ID / section name) rather than raw index, so it survives the row
// list being rebuilt fresh every frame — a mutation (toggling done, e.g.)
// can move a quest to a different position in the list without losing the
// cursor.
type cursorTarget struct {
	kind       ui.RowKind
	projectID  string
	questID    string
	section    string
	label      string
	runeKey    string
	bodyLineID string
	lookoutURL string
	trackEvent string
	prCode     string
	prRepo     string
	bannerID   string
}

func targetFromRow(row ui.Row) cursorTarget {
	return cursorTarget{kind: row.Kind, projectID: row.ProjectID, questID: row.QuestID, section: row.Section, label: row.Label, runeKey: row.RuneKey, bodyLineID: row.BodyLineID, lookoutURL: row.LookoutURL, trackEvent: row.TrackEvent, prCode: row.Code, prRepo: row.Repo, bannerID: row.BannerID}
}

func (t cursorTarget) matches(row ui.Row) bool {
	if t.kind != row.Kind {
		return false
	}
	switch t.kind {
	case ui.RowProject:
		return t.projectID == row.ProjectID
	case ui.RowQuest:
		return t.questID == row.QuestID
	case ui.RowSection:
		return t.section == row.Section
	case ui.RowNewProject:
		return t.bannerID == row.BannerID // one per banner (+ the global one)
	case ui.RowNewBanner:
		return true
	case ui.RowBanner:
		return t.bannerID == row.BannerID
	case ui.RowNewQuest:
		return t.projectID == row.ProjectID && t.bannerID == row.BannerID // banner-loose "+ New Quest" is distinct
	case ui.RowLabel:
		return t.label == row.Label
	case ui.RowRune:
		return t.runeKey == row.RuneKey
	case ui.RowVaultCampaign:
		return t.projectID == row.ProjectID
	case ui.RowRuneQuest:
		return t.questID == row.QuestID
	case ui.RowWildsObjective:
		return t.questID == row.QuestID && t.bodyLineID == row.BodyLineID
	case ui.RowBodyLine:
		return t.projectID == row.ProjectID && t.bodyLineID == row.BodyLineID
	case ui.RowSagaLink:
		return t.projectID == row.ProjectID && t.label == row.Label
	case ui.RowLookout:
		return t.questID == row.QuestID && t.lookoutURL == row.LookoutURL
	case ui.RowTrack:
		return t.questID == row.QuestID && t.trackEvent == row.TrackEvent
	case ui.RowLookoutQuest:
		return t.questID == row.QuestID
	case ui.RowVaultHeader:
		return t.section == row.Section
	case ui.RowTrailQuest:
		return t.questID == row.QuestID
	case ui.RowTrail:
		return t.questID == row.QuestID && t.prCode == row.Code && t.prRepo == row.Repo
	}
	return false
}

func findRowIndex(rows []ui.Row, target cursorTarget) int {
	for i, r := range rows {
		if target.matches(r) {
			return i
		}
	}
	return -1
}

type Model struct {
	store   *store.Store
	path    string
	darkBg  bool
	watcher *fsnotify.Watcher // watches the quick-add spool for live ingestion (see quickadd_watch.go)

	// cfgPath is ~/.config/quests/config.toml — needed so a resize-drag
	// release can persist the new layout ratios (see endResizeDrag).
	cfgPath string

	// Undo stack of prior store states (JSON snapshots). recordUndo pushes the
	// pre-change state on each save; undo (Ctrl+Z) pops and restores. Bounded
	// to undoLimit. applyingUndo suppresses recording while restoring.
	undoStack    [][]byte
	lastSnapshot []byte
	applyingUndo bool

	// wilds is the "out on the road" view: a flat, filtered list of quests
	// (see wildsRows) instead of the full Tavern outline. quickFilter is the
	// radio chip narrowing it (All / High priority / Taken).
	wilds       bool
	quickFilter quickFilter
	animate     bool // whether the intro/transition animation plays (config: intro)

	// Venture (the Wilds focus loop): venturedID is the single quest currently
	// ventured out of Camp into the Wilds (empty = at Camp, the full agenda).
	// ventureStart is when this venture began, for the count-up session timer;
	// ventureTickGen guards the once-a-second timer so a stale ticker stops on
	// its next fire (see ventureTick / wilds_venture.go).
	venturedID     string
	ventureStart   time.Time
	ventureTickGen int

	// Tavern hall (2c-visual): the borderless left directory + floating content
	// pane, navigated master-detail. hallFocus is true while the cursor walks the
	// hall (left) — false once it has dived into the pane (right). hallCursor is
	// the selected hall entry (a room / banner / campaign), which drives what the
	// pane shows (see paneRows). hallScroll is the hall column's own vertical
	// scroll; the pane reuses scrollOffset. hallSpans + the column geometry are
	// recorded each render for click routing between the two columns.
	hallFocus     bool
	hallCursor    cursorTarget
	hallScroll    int
	hallScrollMax int // largest useful hallScroll, cached each render so the wheel can clamp
	hallSpans     []hallSpan
	hallColX      int
	hallColW      int
	paneColX      int
	paneColW      int
	// paneLineRow maps each pane screen line (post soft-wrap) to its paneRows
	// index, -1 for a spacer/blank — so a click resolves to the right row even
	// when a campaign note wrapped onto several lines. Rebuilt each render.
	paneLineRow []int
	// paneLinkSpans are the shortened-link click targets in campaign notes, in
	// absolute screen coordinates, so a click copies (or a fast second opens)
	// the full URL — the same gesture as a quest body link. Rebuilt each render.
	paneLinkSpans []paneLinkSpan

	// chipLineRow is the screen row of the reserved filter line; chipSpans are
	// the Wilds quick-chip click extents on it (see handleMouse).
	chipLineRow int
	chipSpans   []chipSpan

	// modeToggleRow / modeSpans: the TAVERN/WILDS header's screen row and the
	// two labels' click extents (see handleMouse).
	modeToggleRow int
	modeSpans     []modeSpan

	// tavernHelp{Row,X,Width}: the Tavern's top-right "F1 help" button extents
	// (the copy toast borrows the same slot). Width is 0 while the toast shows,
	// so the toast isn't a click target.
	tavernHelpRow   int
	tavernHelpX     int
	tavernHelpWidth int

	width, height int
	scrollOffset  int
	// scrollMax is the largest useful scrollOffset for the single-column view,
	// cached each render so the wheel handler can clamp (and no-op at the
	// bottom) instead of over-incrementing past the end.
	scrollMax int
	subtitle  string

	// Screen-space overlay (see overlay.go): transient effects composited on
	// top of the final frame. cursorScreen{X,Y} is the last-rendered cursor
	// cell (Y = -1 when off-screen / not applicable), so a completion can spawn
	// a burst where the item visually is.
	overlayParticles []overlayParticle
	overlayGen       int
	cursorScreenX    int
	cursorScreenY    int
	// pendingConnBurstCode spawns a sparkle burst on the next detail render on
	// the just-added connection's own line in the Sigils section (matched by
	// its code/id). Deferred a frame because the add happens with a modal open /
	// before the connection has a rendered position. "" = nothing pending.
	pendingConnBurstCode string

	// Environment-change animation (see transition.go): old rows burn away
	// right-to-left, a pause, then the new view reveals line by line. Runs on
	// startup, Tavern⇄Wilds, and filter changes. transPhase == transNone
	// when idle.
	transPhase  transPhase
	transFrame  int
	transOld    []string  // rendered rows captured before the change, for the dissolve
	transOldSub string    // the subtitle being typed out (mode switches only)
	transFast   bool      // filter changes animate faster than mode switches
	transGen    int       // bumped each beginTransition; ticks from an older gen are ignored (no double-speed)
	transKind   transKind // startup / mode switch / filter — drives header + stagger
	// transAbsolute marks a transition whose captured/revealed body lines already
	// carry their left margin (the Camp⇄Tavern switch, whose two-column Tavern
	// body can't share the single-column margin) — so the animation renders them
	// as-is instead of re-indenting.
	transAbsolute bool

	// set each View() call, used by handleMouse to map screen coordinates
	// back to a row index / in-row column.
	rowsScreenTop int
	leftMargin    int

	// sectionScroll / sectionMaxScroll are each focus view's own vertical scroll
	// offset and clamp bound, keyed by section — used by the quest-detail and
	// campaign/section focus panes so a wheel can scroll them directly.
	sectionScroll    map[string]int
	sectionMaxScroll map[string]int
	// uiVersion bumps whenever displayed Tavern content changes (a save, a
	// collapse toggle, an integration-status update). boxCache holds each
	// section's wrapped lines + spans so scrolling (which changes none of that)
	// reuses them instead of re-rendering every row — the viewport pattern.
	// cursorMoved is true only for the frame(s) after a keyboard cursor move, so
	// scroll follows the cursor then, but a mouse wheel scrolls freely.
	uiVersion   int
	boxCache    map[string]*boxCacheEntry
	cursorMoved bool

	collapsedProjects map[string]bool
	collapsedSections map[string]bool

	// newQuestIDs marks quests just ingested from quick-add (raycast/cli) so a
	// blue "●" shows where each landed; cleared when the quest is selected or
	// opened. In-memory only (a session hint, not persisted).
	newQuestIDs map[string]bool

	cursor cursorTarget
	editor *textinput.Model

	// The shared body-outline editor, lifted off Modal so the same experience
	// (navigation, split/merge, copy-paste) drives a quest's detail modal AND a
	// campaign's inline notes in the Tavern pane. bodyOwnerKind/ID say whose
	// Body is being edited (see currentBody); bodyCursor is the active line,
	// bodyEditor the live line editor. ownerNone = no body is being edited.
	bodyOwnerKind bodyOwnerKind
	bodyOwnerID   string
	bodyCursor    int
	bodyEditor    textinput.Model

	// selAnchor is the other end of a text selection in whichever editor is
	// currently focused (m.editor, the body editor, or the search box) — see
	// selection.go. noSelection means there isn't one. selAnchorLine is the
	// body-line index the anchor sits on — when it differs from bodyCursor, the
	// selection spans lines (copy-only; see multilineSelActive).
	selAnchor     int
	selAnchorLine int

	// focusBodyBaseRow/focusLeftMargin are set each renderFocusView call, so
	// handleFocusMouse can map a click/drag back to a rune position within
	// the body lines. focusHeaderRow and the back/help extents make the
	// header's "← back (esc)" / "F1 help" clickable.
	focusBodyBaseRow int
	focusLeftMargin  int
	focusTextWidth   int
	focusHeaderRow   int
	// focusCaretLine is the 0-based line (within renderFocusContent's output)
	// the caret sits on, so renderFocusView can scroll to keep it in view.
	// focusScroll is the current vertical scroll offset of the focus view
	// (reset to 0 whenever a focus view is opened/left).
	focusCaretLine int
	focusScroll    int
	// focusScrollMax is the largest useful focusScroll for the campaign/section
	// focus page, cached each render so the wheel handler can clamp (and no-op
	// at the bottom) rather than over-incrementing.
	focusScrollMax int
	focusBackWidth int
	focusHelpX     int
	focusHelpWidth int

	// titleEditor is a live inline editor over a detail page's title (quest or
	// campaign) while it's being renamed in place; nil otherwise. focusTitleRow
	// and focusTitleX/Width record the title's screen position so a click can
	// open the editor (and, while editing, so it renders in the right spot).
	titleEditor     *textinput.Model
	focusTitleRow   int
	focusTitleX     int
	focusTitleWidth int
	// titleEditFromSigils records which pane the title editor was entered from
	// (true = Sigils, false = body) so Down returns focus there.
	titleEditFromSigils bool

	// lookoutEditor is an inline editor over a focused Lookout's label while it's
	// being renamed ("r"); lookoutEditURL is the Lookout being edited. Dashboard
	// names can't be scraped (auth-gated SPAs), so the user names them by hand.
	lookoutEditor      *textinput.Model
	lookoutEditURL     string
	lookoutEditQuestID string

	// focusRowLine/focusRowOffset map each rendered body row (soft-wrapped
	// lines span several) back to its body line index and the raw rune
	// offset the row starts at — rebuilt every renderFocusContent, consumed
	// by handleFocusMouse.
	focusRowLine   []int
	focusRowOffset []int

	// Quest-detail two-column geometry (see renderFocusContent): the details
	// sidebar is on the LEFT (focusDetailX), the editable body on the RIGHT
	// (focusBodyX); focusBodyW is the body column's width. Used for click
	// column-routing and burst placement. Non-quest focus views leave both at
	// the content left margin (single column).
	focusDetailX int
	focusBodyX   int
	focusBodyW   int

	// hintSpans maps a visible row index to the clickable extents of its
	// rendered action hints ("→ open (tab)" etc.), rebuilt each View — a
	// click inside a span triggers that hint's action (see handleMouse).
	hintSpans map[int][]hintSpan

	// modal is the topmost open modal; modalStack holds whatever's beneath
	// it, so drilling from a campaign's detail page into one of its quests
	// (Tab) and then closing (Esc) returns to the campaign, not the outline.
	modal      *Modal
	modalStack []*Modal

	// Picker-modal click mapping (ModalProjectPicker/AgentPicker/RunePicker):
	// the screen row of the first selectable list item, the item count, and the
	// box's horizontal extent — recomputed each renderModal so a click on a row
	// maps to an item index. modalItemTop is -1 when no list is showing.
	modalItemTop, modalItemCount, modalItemX0, modalItemX1 int

	// Schedule-picker click mapping: the screen Y of each option (indexed by
	// PickerIndex), and the box's horizontal extent. Its options are separated by
	// group captions, so a simple top+idx map (modalItem*) won't do — see
	// handleScheduleClick / renderModal's ModalSchedulePicker tail.
	scheduleItemYs                 []int
	scheduleItemX0, scheduleItemX1 int

	// Confirm-delete dialog click mapping: the screen row of its two buttons and
	// each button's X extent, recomputed each renderModal so a click resolves to
	// Cancel or the delete button. confirmBtnRow is -1 when no dialog is showing.
	confirmBtnRow                    int
	confirmCancelX0, confirmCancelX1 int
	confirmDeleteX0, confirmDeleteX1 int

	// hover is whatever row the mouse is currently resting over, or the
	// cursor's own row (nil if neither applies) — used to show an action
	// hint ("→ open (tab)", "↓ collapse (enter)") next to it. See
	// updateHover, actionHint, and View()'s row loop. hideHoverTips
	// (Ctrl+K) suppresses those hints without affecting anything else.
	hover         *cursorTarget
	hideHoverTips bool
	// hoverSection is the Tavern section whose title the mouse is currently over
	// ("inbox"/"runes"/"someday"/"campaigns", or ""), so its box title can show
	// the open/collapse hint on hover (set each motion in the two-column view).
	hoverSection string

	// warningText, if non-empty, replaces warningTarget's title for a couple
	// of seconds — used for "vault is read-only" when an action is blocked
	// (see showWarning in anim.go).
	warningTarget cursorTarget
	warningText   string
	warningGen    int

	// The status-line toast is a queued, animated notice (see anim.go): messages
	// type in/out and play one after another rather than replacing. toastQueue
	// holds the pending ones, toastText/toastPhase/toastFrame the active one, and
	// toastGen guards its tick loop.
	toastQueue []string
	toastText  string
	toastPhase toastPhase
	toastFrame int
	toastGen   int
	// lastLinkClickURL / lastLinkClickAt implement single-click-copy /
	// double-click-open for links (sigils + body): a first click copies, a
	// second click on the same URL within linkDoubleClickWindow opens it.
	lastLinkClickURL string
	lastLinkClickAt  time.Time
	// lastRowClick / lastRowClickAt implement double-click-to-open on a list row
	// (a quest/campaign/section): a second click on the same row within the
	// window enters its detail view — the mouse equivalent of Tab.
	lastRowClick   cursorTarget
	lastRowClickAt time.Time

	// lastWheelAt is when the last scroll-wheel event arrived. Bubble Tea's
	// input parser fragments SGR mouse sequences under fast scrolling and
	// leaks the pieces as key runes (charmbracelet/bubbletea#1627); we drop
	// mouse-alphabet key events for a short window after any wheel event so
	// those stragglers can't land in the text.
	lastWheelAt time.Time

	// leftDown is true from a left MouseClickMsg until the next
	// MouseReleaseMsg — v2 splits press/release/motion into distinct message
	// types and a MouseMotionMsg carries no button state, so this is the only
	// way handleMotion/handleFocusMotion know a drag (text selection or panel
	// resize) is in progress.
	leftDown bool

	// resizeDrag/resizeHover drive the one remaining draggable divider — the
	// quest-detail Sigils/body split. resizeHover is which divider the mouse
	// rests on while not dragging (the "you can drag here" highlight).
	// railWidthRatio/railBoxRatios persist in the config Layout for back-compat
	// (the Tavern's rail resize is retired; nothing reads them for rendering now).
	resizeDrag     resizeDragState
	resizeHover    resizeTarget
	railWidthRatio float64
	railBoxRatios  []float64

	// showHiddenSigils reveals the empty connection sections in the quest detail
	// (compact view shows only sections with value); toggled with F3 while a quest
	// is open. Even a loading (being-found) but still-empty section stays hidden
	// in the compact view.
	showHiddenSigils bool

	// detailWidthRatio is the quest-detail Sigils box's fraction of the detail
	// view width (draggable, persisted); detailDividerX/detailPaneTop/Bottom are
	// the last render's divider hit-target, set by viewQuestDetail.
	detailWidthRatio float64
	detailDividerX   int
	detailPaneTop    int
	detailPaneBottom int

	// plansBusyQuest is the quest whose Lookout plans are currently being
	// written (shows a "writing…" hint on its Tracks group); cleared when the
	// generated prompt lands and is copied (see tracks.go / plansMsg).
	plansBusyQuest string

	// lastFoundSHA is the PR head SHA last scanned for tracks, keyed by
	// "repo#num", so auto-find on sync only re-diffs a PR whose head changed.
	lastFoundSHA map[string]string

	// findingQuestID is the quest whose track-find is in flight (drives the
	// "finding…" status on its find affordance); cleared in applyFind.
	findingQuestID string

	// Integration sync (see sync.go). prStatus/jiraStatus cache the latest
	// fetched status keyed by code; neither is persisted nor part of undo.
	// integrationsEnabled gates the whole feature (config); syncInterval is
	// the ticker period (>=15s floor); jiraBaseURL builds clickable Jira
	// links. syncing guards against overlapping fetch passes; lastSyncAt is
	// when the most recent pass landed.
	integrationsEnabled bool
	syncInterval        time.Duration
	jiraBaseURL         string
	ldProject           string       // LaunchDarkly project for rune (flag) lookups
	ldEnv               string       // LaunchDarkly environment a rune's state is read in
	soundCfg            config.Sound // completion-sound settings (see sound.go)
	prStatus            map[string]PRStatus
	jiraStatus          map[string]JiraStatus
	runeStatus          map[string]RuneStatus
	syncing             bool
	lastSyncAt          time.Time

	// codeSpans maps a visible row index to the clickable extents of the
	// integration codes rendered on its meta sub-line, parallel to hintSpans
	// (see handleMouse). focusCodeSpans is the same for the expanded quest
	// focus view (see handleFocusMouse).
	codeSpans      map[int][]codeSpan
	focusCodeSpans []focusCodeSpan

	// focusBodyLineStart is the content-line index (within renderFocusContent's
	// output) at which the first body row is emitted — the title, integration
	// codes, and blanks above it push it down, so handleFocusMouse maps a
	// click Y back to a body row against this rather than a fixed offset.
	// focusContentTop is content line 0's screen row (topPad + header rows -
	// scroll), so a recorded focus code span's screen row is that plus the
	// span's content line.
	focusBodyLineStart int
	focusContentTop    int
	// focusQuestListStart is the content-line index of the first row in a
	// campaign detail page's "Quests" list, so a click Y maps to a quest row.
	focusQuestListStart int

	// focusLinks are the navigable link lines (Jira + each PR) rendered above
	// the body in the expanded quest view, in top-to-bottom order — arrowing up
	// off the first body row steps onto the bottom-most of these. focusLinkIdx
	// is which one the link cursor is on, or noSelection when the body owns the
	// caret. Rebuilt each renderFocusContent. focusLinkConfirmID arms the inline
	// y/n prompt for removing the focused connection in the quest detail (a
	// separate, lightweight confirm from the shared ModalConfirmDelete dialog).
	focusLinks         []focusLink
	focusLinkIdx       int
	focusLinkConfirmID string

	// agents is the latest `herdr agent list`, refreshed by the agent poll
	// (and immediately when the picker opens). A quest shows the state of its
	// pinned agents (see agents.go).
	agents []HerdrAgent

	// Working-agent spinner: spinnerFrame advances while any pinned agent is
	// working, animating its status glyph. The ticker only runs while there's
	// something to animate; spinnerGen guards against double-tickers.
	spinnerFrame int
	spinnerGen   int
	spinnerOn    bool

	// Agent poll: re-queries `herdr workspace list` on a short interval while a
	// workspace is pinned, so pinned agents' state stays live. agentPollGen
	// guards against double-timers.
	agentPollGen int
	agentPollOn  bool

	debug     bool
	lastMsgAt time.Time
}

// codeSpan is an integration code's clickable extent in absolute screen
// columns, carrying the URL a click should open.
type codeSpan struct {
	x0, x1 int
	url    string
}

// focusCodeSpan is a codeSpan in the expanded quest focus view, recorded
// against its content-line index (converted to a screen row at hit-test time
// via focusContentTop, since the view scrolls).
type focusCodeSpan struct {
	line   int // content line index within renderFocusContent
	x0, x1 int
	url    string
}

// linkKind distinguishes the two removable link kinds in the expanded quest
// view, so removing one edits the right field on the quest.
type linkKind int

const (
	linkJira linkKind = iota
	linkPR
	linkAgent
	linkAddAgent    // the "+ add Claude agent" affordance line
	linkRune        // an attached LaunchDarkly flag
	linkTrack       // a found tracking event
	linkLookout     // a usage dashboard (per-quest)
	linkForge       // the "write the Lookout's plans" affordance line
	linkRestore     // the "restore dismissed tracks" affordance line
	linkCopySection // a section header — focusable so it can be copied as a list
)

// focusLink is one navigable link line (the Jira line or a PR line) in the
// expanded quest view: its content-line index (for caret tracking / scroll),
// the URL Enter opens, and the identity needed to remove it (kind + code).
type focusLink struct {
	line int
	kind linkKind
	code string // "EPDCHAIR-5713" or "#47477"
	url  string
}

// Options are the config-driven behavior knobs New consumes.
type Options struct {
	ShowHints  bool
	Animations bool
	Greeting   string // empty picks a random tavern greeting

	// Integrations wiring (see sync.go). IntegrationsEnabled gates the whole
	// Jira/PR feature; SyncInterval is the refresh period (a 15s floor is
	// enforced by the caller); JiraBaseURL builds clickable Jira links.
	IntegrationsEnabled bool
	SyncInterval        time.Duration
	JiraBaseURL         string
	LDProject           string // LaunchDarkly project for rune (flag) lookups
	LDEnv               string // LaunchDarkly environment for rune state

	// CfgPath is config.toml's path — threaded through so a resize-drag
	// release can persist the new layout ratios (see endResizeDrag).
	CfgPath string
	// RailWidthRatio/RailBoxRatios seed the two-column Tavern's draggable
	// divider ratios from config (see internal/config.Layout). RailBoxRatios is
	// normalized to railBoxCount entries in New (older configs held 3).
	RailWidthRatio   float64
	RailBoxRatios    []float64
	DetailWidthRatio float64
	// CollapsedSections seeds which rail sections ("inbox"/"runes"/"someday")
	// start collapsed, from config (see internal/config.Layout).
	CollapsedSections []string
	// Sound configures the completion sounds (see sound.go / config.Sound).
	Sound config.Sound
}

func New(s *store.Store, path string, darkBg bool, opts Options) *Model {
	// The app opens into the Tavern (the full hall + pane), so seed a Tavern
	// greeting unless the config pins one.
	subtitle := opts.Greeting
	if subtitle == "" {
		subtitle = ui.RandomGreeting() // launch lands in the Tavern
	}
	railWidthRatio := opts.RailWidthRatio
	if railWidthRatio <= 0 {
		railWidthRatio = 0.34
	}
	railBoxRatios := normalizeRailRatios(opts.RailBoxRatios)
	detailWidthRatio := opts.DetailWidthRatio
	if detailWidthRatio <= 0 {
		detailWidthRatio = 0.42
	}
	m := &Model{
		store:             s,
		path:              path,
		wilds:             false, // launch into the Tavern (the full hall + pane), not Camp
		darkBg:            darkBg,
		cfgPath:           opts.CfgPath,
		railWidthRatio:    railWidthRatio,
		railBoxRatios:     railBoxRatios,
		detailWidthRatio:  detailWidthRatio,
		subtitle:          subtitle,
		collapsedProjects: map[string]bool{},
		newQuestIDs:       map[string]bool{},
		// Every section (Questboard / Runes / Vault) starts expanded, unless
		// config says otherwise (see internal/config.Layout).
		collapsedSections:   collapsedSectionsFromList(opts.CollapsedSections),
		sectionScroll:       map[string]int{},
		sectionMaxScroll:    map[string]int{},
		boxCache:            map[string]*boxCacheEntry{},
		selAnchor:           noSelection,
		selAnchorLine:       noSelection,
		hideHoverTips:       !opts.ShowHints,
		animate:             opts.Animations,
		lastSnapshot:        s.Snapshot(),
		integrationsEnabled: opts.IntegrationsEnabled,
		syncInterval:        opts.SyncInterval,
		jiraBaseURL:         opts.JiraBaseURL,
		ldProject:           opts.LDProject,
		ldEnv:               opts.LDEnv,
		soundCfg:            opts.Sound,
		prStatus:            map[string]PRStatus{},
		jiraStatus:          map[string]JiraStatus{},
		runeStatus:          map[string]RuneStatus{},
		lastFoundSHA:        map[string]string{},
		focusLinkIdx:        noSelection,
	}
	// Camp re-sorts by priority on every entry; a fresh launch is an entry too.
	m.store.WildsOrder = nil
	if rows := m.visibleRows(); len(rows) > 0 {
		m.setCursor(rows[0])
	}
	return m
}

// collapsedSectionsFromList builds the collapsedSections lookup map from
// config's flat list of collapsed section keys.
func collapsedSectionsFromList(sections []string) map[string]bool {
	m := make(map[string]bool, len(sections))
	for _, s := range sections {
		m[s] = true
	}
	return m
}

// SetDebug turns on per-message timing logs (QUESTS_DEBUG in main.go).
func (m *Model) SetDebug(on bool) {
	m.debug = on
}

func (m *Model) Init() tea.Cmd {
	// The splash ticker starts from the first WindowSizeMsg instead (see
	// Update) so it doesn't burn frames before there's a size to render into.
	// Start watching the quick-add spool so captures made elsewhere (CLI,
	// Raycast) show up live without a relaunch.
	// The app opens into the Tavern (see New) — play its arrival sound, not the
	// Wilds one.
	cmds := []tea.Cmd{m.watchQuickAdd(), m.playSound(sndEnterTavern)}
	if m.integrationsEnabled {
		// Fire the first sync almost immediately so linked PR/Jira/rune
		// statuses resolve on launch instead of sitting at "fetching…" for a
		// full interval; onSyncTick re-arms every tick after at syncInterval.
		cmds = append(cmds, syncTick(750*time.Millisecond))
		// Heartbeat for the Trails room's "synced Xs ago" counter.
		cmds = append(cmds, clockTick())
		// Poll herdr for pinned-agent state (fetches once up front, too).
		if c := m.maybeStartAgentPoll(); c != nil {
			cmds = append(cmds, c)
		}
		if c := m.maybeStartSpinner(); c != nil {
			cmds = append(cmds, c)
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.debug {
		now := time.Now()
		gap := now.Sub(m.lastMsgAt)
		m.lastMsgAt = now
		log.Printf("update: %T %+v (gap since last msg: %s)", msg, msg, gap)
	}

	switch msg := msg.(type) {
	case quickAddMsg:
		// A capture landed in the spool while we're running — ingest it and
		// keep listening. Cursor is tracked by identity, so appending quests
		// doesn't disturb it; the new row appears on the next render.
		if m.watcher != nil {
			before := make(map[string]bool, len(m.store.Quests))
			for i := range m.store.Quests {
				before[m.store.Quests[i].ID] = true
			}
			if n := quickadd.Drain(filepath.Dir(m.path), m.store); n > 0 {
				for i := range m.store.Quests {
					if id := m.store.Quests[i].ID; !before[id] {
						m.newQuestIDs[id] = true // flag where it landed, until selected/opened
					}
				}
				m.save()
				m.invalidateRender()
			}
			return m, waitForQuickAdd(m.watcher)
		}
		return m, nil

	case tea.WindowSizeMsg:
		// The splash's frame ticker only starts once we know the terminal
		// size — starting it from Init() ticks in the dark until the first
		// WindowSizeMsg arrives (which can lag on some terminals), burning
		// most of the animation before there's anything to render it into.
		firstSize := m.width == 0
		m.width, m.height = msg.Width, msg.Height
		if firstSize && m.animate {
			// Reveal the opening view with the environment animation.
			return m, m.beginTransition(nil, kindStartup)
		}
		return m, nil

	case transTickMsg:
		if msg.gen != m.transGen {
			return m, nil // stale ticker from an interrupted transition
		}
		return m, m.advanceTransition()

	case syncTickMsg:
		return m, m.onSyncTick()

	case syncResultMsg:
		m.applySyncResult(msg)
		return m, tea.Batch(m.maybeStartSpinner(), m.autoFindCmd(), m.autoStackExpandCmd())

	case agentsMsg:
		m.agents = msg.agents
		if m.healAgentPins() {
			m.save()
		}
		m.invalidateRender()
		return m, m.maybeStartSpinner()

	case runesMsg:
		for _, st := range msg.runes {
			m.runeStatus[st.Key] = st
		}
		m.invalidateRender()
		return m, m.maybeStartSpinner()

	case plansMsg:
		return m, m.applyPlans(msg)

	case findMsg:
		return m, m.applyFind(msg)

	case stackMsg:
		q := m.findQuest(msg.questID)
		if q == nil {
			return m, nil
		}
		var added []string
		for _, c := range msg.codes {
			before := len(q.PRs)
			appendPRLink(q, model.PRRef{Code: c, Repo: msg.repo})
			if len(q.PRs) > before {
				added = append(added, c)
			}
		}
		if len(added) == 0 {
			return m, nil
		}
		m.save()
		m.invalidateRender()
		cmds := []tea.Cmd{m.syncNow(added), m.maybeStartSpinner()}
		if m.detailOpenFor(msg.questID) { // feedback only while viewing the quest
			m.pendingConnBurstCode = added[0]
			cmds = append(cmds, m.playSound(sndAddConnection), m.pokeOverlayTick())
		}
		return m, tea.Batch(cmds...)

	case ventureTickMsg:
		// The Wilds session timer: re-arm once a second while still ventured.
		// Returning here re-renders the frame, so the count-up advances. A stale
		// gen (venture ended / restarted) just stops.
		if msg.gen != m.ventureTickGen || m.venturedID == "" {
			return m, nil
		}
		return m, ventureTick(m.ventureTickGen)

	case spinnerTickMsg:
		return m, m.onSpinnerTick(msg.gen)

	case clockTickMsg:
		return m, m.onClockTick()

	case overlayTickMsg:
		return m, m.onOverlayTick(msg.gen)

	case agentPollTickMsg:
		return m, m.onAgentPollTick(msg.gen)

	case warningExpireMsg:
		m.clearWarningIfCurrent(msg.gen)
		return m, nil

	case toastTickMsg:
		return m, m.advanceToast(msg.gen)

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// Mute toggle is global (works in any view/modal) and persists.
		if key.Matches(msg, Keys.MuteSound) {
			return m, m.toggleMute()
		}
		// Some terminals/multiplexers occasionally feed unparsed mouse-report
		// escapes (e.g. "[<65;80;30M" from scroll wheels) into the key stream;
		// drop them so they can't get typed into a title or body line. Right
		// after a wheel event, also drop any lone mouse-alphabet key (a "["
		// fragment fast scrolling split off), which the structural check
		// above can't tell from a real keystroke on its own.
		if isMouseLeak(tea.Key(msg)) {
			return m, nil
		}
		if time.Since(m.lastWheelAt) < 300*time.Millisecond && mouseAlphabetKey(tea.Key(msg)) {
			return m, nil
		}
		// The intro/transition animation is non-blocking: keys pass straight
		// through and act normally while it plays (it finishes on its own).
		if m.modal != nil {
			if isFocusModal(m.modal.Kind) && key.Matches(msg, Keys.Help) {
				m.pushModal(detailHelpModal())
				return m, nil
			}
			return m, m.updateModal(msg)
		}
		// handleKey runs first (a move may drop a cursor-trail ghost), then the
		// overlay ticker starts if anything's now animating.
		return m, tea.Batch(m.handleKey(msg), m.maybeStartOverlayTick())

	case tea.PasteMsg:
		// v2 decouples bracketed-paste from key events entirely (v1 delivered a
		// multi-line paste as a multi-rune KeyMsg); route it to whichever field
		// is actually focused.
		if m.transitioning() {
			return m, nil
		}
		if m.modal != nil {
			return m, m.pasteIntoModal(msg)
		}
		// Campaign notes edit through the shared body editor (m.editor is nil),
		// so a paste there — the common case of dropping in a document link —
		// routes to its own handler, which also shortens the pasted URL.
		if m.cursor.kind == ui.RowBodyLine && m.bodyOwnerKind == ownerCampaign {
			return m, m.pasteIntoCampaignNotes(msg)
		}
		if m.editor != nil {
			var cmd tea.Cmd
			*m.editor, cmd = m.editor.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.MouseClickMsg:
		if m.transitioning() {
			return m, nil // ignore mouse mid-animation
		}
		if msg.Mouse().Button == tea.MouseLeft {
			m.leftDown = true
		}
		if m.modal != nil {
			if isFocusModal(m.modal.Kind) {
				return m, m.handleFocusClick(msg)
			}
			if isPickerModal(m.modal.Kind) {
				return m, m.handlePickerClick(msg)
			}
			if m.modal.Kind == ModalSchedulePicker {
				return m, m.handleScheduleClick(msg)
			}
			if m.modal.Kind == ModalConfirmDelete {
				return m, m.handleConfirmDeleteClick(msg)
			}
			// A help / overlay modal: a click anywhere dismisses it (there's
			// nothing to interact with inside), matching "press any key to close".
			m.closeModal()
			return m, nil
		}
		return m, m.handleClick(msg)

	case tea.MouseReleaseMsg:
		if m.transitioning() {
			return m, nil
		}
		m.leftDown = false
		m.endResizeDrag()
		return m, nil

	case tea.MouseMotionMsg:
		if m.transitioning() {
			return m, nil // ignore mouse mid-animation
		}
		if m.modal != nil {
			if isFocusModal(m.modal.Kind) {
				return m, m.handleFocusMotion(msg)
			}
			return m, nil
		}
		return m, m.handleMotion(msg)

	case tea.MouseWheelMsg:
		if m.transitioning() {
			return m, nil // ignore mouse mid-animation
		}
		m.lastWheelAt = time.Now() // for the key-leak guard
		if m.modal != nil {
			if isFocusModal(m.modal.Kind) {
				return m, m.handleFocusWheel(msg)
			}
			if isPickerModal(m.modal.Kind) || m.modal.Kind == ModalSchedulePicker {
				return m, m.handlePickerWheel(msg)
			}
			return m, nil
		}
		return m, m.handleWheel(msg)
	}

	return m, nil
}

func (m *Model) findProject(id string) *model.Project {
	for i := range m.store.Projects {
		if m.store.Projects[i].ID == id {
			return &m.store.Projects[i]
		}
	}
	return nil
}

func (m *Model) findBanner(id string) *model.Banner {
	for i := range m.store.Banners {
		if m.store.Banners[i].ID == id {
			return &m.store.Banners[i]
		}
	}
	return nil
}

func (m *Model) findQuest(id string) *model.Quest {
	for i := range m.store.Quests {
		if m.store.Quests[i].ID == id {
			return &m.store.Quests[i]
		}
	}
	return nil
}

func (m *Model) projectQuestCount(id string) int {
	n := 0
	for _, q := range m.store.Quests {
		if q.ProjectID == id {
			n++
		}
	}
	return n
}

const undoLimit = 100

func (m *Model) save() {
	if !m.applyingUndo {
		m.recordUndo()
	}
	m.uiVersion++ // store content changed → invalidate the Tavern render cache
	_ = store.Save(m.path, m.store)
}

// invalidateRender marks the Tavern's cached section content stale — for
// changes that don't go through save() (collapse toggles, integration-status
// updates).
func (m *Model) invalidateRender() { m.uiVersion++ }

type clockTickMsg struct{}

// clockTick is a once-a-second heartbeat so the Trails room's "synced Xs ago"
// counter advances on its own. It re-renders only while that room is open, so
// idle Camp/detail views aren't churned.
func clockTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return clockTickMsg{} })
}

func (m *Model) onClockTick() tea.Cmd {
	// The synced-ago label rides on the resync affordance, which shows in the
	// Trails room and in a quest's detail Trails header — bump the render in
	// either so it counts up on its own.
	room := m.hallCursor.section
	inResyncRoom := m.modal == nil && m.inTavern() && m.hallCursor.kind == ui.RowSection &&
		(room == "trails" || room == "runes" || room == "lookouts")
	detailOpen := m.modal != nil && m.modal.Kind == ModalQuestDetail
	if inResyncRoom || detailOpen {
		m.invalidateRender()
	}
	return clockTick()
}

// recordUndo pushes the last snapshot onto the undo stack when the store has
// actually changed since it was taken, then remembers the new state. Called
// from save() (after the mutation), so the pushed snapshot is the pre-change
// state that Ctrl+Z restores.
func (m *Model) recordUndo() {
	cur := m.store.Snapshot()
	if bytes.Equal(cur, m.lastSnapshot) {
		return
	}
	if m.lastSnapshot != nil {
		m.undoStack = append(m.undoStack, m.lastSnapshot)
		if len(m.undoStack) > undoLimit {
			m.undoStack = m.undoStack[len(m.undoStack)-undoLimit:]
		}
	}
	m.lastSnapshot = cur
}

// undo restores the most recent pre-change store snapshot. Cursor is kept if
// its row still exists, else it lands on the nearest surviving row.
func (m *Model) undo() {
	if len(m.undoStack) == 0 {
		return
	}
	prev := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]

	restored, err := store.RestoreSnapshot(prev)
	if err != nil {
		return
	}
	*m.store = *restored
	m.lastSnapshot = prev

	m.applyingUndo = true
	m.save()
	m.applyingUndo = false

	m.editor = nil
	m.clearSelection()
	rows := m.visibleRows()
	if row, ok := nearestSelectableRow(rows, findRowIndex(rows, m.cursor)); ok {
		m.setCursor(row)
	}
}

// pushModal opens next, keeping whatever's currently open (if anything) on
// the stack beneath it.
func (m *Model) pushModal(next *Modal) {
	if m.modal != nil {
		m.modalStack = append(m.modalStack, m.modal)
	}
	m.modal = next
	m.hover = nil // mouse is disabled while any modal is open; don't leave a stale hint behind
	m.clearSelection()
	m.clearFocusLink() // a freshly opened focus view starts with the caret in the body
	m.focusScroll = 0  // a freshly opened focus view starts at the top
}

// closeModal closes the current modal, returning to whatever was beneath it
// on the stack (or to the outline if nothing was).
func (m *Model) closeModal() {
	// A selection made in the field being left behind (a body line, the
	// search box) must not leak into whatever's focused next — its anchor
	// is just a rune index, meaningless (and potentially out of bounds) for
	// a different, unrelated textinput.
	m.titleEditor = nil   // don't carry an open rename into the next view
	m.lookoutEditor = nil // nor an open lookout rename
	m.clearSelection()
	m.clearFocusLink()
	m.focusScroll = 0 // re-entering a view below re-scrolls to its caret
	if n := len(m.modalStack); n > 0 {
		m.modal = m.modalStack[n-1]
		m.modalStack = m.modalStack[:n-1]
		m.reseedBodyForModal()
		return
	}
	m.modal = nil
	// Back to the pane/outline: no modal owns the body editor. The Tavern pane
	// re-seeds ownerCampaign itself when the caret re-enters a campaign's notes.
	m.seedBody(ownerNone, "")
}

// reseedBodyForModal repoints the shared body editor at whatever detail modal
// is now current — used when popping back to a buried modal, since the body
// state is a single Model-level editor (not saved per-modal on the stack).
func (m *Model) reseedBodyForModal() {
	switch mod := m.modal; {
	case mod == nil:
		m.seedBody(ownerNone, "")
	case mod.Kind == ModalQuestDetail:
		m.seedBody(ownerQuest, mod.QuestID)
	default:
		m.seedBody(ownerNone, "")
	}
}

// visibleRows is the row list every navigation/mutation/render path should
// use — it applies the live search filter on top of ui.BuildRows so a
// filtered view can't be navigated "past" into hidden rows.
// chipSpan is a quick-chip's clickable extent in absolute screen columns.
type chipSpan struct {
	x0, x1 int
	filter quickFilter
}

// renderFilterLine renders the reserved line above the rows — blank now that
// search is a modal (and the Wilds quick-filter chips were removed).
func (m *Model) renderFilterLine(width int, margin string) string {
	m.chipSpans = nil
	return ""
}

// quickFilter is the fixed Wilds filter (Taken). The chip switcher was removed,
// so it never changes at runtime, but the type/matcher stays to scope the list.
type quickFilter int

const (
	filterTaken quickFilter = iota
	filterPriority
	filterAll
)

// wildsRows is the flat quest list shown out on the road: every quest under a
// non-archived campaign that passes the quick filter, tagged with its campaign
// name. Unlike the Tavern (grouped per campaign), the Wilds is one list sorted
// GLOBALLY by the same tier logic — priority/main to the top, low then done to
// the bottom — so it reads like a single focused agenda. No Questboard/Vault,
// no headers, no "+ New" affordances — those are Tavern activities.
func (m *Model) wildsRows() []ui.Row {
	byID := m.wildsEligible()
	now := time.Now()
	var rows []ui.Row
	first := true
	for _, id := range m.wildsOrderedIDs(byID) {
		q := byID[id]
		// Camp is today's agenda: the quests you've taken up (active) plus any
		// "called" ones — a muster due today or overdue — which show even while
		// still inactive, until you take them up. Done quests never show.
		if q.Status == model.StatusDone {
			continue
		}
		if q.Status != model.StatusActive && !q.Called(now) {
			continue
		}
		if !first {
			rows = append(rows, ui.Row{Kind: ui.RowSpacer}) // a little air between quests
		}
		first = false
		rows = append(rows, ui.Row{Kind: ui.RowQuest, ProjectID: q.ProjectID, QuestID: id, ShowProjectTag: true})
		rows = append(rows, wildsObjectiveRows(q)...)
	}
	return m.insertQuestMetaRows(rows)
}

// wildsObjectiveRows lists a quest's pending (not-done) objectives as
// indented child rows for the Wilds agenda. A done quest contributes none —
// its remaining objectives are moot once the whole quest is finished.
func wildsObjectiveRows(q model.Quest) []ui.Row {
	if q.Status == model.StatusDone {
		return nil
	}
	var rows []ui.Row
	for _, l := range q.Body {
		kind, _ := model.ClassifyBodyLine(l.Text)
		if kind != model.BodyObjective || l.Done {
			continue
		}
		rows = append(rows, ui.Row{Kind: ui.RowWildsObjective, ProjectID: q.ProjectID, QuestID: q.ID, BodyLineID: l.ID})
	}
	return rows
}

// wildsEligible maps every quest that can appear in the Wilds (under a
// non-archived campaign, not vaulted) by ID.
func (m *Model) wildsEligible() map[string]model.Quest {
	out := map[string]model.Quest{}
	for i := range m.store.Projects {
		if m.store.Projects[i].Archived {
			continue
		}
		for _, q := range ui.QuestsForCampaign(m.store, m.store.Projects[i].ID) {
			out[q.ID] = q
		}
	}
	// Loose quests under a banner (no campaign) are live work too — an active
	// one should reach Camp / the Wilds like any other. Any "called" quest (a
	// muster due today/overdue) is also eligible wherever it lives — including
	// the Questboard — so a scheduled errand surfaces on its day.
	now := time.Now()
	for _, q := range m.store.Quests {
		if q.Vaulted {
			continue
		}
		if (q.ProjectID == "" && q.BannerID != "") || q.Called(now) {
			out[q.ID] = q
		}
	}
	return out
}

// wildsOrderedIDs is the canonical Wilds order: quests the user has manually
// placed this visit (WildsOrder) first, in that order, then any remaining
// eligible quests sorted by tier (priority/main up, low/done down). WildsOrder
// is cleared on every entry (see setWilds), so a fresh trip always reads by
// priority; in-visit nudges reorder from there but don't persist.
func (m *Model) wildsOrderedIDs(byID map[string]model.Quest) []string {
	order := make([]string, 0, len(byID))
	seen := map[string]bool{}
	for _, id := range m.store.WildsOrder {
		if _, ok := byID[id]; ok && !seen[id] {
			order = append(order, id)
			seen[id] = true
		}
	}
	var rest []model.Quest
	for id, q := range byID {
		if !seen[id] {
			rest = append(rest, q)
		}
	}
	sort.SliceStable(rest, func(a, b int) bool {
		if ba, bb := ui.SortBucket(rest[a]), ui.SortBucket(rest[b]); ba != bb {
			return ba < bb
		}
		return rest[a].ID < rest[b].ID // deterministic tiebreak (map order isn't)
	})
	for _, q := range rest {
		order = append(order, q.ID)
	}
	return order
}

// moveWildsQuest reorders the cursor quest within the Wilds list by swapping it
// with its visible neighbor, persisting the new order to WildsOrder. This is
// independent of the Tavern's per-campaign order.
func (m *Model) moveWildsQuest(delta int) {
	if m.cursor.kind != ui.RowQuest {
		return
	}
	vis := m.wildsRows()
	vIdx := findRowIndex(vis, m.cursor)
	if vIdx < 0 {
		return
	}
	// Objectives are interleaved between quests now; step past them to the
	// neighboring quest so a nudge swaps quest-with-quest, not quest-with-child.
	nIdx := vIdx + delta
	for nIdx >= 0 && nIdx < len(vis) && vis[nIdx].Kind != ui.RowQuest {
		nIdx += delta
	}
	if nIdx < 0 || nIdx >= len(vis) {
		return
	}
	idA, idB := m.cursor.questID, vis[nIdx].QuestID
	full := m.wildsOrderedIDs(m.wildsEligible())
	ia, ib := indexOfStr(full, idA), indexOfStr(full, idB)
	if ia < 0 || ib < 0 {
		return
	}
	full[ia], full[ib] = full[ib], full[ia]
	// Kept in memory for this visit only — not persisted, so re-entering the
	// Wilds (see setWilds) resets to priority order.
	m.store.WildsOrder = full
}

func indexOfStr(ss []string, s string) int {
	for i, v := range ss {
		if v == s {
			return i
		}
	}
	return -1
}

// setWilds switches between the Tavern and Wilds views, replaying the
// intro transition and rerolling the flavor subtitle for the destination.
// Setting out defaults the quick chip to Taken (your active quests).
func (m *Model) setWilds(on bool) tea.Cmd {
	if m.wilds == on {
		return nil
	}
	m.commitEdit()
	old := m.currentBodyAbsolute() // snapshot the departing view (either layout) for the dissolve
	m.transOldSub = m.subtitle     // the subtitle to type out
	m.wilds = on
	m.editor = nil
	m.scrollOffset = 0
	if on {
		m.quickFilter = filterTaken
		// Re-sort by priority on every entry: manual nudges are per-visit only,
		// so a fresh trip to Camp always reads top-down by priority.
		m.store.WildsOrder = nil
		m.subtitle = ui.RandomCampGreeting()
	} else {
		m.subtitle = ui.RandomGreeting()
	}
	if rows := m.visibleRows(); len(rows) > 0 {
		m.setCursor(rows[0])
	} else {
		m.cursor = cursorTarget{}
	}
	cmd := m.beginTransition(old, kindMode)
	m.transAbsolute = true // the captured/revealed body lines carry their own margin
	snd := sndEnterTavern
	if on {
		snd = sndEnterWilds
	}
	return tea.Batch(cmd, m.playSound(snd))
}

// contentWidth is the centered column the outline/header/footer live in.
func (m *Model) contentWidth() int {
	cw := m.width - 4
	if cw > 80 {
		cw = 80
	}
	if cw < 20 {
		cw = 20
	}
	return cw
}

// animateFilter runs a filter change (chip/facet) wrapped in the fast
// dissolve→reveal so the list visibly re-forms; instant when animations off.
// insertQuestMetaRows previously inserted a RowQuestMeta integration sub-line
// under each linked quest. Connections now render as compact status icons
// inlined after the quest title (see connectionIcons / renderOutlineRowLine),
// so this is a passthrough — kept as the single hook in case per-row expansion
// returns later.
func (m *Model) insertQuestMetaRows(rows []ui.Row) []ui.Row {
	return rows
}

// inTavern reports whether the Tavern (its rooms) is the active view — i.e. not
// Camp and not mid-search.
func (m *Model) inTavern() bool {
	return !m.wilds
}

func (m *Model) visibleRows() []ui.Row {
	if m.venturing() {
		return m.ventureRows() // the single quest ventured into the Wilds
	}
	if m.wilds {
		return m.wildsRows()
	}
	// The Tavern is the hall + pane: the pane shows the current hall selection.
	m.ensureHallCursor()
	return m.paneRows()
}

// setCursor moves the cursor to row and (re)seeds the live editor for it —
// the row under the cursor is always editable text unless it's a section
// header or the "+ New Project" affordance.
func (m *Model) setCursor(row ui.Row) {
	m.cursor = targetFromRow(row)
	m.cursorMoved = true // so the Tavern scroll follows the cursor this frame
	m.clearSelection()
	if row.Kind == ui.RowQuest {
		delete(m.newQuestIDs, row.QuestID) // selecting a quick-add quest clears its "new" dot
	}

	switch row.Kind {
	case ui.RowProject:
		p := m.findProject(row.ProjectID)
		if p == nil {
			m.editor = nil
			return
		}
		ti := textinput.New()
		ti.Prompt = ""
		ti.SetValue(p.Name)
		ti.CursorEnd()
		_ = ti.Focus()
		m.editor = &ti
	case ui.RowBanner:
		b := m.findBanner(row.BannerID)
		if b == nil {
			m.editor = nil
			return
		}
		ti := textinput.New()
		ti.Prompt = ""
		ti.SetValue(b.Name)
		ti.CursorEnd()
		_ = ti.Focus()
		m.editor = &ti
	case ui.RowQuest:
		q := m.findQuest(row.QuestID)
		if q == nil {
			m.editor = nil
			return
		}
		ti := textinput.New()
		ti.Prompt = ""
		ti.SetValue(q.Title)
		ti.CursorEnd()
		_ = ti.Focus()
		m.editor = &ti
	case ui.RowWildsObjective:
		// A Wilds objective is inline-editable just like a quest title (click the
		// text or type) — seed the editor with the objective's display text
		// (the body line minus its "- " marker; see commitEdit for the write-back).
		q := m.findQuest(row.QuestID)
		if q == nil {
			m.editor = nil
			return
		}
		text := ""
		for _, l := range q.Body {
			if l.ID == row.BodyLineID {
				_, text = model.ClassifyBodyLine(l.Text)
				break
			}
		}
		ti := textinput.New()
		ti.Prompt = ""
		ti.SetValue(text)
		ti.CursorEnd()
		_ = ti.Focus()
		m.editor = &ti
	case ui.RowBodyLine:
		// Campaign notes use the shared body editor, not m.editor.
		m.editor = nil
		m.enterCampaignNotes(row)
	default:
		m.editor = nil
	}
}

// commitEdit writes the live editor's value back to whatever the cursor
// currently targets. Call before moving the cursor anywhere else.
func (m *Model) commitEdit() {
	// A campaign notes line is driven by the shared body editor, not m.editor,
	// so nav-away commits go through commitBodyLine (m.editor is nil here).
	if m.cursor.kind == ui.RowBodyLine {
		m.commitBodyLine()
		return
	}
	if m.editor == nil {
		return
	}
	value := strings.TrimSpace(m.editor.Value())
	switch m.cursor.kind {
	case ui.RowProject:
		if p := m.findProject(m.cursor.projectID); p != nil {
			p.Name = value
		}
	case ui.RowBanner:
		if b := m.findBanner(m.cursor.bannerID); b != nil {
			b.Name = value
		}
	case ui.RowQuest:
		if q := m.findQuest(m.cursor.questID); q != nil {
			q.Title = value
			q.UpdatedAt = time.Now()
		}
	case ui.RowWildsObjective:
		// Re-prefix the "- " objective marker; the line's ID/Done/Indent are
		// left untouched so an edit never disturbs its checked state or nesting.
		if q := m.findQuest(m.cursor.questID); q != nil {
			for i := range q.Body {
				if q.Body[i].ID == m.cursor.bodyLineID {
					q.Body[i].Text = "- " + value
					q.UpdatedAt = time.Now()
					break
				}
			}
		}
	}
	m.save()
}

// discardEmptyPaneDraft drops a still-unnamed quest or campaign in the Tavern
// pane when focus leaves it — the outliner rule that an empty new line vanishes
// if you never name it. Scoped to the pane so Camp/Wilds/modal editing is
// untouched, and guarded on questHasDetails/quest count so a row holding real
// data is never silently removed. Returns true when it deleted the cursor's row
// (removeCurrentRow having already relocated the cursor to a safe neighbor).
func (m *Model) discardEmptyPaneDraft() bool {
	if !m.inTavern() || m.hallFocus {
		return false
	}
	switch m.cursor.kind {
	case ui.RowQuest:
		q := m.findQuest(m.cursor.questID)
		if q != nil && strings.TrimSpace(q.Title) == "" && !questHasDetails(q) {
			id := q.ID
			m.removeCurrentRow(func() { m.deleteQuestByID(id) })
			return true
		}
	case ui.RowProject:
		p := m.findProject(m.cursor.projectID)
		if p != nil && strings.TrimSpace(p.Name) == "" && m.projectQuestCount(p.ID) == 0 {
			id := p.ID
			m.removeCurrentRow(func() { m.deleteProjectByID(id) })
			return true
		}
	}
	return false
}

// removeCurrentRow deletes the cursor's row via fn, then relocates the
// cursor to the previous selectable row within currentRowScope — never
// arbitrarily to the top, and never spilling out into the rest of the
// outline when the cursor is inside a focused section page.
func (m *Model) removeCurrentRow(fn func()) {
	rows := m.currentRowScope()
	idx := findRowIndex(rows, m.cursor)
	fn()

	newRows := m.currentRowScope()
	if row, ok := nearestSelectableRow(newRows, idx-1); ok {
		m.setCursor(row)
		return
	}
	m.editor = nil
}

// currentRowScope is the row list cursor navigation/removal should operate
// against — the full outline normally, or just one campaign's quest section
// (its quests plus "+ New Quest") while focused on that campaign's quest
// list, so deleting a quest there can't relocate the cursor out into the
// rest of the outline underneath.
func (m *Model) currentRowScope() []ui.Row {
	if m.modal != nil && m.modal.Kind == ModalSectionDetail {
		return m.sectionRows(m.modal.Section)
	}
	return m.visibleRows()
}

// toggleAllCampaigns collapses every campaign if any is currently expanded,
// or expands them all if every one is already collapsed — the reactive
// action behind Enter on the "Campaigns" label (see RenderRow's RowLabel
// case for the matching hint text).
func (m *Model) toggleAllCampaigns() {
	anyExpanded := false
	for _, p := range m.store.Projects {
		if !p.Archived && !m.collapsedProjects[p.ID] {
			anyExpanded = true
			break
		}
	}
	for _, p := range m.store.Projects {
		if !p.Archived {
			m.collapsedProjects[p.ID] = anyExpanded
		}
	}
	m.invalidateRender()
}

// stepSelectable finds the next selectable row from the cursor in the given
// direction (delta ±1), skipping spacers/day-headers/etc. Falls back to the
// first selectable row when the cursor isn't in the list.
func stepSelectable(rows []ui.Row, cur cursorTarget, delta int) (ui.Row, bool) {
	i := findRowIndex(rows, cur)
	if i < 0 {
		return nearestSelectableRow(rows, 0)
	}
	for j := i + delta; j >= 0 && j < len(rows); j += delta {
		if rows[j].Selectable() {
			return rows[j], true
		}
	}
	return ui.Row{}, false
}

// nearestSelectableRow finds the closest selectable row to idx, preferring
// to search backward first (so callers land on "the previous line") and
// falling back to searching forward if nothing selectable precedes it.
func nearestSelectableRow(rows []ui.Row, idx int) (ui.Row, bool) {
	if len(rows) == 0 {
		return ui.Row{}, false
	}
	if idx >= len(rows) {
		idx = len(rows) - 1
	}
	if idx < 0 {
		idx = 0
	}
	for i := idx; i >= 0; i-- {
		if rows[i].Selectable() {
			return rows[i], true
		}
	}
	for i := idx + 1; i < len(rows); i++ {
		if rows[i].Selectable() {
			return rows[i], true
		}
	}
	return ui.Row{}, false
}

// View wraps renderContent's string in a tea.View, declaring alt-screen and
// all-motion mouse reporting per-frame — v2 moved these off tea.NewProgram's
// options (WithAltScreen/WithMouseAllMotion) onto the View itself.
func (m *Model) View() tea.View {
	v := tea.NewView(m.padToScreen(m.compositeOverlay(m.renderContent())))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

// padToScreen forces every frame to exactly m.height lines. A frame whose line
// count changes between renders makes Bubble Tea's alt-screen renderer repaint
// the WHOLE screen, which flickers — most visibly when scrolling a view whose
// content overflows by only a line or two, where the fold hint / last content
// row toggles the height by one on the final scroll notch. Padding with blank
// lines (never moving content) keeps the size fixed so the renderer diffs in
// place. Frames are clipped to m.height defensively; in practice every view
// already fits within it.
func (m *Model) padToScreen(frame string) string {
	if m.height <= 0 {
		return frame
	}
	lines := strings.Split(frame, "\n")
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderContent() string {
	if m.width == 0 {
		return ""
	}

	if m.modal != nil {
		// Modals/focus views don't record the cursor's screen cell yet, so
		// suppress overlay effects there (a later phase adds detail-view
		// recording) rather than bursting at a stale outline position.
		m.cursorScreenY = -1
		if m.modal.Kind == ModalQuestDetail {
			return m.viewQuestDetail() // two bordered/scrollable panes + draggable divider
		}
		if isFocusModal(m.modal.Kind) {
			return m.renderFocusView()
		}
		return m.renderModal()
	}

	// A Camp⇄Tavern switch runs the SAME dissolve→pause→reveal as Camp⇄Wilds
	// (renderTransitionView, re-centered every frame), just with margin-baked
	// body lines — so both directions collapse to the header and grow back with
	// no bounce.
	if m.transitioning() && m.transKind == kindMode {
		return m.renderTransitionView()
	}

	// The Tavern is the borderless hall + content pane (master-detail), static
	// at rest.
	if m.inTavern() {
		return m.renderTavernView()
	}

	// Transitions use the single-column sliding dissolve/reveal (Camp/Wilds).
	if m.transitioning() {
		return m.renderTransitionView()
	}

	contentWidth := m.contentWidth()
	m.leftMargin = (m.width - contentWidth) / 2
	if m.leftMargin < 0 {
		m.leftMargin = 0
	}
	margin := strings.Repeat(" ", m.leftMargin)

	footer := indentLines(m.statusBar(contentWidth), margin)
	availableHeight := m.height - lipgloss.Height(footer)
	if availableHeight < 1 {
		availableHeight = 1
	}

	logoLines := m.renderHeader(contentWidth)
	logoHeight := len(logoLines) + 3 // blank after logo, reserved filter line, blank after filter

	// Keep viewVPad blank rows top and bottom, but never let the padding eat
	// more than half the screen (so short terminals stay usable). The logo +
	// rows block is centered within the region between them.
	vpad := viewVPad
	if maxPad := availableHeight / 4; vpad > maxPad {
		vpad = maxPad
	}
	if vpad < 0 {
		vpad = 0
	}
	innerHeight := availableHeight - 2*vpad
	if innerHeight < 1 {
		innerHeight = 1
	}
	viewHeight := innerHeight - logoHeight
	if viewHeight < 1 {
		viewHeight = 1
	}

	rows := m.visibleRows()
	idx := findRowIndex(rows, m.cursor)
	if idx < 0 && len(rows) > 0 {
		idx = 0
		m.setCursor(rows[0])
	}

	// Re-center on the cursor only when it actually moved (keyboard) — a wheel
	// scroll leaves the cursor put and must not be snapped back to it.
	if idx >= 0 && m.cursorMoved {
		if idx < m.scrollOffset {
			m.scrollOffset = idx
		}
		if idx >= m.scrollOffset+viewHeight {
			m.scrollOffset = idx - viewHeight + 1
		}
	}
	maxScroll := len(rows) - viewHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	m.scrollMax = maxScroll // let the wheel handler clamp/no-op at the ends
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	end := m.scrollOffset + viewHeight
	if end > len(rows) {
		end = len(rows)
	}
	shown := end - m.scrollOffset
	if shown < 0 {
		shown = 0
	}

	// Wilds empty-state help (e.g. the Taken chip with nothing taken up).
	var emptyHelp []string
	if m.wilds && len(rows) == 0 {
		emptyHelp = m.wildsEmptyHelp(contentWidth)
	}

	blockHeight := logoHeight + shown + len(emptyHelp)
	topPad := vpad + (innerHeight-blockHeight)/2
	if m.inTavern() {
		// Pin the header (mode toggle + door-strip) to a fixed top instead of
		// centering the block — so switching rooms, whose contents differ in
		// height, never shifts the header up or down. Only the body changes.
		topPad = vpad
	}
	if topPad < 0 {
		topPad = 0
	}
	bottomPad := availableHeight - blockHeight - topPad
	if bottomPad < 0 {
		bottomPad = 0
	}
	m.rowsScreenTop = topPad + logoHeight
	// Record the cursor's on-screen cell (for overlay effects like the
	// completion burst); -1 when it's scrolled out of view.
	m.cursorScreenY = -1
	if idx >= m.scrollOffset && idx < end {
		m.cursorScreenY = m.rowsScreenTop + (idx - m.scrollOffset)
		m.cursorScreenX = m.leftMargin // the cursor "›" marker column
	}
	m.modeToggleRow = topPad          // the header's first line is the TAVERN/CAMP toggle
	m.tavernHelpRow = m.modeToggleRow // F1 help sits on the header row
	// The reserved filter/chip line sits just above the rows (after the logo
	// and its blank line) — its screen row is used for chip click hit-testing.
	m.chipLineRow = topPad + len(logoLines) + 1

	m.hintSpans = map[int][]hintSpan{}
	m.codeSpans = map[int][]codeSpan{}
	hoverIdx := -1
	if m.hover != nil {
		hoverIdx = findRowIndex(rows, *m.hover)
	}
	vaultIdx := -1
	for i, r := range rows {
		if r.Kind == ui.RowSection && r.Section == "someday" {
			vaultIdx = i
			break
		}
	}

	clip := lipgloss.NewStyle().MaxWidth(m.width)
	var b strings.Builder
	for i := 0; i < topPad; i++ {
		b.WriteString("\n")
	}
	for _, line := range logoLines {
		b.WriteString(margin + line + "\n")
	}
	// The blank row between the logo and the rows doubles as a "more above"
	// hint when the list is scrolled down past its start.
	if m.scrollOffset > 0 {
		b.WriteString(foldHint(margin, contentWidth) + "\n")
	} else {
		b.WriteString("\n")
	}
	// The reserved filter line: Wilds quick chips, the search bar when open,
	// or blank — always present so toggling it never reflows the list.
	b.WriteString(clip.Render(m.renderFilterLine(contentWidth, margin)) + "\n")
	b.WriteString("\n") // breathing room between the filter line and the list
	for i := m.scrollOffset; i < end; i++ {
		b.WriteString(clip.Render(margin + m.renderOutlineRowLine(rows, i, idx, hoverIdx, vaultIdx, contentWidth, m.leftMargin)))
		b.WriteString("\n")
	}
	for _, line := range emptyHelp {
		b.WriteString(clip.Render(margin+line) + "\n")
	}
	for i := 0; i < bottomPad; i++ {
		// First row of the bottom padding signals "more below".
		if i == 0 && end < len(rows) {
			b.WriteString(foldHint(margin, contentWidth) + "\n")
			continue
		}
		b.WriteString("\n")
	}

	m.cursorMoved = false // consumed; the wheel scrolls freely until the next key move
	// A couple of blank lines set the footer apart from the content body.
	return strings.TrimRight(b.String(), "\n") + "\n\n\n" + footer
}

// renderOutlineRowLine renders one outline row (or its integration meta
// sub-line) to a single line WITHOUT the leading margin, recording any
// clickable hint/code spans against absolute row index i. Shared by the
// single-column View and the two-column left column so they render and
// hit-test identically. width is the column width (contentWidth for a single
// column, the left column's width in two-column mode).
func (m *Model) renderOutlineRowLine(rows []ui.Row, i, idx, hoverIdx, vaultIdx, width, xBase int) string {
	row := rows[i]
	if row.Kind == ui.RowQuestMeta {
		line, spans := m.renderQuestMetaLine(row, width, xBase)
		if len(spans) > 0 {
			m.codeSpans[i] = spans
		}
		return line
	}
	isCursor := i == idx
	warning := m.warningText != "" && m.warningTarget.matches(row)
	titleView := ""
	if warning {
		titleView = ui.StyleMuted.Render(m.warningText)
	} else {
		titleView = m.rowTitleView(row, isCursor)
	}
	// Action hints render on the fixed bottom status line (see statusBar), never
	// inline — an inline hint on a long row wrapped and shifted the layout. The
	// only inline exception is the Vault's read-only note, which is row-specific
	// state, not a generic action tip.
	hint := ""
	if !warning && !m.hideHoverTips && row.Kind == ui.RowSection && row.Section == "someday" && vaultIdx >= 0 && hoverIdx >= vaultIdx {
		hint = "  " + ui.StyleMuted.Render("(read only)")
	}
	rendered, _ := ui.RenderRow(row, m.store, titleView, isCursor, m.isNewQuest(row), width, hint)
	if !warning {
		rendered = m.withConnectionIcons(rendered, row) // inline emblems (quests)
	}
	return rendered
}

// rowTitleView is the title text for ANY list row — the live editor when it's
// the cursor, else the kind-specific content. ONE definition, used by every
// surface that renders rows (the outline, both Tavern columns, and the
// campaign/section focus pages) so a change to how a row looks lands in all of
// them at once. Returns "" for rows RenderRow styles itself (plain campaigns,
// sections, "+ New …", etc. — unless they're the cursor being edited).
func (m *Model) rowTitleView(row ui.Row, isCursor bool) string {
	switch row.Kind {
	case ui.RowQuest:
		return m.questTitleView(row, isCursor)
	case ui.RowRune:
		return m.runeRowContent(row.QuestID, row.RuneKey)
	case ui.RowLookout:
		return m.lookoutRowTitle(row.QuestID, row.LookoutURL)
	case ui.RowTrack:
		return m.trackRowContent(row.QuestID, row.TrackEvent)
	case ui.RowTrail:
		return m.trailRowContent(row.Code)
	case ui.RowTrailQuest, ui.RowRuneQuest, ui.RowLookoutQuest:
		return m.roomQuestTitle(row, isCursor)
	case ui.RowBodyLine:
		// The active notes line renders the shared body editor (muted prose).
		if isCursor && m.bodyOwnerKind == ownerCampaign {
			return m.renderEditableStyled(&m.bodyEditor, ui.StyleMuted)
		}
		return ""
	}
	if isCursor && m.editor != nil {
		return m.renderEditableStyled(m.editor, m.cursorTitleStyle(row))
	}
	return ""
}

// withConnectionIcons appends a quest's inline status emblems (NPC/Jira/PR/
// Rune/Track/Lookout) after its rendered line — shown on every list a quest
// appears in (outline, Tavern, campaign/section pages), focused or not.
func (m *Model) withConnectionIcons(rendered string, row ui.Row) string {
	if row.Kind == ui.RowQuest && m.integrationsEnabled {
		if q := m.findQuest(row.QuestID); q != nil {
			return rendered + m.connectionIcons(q)
		}
	}
	return rendered
}

// constantWidthTitle renders an editable title at the SAME width whether it's
// being edited or not, so content after it (a chip, emblems, progress) never
// shifts — not when the caret moves AND not when you select the line to edit.
// Editing → the live editor with the end-caret cell reserved; plain → the
// styled text with the same one cell reserved. THE shared title renderer for
// the Tavern list and the detail pages (see docs/ui-consistency.md).
func (m *Model) constantWidthTitle(text string, editor *textinput.Model, editStyle, plainStyle lipgloss.Style) string {
	if editor != nil {
		return m.renderEditableFixedWidth(editor, editStyle)
	}
	return plainStyle.Render(text) + " " // reserve the caret cell the edited state uses
}

// questTitleView renders a quest row's title at a constant width (see
// constantWidthTitle) so the emblems/progress after it never shift.
func (m *Model) questTitleView(row ui.Row, isCursor bool) string {
	q := m.findQuest(row.QuestID)
	if q == nil {
		return ""
	}
	plainStyle := ui.StyleName
	if q.Status == model.StatusDone {
		plainStyle = ui.StyleDone
	}
	var editor *textinput.Model
	if isCursor {
		editor = m.editor // nil-safe: nil falls through to the plain render
	}
	return m.constantWidthTitle(q.Title, editor, m.cursorTitleStyle(row), plainStyle)
}

// wildsEmptyHelp is the centered flavor + how-to shown when the Wilds list
// is empty for the active chip — most usefully explaining how to take quests.
func (m *Model) wildsEmptyHelp(width int) []string {
	var msg, hint string
	switch {
	case m.quickFilter == filterTaken:
		msg = "The road is quiet — nothing taken up."
		hint = "Take up a quest with Ctrl+A"
	case m.quickFilter == filterPriority:
		msg = "No priority quests wilds."
		hint = "Flag one with Ctrl+P"
	default:
		msg = "No quests under any campaign yet."
	}
	lines := []string{ui.CenterText(ui.StyleMuted.Render(msg), width)}
	if hint != "" {
		lines = append(lines, ui.CenterText(ui.StyleMuted.Render(hint), width))
	}
	return lines
}

// hintPart is one "<icon> <verb> (<key>)" action tip; action names the key
// it stands in for ("enter"/"tab"), so a mouse click on the rendered label
// can trigger the same thing (see hintSpan / handleMouse).
type hintPart struct {
	key  string // the keyboard key, e.g. "enter", "tab", "c", "ctrl+d"
	verb string // what it does, e.g. "open", "copy", "mark done"
}

// hintSpan is a hint part's clickable extent in absolute screen columns.
type hintSpan struct {
	x0, x1 int
	action string
}

// actionHintParts lists the action tips for row as (key, verb) pairs — e.g.
// collapse + open for a campaign, just open for a quest. Rendered uniformly as
// "<key> to <verb>" by renderHintParts.
func actionHintParts(row ui.Row) []hintPart {
	switch row.Kind {
	case ui.RowProject, ui.RowSection, ui.RowLabel:
		return []hintPart{{"enter", collapseVerb(row.Collapsed)}, {"tab", "open"}}
	case ui.RowQuest:
		return []hintPart{{"tab", "open"}}
	case ui.RowWildsObjective:
		return []hintPart{{"ctrl+d", "mark done"}}
	case ui.RowRune:
		return []hintPart{{"enter", "open"}, {"c", "copy"}}
	case ui.RowTrack:
		return []hintPart{{"c", "copy"}}
	case ui.RowLookout:
		return []hintPart{{"enter", "open"}, {"c", "copy"}, {"r", "rename"}}
	case ui.RowRuneQuest, ui.RowLookoutQuest, ui.RowVaultHeader:
		// Collapsible group headers (a quest's runes/lookouts, the Vaulted group).
		return []hintPart{{"enter", collapseVerb(row.Collapsed)}}
	case ui.RowNewProject, ui.RowNewQuest:
		return []hintPart{{"enter", "add"}}
	case ui.RowVaultCampaign:
		return []hintPart{{"tab", "open"}}
	}
	return nil
}

// keyHint renders a single "<key> to <verb>" hint — the key in the muted-orange
// key style, the rest muted. THE one hint format, used by every view (status
// bar, sigil status line). Change it here and everywhere follows.
func keyHint(key, verb string) string {
	return ui.StyleKey.Render(key) + ui.StyleMuted.Render(" to "+verb)
}

// joinHints joins several "<key> to <verb>" hints with a muted middot.
func joinHints(parts ...string) string {
	return strings.Join(parts, ui.StyleMuted.Render(" · "))
}

// renderHintParts renders the joined hints, prefixed with a two-space gap.
func renderHintParts(parts []hintPart) string {
	if len(parts) == 0 {
		return ""
	}
	hs := make([]string, len(parts))
	for i, p := range parts {
		hs[i] = keyHint(p.key, p.verb)
	}
	return "  " + joinHints(hs...)
}

// isNewQuest reports whether a row is a quest freshly ingested from quick-add
// and not yet selected/opened — it shows the blue "●" landing marker.
func (m *Model) isNewQuest(row ui.Row) bool {
	return row.Kind == ui.RowQuest && m.newQuestIDs[row.QuestID]
}

// collapseVerb is the verb for a collapsible row's enter action.
func collapseVerb(collapsed bool) string {
	if collapsed {
		return "expand"
	}
	return "collapse"
}

// renderFooter is deliberately just a short, right-aligned pointer to the
// help overlay — not an inline dump of every keybinding.
// statusRow resolves the row whose actions the status line should show — the
// mouse-hovered row when hovering (so hover-discovery survives the move off
// inline hints), else the cursor/selected row — searching whichever row set
// the active view uses.
func (m *Model) statusRow() (ui.Row, bool) {
	target := m.cursor
	if m.hover != nil {
		target = *m.hover
	}
	var sets [][]ui.Row
	switch {
	case m.modal != nil && m.modal.Kind == ModalSectionDetail:
		sets = [][]ui.Row{m.sectionRows(m.modal.Section)}
	default:
		sets = [][]ui.Row{m.visibleRows()}
	}
	for _, rows := range sets {
		if idx := findRowIndex(rows, target); idx >= 0 {
			return rows[idx], true
		}
	}
	return ui.Row{}, false
}

// statusHint is the action-hint text shown on the fixed bottom status line in
// EVERY view (never inline — inline hints wrapped long rows and shifted the
// layout). In the quest detail it's the focused sigil's actions; elsewhere the
// hovered-or-cursor row's. Empty when hints are toggled off (Ctrl+K).
func (m *Model) statusHint() string {
	if m.hideHoverTips {
		return ""
	}
	if m.venturing() {
		// The Wilds is distraction-free: one clear instruction, not per-row tips.
		return renderHintParts([]hintPart{{"ctrl+d", "finish"}, {"esc", "make camp"}})
	}
	// At Camp, a quest can be peeked (Tab) or ventured into the Wilds (Enter).
	if m.wilds {
		if row, ok := m.statusRow(); ok && row.Kind == ui.RowQuest {
			return renderHintParts([]hintPart{{"tab", "peek"}, {"enter", "venture"}, {"ctrl+e", "schedule"}})
		}
	}
	if m.modal != nil && m.modal.Kind == ModalQuestDetail {
		if q := m.findQuest(m.modal.QuestID); q != nil {
			return m.sigilStatusLine(q)
		}
		return ""
	}
	// The Tavern's shortcuts follow the selected line: Enter adds, Tab navigates,
	// Ctrl+N is the "new" one level up, Esc steps back.
	if m.inTavern() && m.modal == nil {
		return renderHintParts(m.tavernHint())
	}
	if row, ok := m.statusRow(); ok {
		return renderHintParts(actionHintParts(row))
	}
	return ""
}

// tavernHint is the footer's contextual shortcut list for the selected hall or
// pane line.
func (m *Model) tavernHint() []hintPart {
	if m.hallFocus {
		switch m.hallCursor.kind {
		case ui.RowBanner, ui.RowProject:
			// Errands hold quests, not campaigns — Enter there adds a quest.
			addVerb := "new campaign"
			if m.hallCursor.kind == ui.RowBanner && m.hallCursor.bannerID == errandsBanner {
				addVerb = "new quest"
			}
			return []hintPart{{"enter", addVerb}, {"tab", "open"}, {"ctrl+n", "new banner"}}
		case ui.RowSection:
			return []hintPart{{"tab", "open"}, {"ctrl+n", "new banner"}}
		}
		return []hintPart{{"ctrl+n", "new banner"}}
	}
	switch m.cursor.kind {
	case ui.RowQuest:
		return []hintPart{{"enter", "add"}, {"tab", "open"}, {"ctrl+e", "schedule"}, {"esc", "back"}}
	case ui.RowProject:
		// On a campaign's own pane header, Ctrl+L links the next saga chapter.
		if m.hallCursor.kind == ui.RowProject && m.cursor.projectID == m.hallCursor.projectID {
			return []hintPart{{"enter", "add"}, {"tab", "open"}, {"ctrl+l", "link chapter"}, {"esc", "back"}}
		}
		return []hintPart{{"enter", "add"}, {"tab", "open"}, {"esc", "back"}}
	case ui.RowSagaLink:
		return []hintPart{{"enter", "open chapter"}, {"esc", "back"}}
	case ui.RowBanner:
		// Errands add quests; every other area adds campaigns.
		if m.cursor.bannerID == errandsBanner {
			return []hintPart{{"enter", "add quest"}, {"esc", "back"}}
		}
		return []hintPart{{"enter", "add campaign"}, {"esc", "back"}}
	case ui.RowNewQuest, ui.RowNewProject:
		return []hintPart{{"enter", "add"}, {"esc", "back"}}
	case ui.RowVaultHeader:
		return []hintPart{{"enter", "toggle"}, {"esc", "back"}}
	case ui.RowRune, ui.RowLookout, ui.RowTrail:
		// A sigil line: Enter opens its own link, Tab jumps to the owning quest.
		parts := []hintPart{{"enter", "open"}, {"tab", "quest"}, {"c", "copy"}}
		switch m.cursor.kind {
		case ui.RowLookout:
			parts = append(parts, hintPart{"r", "rename"})
		default: // RowRune / RowTrail — found from trails, so "r" resyncs
			parts = append(parts, hintPart{"r", "resync"})
		}
		return append(parts, hintPart{"esc", "back"})
	case ui.RowTrack:
		return []hintPart{{"tab", "quest"}, {"c", "copy"}, {"r", "resync"}, {"esc", "back"}}
	case ui.RowTrailQuest, ui.RowRuneQuest, ui.RowLookoutQuest:
		// Room quest headers aren't collapsible — Enter/r resync the quest.
		return []hintPart{{"enter", "resync"}, {"tab", "quest"}, {"esc", "back"}}
	}
	return []hintPart{{"esc", "back"}}
}

// statusBar is the app-wide bottom line: the current selection's action hints
// on the left, the taken-up count on the right — one consistent presentation
// across the Tavern, Wilds, section pages, and the quest detail.
func (m *Model) statusBar(width int) string {
	left := strings.TrimPrefix(m.statusHint(), "  ") // renderHintParts prepends a 2-space gap; the bar owns spacing
	right := ""
	// Camp/Wilds stay uncluttered — no counts here. The Tavern shows how many
	// quests are currently taken up.
	if !m.wilds {
		if taken := m.takenCount(); taken > 0 {
			right = ui.StyleFooter.Render(fmt.Sprintf("%d taken up", taken))
		}
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// takenCount is how many quests are currently taken up (active) under a
// non-archived campaign — the number you'd take Wilds.
func (m *Model) takenCount() int {
	n := 0
	for i := range m.store.Quests {
		q := &m.store.Quests[i]
		if q.Status == model.StatusActive && q.ProjectID != "" && !q.Vaulted {
			if p := m.findProject(q.ProjectID); p != nil && !p.Archived {
				n++
			}
		}
	}
	return n
}

// indentLines prepends prefix to every line of s (a possibly multi-line,
// already-wrapped block), not just the first.
func indentLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
