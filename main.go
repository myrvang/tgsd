package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	file := flag.String("file", "", "path to the tasks JSON file (overrides tasks_file in ~/.config/tgsd/tgsd.conf)")
	doneFile := flag.String("done", "", "path to the archive JSON file (overrides done_file in the config)")
	demo := flag.Bool("demo", false, "start with sample tasks; nothing is saved")
	flag.Parse()

	var store *Store
	if *demo {
		store = demoStore()
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "cannot find home directory:", err)
			os.Exit(1)
		}
		cfg, err := loadConfig(home)
		if err != nil {
			fmt.Fprintln(os.Stderr, "cannot load config:", err)
			os.Exit(1)
		}
		path, donePath := cfg.TasksFile, cfg.DoneFile
		if *file != "" {
			path = *file
		}
		if *doneFile != "" {
			donePath = *doneFile
		}
		store, err = LoadStore(path, donePath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "cannot load tasks:", err)
			os.Exit(1)
		}
	}

	if _, err := tea.NewProgram(newModel(store), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
