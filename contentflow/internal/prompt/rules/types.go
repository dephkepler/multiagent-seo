// Package rules defines composable writing rules for LLM prompts.
//
// A Rule is an atomic instruction (e.g. "H1 must be 50-70 characters").
// A Group bundles related Rules (e.g. all Title rules).
// A Preset is a named composition of Groups that a prompt renders into its
// final body. Presets are immutable — Without/With return new copies.
package rules

import (
	"fmt"
	"strings"
)

// Rule is an atomic instruction injected into an LLM prompt.
type Rule struct {
	ID   string // stable identifier, e.g. "titles.h1_contains_keyword"
	Body string // free-form text sent to the model
}

// Group bundles related Rules under a section heading.
type Group struct {
	Name  string
	Rules []Rule
}

// Preset is the composable unit used by prompt builders.
type Preset struct {
	Name   string
	Groups []Group
}

// Without returns a copy of the preset with the given rule IDs excluded.
// Empty groups left after filtering are dropped.
func (p Preset) Without(ids ...string) Preset {
	if len(ids) == 0 {
		return p
	}
	exclude := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		exclude[id] = struct{}{}
	}

	out := Preset{Name: p.Name}
	for _, g := range p.Groups {
		var kept []Rule
		for _, r := range g.Rules {
			if _, skip := exclude[r.ID]; skip {
				continue
			}
			kept = append(kept, r)
		}
		if len(kept) > 0 {
			out.Groups = append(out.Groups, Group{Name: g.Name, Rules: kept})
		}
	}
	return out
}

// With appends rules to a group by name (creating it if missing) and returns
// a new preset. Existing order is preserved.
func (p Preset) With(groupName string, extra ...Rule) Preset {
	if len(extra) == 0 {
		return p
	}
	out := Preset{Name: p.Name, Groups: make([]Group, len(p.Groups))}
	copy(out.Groups, p.Groups)
	for i, g := range out.Groups {
		if g.Name == groupName {
			out.Groups[i].Rules = append(append([]Rule{}, g.Rules...), extra...)
			return out
		}
	}
	out.Groups = append(out.Groups, Group{Name: groupName, Rules: extra})
	return out
}

// Render formats the preset as markdown sections with grouped numbered lists.
// The numbering resets per group.
func (p Preset) Render() string {
	var b strings.Builder
	for i, g := range p.Groups {
		if len(g.Rules) == 0 {
			continue
		}
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "## %s\n", g.Name)
		for j, r := range g.Rules {
			fmt.Fprintf(&b, "%d. %s\n", j+1, r.Body)
		}
	}
	return b.String()
}
