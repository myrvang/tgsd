package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

type mode int

const (
	modeList mode = iota
	modeSearch
	modePicker
	modeForm
	modeConfirm
)

type pane int

const (
	paneList pane = iota
	paneDetails
)

type sortColumn int

const (
	sortCreated sortColumn = iota
	sortStatus
	sortDescription
	sortCategory
	sortProject
	sortCount
)

type statusFilter int

const (
	statusAll statusFilter = iota
	statusOpen
	statusDone
)

type pickerKind int

const (
	pickCategory pickerKind = iota
	pickProject
)

// Form field indexes.
const (
	fieldDescription = iota
	fieldCategory
	fieldProject
	fieldNote
	fieldCount
)

type model struct {
	store         *Store
	width, height int
	mode          mode
	focus         pane
	status        string // one-shot message shown in the stats line

	search  textinput.Model
	visible []int // indexes into store.Tasks, in display order
	cursor  int
	offset  int

	detailScroll int

	sortCol  sortColumn
	sortDesc bool

	catFilter    map[string]bool
	projFilter   map[string]bool
	statusFilter statusFilter

	pickKind   pickerKind
	pickItems  []string
	pickCounts map[string]int
	pickCursor int

	inputs    [3]textinput.Model
	note      textarea.Model
	formField int
	editID    int // 0 = new task

	confirmArchive bool // modeConfirm is for archive rather than delete
}

func newModel(store *Store) *model {
	search := textinput.New()
	search.Prompt = "/ "
	search.Placeholder = "Search tasks..."
	search.PromptStyle = lipgloss.NewStyle().Foreground(colText)
	search.PlaceholderStyle = lipgloss.NewStyle().Foreground(colDim)

	m := &model{
		store:      store,
		search:     search,
		catFilter:  map[string]bool{},
		projFilter: map[string]bool{},
	}
	for i := range m.inputs {
		in := textinput.New()
		in.Prompt = "> "
		in.PromptStyle = lipgloss.NewStyle().Foreground(colDim)
		m.inputs[i] = in
	}
	m.inputs[fieldDescription].Placeholder = "What needs to be done?"
	m.inputs[fieldCategory].Placeholder = "e.g. Work"
	m.inputs[fieldProject].Placeholder = "optional"

	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.CharLimit = 0
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.Placeholder = "Notes for this task..."
	m.note = ta

	m.refresh()
	return m
}

// reloadTick is how often the TUI checks the tasks file for changes made by
// "gsd add" in another terminal.
type reloadTick struct{}

func pollReload() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return reloadTick{} })
}

func (m *model) Init() tea.Cmd { return pollReload() }

// reloadIfChanged picks up external changes, except while a form or delete
// confirmation is open (they refer to the current tasks; saving merges).
func (m *model) reloadIfChanged() {
	if m.mode == modeForm || m.mode == modeConfirm || !m.store.Changed() {
		return
	}
	selID := 0
	if t := m.selected(); t != nil {
		selID = t.ID
	}
	before := len(m.store.Tasks)
	if err := m.store.Reload(); err != nil {
		m.status = "Reload failed: " + err.Error()
		return
	}
	m.visible = nil // indexes are stale
	m.refresh()
	m.selectID(selID)
	if m.mode == modePicker {
		m.pickItems, m.pickCounts = m.distinct(m.pickKind)
		m.pickCursor = clamp(m.pickCursor, 0, len(m.pickItems)-1)
	}
	m.ensureVisible()
	if n := len(m.store.Tasks) - before; n > 0 {
		m.status = fmt.Sprintf("%d new task(s) added from the command line", n)
	}
}

// ---------------------------------------------------------------- data ---

func (m *model) selected() *Task {
	if m.cursor < 0 || m.cursor >= len(m.visible) || m.visible[m.cursor] >= len(m.store.Tasks) {
		return nil
	}
	return &m.store.Tasks[m.visible[m.cursor]]
}

func (m *model) selectID(id int) {
	for i, idx := range m.visible {
		if m.store.Tasks[idx].ID == id {
			m.cursor = i
			return
		}
	}
}

