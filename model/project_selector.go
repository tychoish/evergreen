package model

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/evergreen-ci/evergreen/util"
	"strings"
)

// Universal Selector Logic

const SelectAll = "*"

// Selector holds the information necessary to build a set of elements
// based on name and tag combinations.
type Selector []selectCriterion

// String returns a readable representation of the Selector
func (s Selector) String() string {
	buf := bytes.Buffer{}
	for i, sc := range s {
		if i > 0 {
			buf.WriteRune(' ')
		}
		buf.WriteString(sc.String())
	}
	return buf.String()
}

// selectCriterions are intersected to form the results of a selector.
type selectCriterion struct {
	name string

	// modifiers
	tag     bool
	negated bool
}

// String returns a readable representation of the criterion
func (sc selectCriterion) String() string {
	buf := bytes.Buffer{}
	if sc.negated {
		buf.WriteRune('!')
	}
	if sc.tag {
		buf.WriteRune('.')
	}
	buf.WriteString(sc.name)
	return buf.String()
}

// ParseSelector reads in a set of selection criteria defined as a string.
// This function only parses; it does not evaluate.
// Returns nil on an empty selection string.
func ParseSelector(s string) Selector {
	var criteria []selectCriterion
	// read the white-space delimited criteria
	scanner := bufio.NewScanner(strings.NewReader(s)) //TODO use strings.Fields
	scanner.Split(bufio.ScanWords)                    // bufio.ScanWords cannot error
	for scanner.Scan() {
		criteria = append(criteria, stringToCriterion(scanner.Text()))
	}
	return criteria
}

// stringToCriterion parses out a single criterion.
// This helper assumes that s != "".
func stringToCriterion(s string) selectCriterion {
	sc := selectCriterion{}
	if s[0] == '!' { // negation
		sc.negated = true
		s = s[1:]
	}
	if s[0] == '.' { // tags
		sc.tag = true
		s = s[1:]
	}
	sc.name = s
	return sc
}

// TODO String() method

// Task Selector Logic

type taskSelectorEvaluator struct {
	tasks  []ProjectTask
	byName map[string]*ProjectTask
	byTag  map[string][]*ProjectTask
}

func NewTaskSelectorEvaluator(tasks []ProjectTask) *taskSelectorEvaluator {
	// cache everything
	byName := map[string]*ProjectTask{}
	byTag := map[string][]*ProjectTask{}
	for i, t := range tasks {
		byName[t.Name] = &tasks[i]
		for _, tag := range t.Tags {
			byTag[tag] = append(byTag[tag], &tasks[i])
		}
	}
	return &taskSelectorEvaluator{
		tasks:  tasks,
		byName: byName,
		byTag:  byTag,
	}
}

func (tse *taskSelectorEvaluator) evalSelector(s Selector) ([]string, error) {
	// keep a slice of results per criterion
	results := [][]string{}
	if len(s) == 0 {
		return nil, fmt.Errorf("cannot evaluate selector with no criteria")
	}
	for _, sc := range s {
		taskNames, err := tse.evalCriterion(sc)
		if err != nil {
			return nil, fmt.Errorf("error evaluating '%v' selector: %v", s, err)
		}
		results = append(results, taskNames)
	}
	// intersect all evaluated criteria for the final selection
	final := results[0]
	for _, result := range results[1:] {
		final = util.StringSliceIntersection(final, result)
	}
	return final, nil
}

func (tse *taskSelectorEvaluator) evalCriterion(sc selectCriterion) ([]string, error) {
	switch {
	case sc.name == "":
		return nil, fmt.Errorf("cannot evaluate an empty criterion '%v'", sc)

	case sc.name == SelectAll: // special "All Tasks" case
		if sc.tag {
			return nil, fmt.Errorf("cannot use '.' with special name '*'")
		}
		if sc.negated {
			return nil, fmt.Errorf("cannot use '!' with special name '*'")
		}
		names := []string{}
		for _, task := range tse.tasks {
			names = append(names, task.Name)
		}
		return names, nil

	case !sc.tag && !sc.negated: // just a regular name
		task := tse.byName[sc.name]
		if task == nil {
			return nil, fmt.Errorf("no task named '%v'", sc.name)
		}
		return []string{task.Name}, nil

	case sc.tag && !sc.negated: // expand a tag
		tasks := tse.byTag[sc.name]
		if len(tasks) == 0 {
			return nil, fmt.Errorf("no tasks have the tag '%v'", sc.name)
		}
		names := []string{}
		for _, task := range tasks {
			names = append(names, task.Name)
		}
		return names, nil

	case !sc.tag && sc.negated: // everything *but* a specific task
		if tse.byName[sc.name] == nil {
			// we want to treat this as an error for better usability
			return nil, fmt.Errorf("no task named '%v'", sc.name)
		}
		names := []string{}
		for _, task := range tse.tasks {
			if task.Name != sc.name {
				names = append(names, task.Name)
			}
		}
		return names, nil

	case sc.tag && sc.negated: // everything *but* a tag
		tasks := tse.byTag[sc.name]
		if len(tasks) == 0 {
			// we want to treat this as an error for better usability
			return nil, fmt.Errorf("no tasks have the tag '%v'", sc.name)
		}
		// compare tasks by address to avoid the ones with a negated tag
		illegalTasks := map[*ProjectTask]bool{}
		for _, taskPtr := range tasks {
			illegalTasks[taskPtr] = true
		}
		names := []string{}
		for _, taskPtr := range tse.byName {
			if !illegalTasks[taskPtr] {
				names = append(names, taskPtr.Name)
			}
		}
		return names, nil

	}
	// We have returns for all possible boolean combination in the switch,
	// but the gc compiler doesn't realize that, so we need a panic.
	panic("this should not be reachable")
}
