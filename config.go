package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config is read from ~/.config/tgsd/tgsd.conf, a plain "key = value" file.
// Blank lines and lines starting with # are ignored.
type Config struct {
	// TasksFile is where tasks are stored. "~" expands to the home directory;
	// a relative path is taken relative to the home directory.
	TasksFile string
	// DoneFile is where the archive command moves completed tasks.
	DoneFile string
}

const defaultConfig = `# terminalGSD configuration

# Where tasks are stored. "~" is your home directory.
tasks_file = ~/.terminalgsd/tasks.json

# Where the archive command (A) moves completed tasks.
done_file = ~/.terminalgsd/done.json
`

func configPath(home string) string {
	return filepath.Join(home, ".config", "tgsd", "tgsd.conf")
}

func defaultTasksFile(home string) string {
	return filepath.Join(home, ".terminalgsd", "tasks.json")
}

func defaultDoneFile(home string) string {
	return filepath.Join(home, ".terminalgsd", "done.json")
}

// loadConfig reads the config file, creating it with defaults when it does
// not exist. The returned TasksFile is absolute.
func loadConfig(home string) (Config, error) {
	path := configPath(home)
	cfg := Config{}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return cfg, err
		}
		if err := os.WriteFile(path, []byte(defaultConfig), 0o644); err != nil {
			return cfg, err
		}
		data = []byte(defaultConfig)
	} else if err != nil {
		return cfg, err
	}

	for n, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return cfg, fmt.Errorf("%s:%d: expected \"key = value\"", path, n+1)
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch key {
		case "tasks_file":
			cfg.TasksFile = value
		case "done_file":
			cfg.DoneFile = value
		default:
			return cfg, fmt.Errorf("%s:%d: unknown setting %q", path, n+1, key)
		}
	}

	cfg.TasksFile = expandPath(cfg.TasksFile, home)
	if cfg.TasksFile == "" {
		cfg.TasksFile = defaultTasksFile(home)
	}
	cfg.DoneFile = expandPath(cfg.DoneFile, home)
	if cfg.DoneFile == "" {
		cfg.DoneFile = defaultDoneFile(home)
	}
	return cfg, nil
}

func expandPath(p, home string) string {
	p = strings.TrimSpace(p)
	switch {
	case p == "":
		return ""
	case p == "~":
		return home
	case strings.HasPrefix(p, "~/"):
		return filepath.Join(home, p[2:])
	case filepath.IsAbs(p):
		return filepath.Clean(p)
	}
	return filepath.Join(home, p)
}
