package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	colAccent = lipgloss.Color("#FFD479")
	colText   = lipgloss.Color("#C8C8C8")
	colDim    = lipgloss.Color("#6C6C6C")
	colBorder = lipgloss.Color("#8A8A8A")
	colBlack  = lipgloss.Color("#000000")
	colGreen  = lipgloss.Color("#9ECE6A")

	styText     = lipgloss.NewStyle().Foreground(colText)
	styDim      = lipgloss.NewStyle().Foreground(colDim)
	styAccent   = lipgloss.NewStyle().Foreground(colAccent)
	styHeader   = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	stySelected = lipgloss.NewStyle().Foreground(colBlack).Background(colAccent).Bold(true)
	styDone     = lipgloss.NewStyle().Foreground(colGreen)
)

const (
	topH      = 3 // search / filters row
	footerH   = 7 // blank, stats, blank, 4 help lines
	minWidth  = 70
	minHeight = 18
	noneLabel = "(none)"
)

// dims returns the widths of the two columns and the height of the main row.
func (m *model) dims() (leftW, rightW, mainH int) {
	leftW = m.width * 58 / 100
	rightW = m.width - leftW - 1
	mainH = m.height - topH - footerH
	return
}

// listRows is how many task rows fit in the table.
func (m *model) listRows() int {
	_, _, mainH := m.dims()
	rows := mainH - 4 // borders + header + separator
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m *model) ensureVisible() {
	rows := m.listRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	if max := len(m.visible) - rows; m.offset > max {
		m.offset = max
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// resize pushes the current terminal size into the text widgets.
func (m *model) resize() {
	leftW, _, _ := m.dims()
	m.search.Width = clamp(leftW-7, 5, 500)
	mw, noteH := m.modalDims()
	for i := range m.inputs {
		m.inputs[i].Width = clamp(mw-8, 5, 500)
	}
	m.note.SetWidth(clamp(mw-4, 10, 500))
	m.note.SetHeight(noteH)
	m.ensureVisible()
}

// modalFixedH is the task modal's height without the note editor: borders,
// padding, the form's fields and the hint line.
const modalFixedH = 17

// modalDims returns the task modal's width and the note editor's height.
func (m *model) modalDims() (w, noteH int) {
	w = clamp(m.width*55/100, 60, 84)
	if w > m.width-4 {
		w = m.width - 4
	}
	noteH = clamp(m.height-2-modalFixedH, 1, 8)
	return w, noteH
}

// ---------------------------------------------------------------- view ---

func (m *model) View() string {
	if m.width == 0 {
		return ""
	}
	if m.width < minWidth || m.height < minHeight {
		return fmt.Sprintf("Terminal too small (%dx%d). Need at least %dx%d.", m.width, m.height, minWidth, minHeight)
	}
	leftW, rightW, mainH := m.dims()

	searchCol, listCol, rightCol := colBorder, colBorder, colBorder
	switch {
	case m.mode == modeSearch:
		searchCol = colAccent
	case m.mode == modeForm:
		// the modal has the focus; leave both panes unhighlighted
	case m.mode == modePicker || m.focus == paneDetails:
		rightCol = colAccent
	default:
		listCol = colAccent
	}

	top := lipgloss.JoinHorizontal(lipgloss.Top,
		box("", " "+m.search.View(), leftW, topH, searchCol),
		" ",
		box("Filters", " "+m.filterSummary(), rightW, topH, colBorder),
	)

	var rightTitle, rightBody string
	switch m.mode {
	case modePicker:
		rightTitle = "Filter: categories"
		if m.pickKind == pickProject {
			rightTitle = "Filter: projects"
		}
		rightBody = m.pickerView()
	default:
		rightTitle = "Details"
		rightBody = m.detailsView(rightW-2, mainH-2)
	}

	main := lipgloss.JoinHorizontal(lipgloss.Top,
		box("", m.tableView(leftW-2, mainH-2), leftW, mainH, listCol),
		" ",
		box(rightTitle, rightBody, rightW, mainH, rightCol),
	)

	footer := []string{"", " " + m.statsLine(), ""}
	footer = append(footer, m.helpLines()...)
	for i, l := range footer {
		footer[i] = ansi.Truncate(l, m.width, "…")
	}

	screen := top + "\n" + main + "\n" + strings.Join(footer, "\n")
	if m.mode == modeForm {
		screen = m.withModal(screen)
	}
	return screen
}

// withModal dims the screen and draws the task form centered on top of it.
func (m *model) withModal(screen string) string {
	title := " Edit task "
	if m.editID == 0 {
		title = " New task "
	}
	w, noteH := m.modalDims()
	h := modalFixedH + noteH
	hint := styAccent.Render("tab") + styDim.Render(" next field  ") +
		styAccent.Render("ctrl+s") + styDim.Render(" save  ") +
		styAccent.Render("esc") + styDim.Render(" cancel")
	body := "\n" + m.formView() + "\n\n " + hint
	modal := box(title, body, w, h, colAccent)

	bg := strings.Split(screen, "\n")
	for i, l := range bg {
		bg[i] = styDim.Render(ansi.Strip(l))
	}
	x := (m.width - w) / 2
	y := (m.height - h) / 2
	if y < 0 {
		y = 0
	}
	for i, line := range strings.Split(modal, "\n") {
		row := y + i
		if row >= len(bg) {
			break
		}
		base := bg[row]
		left := ansi.Truncate(base, x, "")
		left += strings.Repeat(" ", x-lipgloss.Width(left))
		right := ansi.TruncateLeft(base, x+w, "")
		bg[row] = left + line + right
	}
	return strings.Join(bg, "\n")
}

func (m *model) filterSummary() string {
	var parts []string
	if len(m.catFilter) > 0 {
		parts = append(parts, styAccent.Render("Category: ")+styText.Render(joinSet(m.catFilter)))
	}
	if len(m.projFilter) > 0 {
		parts = append(parts, styAccent.Render("Project: ")+styText.Render(joinSet(m.projFilter)))
	}
	switch m.statusFilter {
	case statusOpen:
		parts = append(parts, styAccent.Render("Status: ")+styText.Render("open"))
	case statusDone:
		parts = append(parts, styAccent.Render("Status: ")+styText.Render("done"))
	}
	if len(parts) == 0 {
		return styText.Render("None")
	}
	return strings.Join(parts, styDim.Render(" │ "))
}

func (m *model) tableView(w, h int) string {
	catW := clamp(w/6, 8, 18)
	projW := clamp(w/5, 8, 22)
	descW := w - 2 - 3 - 6 - catW - projW

	label := func(col sortColumn, name string) string {
		if m.sortCol != col || strings.TrimSpace(m.search.Value()) != "" {
			return name
		}
		if m.sortDesc {
			return "↓ " + name
		}
		return "↑ " + name
	}
	row := func(check, desc, cat, proj string) string {
		return " " + fit(check, 3) + "  " + fit(desc, descW) + "  " + fit(cat, catW) + "  " + fit(proj, projW) + " "
	}

	lines := []string{
		styHeader.Render(row(strings.TrimSpace(label(sortStatus, "")), label(sortDescription, "Description"), label(sortCategory, "Category"), label(sortProject, "Project"))),
		styDim.Render(" " + strings.Repeat("─", w-2)),
	}

	if len(m.visible) == 0 {
		msg := "No tasks match the current search and filters."
		if len(m.store.Tasks) == 0 {
			msg = "No tasks yet — press n to create one."
		}
		return strings.Join(append(lines, "", styDim.Render(" "+msg)), "\n")
	}

	end := m.offset + h - 2
	if end > len(m.visible) {
		end = len(m.visible)
	}
	for i := m.offset; i < end; i++ {
		t := m.store.Tasks[m.visible[i]]
		check := "[ ]"
		if t.Done {
			check = "[✓]"
		}
		text := row(check, t.Description, t.Category, t.Project)
		switch {
		case i == m.cursor:
			text = stySelected.Render(text)
		case t.Done:
			text = styDim.Render(text)
		default:
			text = styText.Render(text)
		}
		lines = append(lines, text)
	}
	return strings.Join(lines, "\n")
}

func (m *model) detailsView(w, h int) string {
	t := m.selected()
	if t == nil {
		return styDim.Render(" No task selected.")
	}
	wrapW := w - 2
	or := func(s string) string {
		if s == "" {
			return "—"
		}
		return s
	}

	check := "[ ] "
	status := styText.Render("○ Open")
	if t.Done {
		check = "[✓] "
		status = styDone.Render("✓ Done")
	}

	var lines []string
	lines = append(lines, wrap(check+t.Description, wrapW, styHeader)...)
	lines = append(lines, "")
	field := func(name, value string) {
		lines = append(lines, styText.Render(name+": ")+value)
	}
	field("Status", status)
	field("Category", styText.Render(or(t.Category)))
	field("Project", styText.Render(or(t.Project)))
	field("Created", styText.Render(t.CreatedAt.Format("2006-01-02 15:04")))
	if t.CompletedAt != nil {
		field("Completed", styText.Render(t.CompletedAt.Format("2006-01-02 15:04")))
	}
	lines = append(lines, "", styAccent.Render("Note:"))
	if strings.TrimSpace(t.Note) == "" {
		lines = append(lines, styDim.Render("No note — press N to add one."))
	} else {
		lines = append(lines, wrap(t.Note, wrapW, styText)...)
	}

	if max := len(lines) - h; m.detailScroll > max {
		m.detailScroll = max
	}
	if m.detailScroll < 0 {
		m.detailScroll = 0
	}
	lines = lines[m.detailScroll:]
	if len(lines) > h {
		lines = lines[:h]
		lines[h-1] = styDim.Render("↓ more — tab to focus, j/k to scroll")
	}
	for i := range lines {
		lines[i] = " " + lines[i]
	}
	return strings.Join(lines, "\n")
}

func (m *model) pickerView() string {
	set := m.pickerSet()
	lines := []string{styDim.Render(" Showing tasks matching any checked item."), ""}
	for i, v := range m.pickItems {
		name := v
		if name == "" {
			name = noneLabel
		}
		check := "[ ]"
		if set[v] {
			check = "[✓]"
		}
		text := fmt.Sprintf(" %s %s (%d) ", check, name, m.pickCounts[v])
		if i == m.pickCursor {
			text = stySelected.Render(text)
		} else {
			text = styText.Render(text)
		}
		lines = append(lines, " "+text)
	}
	return strings.Join(lines, "\n")
}

func (m *model) formView() string {
	label := func(field int, name string) string {
		if m.formField == field {
			return " " + styHeader.Render(name)
		}
		return " " + styText.Render(name)
	}
	existing := func(kind pickerKind) string {
		values, _ := m.distinct(kind)
		var named []string
		for _, v := range values {
			if v != "" {
				named = append(named, v)
			}
		}
		if len(named) == 0 {
			return ""
		}
		return styDim.Render("   existing: " + strings.Join(named, ", "))
	}

	lines := []string{
		label(fieldDescription, "Description"),
		" " + m.inputs[fieldDescription].View(),
		"",
		label(fieldCategory, "Category"),
		" " + m.inputs[fieldCategory].View(),
		existing(pickCategory),
		"",
		label(fieldProject, "Project"),
		" " + m.inputs[fieldProject].View(),
		existing(pickProject),
		"",
		label(fieldNote, "Note"),
	}
	for _, l := range strings.Split(m.note.View(), "\n") {
		lines = append(lines, " "+l)
	}
	return strings.Join(lines, "\n")
}

func (m *model) statsLine() string {
	if m.mode == modeConfirm && m.confirmArchive {
		target := m.store.donePath
		if target == "" {
			target = "the archive (demo mode, not saved)"
		}
		return styAccent.Render(fmt.Sprintf("Archive %d completed task(s) to %s? Press y to confirm, any other key to cancel.", m.store.DoneCount(), target))
	}
	if m.mode == modeConfirm {
		if t := m.selected(); t != nil {
			return styAccent.Render(fmt.Sprintf("Delete %q? Press y to confirm, any other key to cancel.", t.Description))
		}
	}
	if m.status != "" {
		return styAccent.Render(m.status)
	}
	done := 0
	for _, t := range m.store.Tasks {
		if t.Done {
			done++
		}
	}
	cats, _ := m.distinct(pickCategory)
	projs, _ := m.distinct(pickProject)
	named := func(values []string) int {
		n := 0
		for _, v := range values {
			if v != "" {
				n++
			}
		}
		return n
	}
	num := func(n int) string { return styAccent.Render(fmt.Sprint(n)) }
	sep := styText.Render(" | ")
	return num(len(m.store.Tasks)) + styText.Render(" Tasks") + sep +
		num(len(m.store.Tasks)-done) + styText.Render(" open") + sep +
		num(done) + styText.Render(" done") + sep +
		num(named(cats)) + styText.Render(" Categories") + sep +
		num(named(projs)) + styText.Render(" Projects") + sep +
		num(len(m.visible)) + styText.Render(" shown")
}

func (m *model) helpLines() []string {
	switch m.mode {
	case modeForm:
		return []string{
			helpLine("Form", "tab", "next field", "shift+tab", "previous field", "enter", "next field (new line in note)"),
			helpLine("Commands", "ctrl+s", "save", "esc", "cancel"),
			"", "",
		}
	case modePicker:
		return []string{
			helpLine("Navigation", "j/↓", "cursor down", "k/↑", "cursor up"),
			helpLine("Filter", "space", "toggle", "a", "clear", "c", "categories", "p", "projects", "enter/esc", "close"),
			"", "",
		}
	}
	return []string{
		helpLine("General", "q", "quit", "tab", "switch focus", "/", "search", "esc", "clear search", "enter", "exit search", "s/S", "sort column/direction"),
		helpLine("Navigation", "j/↓", "cursor down", "k/↑", "cursor up", "PageUp", "prev page", "PageDown", "next page", "g", "go to top", "G", "go to bottom"),
		helpLine("Filter", "a", "all (no filter)", "c", "categories", "p", "projects", "o", "open only", "d", "done only"),
		helpLine("Commands", "space", "toggle done", "n", "new", "e", "edit", "N", "edit note", "x", "delete", "A", "archive done"),
	}
}

// ------------------------------------------------------------- helpers ---

// helpLine renders "Label     : key: text key: text ..." from key/text pairs.
func helpLine(label string, pairs ...string) string {
	var b strings.Builder
	b.WriteString(" " + styText.Render(fmt.Sprintf("%-10s: ", label)))
	for i := 0; i+1 < len(pairs); i += 2 {
		b.WriteString(styAccent.Render(pairs[i]) + styText.Render(": "+pairs[i+1]+" "))
	}
	return b.String()
}

// box draws a rounded border of exactly w x h cells around body, with an
// optional title set into the top edge. Body lines are truncated and padded.
func box(title, body string, w, h int, color lipgloss.Color) string {
	innerW, innerH := w-2, h-2
	border := lipgloss.NewStyle().Foreground(color)

	title = ansi.Truncate(title, innerW, "")
	top := border.Render("╭") + styText.Render(title) +
		border.Render(strings.Repeat("─", innerW-lipgloss.Width(title))+"╮")

	lines := strings.Split(body, "\n")
	out := make([]string, 0, h)
	out = append(out, top)
	for i := 0; i < innerH; i++ {
		line := ""
		if i < len(lines) {
			line = ansi.Truncate(lines[i], innerW, "…")
		}
		pad := innerW - lipgloss.Width(line)
		out = append(out, border.Render("│")+line+strings.Repeat(" ", pad)+border.Render("│"))
	}
	out = append(out, border.Render("╰"+strings.Repeat("─", innerW)+"╯"))
	return strings.Join(out, "\n")
}

// fit truncates or pads a single-line string to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = strings.Join(strings.Fields(s), " ")
	s = ansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", w-lipgloss.Width(s))
}

// wrap word-wraps s to w cells and styles each resulting line.
func wrap(s string, w int, style lipgloss.Style) []string {
	if w < 1 {
		w = 1
	}
	lines := strings.Split(ansi.Wrap(s, w, ""), "\n")
	for i := range lines {
		lines[i] = style.Render(lines[i])
	}
	return lines
}

func joinSet(set map[string]bool) string {
	values := make([]string, 0, len(set))
	for v := range set {
		if v == "" {
			v = noneLabel
		}
		values = append(values, v)
	}
	sort.Strings(values)
	return strings.Join(values, ", ")
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
