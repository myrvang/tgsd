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
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage:\n  gsd [flags]                 open the task manager\n  gsd [flags] add <task...>   add a task without opening it (@category +project)\n\nflags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	adding := len(args) > 0 && args[0] == "add"
	if len(args) > 0 && !adding {
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		flag.Usage()
		os.Exit(2)
	}
	if adding && (len(args) == 1 || args[1] == "-h" || args[1] == "--help") {
		fmt.Fprintln(os.Stderr, addUsage)
		os.Exit(2)
	}
	if adding && *demo {
		fmt.Fprintln(os.Stderr, "add cannot be used with -demo")
		os.Exit(2)
	}

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

	if adding {
		msg, err := runAdd(store, args[1:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "gsd add:", err)
			os.Exit(1)
		}
		fmt.Println(msg)
		return
	}

	if _, err := tea.NewProgram(newModel(store), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
