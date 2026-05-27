package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ─── screen / focus enums ──────────────────────────────────────────────────

type screen int

const (
	screenMain screen = iota
	screenNewExec
	screenEditEnv
	screenQuitConfirm
	screenRerunConfirm
)

type paneFocus int

const (
	paneLeft  paneFocus = iota // sidebar
	paneRight                  // test list / log
)

type leftFocus int

const (
	leftExec leftFocus = iota
	leftEnv
)

type rightFocus int

const (
	rightTests rightFocus = iota
	rightLog
)

// ─── environment variable ─────────────────────────────────────────────────

// EnvVar is a configurable environment variable shown in the sidebar.
type EnvVar struct {
	Key     string
	Value   string
	Default string
}

// ─── messages ─────────────────────────────────────────────────────────────

type tickMsg time.Time

type testStartedMsg struct {
	execID  string
	testIdx int
	startAt time.Time
}

type testDoneMsg struct {
	execID  string
	testIdx int
	endAt   time.Time
	passed  bool
	stopped bool
}

type execDoneMsg struct {
	execID    string
	endAt     time.Time
	cancelled bool
}

// ─── new-execution dialog state ───────────────────────────────────────────

type newExecStep int

const (
	newExecStepFilter   newExecStep = iota // choosing tier / custom
	newExecStepCustom                      // typing custom focus
	newExecStepParallel                    // setting parallelism
)

type newExecModel struct {
	step          newExecStep
	filterIdx     int // 0=tier0 1=tier1 2=tier2 3=all 4=custom
	customInput   textinput.Model
	parallelInput textinput.Model
}

var filterOptions = []struct {
	label string
	value string // label-filter value; "custom" triggers custom input; "" = no filter (all)
}{
	{"Tier 0 (critical)", "tier0"},
	{"Tier 1 (major)", "tier1"},
	{"Tier 2 (minor)", "tier2"},
	{"All tests", ""},
	{"Custom focus…", "custom"},
}

// ─── main model ───────────────────────────────────────────────────────────

// Model is the bubbletea model. It must be copyable (no embedded mutexes).
type Model struct {
	// terminal dimensions
	width  int
	height int

	// core data
	binaryPath string
	e2eDir     string
	suites     []TestSuite
	store      *HistoryStore

	// executions list (left sidebar)
	executions  []*Execution
	execCursor  int
	viewingExec int // index into executions; -1 = nothing selected

	// env vars (left sidebar bottom)
	envVars      []EnvVar
	envCursor    int
	envFilter    string
	envFiltering bool

	// pane / focus routing
	currentScreen screen
	pane          paneFocus
	leftPane      leftFocus
	rightPane     rightFocus

	// right pane — test list
	testCursor int

	// right pane — log viewport
	logViewport    viewport.Model
	logViewKey     string // exec:testIdx:path — detects test/log switches
	logLastContent string // skip redundant SetContent to preserve scroll
	logFollowTail  bool   // scroll to bottom when log content updates

	// modals
	newExec    newExecModel
	editInput  textinput.Model
	editEnvIdx int

	quitReturnScreen screen
	quitConfirmIdx   int // 0=yes 1=no

	rerunReturnScreen screen
	rerunConfirmIdx   int // 0=yes 1=no
	rerunExecID       string
	rerunIndices      []int
	rerunSummary      string

	// active test run cancellation
	runCancel context.CancelFunc
	runExecID string

	statusMsg string // ephemeral user feedback

	// wire back to tea.Program so goroutines can send messages
	program *tea.Program
}

// ─── constructor ──────────────────────────────────────────────────────────

// NewModel creates the initial model.
func NewModel(binaryPath, e2eDir, projectDir string, suites []TestSuite, history []*Execution) Model {
	store, _ := NewHistoryStore(projectDir)

	vars := make([]EnvVar, len(knownEnvVars))
	copy(vars, knownEnvVars)
	for i, v := range vars {
		if val := os.Getenv(v.Key); val != "" {
			vars[i].Value = val
		} else {
			vars[i].Value = v.Default
		}
	}
	sortEnvVarsByKey(vars)

	vp := viewport.New(0, 0)

	ci := textinput.New()
	ci.Placeholder = "focus regex…"
	ci.CharLimit = 512

	pi := textinput.New()
	pi.Placeholder = "1"
	pi.CharLimit = 3

	ei := textinput.New()
	ei.CharLimit = 2048

	return Model{
		binaryPath:  binaryPath,
		e2eDir:      e2eDir,
		suites:      suites,
		store:       store,
		executions:  history,
		viewingExec: -1,
		envVars:     vars,
		logViewport:   vp,
		logFollowTail: true,
		newExec: newExecModel{
			customInput:   ci,
			parallelInput: pi,
		},
		editInput: ei,
	}
}