// distinct returns the sorted distinct categories or projects and how many
// tasks use each. Tasks without a value are counted under "".
func (m *model) distinct(kind pickerKind) ([]string, map[string]int) {
	counts := map[string]int{}
	for _, t := range m.store.Tasks {
		if kind == pickCategory {
			counts[t.Category]++
		} else {
			counts[t.Project]++
		}
	}
	values := make([]string, 0, len(counts))
	for v := range counts {
		values = append(values, v)
	}
	sort.Slice(values, func(i, j int) bool {
		return strings.ToLower(values[i]) < strings.ToLower(values[j])
	})
	return values, counts
}

// refresh rebuilds the visible list from filters, search and sorting, keeping
// the cursor on the same task when it is still visible.
func (m *model) refresh() {
	selID := 0
	if t := m.selected(); t != nil {
		selID = t.ID
	}

	// Drop filter values that no task uses any more, so a stale filter can
	// never hide everything.
	_, cats := m.distinct(pickCategory)
	for v := range m.catFilter {
		if _, ok := cats[v]; !ok {
			delete(m.catFilter, v)
		}
	}
	_, projs := m.distinct(pickProject)
	for v := range m.projFilter {
		if _, ok := projs[v]; !ok {
			delete(m.projFilter, v)
		}
	}

	var idx []int
	for i, t := range m.store.Tasks {
		if len(m.catFilter) > 0 && !m.catFilter[t.Category] {
			continue
		}
		if len(m.projFilter) > 0 && !m.projFilter[t.Project] {
			continue
		}
		if m.statusFilter == statusOpen && t.Done || m.statusFilter == statusDone && !t.Done {
			continue
		}
		idx = append(idx, i)
	}

	if q := strings.TrimSpace(m.search.Value()); q != "" {
		// Fuzzy search ranks by match quality, which overrides column sorting.
		hay := make([]string, len(idx))
		for k, i := range idx {
			t := m.store.Tasks[i]
			hay[k] = t.Description + " " + t.Category + " " + t.Project
		}
		matches := fuzzy.Find(q, hay)
		ranked := make([]int, 0, len(matches))
		for _, mt := range matches {
			ranked = append(ranked, idx[mt.Index])
		}
		idx = ranked
	} else {
		sort.SliceStable(idx, func(a, b int) bool {
			if m.sortDesc {
				a, b = b, a
			}
			return m.less(m.store.Tasks[idx[a]], m.store.Tasks[idx[b]])
		})
	}

	m.visible = idx
	if selID != 0 {
		m.selectID(selID)
	}
	m.clampCursor()
}

func (m *model) less(a, b Task) bool {
	cmp := 0
	switch m.sortCol {
	case sortStatus:
		if a.Done != b.Done {
			return !a.Done
		}
	case sortDescription:
		cmp = strings.Compare(strings.ToLower(a.Description), strings.ToLower(b.Description))
	case sortCategory:
		cmp = strings.Compare(strings.ToLower(a.Category), strings.ToLower(b.Category))
	case sortProject:
		cmp = strings.Compare(strings.ToLower(a.Project), strings.ToLower(b.Project))
	}
	if cmp != 0 {
		return cmp < 0
	}
	return a.ID < b.ID
}

func (m *model) clampCursor() {
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *model) move(delta int) {
	m.cursor += delta
	m.clampCursor()
	m.detailScroll = 0
}

func (m *model) persist() {
	if err := m.store.Save(); err != nil {
		m.status = "Save failed: " + err.Error()
	}
}

// -------------------------------------------------------------- update ---

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case reloadTick:
		m.reloadIfChanged()
		return m, pollReload()

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		m.status = ""
		var cmd tea.Cmd
		switch m.mode {
		case modeList:
			cmd = m.updateList(msg)
		case modeSearch:
			cmd = m.updateSearch(msg)
		case modePicker:
			m.updatePicker(msg)
		case modeForm:
			cmd = m.updateForm(msg)
		case modeConfirm:
			m.updateConfirm(msg)
		}
		m.ensureVisible()
		return m, cmd
	}

	// Anything else (cursor blink etc.) goes to the focused text widget.
	var cmd tea.Cmd
	switch m.mode {
	case modeSearch:
		m.search, cmd = m.search.Update(msg)
	case modeForm:
		cmd = m.updateFormWidget(msg)
	}
	return m, cmd
}

