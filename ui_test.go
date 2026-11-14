package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func press(m *model, keys ...string) {
	for _, k := range keys {
		m.Update(key(k))
	}
}

func typeText(m *model, s string) {
	for _, r := range s {
		press(m, string(r))
	}
}

func newTestModel(t *testing.T, w, h int) *model {
	t.Helper()
	m := newModel(demoStore())
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

// checkFrame asserts the view fills the terminal exactly and never overflows.
func checkFrame(t *testing.T, m *model, w, h int) string {
	t.Helper()
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != h {
		t.Errorf("view has %d lines, want %d", len(lines), h)
	}
	for i, l := range lines {
		if lw := lipgloss.Width(l); lw > w {
			t.Errorf("line %d is %d cells wide, max %d: %q", i, lw, w, l)
		}
	}
	return view
}

func TestLayoutSizes(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}, {70, 18}, {200, 60}} {
		m := newTestModel(t, size[0], size[1])
		view := checkFrame(t, m, size[0], size[1])
		if size[0] == 120 {
			t.Log("\n" + view)
		}
		press(m, "c")
		checkFrame(t, m, size[0], size[1])
		press(m, "esc", "n")
		checkFrame(t, m, size[0], size[1])
	}
}

func TestFuzzySearch(t *testing.T) {
	m := newTestModel(t, 120, 40)
	press(m, "/")
	typeText(m, "tui")
	press(m, "enter")
	if len(m.visible) == 0 || m.selected().Description != "Build a TUI with Bubble Tea" {
		t.Fatalf("fuzzy search top hit = %+v", m.selected())
	}
	if len(m.visible) >= len(m.store.Tasks) {
		t.Errorf("search did not narrow the list: %d shown", len(m.visible))
	}
	press(m, "esc")
	if len(m.visible) != len(m.store.Tasks) {
		t.Errorf("esc did not clear search: %d shown", len(m.visible))
	}
}

func TestCategoryAndProjectFilter(t *testing.T) {
	m := newTestModel(t, 120, 40)

	press(m, "c") // items sorted: Chores, Learning, Personal, Work
	press(m, "j", " ", "enter")
	for _, i := range m.visible {
		if c := m.store.Tasks[i].Category; c != "Learning" {
			t.Errorf("category filter let through %q", c)
		}
	}
	if len(m.visible) != 2 {
		t.Errorf("want 2 Learning tasks, got %d", len(m.visible))
	}
	if !strings.Contains(m.View(), "Category: Learning") {
		t.Error("filters box does not show the active category")
	}

	press(m, "a")
	if len(m.visible) != len(m.store.Tasks) {
		t.Errorf("a did not clear filters: %d shown", len(m.visible))
	}

	press(m, "p") // (none), Admin, Cabin Extension, ...
	press(m, "j", " ", "j", " ", "enter")
	if len(m.visible) != 4 {
		t.Errorf("want 4 tasks in Admin + Cabin Extension, got %d", len(m.visible))
	}

	press(m, "o")
	for _, i := range m.visible {
		if m.store.Tasks[i].Done {
			t.Error("open-only filter let through a done task")
		}
	}
}

func TestEditingPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "tasks.json")
	store, err := LoadStore(path, "")
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(store)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	press(m, "n")
	typeText(m, "Call the plumber")
	press(m, "tab")
	typeText(m, "Chores")
	press(m, "tab")
	typeText(m, "House")
	press(m, "tab")
	typeText(m, "Leak under the sink")
	press(m, "ctrl+s")

	if m.mode != modeList || len(store.Tasks) != 1 {
		t.Fatalf("task not created: mode=%d tasks=%d", m.mode, len(store.Tasks))
	}
	press(m, " ") // mark done
	if !strings.Contains(m.View(), "Leak under the sink") {
		t.Error("details pane does not show the note")
	}

	reloaded, err := LoadStore(path, "")
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.Tasks[0]
	if got.Description != "Call the plumber" || got.Category != "Chores" || got.Project != "House" ||
		got.Note != "Leak under the sink" || !got.Done {
		t.Errorf("reloaded task = %+v", got)
	}

	press(m, "x", "y")
	if reloaded, _ = LoadStore(path, ""); len(reloaded.Tasks) != 0 {
		t.Errorf("delete not persisted: %d tasks", len(reloaded.Tasks))
	}
}

func TestArchive(t *testing.T) {
	dir := t.TempDir()
	path, donePath := filepath.Join(dir, "tasks.json"), filepath.Join(dir, "archive", "done.json")
	store, err := LoadStore(path, donePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"one", "two", "three", "four"} {
		store.Add(Task{Description: d})
	}
	m := newModel(store)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	press(m, "A") // nothing done yet
	if m.mode != modeList || !strings.Contains(m.View(), "No completed tasks") {
		t.Error("archive with nothing done should only show a message")
	}

	press(m, " ", "j", "j", " ") // complete "one" and "three"
	press(m, "A", "n")           // cancel
	if len(store.Tasks) != 4 {
		t.Fatal("cancelled archive removed tasks")
	}
	press(m, "A")
	checkFrame(t, m, 120, 40)
	press(m, "y")

	readDone := func() []Task {
		var a archiveFile
		data, err := os.ReadFile(donePath)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &a); err != nil {
			t.Fatal(err)
		}
		return a.Tasks
	}
	if got := readDone(); len(got) != 2 || got[0].Description != "one" || got[1].Description != "three" || !got[0].Done {
		t.Errorf("done file = %+v", got)
	}
	reloaded, _ := LoadStore(path, donePath)
	if len(reloaded.Tasks) != 2 || reloaded.Tasks[0].Description != "two" || reloaded.Tasks[1].Description != "four" {
		t.Errorf("tasks file after archive = %+v", reloaded.Tasks)
	}
	if m.selected() == nil || m.selected().Description != "four" {
		t.Errorf("selection after archive = %+v", m.selected())
	}

	// A second archive appends rather than overwrites.
	press(m, " ", "A", "y")
	if got := readDone(); len(got) != 3 || got[2].Description != "four" {
		t.Errorf("done file after second archive = %+v", got)
	}
	if len(store.Tasks) != 1 {
		t.Errorf("%d tasks left, want 1", len(store.Tasks))
	}
}
