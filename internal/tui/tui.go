package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tt-a1i/doppel/internal/core/apperr"
	"github.com/tt-a1i/doppel/internal/core/appinfo"
	"github.com/tt-a1i/doppel/internal/core/clone"
	"github.com/tt-a1i/doppel/internal/core/doctor"
	"github.com/tt-a1i/doppel/internal/core/macos"
)

// ——— Layout ———————————————————————————————————————————————————————

const (
	cardMinW = 64
	cardMaxW = 96
	cardHPad = 2
	cardVPad = 1
	// overhead = border (2) + padding (2*cardHPad)
	cardOverheadW = 2 + 2*cardHPad
)

// cardWidth picks a reasonable card width for the current terminal.
func (m model) cardWidth() int {
	w := m.width - 4
	if w < cardMinW {
		w = cardMinW
	}
	if w > cardMaxW {
		w = cardMaxW
	}
	return w
}

// innerWidth returns usable content width inside a card.
func (m model) innerWidth() int {
	w := m.cardWidth() - cardOverheadW
	if w < 20 {
		w = 20
	}
	return w
}

// ——— Theme ——————————————————————————————————————————————————————————

var (
	cPrimary = lipgloss.Color("212") // rose — focus, keys, titles
	cOK      = lipgloss.Color("42")  // green — success
	cWarn    = lipgloss.Color("214") // amber — warning / attention
	cErr     = lipgloss.Color("196") // red — failure
	cMuted   = lipgloss.Color("240") // structure (borders, separators)
	cDim     = lipgloss.Color("245") // de-emphasized text
	cInk     = lipgloss.Color("231") // near-white for banner text
	cPaper   = lipgloss.Color("16")  // near-black for banner text on light bg
)

// Text roles — use consistently.
var (
	tTitle   = lipgloss.NewStyle().Bold(true).Foreground(cPrimary)
	tLabel   = lipgloss.NewStyle().Foreground(cMuted).Bold(true)
	tCaption = lipgloss.NewStyle().Foreground(cDim)
	tFaint   = lipgloss.NewStyle().Foreground(cDim)
	tKey     = lipgloss.NewStyle().Foreground(cPrimary).Bold(true)
	tOK      = lipgloss.NewStyle().Foreground(cOK)
	tOKBold  = lipgloss.NewStyle().Foreground(cOK).Bold(true)
	tWarn    = lipgloss.NewStyle().Foreground(cWarn)
	tErr     = lipgloss.NewStyle().Foreground(cErr).Bold(true)
	tToast   = lipgloss.NewStyle().Foreground(cPrimary).Bold(true)
)

// Chrome styles.
var (
	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cMuted).
			Padding(cardVPad, cardHPad)

	footerStyle = lipgloss.NewStyle().
			Foreground(cDim).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(cMuted).
			Padding(0, 1)

	// Full-width status banners (screen width minus card border).
	bannerOK = lipgloss.NewStyle().
			Bold(true).
			Foreground(cInk).
			Background(cOK).
			Padding(0, 2)
	bannerWarn = lipgloss.NewStyle().
			Bold(true).
			Foreground(cPaper).
			Background(cWarn).
			Padding(0, 2)
	bannerErr = lipgloss.NewStyle().
			Bold(true).
			Foreground(cInk).
			Background(cErr).
			Padding(0, 2)
)

// ——— Screens ————————————————————————————————————————————————————————

type screen int

const (
	scrList screen = iota
	scrPath
	scrForm
	scrProgress
	scrResult
)

var stepLabels = []string{"Pick", "Configure", "Clone", "Done"}

// Stage order drives the fixed progress panel.
var stageOrder = []string{"copy", "plist", "entitlements", "discover", "resign", "verify"}

// Friendly labels for the stage names shown in progress + result views.
var stageLabel = map[string]string{
	"copy":         "Copy files",
	"plist":        "Update identity",
	"entitlements": "Adjust permissions",
	"discover":     "Scan components",
	"resign":       "Re-sign code",
	"verify":       "Verify signature",
}

// ——— Messages ———————————————————————————————————————————————————————

type appsLoadedMsg struct {
	items   []list.Item
	skipped []appinfo.ScanSkip
}
type appSelectedMsg struct {
	report *appinfo.InspectionReport
	err    error
}
type planBuiltMsg struct {
	plan              *clone.ClonePlan
	preflightFindings []doctor.Finding
	err               error
}
type pipelineMsg struct {
	event  *clone.StageEvent
	done   bool
	result *clone.RunResult
	err    error
}
type toastClearMsg time.Time

// ——— Model ————————————————————————————————————————————————————————————

type stageState struct {
	status clone.StageStatus
	msg    string
	start  time.Time
	end    time.Time
}

type model struct {
	screen        screen
	width, height int

	// list
	list        list.Model
	listLoading bool
	listSpin    spinner.Model
	appCount    int
	listSkipped []appinfo.ScanSkip

	// manual path
	pathInput textinput.Model
	pathErr   string

	// form
	selected      *appinfo.InspectionReport
	inputs        []textinput.Model
	formIdx       int
	formErr       string
	formErrDetail string
	launchTest    bool

	// progress
	plan        *clone.ClonePlan
	stages      map[string]*stageState
	activeStage string
	progSpin    spinner.Model
	pipeCh      chan pipelineMsg

	// result
	result            *clone.RunResult
	preflightFindings []doctor.Finding
	runErr            error

	// help overlay
	showHelp bool

	// toast
	toast   string
	toastAt time.Time
}

