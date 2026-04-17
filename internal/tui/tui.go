package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tt-a1i/appclone/internal/core/appinfo"
	"github.com/tt-a1i/appclone/internal/core/clone"
	"github.com/tt-a1i/appclone/internal/core/macos"
)

type screen int

const (
	scrList screen = iota
	scrForm
	scrProgress
	scrResult
)

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

// ——— Model ————————————————————————————————————————————————————————————

type model struct {
	screen        screen
	width, height int

	// list
	list        list.Model
	listLoading bool
	listSpin    spinner.Model

	// form
	selected *appinfo.InspectionReport
	inputs   []textinput.Model
	formIdx  int
	formErr  string

	// progress
	plan   *clone.ClonePlan
	stages []stageRow
	pipeCh chan pipelineMsg

	// result
	result *clone.RunResult
	runErr error
}

type stageRow struct {
	stage  string
	status clone.StageStatus
	msg    string
}

// ——— Styles ———————————————————————————————————————————————————————————

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	faintStyle  = lipgloss.NewStyle().Faint(true)
	labelStyle  = lipgloss.NewStyle().Width(14).Faint(true)
	focusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	borderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).BorderForeground(lipgloss.Color("240"))
)

// ——— Entry ———————————————————————————————————————————————————————————

func Run() error {
	_, err := tea.NewProgram(initialModel(), tea.WithAltScreen()).Run()
	return err
}

func initialModel() model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	delegate := list.NewDefaultDelegate()
	l := list.New(nil, delegate, 0, 0)
	l.Title = "AppClone — pick an app to clone"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()

	return model{
		screen:      scrList,
		listLoading: true,
		listSpin:    sp,
		list:        l,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.listSpin.Tick, scanAppsCmd())
}

// ——— App scanning ——————————————————————————————————————————————————————

type appItem struct {
	path       string
	name       string
	bundleID   string
	signed     bool
	components int
}

func (i appItem) FilterValue() string { return i.name + " " + i.bundleID }
func (i appItem) Title() string       { return i.name }
func (i appItem) Description() string {
	sig := "unsigned"
	if i.signed {
		sig = "signed"
	}
	return fmt.Sprintf("%s · %s", i.bundleID, sig)
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
		m.list.SetSize(msg.Width, msg.Height-4)
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
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
		var cmd tea.Cmd
		m.listSpin, cmd = m.listSpin.Update(msg)
		if m.listLoading {
			return m, cmd
		}
	case appsLoadedMsg:
		m.listLoading = false
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
		return fmt.Sprintf("%s Scanning /Applications …\n\n%s",
			m.listSpin.View(),
			faintStyle.Render("(Takes a moment; reads every Info.plist.)"),
		)
	}
	return m.list.View() + "\n" + faintStyle.Render("enter = pick · / = filter · q = quit")
}

// ——— Screen: form ——————————————————————————————————————————————————————

const (
	fldName = iota
	fldBundleID
	fldDisplay
	fldTarget
	nFields
)

func buildFormInputs(report *appinfo.InspectionReport) []textinput.Model {
	inputs := make([]textinput.Model, nFields)

	name := textinput.New()
	name.Placeholder = defaultCloneName(report)
	name.CharLimit = 64
	name.Prompt = "› "
	inputs[fldName] = name

	bid := textinput.New()
	bid.Placeholder = defaultBundleID(report)
	bid.CharLimit = 128
	bid.Prompt = "› "
	inputs[fldBundleID] = bid

	dn := textinput.New()
	dn.Placeholder = "(defaults to Name)"
	dn.CharLimit = 64
	dn.Prompt = "› "
	inputs[fldDisplay] = dn

	target := textinput.New()
	target.Placeholder = "(defaults to /Applications/<Name>.app)"
	target.CharLimit = 512
	target.Prompt = "› "
	inputs[fldTarget] = target

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
			if m.formIdx < nFields-1 {
				m.focusForm(m.formIdx + 1)
				return m, textinput.Blink
			}
			return m, m.submitForm()
		}
	case planBuiltMsg:
		if msg.err != nil {
			m.formErr = msg.err.Error()
			return m, nil
		}
		m.plan = msg.plan
		m.screen = scrProgress
		m.pipeCh = make(chan pipelineMsg, 64)
		return m, tea.Batch(startPipeline(m.plan, m.pipeCh), waitForPipeline(m.pipeCh))
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
	if v := t.Value(); v != "" {
		return v
	}
	return t.Placeholder
}