// SetProgram wires the tea.Program reference so goroutines can send messages.
func (m *Model) SetProgram(p *tea.Program) { m.program = p }

// ─── Init ─────────────────────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	return tea.Batch(tick(), tea.WindowSize())
}

func tick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// ─── Update ───────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:cyclop
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.logViewport.Width = m.rightWidth()
		m.logViewport.Height = m.logHeight()
		if m.currentScreen == screenEditEnv {
			m.editInput.Width = m.editEnvInputWidth()
		}
		if m.currentScreen == screenNewExec {
			m.syncNewExecInputWidths()
		}
		return m, nil

	case tickMsg:
		m.refreshLog()
		return m, tick()

	// goroutine → model: test lifecycle events
	// Update() is single-threaded, no mutex needed.

	case testStartedMsg:
		if e := m.findExec(msg.execID); e != nil && msg.testIdx < len(e.Tests) {
			e.Tests[msg.testIdx].Status = StatusRunning
			e.Tests[msg.testIdx].StartAt = msg.startAt
		}
		return m, nil

	case testDoneMsg:
		if e := m.findExec(msg.execID); e != nil && msg.testIdx < len(e.Tests) {
			t := e.Tests[msg.testIdx]
			t.EndAt = msg.endAt
			switch {
			case msg.stopped:
				t.Status = StatusStopped
			case msg.passed:
				t.Status = StatusPassed
			default:
				t.Status = StatusFailed
			}
		}
		return m, nil

	case execDoneMsg:
		if msg.execID == m.runExecID {
			m.runCancel = nil
			m.runExecID = ""
		}
		m.statusMsg = ""
		if e := m.findExec(msg.execID); e != nil {
			e.EndedAt = msg.endAt
			finalizeExecution(e, msg.cancelled)
			if m.store != nil {
				_ = m.store.Save(e)
			}
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	return m, nil
}

// findExec returns the *Execution with the given ID, or nil.
func (m *Model) findExec(id string) *Execution {
	for _, e := range m.executions {
		if e.ID == id {
			return e
		}
	}
	return nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.currentScreen {
	case screenNewExec:
		return m.handleKeyNewExec(msg)
	case screenEditEnv:
		return m.handleKeyEditEnv(msg)
	case screenQuitConfirm:
		return m.handleKeyQuitConfirm(msg)
	case screenRerunConfirm:
		return m.handleKeyRerunConfirm(msg)
	default:
		return m.handleKeyMain(msg)
	}
}

// ─── main-screen key handler ──────────────────────────────────────────────

func (m Model) handleKeyMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) { //nolint:cyclop
	key := msg.String()

	switch key {
	case "ctrl+c":
		m = m.requestQuit()
		return m, nil
	}

	if m.envFiltering {
		switch key {
		case "esc":
			m.envFiltering = false
			m.envFilter = ""
		case "enter":
			m.envFiltering = false
		case "backspace":
			if len(m.envFilter) > 0 {
				m.envFilter = m.envFilter[:len(m.envFilter)-1]
			}
		default:
			if len(key) == 1 {
				m.envFilter += key
			}
		}
		m.envCursor = 0
		return m, nil
	}

	if key == "q" {
		m = m.requestQuit()
		return m, nil
	}

	switch key {
	case "s", "S":
		if m.hasActiveExecution() {
			m.stopActiveExecution()
			m.statusMsg = "Stopping tests…"
		}
		return m, nil

	case "n", "N":
		if m.hasActiveExecution() {
			m.statusMsg = "Cannot start: a run is in progress (s to stop)"
			return m, nil
		}
		m.statusMsg = ""
		m.currentScreen = screenNewExec
		m.newExec.step = newExecStepFilter
		m.newExec.filterIdx = 0
		m.newExec.customInput.SetValue("")
		m.syncNewExecInputWidths()
		return m, nil

	case "tab":
		m = m.cycleFocus(false)
		return m, nil

	case "shift+tab":
		m = m.cycleFocus(true)
		return m, nil

	case "up", "k":
		if m.isLogFocused() {
			m.logFollowTail = false
			m.logViewport.LineUp(1)
			return m, nil
		}
		m = m.moveUp()
		return m, nil

	case "down", "j":
		if m.isLogFocused() {
			m.logViewport.LineDown(1)
			if m.logViewport.AtBottom() {
				m.logFollowTail = true
			}
			return m, nil
		}
		m = m.moveDown()
		return m, nil

	case "enter":
		return m.handleEnter()

	case "/":
		if m.pane == paneLeft && m.leftPane == leftEnv {
			m.envFiltering = true
			m.envFilter = ""
		}
		return m, nil

	case "esc":
		return m, nil

	case "pgdown", "ctrl+d":
		if m.isLogFocused() {
			m.logViewport.HalfPageDown()
			if m.logViewport.AtBottom() {
				m.logFollowTail = true
			}
		}
		return m, nil

	case "pgup", "ctrl+u":
		if m.isLogFocused() {
			m.logFollowTail = false
			m.logViewport.HalfPageUp()
		}
		return m, nil

	case "G":
		if m.isLogFocused() {
			m.logFollowTail = true
			m.logViewport.GotoBottom()
		}
		return m, nil

	case "g":
		if m.isLogFocused() {
			m.logFollowTail = false
			m.logViewport.GotoTop()
		}
		return m, nil

	case "r":
		return m.handleRerunSingle()

	case "R":
		return m.handleRerunAllFailed()
	}

	return m, nil
}