// ——— Entry ———————————————————————————————————————————————————————————

func Run() error {
	_, err := tea.NewProgram(initialModel(), tea.WithAltScreen()).Run()
	return err
}

func initialModel() model {
	loadSpin := spinner.New()
	loadSpin.Spinner = spinner.Dot
	loadSpin.Style = lipgloss.NewStyle().Foreground(cPrimary)

	progSpin := spinner.New()
	progSpin.Spinner = spinner.MiniDot
	progSpin.Style = lipgloss.NewStyle().Foreground(cPrimary)

	l := list.New(nil, itemDelegate{}, 0, 0)
	l.Title = "Pick an app to clone"
	l.Styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(cInk).
		Background(cPrimary).
		Padding(0, 1)
	l.Styles.StatusBar = lipgloss.NewStyle().Foreground(cDim).Padding(0, 0, 1, 2)
	l.Styles.NoItems = lipgloss.NewStyle().Foreground(cDim).Padding(1, 2)
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()

	pathInput := textinput.New()
	pathInput.Placeholder = "/Applications/App.app"
	pathInput.CharLimit = 1024
	pathInput.Prompt = "› "
	pathInput.PromptStyle = lipgloss.NewStyle().Foreground(cPrimary)
	pathInput.PlaceholderStyle = lipgloss.NewStyle().Foreground(cDim).Italic(true)

	return model{
		screen:      scrList,
		listLoading: true,
		listSpin:    loadSpin,
		progSpin:    progSpin,
		list:        l,
		pathInput:   pathInput,
		stages:      map[string]*stageState{},
		launchTest:  true,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.listSpin.Tick, scanAppsCmd())
}

// ——— Custom list delegate ———————————————————————————————————————————

type itemDelegate struct{}

func (itemDelegate) Height() int                         { return 2 }
func (itemDelegate) Spacing() int                        { return 1 }
func (itemDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, li list.Item) {
	it, ok := li.(appItem)
	if !ok {
		return
	}
	selected := index == m.Index()

	// Work out inside-card width. m.Width() is the list widget width.
	width := m.Width() - 2
	if width < 40 {
		width = 40
	}

	var prefix, nameS, metaS, bidS lipgloss.Style
	if selected {
		prefix = lipgloss.NewStyle().Foreground(cPrimary).Bold(true)
		nameS = lipgloss.NewStyle().Foreground(cPrimary).Bold(true)
		metaS = lipgloss.NewStyle().Foreground(cPrimary)
		bidS = lipgloss.NewStyle().Foreground(cPrimary)
	} else {
		prefix = lipgloss.NewStyle().Foreground(cMuted)
		nameS = lipgloss.NewStyle()
		metaS = lipgloss.NewStyle().Foreground(cDim)
		bidS = lipgloss.NewStyle().Foreground(cDim)
	}

	arrow := prefix.Render("▸ ")
	if !selected {
		arrow = "  "
	}

	// Line 1: arrow + name ......................  v1.2.3
	ver := it.version
	if ver == "" {
		ver = "—"
	}
	verText := metaS.Render("v" + ver)

	nameAvail := width - 2 - lipgloss.Width(verText) - 1
	name := truncateRunes(it.name, nameAvail)
	nameText := nameS.Render(name)
	line1 := arrow + nameText + padTo(width-2, lipgloss.Width(nameText)+lipgloss.Width(verText)) + verText

	// Line 2:   bundle.id ...........................  · signed
	var tagText string
	if it.signed {
		tagText = lipgloss.NewStyle().Foreground(cOK).Render("· signed")
	} else {
		tagText = lipgloss.NewStyle().Foreground(cWarn).Render("· unsigned")
	}
	bidAvail := width - 2 - lipgloss.Width(tagText) - 1
	bid := truncateRunes(it.bundleID, bidAvail)
	bidText := bidS.Render(bid)
	line2 := "  " + bidText + padTo(width-2, lipgloss.Width(bidText)+lipgloss.Width(tagText)) + tagText

	fmt.Fprint(w, line1+"\n"+line2)
}

func padTo(total, used int) string {
	n := total - used
	if n < 1 {
		n = 1
	}
	return strings.Repeat(" ", n)
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max < 2 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}

// ——— App scanning ——————————————————————————————————————————————————————

type appItem struct {
	path     string
	name     string
	bundleID string
	version  string
	signed   bool
}

func (i appItem) FilterValue() string { return i.name + " " + i.bundleID }

func scanAppsCmd() tea.Cmd {
	return func() tea.Msg {
		result, _ := appinfo.ScanInstalled(nil)
		items := make([]list.Item, 0, len(result.Reports))
		for _, r := range result.Reports {
			name := r.Identity.DisplayName
			if name == "" {
				name = r.Identity.BundleName
			}
			if name == "" {
				name = strings.TrimSuffix(filepath.Base(r.Identity.AppPath), ".app")
			}
			items = append(items, appItem{
				path:     r.Identity.AppPath,
				name:     name,
				bundleID: r.Identity.BundleID,
				version:  r.Identity.Version,
				signed:   r.HasSignature,
			})
		}
		return appsLoadedMsg{items: items, skipped: result.Skipped}
	}
}