func (m model) submitForm() tea.Cmd {
	opts := clone.PlanOptions{
		SourceApp:   m.selected.Identity.AppPath,
		Name:        valueOrPlaceholder(m.inputs[fldName]),
		BundleID:    valueOrPlaceholder(m.inputs[fldBundleID]),
		DisplayName: m.inputs[fldDisplay].Value(),
		TargetApp:   m.inputs[fldTarget].Value(),
	}
	return func() tea.Msg {
		plan, err := clone.DerivePlan(opts)
		return planBuiltMsg{plan: plan, err: err}
	}
}

func (m model) viewForm() string {
	labels := []string{"Name", "Bundle ID", "Display Name", "Target path"}
	var b strings.Builder
	b.WriteString(titleStyle.Render("Configure clone"))
	b.WriteString("\n\n")
	b.WriteString(faintStyle.Render(fmt.Sprintf("Source: %s  (%s)", m.selected.Identity.AppPath, m.selected.Identity.BundleID)))
	b.WriteString("\n\n")
	for i := range m.inputs {
		label := labelStyle.Render(labels[i])
		if i == m.formIdx {
			label = focusStyle.Width(14).Render(labels[i])
		}
		b.WriteString(label)
		b.WriteString(m.inputs[i].View())
		b.WriteString("\n")
	}
	if m.formErr != "" {
		b.WriteString("\n")
		b.WriteString(errStyle.Render("Error: " + m.formErr))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(faintStyle.Render("tab = next · shift+tab = prev · enter (on last) = start · esc = back"))
	return borderStyle.Render(b.String())
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
	case pipelineMsg:
		if msg.done {
			m.result = msg.result
			m.runErr = msg.err
			m.screen = scrResult
			return m, nil
		}
		if msg.event != nil {
			m.stages = append(m.stages, stageRow{
				stage:  msg.event.Stage,
				status: msg.event.Status,
				msg:    msg.event.Message,
			})
		}
		return m, waitForPipeline(m.pipeCh)
	}
	return m, nil
}

func (m model) viewProgress() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Cloning …"))
	b.WriteString("\n\n")
	b.WriteString(faintStyle.Render(fmt.Sprintf("%s → %s", m.plan.SourceApp, m.plan.TargetApp)))
	b.WriteString("\n")
	b.WriteString(faintStyle.Render(fmt.Sprintf("%s → %s", m.plan.BundleIDBefore, m.plan.BundleIDAfter)))
	b.WriteString("\n\n")
	for _, r := range m.stages {
		b.WriteString(stageLine(r))
		b.WriteString("\n")
	}
	return b.String()
}

func stageLine(r stageRow) string {
	symbol := "•"
	switch r.status {
	case clone.StageStart:
		symbol = faintStyle.Render("→")
	case clone.StageOK:
		symbol = okStyle.Render("✓")
	case clone.StageWarn:
		symbol = warnStyle.Render("⚠")
	case clone.StageError:
		symbol = errStyle.Render("✗")
	case clone.StageSkip:
		symbol = faintStyle.Render("-")
	}
	return fmt.Sprintf("  %s  %-14s %s", symbol, r.stage, r.msg)
}

// ——— Screen: result ——————————————————————————————————————————————————

func (m model) updateResult(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "q", "enter", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) viewResult() string {
	var b strings.Builder
	if m.runErr != nil {
		b.WriteString(errStyle.Render("FAILED"))
		b.WriteString("\n\n")
		b.WriteString(m.runErr.Error())
	} else if m.plan.DryRun {
		b.WriteString(okStyle.Render("DRY-RUN OK"))
		b.WriteString("\n\n")
		b.WriteString("Would clone to: " + m.plan.TargetApp)
	} else {
		b.WriteString(okStyle.Render("SUCCESS"))
		b.WriteString("\n\n")
		b.WriteString("Clone at: " + m.plan.TargetApp)
	}
	b.WriteString("\n\n")
	for _, r := range m.stages {
		b.WriteString(stageLine(r))
		b.WriteString("\n")
	}
	if m.result != nil && m.result.Verify != nil {
		b.WriteString("\nVerify:\n")
		if m.result.Verify.Codesign != nil {
			fmt.Fprintf(&b, "  codesign: ok=%v\n", m.result.Verify.Codesign.OK)
		}
		if m.result.Verify.SPCTL != nil {
			fmt.Fprintf(&b, "  spctl:    accepted=%v (ad-hoc clones often rejected)\n", m.result.Verify.SPCTL.Accepted)
		}
		for _, w := range m.result.Verify.Warnings {
			b.WriteString("  ")
			b.WriteString(warnStyle.Render("⚠ " + w))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(faintStyle.Render("press any key to quit"))
	return b.String()
}

// ——— Root View ————————————————————————————————————————————————————————

func (m model) View() string {
	switch m.screen {
	case scrList:
		return m.viewList()
	case scrForm:
		return m.viewForm()
	case scrProgress:
		return m.viewProgress()
	case scrResult:
		return m.viewResult()
	}
	return ""
}