func (m Model) cycleFocus(reverse bool) Model {
	// Tab cycles: executions → env → tests → log (Shift+Tab reverses).
	if reverse {
		switch {
		case m.pane == paneRight && m.rightPane == rightLog:
			m.rightPane = rightTests
		case m.pane == paneRight && m.rightPane == rightTests:
			m.pane = paneLeft
			m.leftPane = leftEnv
		case m.pane == paneLeft && m.leftPane == leftEnv:
			m.leftPane = leftExec
		default:
			m.pane = paneRight
			m.rightPane = rightLog
		}
	} else {
		switch {
		case m.pane == paneLeft && m.leftPane == leftExec:
			m.leftPane = leftEnv
		case m.pane == paneLeft && m.leftPane == leftEnv:
			m.pane = paneRight
			m.rightPane = rightTests
		case m.pane == paneRight && m.rightPane == rightTests:
			m.rightPane = rightLog
		default:
			m.pane = paneLeft
			m.leftPane = leftExec
		}
	}
	return m
}

func (m Model) moveUp() Model {
	switch {
	case m.pane == paneLeft && m.leftPane == leftEnv:
		if m.envCursor > 0 {
			m.envCursor--
		}
	case m.pane == paneLeft && m.leftPane == leftExec:
		if m.execCursor > 0 {
			m.execCursor--
		}
	case m.pane == paneRight && m.rightPane == rightTests:
		if m.testCursor > 0 {
			m.testCursor--
		}
	}
	return m
}

func (m Model) moveDown() Model {
	switch {
	case m.pane == paneLeft && m.leftPane == leftEnv:
		filtered := m.filteredEnvVars()
		if m.envCursor < len(filtered)-1 {
			m.envCursor++
		}
	case m.pane == paneLeft && m.leftPane == leftExec:
		if m.execCursor < len(m.executions)-1 {
			m.execCursor++
		}
	case m.pane == paneRight && m.rightPane == rightTests:
		if m.viewingExec >= 0 && m.viewingExec < len(m.executions) {
			if m.testCursor < len(m.executions[m.viewingExec].Tests)-1 {
				m.testCursor++
			}
		}
	}
	return m
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.pane {
	case paneLeft:
		if m.leftPane == leftEnv {
			m.enterEnvEdit()
			return m, nil
		}
		if m.execCursor < len(m.executions) {
			m.viewingExec = m.execCursor
			m.testCursor = 0
			m.resetLogView()
			m.refreshLog()
		}
	case paneRight:
		if m.rightPane == rightTests {
			m.rightPane = rightLog
			m.resetLogView()
			m.refreshLog()
		}
	}
	return m, nil
}

func (m *Model) enterEnvEdit() {
	filtered := m.filteredEnvVars()
	if m.envCursor >= len(filtered) {
		return
	}
	targetKey := filtered[m.envCursor].Key
	for i, ev := range m.envVars {
		if ev.Key == targetKey {
			m.editEnvIdx = i
			m.editInput.SetValue(ev.Value)
			m.editInput.Width = m.editEnvInputWidth()
			m.editInput.Focus()
			m.currentScreen = screenEditEnv
			return
		}
	}
}