// ——— Root Update ———————————————————————————————————————————————————————

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// List fills inside of the card, minus inner margins the delegate
		// accounts for itself.
		m.list.SetSize(m.innerWidth(), m.height-8)
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if msg.String() == "?" {
			typing := (m.screen == scrForm) ||
				(m.screen == scrPath) ||
				(m.screen == scrList && m.list.SettingFilter())
			if !typing {
				m.showHelp = !m.showHelp
				return m, nil
			}
		}
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
	case toastClearMsg:
		if time.Time(msg).Equal(m.toastAt) {
			m.toast = ""
		}
		return m, nil
	}

	switch m.screen {
	case scrList:
		return m.updateList(msg)
	case scrPath:
		return m.updatePath(msg)
	case scrForm:
		return m.updateForm(msg)
	case scrProgress:
		return m.updateProgress(msg)
	case scrResult:
		return m.updateResult(msg)
	}
	return m, nil
}

// ——— Screen: list ———————————————————————————————————————————————————————

func (m model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.listLoading {
			var cmd tea.Cmd
			m.listSpin, cmd = m.listSpin.Update(msg)
			return m, cmd
		}
	case appsLoadedMsg:
		m.listLoading = false
		m.appCount = len(msg.items)
		m.listSkipped = msg.skipped
		m.list.Title = fmt.Sprintf("Pick an app  ·  %d apps  ·  %d skipped", m.appCount, len(m.listSkipped))
		m.list.SetItems(msg.items)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			if !m.list.SettingFilter() {
				return m, tea.Quit
			}
		case "enter":
			if m.listLoading {
				return m, nil
			}
			if item, ok := m.list.SelectedItem().(appItem); ok {
				return m, selectAppCmd(item.path)
			}
		case "p":
			if !m.list.SettingFilter() {
				m.screen = scrPath
				m.pathErr = ""
				m.pathInput.Focus()
				return m, textinput.Blink
			}
		}
	case appSelectedMsg:
		if msg.err != nil {
			m.pathErr = msg.err.Error()
			return m, nil
		}
		m.selected = msg.report
		m.screen = scrForm
		m.inputs = buildFormInputs(msg.report)
		m.formIdx = 0
		m.inputs[0].Focus()
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func selectAppCmd(path string) tea.Cmd {
	return func() tea.Msg {
		report, err := appinfo.Inspect(path)
		if err != nil {
			return appSelectedMsg{err: err}
		}
		return appSelectedMsg{report: report}
	}
}

func (m model) viewList() string {
	if m.listLoading {
		body := m.listSpin.View() + "  " + "Scanning /Applications…" + "\n\n" +
			tCaption.Render("reading every Info.plist; takes a second")
		return cardStyle.Width(m.cardWidth()).Render(body)
	}
	body := m.list.View()
	if len(m.listSkipped) > 0 {
		body += "\n" + tCaption.Render(fmt.Sprintf("%d app bundle(s) skipped; press p to enter a path manually.", len(m.listSkipped)))
		if first := m.listSkipped[0]; first.Path != "" {
			body += "\n" + tCaption.Render("First skipped: "+filepath.Base(first.Path)+" — "+first.Reason)
		}
	} else {
		body += "\n" + tCaption.Render("Press p to enter an app path manually.")
	}
	if m.pathErr != "" {
		body += "\n" + tErr.Render("✗ "+m.pathErr)
	}
	return cardStyle.Width(m.cardWidth()).Render(body)
}

func (m model) updatePath(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.screen = scrList
			m.pathInput.Blur()
			m.pathErr = ""
			return m, nil
		case "enter":
			path := strings.TrimSpace(m.pathInput.Value())
			if path == "" {
				m.pathErr = "Enter a .app path."
				return m, nil
			}
			return m, selectAppCmd(path)
		}
	case appSelectedMsg:
		if msg.err != nil {
			m.pathErr = msg.err.Error()
			return m, nil
		}
		m.selected = msg.report
		m.screen = scrForm
		m.inputs = buildFormInputs(msg.report)
		m.formIdx = 0
		m.pathInput.Blur()
		m.pathErr = ""
		m.inputs[0].Focus()
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.pathInput, cmd = m.pathInput.Update(msg)
	return m, cmd
}

func (m model) viewPath() string {
	var b strings.Builder
	b.WriteString(tTitle.Render("Enter app path"))
	b.WriteString("\n")
	b.WriteString(tCaption.Render("Use this when the app is outside the scanned folders or was skipped."))
	b.WriteString("\n\n")
	b.WriteString(tLabel.Render("APP PATH"))
	b.WriteString("\n")
	b.WriteString(m.pathInput.View())
	b.WriteString("\n")
	b.WriteString(tCaption.Render("Must point to a readable .app bundle with Contents/Info.plist."))
	if m.pathErr != "" {
		b.WriteString("\n\n")
		b.WriteString(tErr.Render("✗ " + m.pathErr))
	}
	return cardStyle.Width(m.cardWidth()).Render(b.String())
}

// ——— Screen: form ——————————————————————————————————————————————————————

const (
	fldName = iota
	fldBundleID
	fldDisplay
	fldTarget
	nFields
)

var formLabels = []string{"NAME", "BUNDLE ID", "DISPLAY NAME", "TARGET PATH"}

func buildFormInputs(report *appinfo.InspectionReport) []textinput.Model {
	inputs := make([]textinput.Model, nFields)
	mk := func(placeholder string, limit int) textinput.Model {
		t := textinput.New()
		t.Placeholder = placeholder
		t.CharLimit = limit
		t.Prompt = "› "
		t.PromptStyle = lipgloss.NewStyle().Foreground(cPrimary)
		t.PlaceholderStyle = lipgloss.NewStyle().Foreground(cDim).Italic(true)
		return t
	}
	inputs[fldName] = mk(defaultCloneName(report), 64)
	inputs[fldBundleID] = mk(defaultBundleID(report), 128)
	inputs[fldDisplay] = mk("(defaults to Name)", 64)
	inputs[fldTarget] = mk("(defaults to ~/Applications/<Name>.app)", 512)
	return inputs
}

