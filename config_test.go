package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	home := t.TempDir()
	cfgPath := filepath.Join(home, ".config", "tgsd", "tgsd.conf")

	// Missing file: created with the default path.
	cfg, err := loadConfig(home)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".terminalgsd", "tasks.json"); cfg.TasksFile != want {
		t.Errorf("default TasksFile = %q, want %q", cfg.TasksFile, want)
	}
	if want := filepath.Join(home, ".terminalgsd", "done.json"); cfg.DoneFile != want {
		t.Errorf("default DoneFile = %q, want %q", cfg.DoneFile, want)
	}
	os.WriteFile(cfgPath, []byte("done_file = ~/archive/done.json\n"), 0o644)
	if cfg, _ := loadConfig(home); cfg.DoneFile != filepath.Join(home, "archive", "done.json") {
		t.Errorf("DoneFile = %q", cfg.DoneFile)
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("config file was not created: %v", err)
	}

	cases := map[string]string{
		"tasks_file = ~/Documents/tasks.json":           filepath.Join(home, "Documents", "tasks.json"),
		"# comment\n\ntasks_file=/tmp/gsd/tasks.json\n": "/tmp/gsd/tasks.json",
		"tasks_file = notes/tasks.json":                 filepath.Join(home, "notes", "tasks.json"),
		"tasks_file = \"~/My Tasks/tasks.json\"":        filepath.Join(home, "My Tasks", "tasks.json"),
		"# nothing set\n":                               filepath.Join(home, ".terminalgsd", "tasks.json"),
	}
	for content, want := range cases {
		if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := loadConfig(home)
		if err != nil {
			t.Fatalf("%q: %v", content, err)
		}
		if cfg.TasksFile != want {
			t.Errorf("%q: TasksFile = %q, want %q", content, cfg.TasksFile, want)
		}
	}

	for _, bad := range []string{"just some text", "task_file = ~/x.json"} {
		os.WriteFile(cfgPath, []byte(bad), 0o644)
		if _, err := loadConfig(home); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}