func (m *model) updateList(msg tea.KeyMsg) tea.Cmd {
	page := m.listRows()
	switch msg.String() {
	case "q":
		return tea.Quit
	case "tab":
		if m.focus == paneList {
			m.focus = paneDetails
		} else {
			m.focus = paneList
		}
	case "/":
		m.mode = modeSearch
		m.focus = paneList
		return m.search.Focus()
	case "esc":
		if m.search.Value() != "" {
			m.search.SetValue("")
			m.refresh()
		}
		m.focus = paneList

	case "j", "down":
		if m.focus == paneDetails {
			m.detailScroll++
		} else {
			m.move(1)
		}
	case "k", "up":
		if m.focus == paneDetails {
			m.detailScroll--
		} else {
			m.move(-1)
		}
	case "pgdown", "ctrl+f":
		if m.focus == paneDetails {
			m.detailScroll += page
		} else {
			m.move(page)
		}
	case "pgup", "ctrl+b":
		if m.focus == paneDetails {
			m.detailScroll -= page
		} else {
			m.move(-page)
		}
	case "g", "home":
		if m.focus == paneDetails {
			m.detailScroll = 0
		} else {
			m.move(-len(m.visible))
		}
	case "G", "end":
		if m.focus == paneDetails {
			m.detailScroll = 1 << 20 // clamped when rendered
		} else {
			m.move(len(m.visible))
		}

	case "s":
		m.sortCol = (m.sortCol + 1) % sortCount
		m.refresh()
	case "S":
		m.sortDesc = !m.sortDesc
		m.refresh()

	case "a":
		m.catFilter = map[string]bool{}
		m.projFilter = map[string]bool{}
		m.statusFilter = statusAll
		m.refresh()
	case "c":
		m.openPicker(pickCategory)
	case "p":
		m.openPicker(pickProject)
	case "o":
		m.toggleStatusFilter(statusOpen)
	case "d":
		m.toggleStatusFilter(statusDone)

	case " ":
		if t := m.selected(); t != nil {
			t.Done = !t.Done
			t.CompletedAt = nil
			if t.Done {
				now := time.Now()
				t.CompletedAt = &now
			}
			m.persist()
			m.refresh()
		}
	case "n":
		return m.openForm(nil, fieldDescription)
	case "e", "enter":
		if t := m.selected(); t != nil {
			return m.openForm(t, fieldDescription)
		}
	case "N":
		if t := m.selected(); t != nil {
			return m.openForm(t, fieldNote)
		}
	case "x":
		if m.selected() != nil {
			m.confirmArchive = false
			m.mode = modeConfirm
		}
	case "A":
		if m.store.DoneCount() == 0 {
			m.status = "No completed tasks to archive"
		} else {
			m.confirmArchive = true
			m.mode = modeConfirm
		}
	}
	return nil
}

func (m *model) toggleStatusFilter(f statusFilter) {
	if m.statusFilter == f {
		m.statusFilter = statusAll
	} else {
		m.statusFilter = f
	}
	m.refresh()
}

func (m *model) updateSearch(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		m.search.Blur()
		m.mode = modeList
		return nil
	case "esc":
		m.search.SetValue("")
		m.search.Blur()
		m.mode = modeList
		m.refresh()
		return nil
	case "down":
		m.move(1)
		return nil
	case "up":
		m.move(-1)
		return nil
	}
	before := m.search.Value()
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	if m.search.Value() != before {
		m.refresh()
		m.cursor = 0 // best match first
		m.detailScroll = 0
	}
	return cmd
}

func (m *model) openPicker(kind pickerKind) {
	m.pickKind = kind
	m.pickItems, m.pickCounts = m.distinct(kind)
	m.pickCursor = 0
	m.mode = modePicker
}

func (m *model) pickerSet() map[string]bool {
	if m.pickKind == pickCategory {
		return m.catFilter
	}
	return m.projFilter
}