func defaultCloneName(r *appinfo.InspectionReport) string {
	base := r.Identity.DisplayName
	if base == "" {
		base = r.Identity.BundleName
	}
	if base == "" {
		base = strings.TrimSuffix(filepath.Base(r.Identity.AppPath), ".app")
	}
	return base + "2"
}

func defaultBundleID(r *appinfo.InspectionReport) string {
	return clone.DefaultBundleID(r.Identity.BundleID, defaultCloneName(r))
}

func (m model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.screen = scrList
			return m, nil
		case "tab", "down":
			m.focusForm((m.formIdx + 1) % nFields)
			return m, textinput.Blink
		case "shift+tab", "up":
			m.focusForm((m.formIdx - 1 + nFields) % nFields)
			return m, textinput.Blink
		case "enter":
			if m.formIdx < nFields-1 {
				m.focusForm(m.formIdx + 1)
				return m, textinput.Blink
			}
			return m, m.trySubmit()
		case "ctrl+s":
			return m, m.trySubmit()
		case "ctrl+l":
			m.launchTest = !m.launchTest
			return m, nil
		}
	case planBuiltMsg:
		if msg.err != nil {
			m.formErr, m.formErrDetail = humanizeErr(msg.err)
			return m, nil
		}
		m.plan = msg.plan
		m.preflightFindings = msg.preflightFindings
		m.screen = scrProgress
		m.stages = map[string]*stageState{}
		m.activeStage = ""
		m.pipeCh = make(chan pipelineMsg, 64)
		return m, tea.Batch(
			m.progSpin.Tick,
			startPipeline(m.plan, m.pipeCh),
			waitForPipeline(m.pipeCh),
		)
	}
	var cmd tea.Cmd
	m.inputs[m.formIdx], cmd = m.inputs[m.formIdx].Update(msg)
	return m, cmd
}

func (m *model) focusForm(i int) {
	m.inputs[m.formIdx].Blur()
	m.formIdx = i
	m.inputs[m.formIdx].Focus()
}

func (m model) trySubmit() tea.Cmd {
	if err := m.validateForm(); err != nil {
		headline, detail := humanizeErr(err)
		return func() tea.Msg {
			return planBuiltMsg{err: errors.New(headline + "\n" + detail)}
		}
	}
	return m.submitForm()
}

func valueOrPlaceholder(t textinput.Model) string {
	if v := strings.TrimSpace(t.Value()); v != "" {
		return v
	}
	return t.Placeholder
}

func (m model) validateForm() error {
	name := valueOrPlaceholder(m.inputs[fldName])
	if name == "" {
		return fmt.Errorf("%w: Name is required", apperr.ErrInvalidInput)
	}
	bid := valueOrPlaceholder(m.inputs[fldBundleID])
	if bid == "" {
		return fmt.Errorf("%w: Bundle ID is required", apperr.ErrInvalidInput)
	}
	if err := appinfo.ValidateBundleID(bid); err != nil {
		return err
	}
	if bid == strings.TrimSpace(m.selected.Identity.BundleID) {
		return fmt.Errorf("%w: Bundle ID must differ from the source (%s)", apperr.ErrInvalidInput, bid)
	}
	return nil
}

func (m model) submitForm() tea.Cmd {
	opts := clone.PlanOptions{
		SourceApp:   m.selected.Identity.AppPath,
		Name:        valueOrPlaceholder(m.inputs[fldName]),
		BundleID:    valueOrPlaceholder(m.inputs[fldBundleID]),
		DisplayName: strings.TrimSpace(m.inputs[fldDisplay].Value()),
		TargetApp:   strings.TrimSpace(m.inputs[fldTarget].Value()),
		LaunchTest:  m.launchTest,
	}
	return func() tea.Msg {
		plan, err := clone.DerivePlan(opts)
		if err != nil {
			return planBuiltMsg{err: err}
		}
		ex := macos.NewExecer()
		findings, err := doctor.DiagnoseApp(context.Background(), ex, plan.SourceApp)
		if err != nil {
			return planBuiltMsg{err: err}
		}
		if blocking := doctor.BlockingFindings(findings); len(blocking) > 0 {
			return planBuiltMsg{
				preflightFindings: findings,
				err:               fmt.Errorf("%w: preflight blocked clone: %s", apperr.ErrInvalidInput, findingCodesForTUI(blocking)),
			}
		}
		return planBuiltMsg{plan: plan, preflightFindings: findings}
	}
}

func findingCodesForTUI(findings []doctor.Finding) string {
	codes := make([]string, 0, len(findings))
	for _, f := range findings {
		codes = append(codes, f.Code)
	}
	return strings.Join(codes, ", ")
}

func (m model) helpFor(i int) string {
	switch i {
	case fldName:
		return "A short name. Used to derive the target filename if Target is blank."
	case fldBundleID:
		return "Unique ID that separates the clone from the original. Format com.x.y."
	case fldDisplay:
		return "What shows under the icon in Finder/Dock. Optional."
	case fldTarget:
		return "Where to save the clone. Optional — defaults to ~/Applications/<Name>.app."
	}
	return ""
}

