package model

import (
	"bufio"
	"strings"
)

const SelectAll = "*"

// Selector holds the information necessary to build a set of elements
// based on name and tag combinations.
type Selector []selectCriterion

// selectCriterions are intersected to form the results of a selector.
type selectCriterion struct {
	name string

	// modifiers
	tag     bool
	negated bool
}

// ParseSelector reads in a set of selection criteria defined as a string.
// This function only parses; it does not evaluate.
// Returns nil on an empty selection string.
func ParseSelector(s string) Selector {
	var criteria []selectCriterion
	// read the white-space delimited criteria
	scanner := bufio.NewScanner(strings.NewReader(s))
	scanner.Split(bufio.ScanWords) // bufio.ScanWords cannot error
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