// ─── new-exec screen key handler ──────────────────────────────────────────

func (m Model) handleKeyNewExec(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "ctrl+c" {
		m = m.requestQuit()
		return m, nil
	}

	switch m.newExec.step {
	case newExecStepFilter:
		switch key {
		case "esc":
			m.currentScreen = screenMain
		case "up", "k":
			if m.newExec.filterIdx > 0 {
				m.newExec.filterIdx--
			}
		case "down", "j":
			if m.newExec.filterIdx < len(filterOptions)-1 {
				m.newExec.filterIdx++
			}
		case "enter", " ":
			if filterOptions[m.newExec.filterIdx].value == "custom" {
				m.newExec.step = newExecStepCustom
				m.syncNewExecInputWidths()
				m.newExec.customInput.Focus()
			} else {
				m.enterNewExecParallelStep()
			}
		}

	case newExecStepCustom:
		switch key {
		case "esc":
			m.newExec.step = newExecStepFilter
		case "enter":
			if m.newExec.customInput.Value() != "" {
				m.enterNewExecParallelStep()
			}
		default:
			var cmd tea.Cmd
			m.newExec.customInput, cmd = m.newExec.customInput.Update(msg)
			return m, cmd
		}

	case newExecStepParallel:
		switch key {
		case "esc":
			if filterOptions[m.newExec.filterIdx].value == "custom" {
				m.newExec.step = newExecStepCustom
			} else {
				m.newExec.step = newExecStepFilter
			}
		case "enter":
			return m.startExecution()
		default:
			var cmd tea.Cmd
			m.newExec.parallelInput, cmd = m.newExec.parallelInput.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m Model) startExecution() (tea.Model, tea.Cmd) {
	if m.hasActiveExecution() {
		m.statusMsg = "Cannot start: a run is in progress (s to stop)"
		return m, nil
	}

	opt := filterOptions[m.newExec.filterIdx]
	labelFilter := opt.value
	var customFocus string
	if labelFilter == "custom" {
		labelFilter = ""
		customFocus = m.newExec.customInput.Value()
	}

	parallelism, _ := strconv.Atoi(m.newExec.parallelInput.Value())
	if parallelism < 1 {
		parallelism = 1
	}

	matchingSuites := m.resolveSuites(labelFilter, customFocus)

	execID := fmt.Sprintf("%d", time.Now().UnixMilli())
	logDir := m.store.LogDir(execID)
	_ = os.MkdirAll(logDir, 0o750)

	var tests []*TestRun
	for _, s := range matchingSuites {
		logFile := filepath.Join(logDir, sanitizeFilename(s.Name)+".log")
		tests = append(tests, &TestRun{
			Suite:   s,
			Status:  StatusPending,
			LogFile: logFile,
		})
	}

	e := &Execution{
		ID:        execID,
		StartedAt: time.Now(),
		Filter: ExecFilter{
			LabelFilter: labelFilter,
			Focus:       customFocus,
			Parallelism: parallelism,
		},
		Tests:  tests,
		Status: StatusRunning,
		LogDir: logDir,
	}

	m.executions = append([]*Execution{e}, m.executions...)
	m.execCursor = 0
	m.viewingExec = 0
	m.testCursor = 0
	m.currentScreen = screenMain
	m.pane = paneRight
	m.rightPane = rightTests

	if m.store != nil {
		_ = m.store.Save(e)
	}

	indices := make([]int, len(e.Tests))
	for i := range e.Tests {
		indices[i] = i
	}
	m.launchTestRunner(e, indices, parallelism, labelFilter)

	return m, nil
}

// resolveSuites returns the suites to run for a given filter.
func (m *Model) resolveSuites(labelFilter, customFocus string) []TestSuite {
	if customFocus != "" {
		if matched := matchSuitesByCustomFocus(customFocus, m.suites); len(matched) > 0 {
			return matched
		}
		// No Describe matched: run Ginkgo with a flexible word pattern.
		return []TestSuite{{
			Name:  customFocus,
			Focus: ginkgoFocusFromCustomFocus(customFocus),
		}}
	}
	if labelFilter == "tier0" {
		out := make([]TestSuite, len(Tier0Catalog))
		copy(out, Tier0Catalog)
		return out
	}
	return FilterByLabel(m.suites, labelFilter)
}

func runTestProcess(ctx context.Context, binaryPath string, suite TestSuite, logFile, labelFilter string, envVars []EnvVar) (passed, stopped bool) {
	args := []string{"test", "--log-level=debug"}
	if labelFilter != "" {
		args = append(args, "--label-filter="+labelFilter)
	}
	args = append(args, "--focus", ginkgoFocus(suite))

	cmd := exec.CommandContext(ctx, binaryPath, args...) //nolint:gosec

	var env []string
	for _, e := range envVars {
		env = append(env, e.Key+"="+e.Value)
	}
	for _, e := range os.Environ() {
		key := strings.SplitN(e, "=", 2)[0]
		switch key {
		case "PATH", "HOME", "USER", "KUBECONFIG", "TESTDATA_DIR":
			env = append(env, e)
		}
	}
	cmd.Env = env

	lf, err := os.Create(logFile) //nolint:gosec
	if err != nil {
		return false, false
	}
	cmd.Stdout = lf
	cmd.Stderr = lf
	runErr := cmd.Run()
	_ = lf.Close()
	if ctx.Err() != nil {
		return false, true
	}
	if runErr == nil && logIndicatesNoSpecsRan(logFile) {
		return false, false
	}
	return runErr == nil, false
}

// ─── edit-env screen key handler ─────────────────────────────────────────

func (m Model) handleKeyEditEnv(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m = m.requestQuit()
		return m, nil
	case "esc":
		m.currentScreen = screenMain
		return m, nil
	case "enter":
		if m.editEnvIdx < len(m.envVars) {
			m.envVars[m.editEnvIdx].Value = m.editInput.Value()
		}
		m.currentScreen = screenMain
		return m, nil
	default:
		var cmd tea.Cmd
		m.editInput, cmd = m.editInput.Update(msg)
		return m, cmd
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────

func (m Model) filteredEnvVars() []EnvVar {
	if m.envFilter == "" {
		return m.envVars
	}
	var out []EnvVar
	for _, v := range m.envVars {
		if strings.Contains(strings.ToLower(v.Key), strings.ToLower(m.envFilter)) {
			out = append(out, v)
		}
	}
	return out
}

func (m *Model) enterNewExecParallelStep() {
	m.resetParallelInputDefault()
	m.newExec.step = newExecStepParallel
	m.syncNewExecInputWidths()
	m.newExec.parallelInput.Focus()
}

func (m *Model) resetParallelInputDefault() {
	count := m.countMatchingSuites()
	if count < 1 {
		count = 1
	}
	m.newExec.parallelInput.SetValue(strconv.Itoa(count))
}

func (m Model) countMatchingSuites() int {
	opt := filterOptions[m.newExec.filterIdx]
	if opt.value == "tier0" {
		return Tier0SuiteCount()
	}
	if opt.value == "custom" {
		focus := m.newExec.customInput.Value()
		if focus == "" {
			return 0
		}
		if matched := matchSuitesByCustomFocus(focus, m.suites); len(matched) > 0 {
			return len(matched)
		}
		return 1 // ginkgo fallback with flexible word pattern
	}
	return len(FilterByLabel(m.suites, opt.value))
}

func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	name := b.String()
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}

// ─── layout helpers ───────────────────────────────────────────────────────

const sidebarWidth = 38

func (m Model) sidebarW() int   { return sidebarWidth }
func (m Model) rightWidth() int { return max(1, m.width-sidebarWidth-1) }
func (m Model) contentHeight() int {
	h := m.height - 2 // header + footer
	if h < 1 {
		h = 1
	}
	return h
}
func (m Model) execListHeight() int {
	h := m.contentHeight() * 65 / 100
	if h < 4 {
		h = 4
	}
	return h
}
func (m Model) envPaneHeight() int {
	h := m.contentHeight() - m.execListHeight() - 1
	if h < 2 {
		h = 2
	}
	return h
}
func (m Model) testListHeight() int {
	h := m.contentHeight() * 40 / 100
	if h < 4 {
		h = 4
	}
	return h
}
func (m Model) logHeight() int {
	h := m.contentHeight() - m.testListHeight() - 1
	if h < 2 {
		h = 2
	}
	return h
}

// ─── View ─────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 {
		return "Loading…"
	}

	switch m.currentScreen {
	case screenNewExec:
		return m.viewNewExec()
	case screenEditEnv:
		return m.viewEditEnv()
	case screenQuitConfirm:
		return m.viewQuitConfirm()
	case screenRerunConfirm:
		return m.viewRerunConfirm()
	}

	return m.viewHeader() + "\n" + m.viewBody() + "\n" + m.viewFooter()
}

