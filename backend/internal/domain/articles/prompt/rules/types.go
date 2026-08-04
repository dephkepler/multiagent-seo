package rules

import (
	"fmt"
	"strings"
)

type Rule struct {
	ID   string
	Body string
}

type Group struct {
	Name  string
	Rules []Rule
}

type Preset struct {
	Name   string
	Groups []Group
}

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
