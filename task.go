package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Task is a single to-do item. Note is the free-text shown in the details pane.
type Task struct {
	ID          int        `json:"id"`
	Description string     `json:"description"`
	Category    string     `json:"category"`
	Project     string     `json:"project"`
	Note        string     `json:"note"`
	Done        bool       `json:"done"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// Store holds all tasks and persists them as JSON. An empty path means
// in-memory only (demo mode).
type Store struct {
	path     string
	donePath string // archive file for completed tasks
	NextID   int    `json:"next_id"`
	Tasks    []Task `json:"tasks"`
}

func LoadStore(path, donePath string) (*Store, error) {
	s := &Store{path: path, donePath: donePath, NextID: 1}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	for _, t := range s.Tasks {
		if t.ID >= s.NextID {
			s.NextID = t.ID + 1
		}
	}
	return s, nil
}

// Save writes atomically: temp file in the same directory, then rename.
func (s *Store) Save() error {
	if s.path == "" {
		return nil
	}
	return writeJSON(s.path, s)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// archiveFile is the on-disk shape of the done file.
type archiveFile struct {
	Tasks []Task `json:"tasks"`
}

func (s *Store) DoneCount() int {
	n := 0
	for _, t := range s.Tasks {
		if t.Done {
			n++
		}
	}
	return n
}

// Archive moves all completed tasks to the done file, appending to what is
// already there. The done file is written first, so a failure can leave a
// task in both files but never in neither.
func (s *Store) Archive() (int, error) {
	var done, open []Task
	for _, t := range s.Tasks {
		if t.Done {
			done = append(done, t)
		} else {
			open = append(open, t)
		}
	}
	if len(done) == 0 {
		return 0, nil
	}
	if s.donePath != "" {
		var archive archiveFile
		data, err := os.ReadFile(s.donePath)
		if err == nil {
			if err := json.Unmarshal(data, &archive); err != nil {
				return 0, fmt.Errorf("%s: %w", s.donePath, err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}
		archive.Tasks = append(archive.Tasks, done...)
		if err := writeJSON(s.donePath, archive); err != nil {
			return 0, err
		}
	}
	s.Tasks = open
	return len(done), s.Save()
}

func (s *Store) Add(t Task) int {
	t.ID = s.NextID
	s.NextID++
	t.CreatedAt = time.Now()
	s.Tasks = append(s.Tasks, t)
	return t.ID
}

func (s *Store) Find(id int) *Task {
	for i := range s.Tasks {
		if s.Tasks[i].ID == id {
			return &s.Tasks[i]
		}
	}
	return nil
}

func (s *Store) Delete(id int) {
	for i := range s.Tasks {
		if s.Tasks[i].ID == id {
			s.Tasks = append(s.Tasks[:i], s.Tasks[i+1:]...)
			return
		}
	}
}

func demoStore() *Store {
	s := &Store{NextID: 1}
	samples := []Task{
		{Description: "Write Q4 project status report", Category: "Work", Project: "Harbour Office", Note: "Summarise progress per discipline.\n\nInclude:\n- updated drawing list\n- open issues from the last coordination meeting\n- revised delivery dates"},
		{Description: "Review section drawings A-301 to A-305", Category: "Work", Project: "Harbour Office", Note: "Check levels against the structural model and confirm ceiling heights in the lobby."},
		{Description: "Send neighbour notification", Category: "Work", Project: "Cabin Extension", Note: "Use the standard form. Attach site plan and elevations. Keep the receipt for the application."},
		{Description: "Prepare dispensation request", Category: "Work", Project: "Cabin Extension", Note: "Distance to boundary is 3.2 m; argue from the existing building line."},
		{Description: "Book dentist appointment", Category: "Personal", Project: "", Note: ""},
		{Description: "Renew car insurance", Category: "Personal", Project: "Admin", Note: "Compare at least three offers before renewing. Current policy expires at the end of the month."},
		{Description: "Buy winter tyres", Category: "Personal", Project: "Admin", Note: "Size 225/55 R17."},
		{Description: "Plan weekend ski trip", Category: "Personal", Project: "Trips", Note: "Check avalanche forecast on Thursday evening. Bring the spare headlamp."},
		{Description: "Read chapter 4 of the Go book", Category: "Learning", Project: "Go", Note: "Interfaces and embedding. Do the exercises at the end."},
		{Description: "Build a TUI with Bubble Tea", Category: "Learning", Project: "Go", Note: "Model, Update, View. Start with the list, then add the details pane."},
		{Description: "Back up laptop", Category: "Chores", Project: "", Note: "Full disk backup to the external drive."},
		{Description: "Clean the garage", Category: "Chores", Project: "House", Note: ""},
	}
	now := time.Now()
	for i, t := range samples {
		id := s.Add(t)
		task := s.Find(id)
		task.CreatedAt = now.Add(-time.Duration(len(samples)-i) * 26 * time.Hour)
		if i == 1 || i == 6 || i == 10 {
			done := task.CreatedAt.Add(5 * time.Hour)
			task.Done = true
			task.CompletedAt = &done
		}
	}
	return s
}