func (m Model) viewHeader() string {
	title := " HyperFleet E2E  "
	right := fmt.Sprintf(" %d suites ", len(m.suites))
	pad := m.width - len(title) - len(right)
	if pad < 0 {
		pad = 0
	}
	return styleHeader.Width(m.width).Render(title + strings.Repeat(" ", pad) + right)
}

func (m Model) viewFooter() string {
	hints := " n:new  s:stop  tab:panel  ↑↓/jk:nav  click:select  /:filter-env  enter:edit-env  log:scroll  ctrl-c:quit"
	if m.canRerunTests() {
		hints = " r:rerun  R:rerun-failed  " + hints
	}
	if m.hasActiveExecution() {
		hints = styleRun.Render("● running") + "  " + hints
	}
	if m.statusMsg != "" {
		hints = styleAccent.Render(m.statusMsg) + "  |  " + hints
	}
	return styleFooter.Width(m.width).Render(truncate(hints, m.width))
}

func (m Model) viewBody() string {
	sidebar := m.viewSidebar()
	right := m.viewRight()
	sep := styleBorder.Render(
		strings.Repeat("│\n", m.contentHeight()),
	)
	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, sep, right)
}

// ─── sidebar ──────────────────────────────────────────────────────────────

func (m Model) viewSidebar() string {
	w := m.sidebarW()
	divider := styleBorder.Width(w).Render(strings.Repeat("─", w))
	return lipgloss.JoinVertical(lipgloss.Left,
		m.viewExecList(w),
		divider,
		m.viewEnvPane(w),
	)
}

