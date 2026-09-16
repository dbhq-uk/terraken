package assess

import (
	"fmt"
	"sort"
	"strings"
)

// The shape of a plan: what it changes, roughly, before the findings say what
// it changes exactly.
//
// A reviewer's first question is "what is this, broadly" and the ranked list
// answers it only by being read. On a twenty-resource plan that is still a
// wall, just a well-sorted one. This is the orientation; the findings remain
// the substance, and nothing here replaces reading them.
//
// IT COUNTS THE WHOLE PLAN, ALWAYS. --min-level filters what is displayed, and
// a summary that shrank with the filter would tell a reviewer the change is
// smaller than it is - which is worse than no summary, because it is confidently
// wrong rather than absent. Report.AtLeast carries this across untouched.

// ModuleChurn is one module and how many changes it holds.
type ModuleChurn struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Shape is the summary of a whole plan.
type Shape struct {
	Total    int          `json:"total"`
	ByAction map[Kind]int `json:"by_action"`
	// ByType and ByModule are keyed by the strings terraform uses, so a
	// consumer can join them against its own data without a mapping table.
	ByType   map[string]int `json:"by_type"`
	ByModule map[string]int `json:"by_module"`

	// BusiestModules is ByModule ranked, most churn first, with the root
	// module named as "the root module" rather than the empty string
	// terraform actually uses.
	BusiestModules []ModuleChurn `json:"busiest_modules,omitempty"`

	// Headline is the one-line shape, or empty when the plan is too small or
	// too mixed for one sentence to be true about it.
	Headline string `json:"headline,omitempty"`
}

// summaryFloor is the number of findings below which a summary costs more than
// it gives.
//
// The issue asks for a summary that stays useful at both ends: "a
// three-resource plan should not get a summary longer than its findings". Nine
// is where a list stops being takeable at a glance - roughly a terminal's worth
// of findings at three or four lines each. Below it the findings ARE the
// summary and a second telling is noise.
const summaryFloor = 9

// Worth reports whether this plan is big enough that a summary earns its
// space. A renderer asks this rather than deciding for itself, so every format
// makes the same call and the threshold lives in one place.
func (s Shape) Worth() bool { return s.Total >= summaryFloor }

// shapeOf summarises every change in the plan.
//
// Takes findings rather than the raw plan so the kinds it counts are the same
// kinds the report names. Counting actions off tfjson directly would give a
// summary that classifies a change one way and a finding the other, and a
// reader would have no way to tell which was right.
func shapeOf(findings []Finding) Shape {
	s := Shape{
		Total:    len(findings),
		ByAction: map[Kind]int{},
		ByType:   map[string]int{},
		ByModule: map[string]int{},
	}
	for _, f := range findings {
		s.ByAction[f.Kind]++
		s.ByType[f.Type]++
		s.ByModule[moduleName(f.Module)]++
	}

	for name, n := range s.ByModule {
		s.BusiestModules = append(s.BusiestModules, ModuleChurn{Name: name, Count: n})
	}
	// Most churn first, then by name. The name tiebreak is what keeps this
	// deterministic: ranging a map is deliberately unordered in Go, and
	// without it the summary would reshuffle between runs of the same plan.
	sort.Slice(s.BusiestModules, func(i, j int) bool {
		if s.BusiestModules[i].Count != s.BusiestModules[j].Count {
			return s.BusiestModules[i].Count > s.BusiestModules[j].Count
		}
		return s.BusiestModules[i].Name < s.BusiestModules[j].Name
	})

	s.Headline = headlineOf(s)
	return s
}

// headlineOf writes the one-line shape, or returns "" when no single sentence
// would be true.
//
// THE BAR FOR SAYING SOMETHING IS THAT IT IS TRUE, not that a line exists to
// fill. "Mostly creates" on a plan that is 40% destroys is the kind of summary
// that gets somebody to skim past the destroys, so a plan with no dominant
// action gets no headline and the reader goes to the findings, which is where
// they should have gone anyway.
func headlineOf(s Shape) string {
	if s.Total == 0 {
		return ""
	}

	type kc struct {
		k Kind
		n int
	}
	var actions []kc
	for k, n := range s.ByAction {
		actions = append(actions, kc{k, n})
	}
	sort.Slice(actions, func(i, j int) bool {
		if actions[i].n != actions[j].n {
			return actions[i].n > actions[j].n
		}
		return string(actions[i].k) < string(actions[j].k)
	})

	top := actions[0]
	share := float64(top.n) / float64(s.Total)

	var b strings.Builder
	switch {
	case share == 1:
		b.WriteString(fmt.Sprintf("Every change in this plan is %s", actionPhrase(top.k, top.n)))
	case share >= 0.8:
		b.WriteString(fmt.Sprintf("Mostly %s - %d of %d changes", actionPhrase(top.k, top.n), top.n, s.Total))
	case share >= 0.5:
		b.WriteString(fmt.Sprintf("%d of %d changes are %s", top.n, s.Total, actionPhrase(top.k, top.n)))
	default:
		// No dominant action. Say what the mix is rather than pretending
		// one side of it speaks for the whole.
		b.WriteString(fmt.Sprintf("%d changes, mixed", s.Total))
	}

	// Where to look. Named only when one module actually holds most of it -
	// on an even spread "spread across 4 modules" is the more honest line,
	// and pointing at the nominal leader would send a reader to the wrong
	// place first.
	if len(s.BusiestModules) > 0 {
		lead := s.BusiestModules[0]
		switch {
		case len(s.BusiestModules) == 1:
			b.WriteString(", all in " + lead.Name)
		case float64(lead.Count)/float64(s.Total) >= 0.4:
			b.WriteString(fmt.Sprintf(", %d of them in %s", lead.Count, lead.Name))
		default:
			b.WriteString(fmt.Sprintf(", spread across %d modules", len(s.BusiestModules)))
		}
	}

	return b.String()
}

// actionPhrase names a kind in a sentence, in the reader's words rather than
// terraform's. A count of one takes the singular: "1 changes are update in
// place" is the kind of line that makes a reader trust nothing else on the page.
func actionPhrase(k Kind, n int) string {
	one := n == 1
	switch k {
	case KindCreate:
		return pick(one, "a create", "creates")
	case KindUpdate:
		return pick(one, "an update in place", "updates in place")
	case KindDelete:
		return pick(one, "a destroy", "destroys")
	case KindReplace:
		return pick(one, "a replacement", "replacements")
	case KindRead:
		return pick(one, "a data source read", "data source reads")
	case KindImport:
		return pick(one, "an import", "imports")
	case KindForget:
		return pick(one, "a resource being forgotten from state", "resources being forgotten from state")
	case KindNoOp:
		return pick(one, "a no-op", "no-ops")
	}
	return string(k)
}

func pick(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