func (m model) previewFor(i int) string {
	switch i {
	case fldTarget:
		v := strings.TrimSpace(m.inputs[fldTarget].Value())
		if v == "" {
			name := valueOrPlaceholder(m.inputs[fldName])
			if name == "" {
				return ""
			}
			v = filepath.Join(clone.DefaultTargetDir, name+".app")
		}
		return "→ " + v
	case fldBundleID:
		v := strings.TrimSpace(m.inputs[fldBundleID].Value())
		if v != "" || m.inputs[fldBundleID].Placeholder == "" {
			return ""
		}
		return "→ " + m.inputs[fldBundleID].Placeholder
	case fldDisplay:
		v := strings.TrimSpace(m.inputs[fldDisplay].Value())
		name := valueOrPlaceholder(m.inputs[fldName])
		if v == "" {
			v = name
		}
		if v == "" {
			return ""
		}
		return "→ shows as “" + v + "”"
	}
	return ""
}

func (m model) viewForm() string {
	var b strings.Builder
	b.WriteString(tTitle.Render("Configure clone"))
	b.WriteString("\n")
	b.WriteString(tCaption.Render(
		"Makes an independent copy of the app with a new identity so both can run side-by-side.",
	))
	b.WriteString("\n\n")
	b.WriteString(tLabel.Render("SOURCE"))
	b.WriteString("\n")
	b.WriteString(tCaption.Render(fmt.Sprintf(
		"%s  ·  %s",
		filepath.Base(m.selected.Identity.AppPath),
		m.selected.Identity.BundleID,
	)))
	b.WriteString("\n\n")

	for i := range m.inputs {
		labelText := formLabels[i]
		var labelStyle lipgloss.Style
		if i == m.formIdx {
			labelStyle = lipgloss.NewStyle().Foreground(cPrimary).Bold(true)
		} else {
			labelStyle = tLabel
		}
		b.WriteString(labelStyle.Render(labelText))
		b.WriteString("\n")
		b.WriteString(m.inputs[i].View())
		b.WriteString("\n")
		b.WriteString(tCaption.Render(m.helpFor(i)))
		b.WriteString("\n")
		if p := m.previewFor(i); p != "" {
			b.WriteString(tOK.Render(p))
			b.WriteString("\n")
		}
		if i < len(m.inputs)-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(tLabel.Render("LAUNCH TEST"))
	b.WriteString("\n")
	if m.launchTest {
		b.WriteString(tOK.Render("ON"))
		b.WriteString(tCaption.Render("  opens the clone briefly after signing and fails if it exits early"))
	} else {
		b.WriteString(tWarn.Render("OFF"))
		b.WriteString(tCaption.Render("  skips the startup check; faster but less reliable"))
	}
	b.WriteString("\n")
	b.WriteString(tCaption.Render("Press Ctrl+L to toggle."))
	b.WriteString("\n")

	if m.formErr != "" {
		b.WriteString("\n")
		b.WriteString(tErr.Render("✗ " + m.formErr))
		b.WriteString("\n")
		if m.formErrDetail != "" {
			detail := strings.SplitN(m.formErrDetail, "\n", 2)[0]
			b.WriteString(tCaption.Render("  " + detail))
			b.WriteString("\n")
		}
	}

	return cardStyle.Width(m.cardWidth()).Render(b.String())
}

// ——— Screen: progress ——————————————————————————————————————————————————

func startPipeline(plan *clone.ClonePlan, out chan<- pipelineMsg) tea.Cmd {
	return func() tea.Msg {
		events := make(chan clone.StageEvent, 32)
		done := make(chan struct{})
		go func() {
			for ev := range events {
				e := ev
				out <- pipelineMsg{event: &e}
			}
			close(done)
		}()
		ex := macos.NewExecer()
		result, err := clone.Run(context.Background(), plan, ex, events)
		close(events)
		<-done
		out <- pipelineMsg{done: true, result: result, err: err}
		return nil
	}
}

func waitForPipeline(ch chan pipelineMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (m model) updateProgress(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.activeStage != "" {
			var cmd tea.Cmd
			m.progSpin, cmd = m.progSpin.Update(msg)
			return m, cmd
		}
	case pipelineMsg:
		if msg.done {
			m.activeStage = ""
			m.result = msg.result
			m.runErr = msg.err
			m.screen = scrResult
			return m, nil
		}
		if msg.event != nil {
			m.recordStage(msg.event)
		}
		return m, waitForPipeline(m.pipeCh)
	}
	return m, nil
}

func (m *model) recordStage(ev *clone.StageEvent) {
	st, ok := m.stages[ev.Stage]
	if !ok {
		st = &stageState{}
		m.stages[ev.Stage] = st
	}
	st.status = ev.Status
	st.msg = ev.Message
	switch ev.Status {
	case clone.StageStart:
		st.start = time.Now()
		m.activeStage = ev.Stage
	default:
		st.end = time.Now()
		if m.activeStage == ev.Stage {
			m.activeStage = ""
		}
	}
}

func (m model) viewProgress() string {
	var b strings.Builder
	b.WriteString(tTitle.Render("Cloning"))
	b.WriteString("\n")
	b.WriteString(tCaption.Render(fmt.Sprintf(
		"%s  →  %s",
		filepath.Base(m.plan.SourceApp),
		filepath.Base(m.plan.TargetApp),
	)))
	b.WriteString("\n")
	b.WriteString(tCaption.Render(fmt.Sprintf(
		"%s  →  %s",
		m.plan.BundleIDBefore, m.plan.BundleIDAfter,
	)))
	b.WriteString("\n\n")

	// Overall progress bar
	done := 0
	for _, s := range stageOrder {
		if st, ok := m.stages[s]; ok && st.status != clone.StageStart {
			done++
		}
	}
	inner := m.innerWidth()
	barWidth := inner - 10
	if barWidth < 20 {
		barWidth = 20
	}
	b.WriteString(renderProgressBar(done, len(stageOrder), barWidth))
	b.WriteString(fmt.Sprintf("  %s", tCaption.Render(fmt.Sprintf("%d / %d", done, len(stageOrder)))))
	b.WriteString("\n\n")

	// Stage rows
	for _, name := range stageOrder {
		b.WriteString(m.viewStageRow(name, inner))
		b.WriteString("\n")
	}
	return cardStyle.Width(m.cardWidth()).Render(b.String())
}

func renderProgressBar(done, total, width int) string {
	if total <= 0 {
		total = 1
	}
	filled := width * done / total
	if filled > width {
		filled = width
	}
	fillStyle := lipgloss.NewStyle().Foreground(cPrimary)
	emptyStyle := lipgloss.NewStyle().Foreground(cMuted)
	return fillStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", width-filled))
}

func (m model) viewStageRow(name string, availWidth int) string {
	st := m.stages[name]
	var symbol string
	switch {
	case st == nil:
		symbol = lipgloss.NewStyle().Foreground(cMuted).Render("○")
	case st.status == clone.StageStart:
		symbol = m.progSpin.View()
	case st.status == clone.StageOK:
		symbol = tOKBold.Render("✓")
	case st.status == clone.StageWarn:
		symbol = tWarn.Render("⚠")
	case st.status == clone.StageError:
		symbol = tErr.Render("✗")
	case st.status == clone.StageSkip:
		symbol = tFaint.Render("—")
	}

	label := stageLabel[name]
	if label == "" {
		label = name
	}
	var labelStyle lipgloss.Style
	switch {
	case st == nil:
		labelStyle = lipgloss.NewStyle().Foreground(cDim)
	case st.status == clone.StageStart:
		labelStyle = lipgloss.NewStyle().Foreground(cPrimary).Bold(true)
	case st.status == clone.StageError:
		labelStyle = tErr
	case st.status == clone.StageWarn:
		labelStyle = tWarn
	default:
		labelStyle = lipgloss.NewStyle().Foreground(cDim)
	}
	labelText := labelStyle.Render(label)

	// Right side: duration or status detail
	var right string
	if st != nil {
		switch st.status {
		case clone.StageStart:
			right = tCaption.Render(formatDuration(time.Since(st.start)))
		case clone.StageOK, clone.StageWarn, clone.StageError:
			right = tCaption.Render(formatDuration(st.end.Sub(st.start)))
		}
	}

	// Dotted leader between label and right text.
	// " icon  label ········· right"
	left := " " + symbol + "  " + labelText
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	dotCount := availWidth - leftW - rightW - 2
	if dotCount < 2 {
		dotCount = 2
	}
	leader := lipgloss.NewStyle().Foreground(cMuted).Render(" " + strings.Repeat("·", dotCount) + " ")

	return left + leader + right
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

// ——— Screen: result ——————————————————————————————————————————————————

func (m model) updateResult(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "q", "esc":
			return m, tea.Quit
		case "n":
			m.screen = scrList
			m.selected = nil
			m.plan = nil
			m.result = nil
			m.preflightFindings = nil
			m.runErr = nil
			m.stages = map[string]*stageState{}
			m.activeStage = ""
			m.formErr = ""
			m.formErrDetail = ""
			return m, nil
		case "o":
			if m.plan != nil && m.runErr == nil && !m.plan.DryRun {
				_ = exec.Command("open", m.plan.TargetApp).Start()
				m = m.setToast("launched clone")
				return m, clearToastCmd(m.toastAt)
			}
		case "f":
			if m.plan != nil && !m.plan.DryRun {
				_ = exec.Command("open", "-R", m.plan.TargetApp).Start()
				m = m.setToast("revealed in Finder")
				return m, clearToastCmd(m.toastAt)
			}
		case "c":
			if data, err := m.resultJSON(); err == nil {
				if err := clipboard.WriteAll(data); err == nil {
					m = m.setToast("copied JSON to clipboard")
					return m, clearToastCmd(m.toastAt)
				}
			}
		}
	}
	return m, nil
}