func (m Model) viewExecList(w int) string {
	h := m.execListHeight()
	newBtn := styleMuted.Render(" [N] new execution")

	var rows []string
	rows = append(rows, panelSectionTitle("EXECUTIONS", m.isExecPanelFocused()))

	visible := h - 2 // title + new-button
	if visible < 1 {
		visible = 1
	}

	start := 0
	if len(m.executions) > 0 && m.execCursor >= visible {
		start = m.execCursor - visible + 1
	}

	for i := start; i < len(m.executions) && i < start+visible; i++ {
		e := m.executions[i]
		icon := statusIcon(e.Status)
		ts := e.StartedAt.Format("01-02 15:04")
		filter := truncate(e.FilterLabel(), 8)
		dur := formatDuration(e.Duration())
		line := fmt.Sprintf(" %s %s %-8s %s", ts, icon, filter, dur)

		focused := m.pane == paneLeft && m.leftPane == leftExec && i == m.execCursor
		viewing := m.viewingExec == i
		switch {
		case focused && viewing:
			rows = append(rows, styleFocused.Width(w).Render("▶"+truncate(line, w-1)))
		case focused:
			rows = append(rows, styleFocused.Width(w).Render(truncate(line, w)))
		case viewing:
			rows = append(rows, styleSelected.Width(w).Render(truncate(line, w)))
		default:
			rows = append(rows, styleMuted.Width(w).Render(truncate(line, w)))
		}
	}

	if len(m.executions) == 0 {
		rows = append(rows, styleFaint.Render("  (no executions yet)"))
	}

	rows = append(rows, newBtn)
	return applyPanelFocus(m.isExecPanelFocused(), w, h, strings.Join(rows, "\n"))
}

func (m Model) viewEnvPane(w int) string {
	h := m.envPaneHeight()
	if h < 2 {
		return strings.Repeat(" \n", h)
	}

	filterHint := " [/] filter  [enter] edit"
	if m.envFiltering {
		filterHint = " filter: " + m.envFilter + "▌"
	}
	title := panelSectionTitle("ENV", m.isEnvPanelFocused()) + styleFaint.Render(filterHint)
	rows := []string{title}

	filtered := m.filteredEnvVars()
	visible := h - 1
	start := 0
	if m.envCursor >= visible {
		start = m.envCursor - visible + 1
	}

	contentW := m.envListContentWidth(w)
	for i := start; i < len(filtered) && i < start+visible; i++ {
		v := filtered[i]
		focused := m.pane == paneLeft && m.leftPane == leftEnv && i == m.envCursor
		rows = append(rows, renderEnvVarRow(v.Key, v.Value, contentW, focused))
	}

	if len(filtered) == 0 {
		rows = append(rows, styleFaint.Render("  (none)"))
	}

	return applyPanelFocus(m.isEnvPanelFocused(), w, h, strings.Join(rows, "\n"))
}