func (m *model) updatePicker(msg tea.KeyMsg) {
	set := m.pickerSet()
	switch msg.String() {
	case "esc", "enter", "q":
		m.mode = modeList
	case "c":
		if m.pickKind == pickCategory {
			m.mode = modeList
		} else {
			m.openPicker(pickCategory)
		}
	case "p":
		if m.pickKind == pickProject {
			m.mode = modeList
		} else {
			m.openPicker(pickProject)
		}
	case "j", "down":
		if m.pickCursor < len(m.pickItems)-1 {
			m.pickCursor++
		}
	case "k", "up":
		if m.pickCursor > 0 {
			m.pickCursor--
		}
	case " ":
		if m.pickCursor < len(m.pickItems) {
			v := m.pickItems[m.pickCursor]
			if set[v] {
				delete(set, v)
			} else {
				set[v] = true
			}
			m.refresh()
		}
	case "a":
		for v := range set {
			delete(set, v)
		}
		m.refresh()
	}
}

func (m *model) updateConfirm(msg tea.KeyMsg) {
	yes := msg.String() == "y" || msg.String() == "Y"
	if yes && m.confirmArchive {
		selID := 0
		if t := m.selected(); t != nil {
			selID = t.ID
		}
		n, err := m.store.Archive()
		m.visible = nil // indexes are stale
		m.refresh()
		m.selectID(selID)
		if err != nil {
			m.status = "Archive failed: " + err.Error()
		} else {
			m.status = fmt.Sprintf("Archived %d completed task(s)", n)
		}
	} else if yes {
		if t := m.selected(); t != nil {
			m.store.Delete(t.ID)
			m.visible = nil // indexes are stale; cursor position is kept
			m.persist()
			m.refresh()
			if m.status == "" {
				m.status = "Task deleted"
			}
		}
	}
	m.mode = modeList
}

// ---------------------------------------------------------------- form ---

func (m *model) openForm(t *Task, field int) tea.Cmd {
	m.editID = 0
	desc, cat, proj, note := "", "", "", ""
	if t != nil {
		m.editID = t.ID
		desc, cat, proj, note = t.Description, t.Category, t.Project, t.Note
	}
	m.inputs[fieldDescription].SetValue(desc)
	m.inputs[fieldCategory].SetValue(cat)
	m.inputs[fieldProject].SetValue(proj)
	m.note.SetValue(note)
	for i := range m.inputs {
		m.inputs[i].CursorEnd()
	}
	m.mode = modeForm
	m.resize()
	return m.focusField(field)
}

func (m *model) focusField(field int) tea.Cmd {
	m.formField = (field + fieldCount) % fieldCount
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
	m.note.Blur()
	if m.formField == fieldNote {
		return m.note.Focus()
	}
	return m.inputs[m.formField].Focus()
}

func (m *model) updateForm(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		return nil
	case "ctrl+s":
		m.saveForm()
		return nil
	case "tab":
		return m.focusField(m.formField + 1)
	case "shift+tab":
		return m.focusField(m.formField - 1)
	case "enter":
		if m.formField != fieldNote {
			return m.focusField(m.formField + 1)
		}
	}
	return m.updateFormWidget(msg)
}

func (m *model) updateFormWidget(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	if m.formField == fieldNote {
		m.note, cmd = m.note.Update(msg)
	} else {
		m.inputs[m.formField], cmd = m.inputs[m.formField].Update(msg)
	}
	return cmd
}

func (m *model) saveForm() {
	desc := strings.TrimSpace(m.inputs[fieldDescription].Value())
	if desc == "" {
		m.status = "Description is required"
		_ = m.focusField(fieldDescription)
		return
	}
	t := Task{
		Description: desc,
		Category:    strings.TrimSpace(m.inputs[fieldCategory].Value()),
		Project:     strings.TrimSpace(m.inputs[fieldProject].Value()),
		Note:        strings.TrimRight(m.note.Value(), " \n\t"),
	}
	id := m.editID
	if id == 0 {
		id = m.store.Add(t)
	} else if existing := m.store.Find(id); existing != nil {
		existing.Description, existing.Category = t.Description, t.Category
		existing.Project, existing.Note = t.Project, t.Note
	}
	m.persist()
	m.mode = modeList
	m.refresh()
	m.selectID(id)
	m.detailScroll = 0
	if m.status == "" {
		m.status = "Task saved"
	}
}