func (m model) setToast(text string) model {
	m.toast = text
	m.toastAt = time.Now()
	return m
}

func clearToastCmd(at time.Time) tea.Cmd {
	return tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
		return toastClearMsg(at)
	})
}

func (m model) resultJSON() (string, error) {
	payload := map[string]any{
		"source_app":       m.plan.SourceApp,
		"target_app":       m.plan.TargetApp,
		"bundle_id_before": m.plan.BundleIDBefore,
		"bundle_id_after":  m.plan.BundleIDAfter,
		"success":          m.runErr == nil,
	}
	if m.runErr != nil {
		payload["error"] = m.runErr.Error()
	}
	if len(m.preflightFindings) > 0 {
		payload["preflight_findings"] = m.preflightFindings
	}
	if m.result != nil && m.result.Verify != nil {
		payload["verify"] = m.result.Verify
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	return string(b), err
}

func (m model) viewResult() string {
	var banner string
	width := m.cardWidth() - 2 // account for card padding

	switch {
	case m.runErr != nil:
		banner = centerBanner(bannerErr, "FAILED", width)
	case m.plan.DryRun:
		banner = centerBanner(bannerWarn, "DRY-RUN OK", width)
	default:
		banner = centerBanner(bannerOK, "SUCCESS", width)
	}

	var b strings.Builder
	b.WriteString(banner)
	b.WriteString("\n\n")

	if m.runErr != nil {
		headline, detail := humanizeErr(m.runErr)
		b.WriteString(tErr.Render("✗ ") + headline)
		b.WriteString("\n\n")
		if detail != "" {
			b.WriteString(tCaption.Render(detail))
			b.WriteString("\n\n")
		}
	} else if m.plan.DryRun {
		b.WriteString(tWarn.Render("Would clone to: ") + m.plan.TargetApp)
		b.WriteString("\n\n")
	} else {
		b.WriteString(tOKBold.Render("→ ") + m.plan.TargetApp)
		b.WriteString("\n\n")
		b.WriteString(m.nextStepsBlock())
		b.WriteString("\n\n")
	}

	if len(m.preflightFindings) > 0 {
		b.WriteString(tLabel.Render("PREFLIGHT"))
		b.WriteString("\n")
		for _, f := range m.preflightFindings {
			prefix := tCaption.Render("•")
			if f.Severity == "warn" {
				prefix = tWarn.Render("⚠")
			}
			if f.Severity == "error" {
				prefix = tErr.Render("✗")
			}
			b.WriteString(fmt.Sprintf("%s  %s — %s\n", prefix, f.Code, f.Title))
		}
		b.WriteString("\n")
	}

	// Verify summary
	if m.result != nil && m.result.Verify != nil {
		b.WriteString(tLabel.Render("VERIFICATION"))
		b.WriteString("\n")
		if m.result.Verify.Codesign != nil {
			if m.result.Verify.Codesign.OK {
				b.WriteString(tOK.Render("✓") + "  Signature valid\n")
			} else {
				b.WriteString(tErr.Render("✗") + "  Signature invalid\n")
			}
		}
		if m.result.Verify.SPCTL != nil {
			if m.result.Verify.SPCTL.Accepted {
				b.WriteString(tOK.Render("✓") + "  Gatekeeper accepts it\n")
			} else {
				b.WriteString(tWarn.Render("⚠") + "  Gatekeeper will prompt on first launch\n")
				b.WriteString(tCaption.Render("   Right-click the app → Open the first time. This is expected for locally-signed clones.\n"))
			}
		}
		if lt := m.result.Verify.LaunchTest; lt != nil {
			if lt.Survived {
				b.WriteString(tOK.Render("✓") + fmt.Sprintf("  Launch test survived %.1fs\n", float64(lt.SurvivedMs)/1000))
			} else if lt.Launched {
				b.WriteString(tErr.Render("✗") + "  Launch test exited early\n")
				if lt.CrashSummary != "" {
					b.WriteString(tCaption.Render("   " + lt.CrashSummary + "\n"))
				} else if lt.Note != "" {
					b.WriteString(tCaption.Render("   " + lt.Note + "\n"))
				}
			} else {
				b.WriteString(tWarn.Render("⚠") + "  Launch test could not start the app\n")
				if lt.Note != "" {
					b.WriteString(tCaption.Render("   " + lt.Note + "\n"))
				}
			}
		}
	}

	return cardStyle.Width(m.cardWidth()).Render(b.String())
}

func centerBanner(style lipgloss.Style, text string, width int) string {
	return style.Width(width).Align(lipgloss.Center).Render(text)
}

func (m model) nextStepsBlock() string {
	lines := []string{
		tLabel.Render("WHAT NOW"),
		fmt.Sprintf("  %s  launch the clone", tKey.Render("o")),
		fmt.Sprintf("  %s  reveal in Finder", tKey.Render("f")),
		fmt.Sprintf("  %s  copy JSON result", tKey.Render("c")),
		fmt.Sprintf("  %s  clone another app", tKey.Render("n")),
		"",
		tCaption.Render("First launch: macOS will warn about an unidentified developer."),
		tCaption.Render("Right-click the app in Finder → Open to approve once."),
	}
	return strings.Join(lines, "\n")
}

// ——— Step bar (header) ————————————————————————————————————————————————

func (m model) viewStepBar() string {
	current := m.stepIndex()
	parts := make([]string, 0, len(stepLabels)*2)
	for i, label := range stepLabels {
		var s string
		switch {
		case i < current:
			s = tOK.Render("✓ " + label)
		case i == current:
			s = tKey.Render("● " + label)
		default:
			s = lipgloss.NewStyle().Foreground(cMuted).Render("○ " + label)
		}
		if i > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(cMuted).Render("  ›  "))
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "")
}