// ─── right pane ───────────────────────────────────────────────────────────

func (m Model) viewRight() string {
	w := m.rightWidth()
	divider := styleBorder.Width(w).Render(strings.Repeat("─", w))
	return lipgloss.JoinVertical(lipgloss.Left,
		m.viewTestList(w),
		divider,
		m.viewLogPane(w),
	)
}

func (m Model) viewTestList(w int) string {
	h := m.testListHeight()

	if m.viewingExec < 0 || m.viewingExec >= len(m.executions) {
		msg := "  Select an execution or press [N] to start a new one."
		return applyPanelFocus(m.isTestsPanelFocused(), w, h, styleMuted.Render(msg))
	}

	e := m.executions[m.viewingExec]
	contentW := m.testListContentWidth(w)
	suiteW, statusW, durW := testListColumnWidths(contentW)
	titleStr := fmt.Sprintf("TESTS  %s  %s", e.FilterLabel(), formatDuration(e.Duration()))
	header := styleFaint.Render(fmt.Sprintf("   %-*s %-*s %-*s", suiteW, "SUITE", statusW, "STATUS", durW, "TIME"))

	rows := []string{panelSectionTitle(titleStr, m.isTestsPanelFocused()), header}
	visible := h - 2
	if visible < 1 {
		visible = 1
	}

	start := 0
	if m.testCursor >= visible {
		start = m.testCursor - visible + 1
	}

	for i := start; i < len(e.Tests) && i < start+visible; i++ {
		t := e.Tests[i]
		icon := statusIcon(t.Status)
		statusPlain := strings.ToUpper(string(t.Status))
		statusLabel := styleForStatus(t.Status).Render(fmt.Sprintf("%-*s", statusW, statusPlain))
		dur := formatDuration(t.Duration())
		suiteName := truncate(t.Suite.Name, suiteW)
		line := fmt.Sprintf("  %s %-*s %s %-*s", icon, suiteW, suiteName, statusLabel, durW, dur)

		if m.pane == paneRight && m.rightPane == rightTests && i == m.testCursor {
			rows = append(rows, styleFocused.Width(w).Render(line))
		} else {
			rows = append(rows, styleNormal.Width(w).Render(line))
		}
	}

	return applyPanelFocus(m.isTestsPanelFocused(), w, h, strings.Join(rows, "\n"))
}

func (m Model) viewLogPane(w int) string {
	h := m.logHeight()
	if h < 2 {
		return ""
	}

	title := "LOG"
	if m.viewingExec >= 0 && m.viewingExec < len(m.executions) {
		e := m.executions[m.viewingExec]
		if m.testCursor >= 0 && m.testCursor < len(e.Tests) {
			t := e.Tests[m.testCursor]
			if t.LogFile != "" {
				title = "LOG  " + t.LogFile
			} else {
				title = "LOG  " + t.Suite.Name
			}
		}
	}

	logFocused := m.isLogFocused()
	vpW := w
	if logFocused {
		vpW = max(1, w-2)
	}
	m.logViewport.Width = vpW
	m.logViewport.Height = h - 1

	body := panelSectionTitle(title, logFocused) + "\n" + m.logViewport.View()
	return applyPanelFocus(logFocused, w, h, body)
}

// ─── new-exec modal ───────────────────────────────────────────────────────

func (m *Model) syncNewExecInputWidths() {
	w := m.newExecInputWidth()
	m.newExec.customInput.Width = w
	m.newExec.parallelInput.Width = w
}

