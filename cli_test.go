package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestParseAdd(t *testing.T) {
	cases := []struct {
		args           []string
		desc, cat, prj string
	}{
		{[]string{"Buy", "milk", "@Personal", "+Groceries"}, "Buy milk", "Personal", "Groceries"},
		{[]string{"@Work", "Review", "drawings"}, "Review drawings", "Work", ""},
		{[]string{"Review drawings @Work +Harbour"}, "Review drawings", "Work", "Harbour"},
		{[]string{"Review", "drawings", "+Harbour Office"}, "Review drawings", "", "Harbour Office"},
		{[]string{"Email", "a@b.com", "about", "C++"}, "Email a@b.com about C++", "", ""},
		{[]string{"Fix", "+", "@", "thing"}, "Fix + @ thing", "", ""},
		{[]string{"Plain", "task"}, "Plain task", "", ""},
	}
	for _, c := range cases {
		got, err := parseAdd(c.args)
		if err != nil {
			t.Errorf("%q: %v", c.args, err)
			continue
		}
		if got.Description != c.desc || got.Category != c.cat || got.Project != c.prj {
			t.Errorf("%q: got %q @%q +%q, want %q @%q +%q", c.args,
				got.Description, got.Category, got.Project, c.desc, c.cat, c.prj)
		}
	}
	if _, err := parseAdd([]string{"@Work", "+Office"}); err == nil {
		t.Error("tags without a description should be an error")
	}
}

func TestRunAddMatchesExistingCase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store, _ := LoadStore(path, "")
	store.Add(Task{Description: "old", Category: "Work", Project: "Harbour Office"})
	store.Save()

	msg, err := runAdd(store, []string{"New", "one", "@work", "+harbour office"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "Added task 2") {
		t.Errorf("message = %q", msg)
	}
	reloaded, _ := LoadStore(path, "")
	got := reloaded.Tasks[1]
	if got.Category != "Work" || got.Project != "Harbour Office" || got.CreatedAt.IsZero() {
		t.Errorf("added task = %+v", got)
	}
}

// A task added from the command line while the TUI is open must survive the
// TUI's next save and show up in its list.
func TestAddWhileTUIOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	tuiStore, _ := LoadStore(path, "")
	tuiStore.Add(Task{Description: "first"})
	tuiStore.Add(Task{Description: "second"})
	tuiStore.Save()

	m := newModel(tuiStore)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	press(m, "x", "y") // delete "first" in the TUI (saved)

	time.Sleep(10 * time.Millisecond) // make sure the mtime moves
	cliStore, _ := LoadStore(path, "")
	if _, err := runAdd(cliStore, []string{"from", "cli", "@Home"}); err != nil {
		t.Fatal(err)
	}

	// TUI creates a task before noticing the file changed: same ID as the CLI one.
	press(m, "n")
	typeText(m, "from tui")
	press(m, "ctrl+s")

	disk, _ := LoadStore(path, "")
	var descs []string
	ids := map[int]bool{}
	for _, x := range disk.Tasks {
		descs = append(descs, x.Description)
		if ids[x.ID] {
			t.Errorf("duplicate ID %d", x.ID)
		}
		ids[x.ID] = true
	}
	if strings.Join(descs, ",") != "second,from tui,from cli" {
		t.Errorf("tasks on disk = %v", descs)
	}

	// Polling picks up a later CLI add without any save from the TUI.
	time.Sleep(10 * time.Millisecond)
	cliStore, _ = LoadStore(path, "")
	runAdd(cliStore, []string{"another"})
	m.Update(reloadTick{})
	if len(m.store.Tasks) != 4 || !strings.Contains(m.View(), "another") {
		t.Errorf("TUI did not reload: %d tasks", len(m.store.Tasks))
	}
}
