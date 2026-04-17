package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tt-a1i/appclone/internal/core/appinfo"
	"github.com/tt-a1i/appclone/internal/core/clone"
	"github.com/tt-a1i/appclone/internal/core/macos"
)

// ——— Screens ————————————————————————————————————————————————————————

type screen int

const (
	scrList screen = iota
	scrForm
	scrProgress
	scrResult
)

var stepLabels = []string{"Pick", "Configure", "Clone", "Done"}

// Stage order drives the fixed progress panel.
var stageOrder = []string{"copy", "plist", "entitlements", "discover", "resign", "verify"}

// Friendly labels for the stage names shown in progress + result views.
// Internal stage IDs (stageOrder) remain the same for JSON/CLI compatibility.
var stageLabel = map[string]string{
	"copy":         "Copy files",
	"plist":        "Update identity",
	"entitlements": "Adjust permissions",
	"discover":     "Scan components",
	"resign":       "Re-sign code",
	"verify":       "Verify signature",
}

// ——— Messages ———————————————————————————————————————————————————————

type appsLoadedMsg struct{ items []list.Item }

type appSelectedMsg struct{ report *appinfo.InspectionReport }

type planBuiltMsg struct {
	plan *clone.ClonePlan
	err  error
}

type pipelineMsg struct {
	event  *clone.StageEvent
	done   bool
	result *clone.RunResult
	err    error
}

type toastMsg struct {
	text string
	at   time.Time
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

	// form
	selected *appinfo.InspectionReport
	inputs   []textinput.Model
	formIdx  int
	formErr  string

	// progress
	plan        *clone.ClonePlan
	stages      map[string]*stageState
	activeStage string
	progSpin    spinner.Model
	pipeCh      chan pipelineMsg

	// result
	result *clone.RunResult
	runErr error

	// ephemeral toast (status line)
	toast   string
	toastAt time.Time
}

// ——— Styles ———————————————————————————————————————————————————————————

var (
	colorPrimary = lipgloss.Color("212") // rose
	colorAccent  = lipgloss.Color("147") // lilac
	colorOK      = lipgloss.Color("42")  // green
	colorWarn    = lipgloss.Color("214") // amber
	colorErr     = lipgloss.Color("196") // red
	colorMuted   = lipgloss.Color("240")
	colorDim     = lipgloss.Color("245")

	titleStyle      = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	subtitleStyle   = lipgloss.NewStyle().Foreground(colorDim)
	okStyle         = lipgloss.NewStyle().Foreground(colorOK).Bold(true)
	warnStyle       = lipgloss.NewStyle().Foreground(colorWarn)
	errStyle        = lipgloss.NewStyle().Foreground(colorErr).Bold(true)
	faintStyle      = lipgloss.NewStyle().Faint(true)
	labelStyle      = lipgloss.NewStyle().Width(13).Foreground(colorDim)
	labelFocusStyle = lipgloss.NewStyle().Width(13).Foreground(colorPrimary).Bold(true)
	previewStyle    = lipgloss.NewStyle().Foreground(colorMuted).Italic(true).PaddingLeft(13)

	stepActiveStyle = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	stepDoneStyle   = lipgloss.NewStyle().Foreground(colorOK)
	stepPendStyle   = lipgloss.NewStyle().Foreground(colorMuted)
	stepSepStyle    = lipgloss.NewStyle().Foreground(colorMuted)

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMuted).
			Padding(1, 2)

	footerStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorMuted).
			Padding(0, 1)

	toastStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)
)

var bundleIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.\-]*[A-Za-z0-9]$`)

// ——— Entry ———————————————————————————————————————————————————————————

func Run() error {
	_, err := tea.NewProgram(initialModel(), tea.WithAltScreen()).Run()
	return err
}

func initialModel() model {
	loadSpin := spinner.New()
	loadSpin.Spinner = spinner.Dot
	loadSpin.Style = lipgloss.NewStyle().Foreground(colorPrimary)

	progSpin := spinner.New()
	progSpin.Spinner = spinner.MiniDot
	progSpin.Style = lipgloss.NewStyle().Foreground(colorPrimary)

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(colorPrimary).
		BorderLeftForeground(colorPrimary)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(colorAccent).
		BorderLeftForeground(colorPrimary)

	l := list.New(nil, delegate, 0, 0)
	l.Title = "Pick an app to clone"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()
	l.Styles.Title = l.Styles.Title.Background(colorPrimary)

	return model{
		screen:      scrList,
		listLoading: true,
		listSpin:    loadSpin,
		progSpin:    progSpin,
		list:        l,
		stages:      map[string]*stageState{},
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.listSpin.Tick, scanAppsCmd())
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

func (i appItem) Title() string {
	// NOTE: must stay plain-text — bubbles/list's fuzzy filter inserts
	// highlight styles per-rune and mangles any ANSI already in the string.
	return i.name
}

func (i appItem) Description() string {
	// Description is NOT filter-highlighted, so lipgloss ANSI is safe here.
	ver := i.version
	if ver == "" {
		ver = "—"
	}
	var badge string
	if i.signed {
		badge = lipgloss.NewStyle().Foreground(colorOK).Render("✓ signed")
	} else {
		badge = lipgloss.NewStyle().Foreground(colorWarn).Render("! unsigned")
	}
	return fmt.Sprintf("%s   %s  ·  v%s", badge, i.bundleID, ver)
}

func scanAppsCmd() tea.Cmd {
	return func() tea.Msg {
		home, _ := os.UserHomeDir()
		dirs := []string{"/Applications", "/Applications/Utilities"}
		if home != "" {
			dirs = append(dirs, filepath.Join(home, "Applications"))
		}
		var items []list.Item
		seen := map[string]bool{}
		for _, d := range dirs {
			entries, err := os.ReadDir(d)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !strings.HasSuffix(e.Name(), ".app") {
					continue
				}
				app := filepath.Join(d, e.Name())
				if seen[app] {
					continue
				}
				seen[app] = true
				report, err := appinfo.Inspect(app)
				if err != nil {
					continue
				}
				name := report.Identity.DisplayName
				if name == "" {
					name = report.Identity.BundleName
				}
				if name == "" {
					name = strings.TrimSuffix(e.Name(), ".app")
				}
				items = append(items, appItem{
					path:     app,
					name:     name,
					bundleID: report.Identity.BundleID,
					version:  report.Identity.Version,
					signed:   report.HasSignature,
				})
			}
		}
		return appsLoadedMsg{items: items}
	}
}

// ——— Root Update ———————————————————————————————————————————————————————

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		innerW := msg.Width - 4
		innerH := msg.Height - 5 // header + footer + margins
		if innerW < 40 {
			innerW = 40
		}
		if innerH < 10 {
			innerH = 10
		}
		m.list.SetSize(innerW, innerH)
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
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
		m.list.Title = fmt.Sprintf("Pick an app to clone  ·  %d apps", m.appCount)
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
		}
	case appSelectedMsg:
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
			return nil
		}
		return appSelectedMsg{report: report}
	}
}

func (m model) viewList() string {
	if m.listLoading {
		return fmt.Sprintf("%s  Scanning /Applications…\n\n%s",
			m.listSpin.View(),
			faintStyle.Render("reading every Info.plist; takes a second"),
		)
	}
	return m.list.View()
}

// ——— Screen: form ——————————————————————————————————————————————————————

const (
	fldName = iota
	fldBundleID
	fldDisplay
	fldTarget
	nFields
)

var formLabels = []string{"Name", "Bundle ID", "Display", "Target"}

func buildFormInputs(report *appinfo.InspectionReport) []textinput.Model {
	inputs := make([]textinput.Model, nFields)
	mk := func(placeholder string, limit int) textinput.Model {
		t := textinput.New()
		t.Placeholder = placeholder
		t.CharLimit = limit
		t.Prompt = ""
		t.PromptStyle = lipgloss.NewStyle().Foreground(colorPrimary)
		return t
	}
	inputs[fldName] = mk(defaultCloneName(report), 64)
	inputs[fldBundleID] = mk(defaultBundleID(report), 128)
	inputs[fldDisplay] = mk("(defaults to Name)", 64)
	inputs[fldTarget] = mk("(defaults to /Applications/<Name>.app)", 512)
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
	if r.Identity.BundleID == "" {
		return "com.example.clone"
	}
	return r.Identity.BundleID + "2"
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
			// Only submit from the last field; elsewhere enter advances.
			if m.formIdx < nFields-1 {
				m.focusForm(m.formIdx + 1)
				return m, textinput.Blink
			}
			if err := m.validateForm(); err != nil {
				m.formErr = err.Error()
				return m, nil
			}
			m.formErr = ""
			return m, m.submitForm()
		case "ctrl+s":
			// Submit from any field.
			if err := m.validateForm(); err != nil {
				m.formErr = err.Error()
				return m, nil
			}
			m.formErr = ""
			return m, m.submitForm()
		}
	case planBuiltMsg:
		if msg.err != nil {
			m.formErr = msg.err.Error()
			return m, nil
		}
		m.plan = msg.plan
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

func valueOrPlaceholder(t textinput.Model) string {
	if v := strings.TrimSpace(t.Value()); v != "" {
		return v
	}
	return t.Placeholder
}

func (m model) validateForm() error {
	name := valueOrPlaceholder(m.inputs[fldName])
	if name == "" {
		return fmt.Errorf("Name is required")
	}
	bid := valueOrPlaceholder(m.inputs[fldBundleID])
	if bid == "" {
		return fmt.Errorf("Bundle ID is required")
	}
	if !bundleIDPattern.MatchString(bid) {
		return fmt.Errorf("Bundle ID must look like com.example.app (alphanumeric + dots/dashes)")
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
	}
	return func() tea.Msg {
		plan, err := clone.DerivePlan(opts)
		return planBuiltMsg{plan: plan, err: err}
	}
}

// helpFor returns plain-English guidance for each field — always shown.
func (m model) helpFor(i int) string {
	switch i {
	case fldName:
		return "A short name for the clone. Also used to derive the target filename."
	case fldBundleID:
		return "Unique ID that tells macOS the clone is a different app from the original. Keep the com.x.y format."
	case fldDisplay:
		return "The name shown under the icon in Finder, Dock, and Launchpad. Optional — falls back to Name."
	case fldTarget:
		return "Where on disk to save the clone. Optional — defaults to /Applications/<Name>.app."
	}
	return ""
}

// previewFor returns a concrete "this is what will happen" line — only shown
// when the field has a meaningful resolved value.
func (m model) previewFor(i int) string {
	switch i {
	case fldBundleID:
		v := valueOrPlaceholder(m.inputs[fldBundleID])
		if v == "" {
			return ""
		}
		return "→ " + v
	case fldDisplay:
		v := strings.TrimSpace(m.inputs[fldDisplay].Value())
		if v == "" {
			v = valueOrPlaceholder(m.inputs[fldName])
		}
		if v == "" {
			return ""
		}
		return "→ shows as “" + v + "”"
	case fldTarget:
		v := strings.TrimSpace(m.inputs[fldTarget].Value())
		if v == "" {
			name := valueOrPlaceholder(m.inputs[fldName])
			if name == "" {
				return ""
			}
			v = filepath.Join("/Applications", name+".app")
		}
		return "→ " + v
	}
	return ""
}

func (m model) viewForm() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Configure clone"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render(
		"Makes a second copy of the app with a new identity so both can run side-by-side.",
	))
	b.WriteString("\n")
	b.WriteString(faintStyle.Render(fmt.Sprintf(
		"Source: %s  (%s)",
		filepath.Base(m.selected.Identity.AppPath),
		m.selected.Identity.BundleID,
	)))
	b.WriteString("\n\n")

	for i := range m.inputs {
		label := labelStyle.Render(formLabels[i])
		if i == m.formIdx {
			label = labelFocusStyle.Render(formLabels[i])
		}
		b.WriteString(label)
		b.WriteString(m.inputs[i].View())
		b.WriteString("\n")
		// Always-visible plain-English help
		b.WriteString(previewStyle.Render(m.helpFor(i)))
		b.WriteString("\n")
		// Concrete preview only if we have one (saves a line per field)
		if p := m.previewFor(i); p != "" {
			b.WriteString(previewStyle.Render(p))
			b.WriteString("\n")
		}
		if i < len(m.inputs)-1 {
			b.WriteString("\n")
		}
	}

	if m.formErr != "" {
		b.WriteString("\n")
		b.WriteString(errStyle.Render("✗ " + m.formErr))
		b.WriteString("\n")
	}

	return cardStyle.Render(b.String())
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
	b.WriteString(titleStyle.Render("Cloning"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render(fmt.Sprintf("%s → %s", m.plan.SourceApp, m.plan.TargetApp)))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render(fmt.Sprintf("%s → %s", m.plan.BundleIDBefore, m.plan.BundleIDAfter)))
	b.WriteString("\n\n")

	// Overall progress bar
	done := 0
	for _, s := range stageOrder {
		if st, ok := m.stages[s]; ok && st.status != clone.StageStart {
			done++
		}
	}
	b.WriteString(renderBar(done, len(stageOrder)))
	b.WriteString(" ")
	b.WriteString(faintStyle.Render(fmt.Sprintf("%d / %d", done, len(stageOrder))))
	b.WriteString("\n\n")

	for _, name := range stageOrder {
		b.WriteString(m.viewStageRow(name))
		b.WriteString("\n")
	}
	return cardStyle.Render(b.String())
}

func (m model) viewStageRow(name string) string {
	st := m.stages[name]
	var symbol, durStr, msg string
	switch {
	case st == nil:
		symbol = stepPendStyle.Render("○")
		msg = faintStyle.Render("waiting")
	case st.status == clone.StageStart:
		symbol = m.progSpin.View()
		msg = st.msg
		durStr = faintStyle.Render(formatDuration(time.Since(st.start)))
	case st.status == clone.StageOK:
		symbol = okStyle.Render("✓")
		msg = st.msg
		durStr = faintStyle.Render(formatDuration(st.end.Sub(st.start)))
	case st.status == clone.StageWarn:
		symbol = warnStyle.Render("⚠")
		msg = warnStyle.Render(st.msg)
		durStr = faintStyle.Render(formatDuration(st.end.Sub(st.start)))
	case st.status == clone.StageError:
		symbol = errStyle.Render("✗")
		msg = errStyle.Render(st.msg)
		durStr = faintStyle.Render(formatDuration(st.end.Sub(st.start)))
	case st.status == clone.StageSkip:
		symbol = faintStyle.Render("—")
		msg = faintStyle.Render(st.msg)
	}
	label := stageLabel[name]
	if label == "" {
		label = name
	}
	label = lipgloss.NewStyle().Width(20).Foreground(colorDim).Render(label)
	return fmt.Sprintf(" %s  %s  %s  %s", symbol, label, msg, durStr)
}

func renderBar(done, total int) string {
	width := 24
	filled := width * done / total
	fillStyle := lipgloss.NewStyle().Foreground(colorPrimary)
	emptyStyle := lipgloss.NewStyle().Foreground(colorMuted)
	return fillStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", width-filled))
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
			// reset for another run
			m.screen = scrList
			m.selected = nil
			m.plan = nil
			m.result = nil
			m.runErr = nil
			m.stages = map[string]*stageState{}
			m.activeStage = ""
			m.formErr = ""
			return m, nil
		case "o":
			if m.plan != nil && m.runErr == nil && !m.plan.DryRun {
				_ = exec.Command("open", m.plan.TargetApp).Start()
				return m.setToast("launched clone"), clearToastCmd()
			}
		case "f":
			if m.plan != nil && !m.plan.DryRun {
				_ = exec.Command("open", "-R", m.plan.TargetApp).Start()
				return m.setToast("revealed in Finder"), clearToastCmd()
			}
		case "c":
			if data, err := m.resultJSON(); err == nil {
				if err := clipboard.WriteAll(data); err == nil {
					return m.setToast("copied JSON to clipboard"), clearToastCmd()
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

func clearToastCmd() tea.Cmd {
	at := time.Now()
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
	if m.result != nil && m.result.Verify != nil {
		payload["verify"] = m.result.Verify
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	return string(b), err
}

func (m model) viewResult() string {
	var b strings.Builder

	banner := okStyle.Render("  SUCCESS  ")
	line := okStyle.Render("Clone at: " + m.plan.TargetApp)
	if m.runErr != nil {
		banner = errStyle.Render("  FAILED  ")
		line = errStyle.Render(m.runErr.Error())
	} else if m.plan.DryRun {
		banner = warnStyle.Render("  DRY-RUN OK  ")
		line = warnStyle.Render("Would clone to: " + m.plan.TargetApp)
	}

	b.WriteString(banner)
	b.WriteString("\n\n")
	b.WriteString(line)
	b.WriteString("\n\n")

	// compact stage summary
	b.WriteString(faintStyle.Render("Stages:"))
	b.WriteString("\n")
	for _, name := range stageOrder {
		b.WriteString(m.viewStageRow(name))
		b.WriteString("\n")
	}

	if m.result != nil && m.result.Verify != nil {
		b.WriteString("\n")
		if m.result.Verify.Codesign != nil {
			if m.result.Verify.Codesign.OK {
				b.WriteString(okStyle.Render("✓") + "  Signature check passed\n")
			} else {
				b.WriteString(errStyle.Render("✗") + "  Signature check failed\n")
			}
		}
		if m.result.Verify.SPCTL != nil {
			if m.result.Verify.SPCTL.Accepted {
				b.WriteString(okStyle.Render("✓") + "  Gatekeeper accepts the clone\n")
			} else {
				b.WriteString(warnStyle.Render("⚠") + "  Gatekeeper will prompt on first launch\n")
				b.WriteString(faintStyle.Render("   Normal for locally-signed clones. Right-click → Open the first time.\n"))
			}
		}
	}

	return cardStyle.Render(b.String())
}

// ——— Chrome (step bar + footer) ————————————————————————————————————————

func (m model) viewStepBar() string {
	current := int(m.screen)
	parts := make([]string, 0, len(stepLabels)*2)
	for i, label := range stepLabels {
		var s string
		switch {
		case i < current:
			s = stepDoneStyle.Render(fmt.Sprintf("✓ %s", label))
		case i == current:
			s = stepActiveStyle.Render(fmt.Sprintf("● %s", label))
		default:
			s = stepPendStyle.Render(fmt.Sprintf("○ %s", label))
		}
		if i > 0 {
			parts = append(parts, stepSepStyle.Render("›"))
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

func (m model) viewFooter() string {
	var hints string
	switch m.screen {
	case scrList:
		if m.listLoading {
			hints = "loading …  ·  ctrl+c quit"
		} else {
			hints = "enter pick  ·  / search  ·  ↑/↓ navigate  ·  q quit"
		}
	case scrForm:
		hints = "tab next  ·  shift+tab prev  ·  enter confirm  ·  ctrl+s start  ·  esc back"
	case scrProgress:
		hints = "working …  ·  ctrl+c abort"
	case scrResult:
		if m.runErr != nil {
			hints = "c copy JSON  ·  n new clone  ·  q quit"
		} else if m.plan.DryRun {
			hints = "c copy JSON  ·  n new clone  ·  q quit"
		} else {
			hints = "o open  ·  f Finder  ·  c copy JSON  ·  n new clone  ·  q quit"
		}
	}
	line := faintStyle.Render(hints)
	if m.toast != "" {
		line = toastStyle.Render("● "+m.toast) + "    " + line
	}
	return footerStyle.Render(line)
}

// ——— Root View ————————————————————————————————————————————————————————

func (m model) View() string {
	var body string
	switch m.screen {
	case scrList:
		body = m.viewList()
	case scrForm:
		body = m.viewForm()
	case scrProgress:
		body = m.viewProgress()
	case scrResult:
		body = m.viewResult()
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