func (m model) stepIndex() int {
	switch m.screen {
	case scrList, scrPath:
		return 0
	case scrForm:
		return 1
	case scrProgress:
		return 2
	case scrResult:
		return 3
	default:
		return 0
	}
}

// ——— Footer ————————————————————————————————————————————————————————

func (m model) viewFooter() string {
	hints := m.hintsFor(m.screen)
	line := tFaint.Render(hints)
	if m.toast != "" {
		line = tToast.Render("● "+m.toast) + "    " + line
	}
	return footerStyle.Width(m.width - 2).Render(line)
}

func (m model) hintsFor(s screen) string {
	switch s {
	case scrList:
		if m.listLoading {
			return "loading …   ctrl+c quit"
		}
		return "enter pick   p path   / filter   ↑↓ move   ? help   q quit"
	case scrPath:
		return "enter load path   esc back   ctrl+c quit"
	case scrForm:
		return "tab next   shift+tab prev   ctrl+l launch test   ctrl+s start   esc back"
	case scrProgress:
		return "working …   ctrl+c abort"
	case scrResult:
		if m.runErr != nil || m.plan.DryRun {
			return "c copy JSON   n new clone   ? help   q quit"
		}
		return "o open   f finder   c copy JSON   n new clone   ? help   q quit"
	}
	return ""
}