func (m Model) viewNewExec() string {
	modalW := m.newExecModalWidth()
	innerW := modalInnerWidth(modalW)

	var lines []string
	lines = append(lines, modalLine(styleSectionTitle.Render("New Test Execution"), innerW))
	lines = append(lines, "")

	switch m.newExec.step {
	case newExecStepFilter:
		lines = append(lines, modalLine(styleMuted.Render("Select tests to run:"), innerW))
		lines = append(lines, "")
		if m.hasActiveExecution() {
			lines = append(lines, modalLine(styleFail.Render("A run is in progress — press s to stop first."), innerW))
			lines = append(lines, "")
		}
		for i, opt := range filterOptions {
			countStr := ""
			if opt.value != "custom" {
				count := len(FilterByLabel(m.suites, opt.value))
				if opt.value == "tier0" {
					count = Tier0SuiteCount()
				}
				countStr = fmt.Sprintf(" (%d tests)", count)
			}
			text := fmt.Sprintf(" %s %s%s", radioIcon(i == m.newExec.filterIdx), opt.label, countStr)
			if i == m.newExec.filterIdx {
				lines = append(lines, modalLine(styleFocused.Render(text), innerW))
			} else {
				lines = append(lines, modalLine(styleNormal.Render(text), innerW))
			}
		}
		lines = append(lines, "", modalLine(styleFaint.Render("↑↓ navigate  Enter select  Esc cancel"), innerW))

	case newExecStepCustom:
		lines = append(lines, modalLine(styleMuted.Render("Focus regex (matches suite names):"), innerW))
		lines = append(lines, "")
		lines = append(lines, modalLine(m.newExec.customInput.View(), innerW))
		lines = append(lines, "")
		count := m.countMatchingSuites()
		lines = append(lines, modalLine(styleFaint.Render(fmt.Sprintf("%d suite(s) match", count)), innerW))
		lines = append(lines, modalLine(styleFaint.Render("Enter confirm  Esc back"), innerW))

	case newExecStepParallel:
		count := m.countMatchingSuites()
		lines = append(lines, modalLine(styleNormal.Render(fmt.Sprintf("%d suite(s) will run", count)), innerW))
		lines = append(lines, "")
		lines = append(lines, modalLine(styleMuted.Render("Parallel workers:"), innerW))
		lines = append(lines, modalLine(m.newExec.parallelInput.View(), innerW))
		lines = append(lines, "")
		lines = append(lines, modalLine(styleFaint.Render("Enter start  Esc back"), innerW))
	}

	return m.modalScreen(modalBox(joinModalLines(lines...), modalW))
}

func radioIcon(selected bool) string {
	if selected {
		return styleAccent.Render("◉")
	}
	return styleFaint.Render("○")
}

// ─── edit-env modal ───────────────────────────────────────────────────────

func (m Model) viewEditEnv() string {
	if m.editEnvIdx >= len(m.envVars) {
		return ""
	}
	v := m.envVars[m.editEnvIdx]

	modalW := m.editEnvModalWidth()
	innerW := modalInnerWidth(modalW)

	var lines []string
	lines = append(lines, modalLine(styleSectionTitle.Render("Edit Environment Variable"), innerW))
	lines = append(lines, "")
	lines = append(lines, modalLine(styleMuted.Render("Key"), innerW))
	lines = append(lines, modalLine(styleAccent.Render(v.Key), innerW))
	lines = append(lines, "")
	lines = append(lines, modalLine(styleMuted.Render("Default"), innerW))
	if v.Default == "" {
		lines = append(lines, modalLine(styleFaint.Render("(empty)"), innerW))
	} else {
		for _, l := range wrapText(v.Default, innerW) {
			lines = append(lines, modalLine(styleFaint.Render(l), innerW))
		}
	}
	lines = append(lines, "")
	lines = append(lines, modalLine(styleMuted.Render("Value"), innerW))
	lines = append(lines, modalLine(m.editInput.View(), innerW))
	lines = append(lines, "")
	lines = append(lines, modalLine(styleFaint.Render("Enter confirm  Esc cancel"), innerW))

	return m.modalScreen(modalBox(joinModalLines(lines...), modalW))
}

// ─── render helpers ───────────────────────────────────────────────────────

func statusIcon(s RunStatus) string {
	switch s {
	case StatusRunning:
		return styleRun.Render("●")
	case StatusPassed:
		return stylePass.Render("✓")
	case StatusFailed:
		return styleFail.Render("✗")
	case StatusStopped, StatusCancelled:
		return styleStop.Render("■")
	default:
		return stylePend.Render("○")
	}
}

func styleForStatus(s RunStatus) lipgloss.Style {
	switch s {
	case StatusPassed:
		return stylePass
	case StatusFailed:
		return styleFail
	case StatusRunning:
		return styleRun
	case StatusStopped, StatusCancelled:
		return styleStop
	default:
		return stylePend
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "-"
	}
	secs := int(d.Seconds())
	if secs >= 60 {
		return fmt.Sprintf("%dm%02ds", secs/60, secs%60)
	}
	return fmt.Sprintf("%ds", secs)
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-1]) + "…"
}

func padHeight(content string, h, _ int) string {
	lines := strings.Split(content, "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return strings.Join(lines, "\n")
}

