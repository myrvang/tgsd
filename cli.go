package main

import (
	"errors"
	"fmt"
	"strings"
)

const addUsage = `usage: gsd add <description> [@category] [+project]

  gsd add Buy milk @Personal +Groceries
  gsd add "Fix the gate @Chores +House"            (whole line quoted)
  gsd add Review drawings @Work "+Harbour Office"  (quote a tag with spaces)

@ sets the category and + sets the project. They can appear anywhere.
Existing categories and projects are matched without regard to case.`

// parseAdd turns the words after "gsd add" into a task. A shell argument
// that starts with @ or + is a tag in full, so "+Harbour Office" (quoted)
// keeps its space. Other arguments are split on whitespace and any word
// starting with @ or + inside them is a tag too, so the whole line can be
// passed as one quoted string. A lone "@" or "+" is ordinary text.
func parseAdd(args []string) (Task, error) {
	var t Task
	var words []string

	tag := func(word string) bool {
		if len(word) < 2 {
			return false
		}
		value := strings.TrimSpace(word[1:])
		switch word[0] {
		case '@':
			t.Category = value
		case '+':
			t.Project = value
		default:
			return false
		}
		return value != ""
	}

	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if tag(arg) {
			continue
		}
		for _, w := range strings.Fields(arg) {
			if !tag(w) {
				words = append(words, w)
			}
		}
	}

	t.Description = strings.Join(words, " ")
	if t.Description == "" {
		return t, errors.New("a task needs a description")
	}
	return t, nil
}

// matchExisting returns the spelling already in use for value, ignoring
// case, so "@work" lands in the existing "Work" category.
func matchExisting(value string, existing []string) string {
	for _, e := range existing {
		if strings.EqualFold(e, value) {
			return e
		}
	}
	return value
}

// runAdd adds one task to the store and saves it.
func runAdd(store *Store, args []string) (string, error) {
	t, err := parseAdd(args)
	if err != nil {
		return "", err
	}
	var cats, projs []string
	for _, x := range store.Tasks {
		cats = append(cats, x.Category)
		projs = append(projs, x.Project)
	}
	t.Category = matchExisting(t.Category, cats)
	t.Project = matchExisting(t.Project, projs)

	id := store.Add(t)
	if err := store.Save(); err != nil {
		return "", err
	}

	msg := fmt.Sprintf("Added task %d: %s", id, t.Description)
	if t.Category != "" {
		msg += "  @" + t.Category
	}
	if t.Project != "" {
		msg += "  +" + t.Project
	}
	return msg, nil
}