// ——— Help overlay ———————————————————————————————————————————————————

func (m model) viewHelp() string {
	var b strings.Builder
	b.WriteString(tTitle.Render("Help"))
	b.WriteString("\n\n")

	b.WriteString(tLabel.Render("WHAT THIS DOES"))
	b.WriteString("\n")
	b.WriteString(tCaption.Render(
		"Makes a second, independently-launchable copy of a macOS app.\n" +
			"The clone gets a new identity (bundle ID + name) so macOS treats\n" +
			"it as a different app. Useful for running two accounts, keeping\n" +
			"two preference sets, or testing with isolated state.",
	))
	b.WriteString("\n\n")

	b.WriteString(tLabel.Render("HOW IT WORKS"))
	b.WriteString("\n")
	for _, id := range stageOrder {
		b.WriteString(fmt.Sprintf("  %s  %s\n",
			tKey.Render(fmt.Sprintf("%d", indexOf(stageOrder, id)+1)),
			stageLabel[id],
		))
	}
	b.WriteString("\n")

	b.WriteString(tLabel.Render("CAVEATS"))
	b.WriteString("\n")
	b.WriteString(tCaption.Render(
		"•  Clones are locally signed. Gatekeeper will prompt on first launch.\n" +
			"•  Sandboxed apps start with empty containers (no synced data).\n" +
			"•  Launch test briefly opens the clone after signing.\n" +
			"•  Auto-updaters (Sparkle, etc.) usually don't work on clones.",
	))
	b.WriteString("\n\n")

	b.WriteString(tCaption.Render("press any key to close"))
	return cardStyle.Width(m.cardWidth()).Render(b.String())
}

func indexOf(ss []string, target string) int {
	for i, s := range ss {
		if s == target {
			return i
		}
	}
	return -1
}

// ——— humanizeErr ———————————————————————————————————————————————————

func humanizeErr(err error) (headline, detail string) {
	if err == nil {
		return "", ""
	}
	raw := err.Error()
	switch {
	case errors.Is(err, apperr.ErrTargetExists):
		return "A file already exists at the target path.",
			"Choose a different Name or Target, or remove the existing copy first.\n\n" + raw
	case errors.Is(err, apperr.ErrAppMissing):
		return "That app can't be found on disk.",
			"It may have been moved or deleted. Go back to the list and re-select.\n\n" + raw
	case errors.Is(err, apperr.ErrNotAnApp):
		return "The path isn't a valid .app bundle.", raw
	case errors.Is(err, apperr.ErrInvalidInput):
		return "Some input isn't valid.", raw
	case isLaunchTestFailure(err):
		return "The clone was created but failed the startup test.",
			"The app launched and then exited early. This usually means the app has its own integrity check or startup guard. The copy is still on disk, but it is not reliable for normal use.\n\n" + raw
	case strings.Contains(raw, "Permission denied"), strings.Contains(raw, "permission denied"):
		return "Permission denied while writing the clone.",
			"macOS restricts writing to /Applications for some folders. Try a Target under ~/Applications/ instead.\n\n" + raw
	case strings.Contains(raw, "resource fork, Finder information"):
		return "Source bundle has metadata that strict signing doesn't allow.",
			"The original app ships with extra macOS xattrs. Try 'xattr -rc' on the source, or pick a different app.\n\n" + raw
	case strings.Contains(raw, "codesign failed"):
		return "Re-signing the clone failed.",
			"codesign rejected one of the nested items. Try 'doppel doctor' on the source for hints.\n\n" + raw
	case strings.Contains(raw, "ditto failed"):
		return "Copying the bundle failed.",
			"Usually a permissions issue or the source disappeared mid-copy. Try a different Target.\n\n" + raw
	case strings.Contains(raw, "verify failed"):
		return "Clone was created but the signature doesn't verify.",
			"The copy exists on disk but macOS may refuse to launch it. Try 'doppel doctor' for details.\n\n" + raw
	default:
		return "Something went wrong.", raw
	}
}

func isLaunchTestFailure(err error) bool {
	var stageErr clone.StageFailure
	if errors.As(err, &stageErr) && stageErr.Stage == clone.StageLaunchTest {
		return true
	}
	return strings.Contains(err.Error(), "launch test:")
}

// ——— Root View ————————————————————————————————————————————————————————

func (m model) View() string {
	var body string
	if m.showHelp {
		body = m.viewHelp()
	} else {
		switch m.screen {
		case scrList:
			body = m.viewList()
		case scrPath:
			body = m.viewPath()
		case scrForm:
			body = m.viewForm()
		case scrProgress:
			body = m.viewProgress()
		case scrResult:
			body = m.viewResult()
		}
	}
	header := m.viewStepBar()
	footer := m.viewFooter()
	return lipgloss.JoinVertical(lipgloss.Left,
		" "+header,
		"",
		body,
		footer,
	)
}
