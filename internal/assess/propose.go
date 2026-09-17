package assess

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Proposing the moved blocks a plan looks like it forgot.
//
// The detector has always reported that a rename went without a moved block.
// This writes the block. It goes to stdout, or to a path the caller names, and
// NEVER into a file the tool chose - the user decides where a proposal lands,
// and that is what keeps this read-only rather than a refactoring tool with an
// undo problem.
//
// WHAT MAKES THIS SAFE TO PASTE, which is a higher bar than what made the
// annotation safe to read. The annotation says "verify before using" and a
// reader weighs it. A block in a .tf file is executed. So a proposal is emitted
// only where the evidence alone picked a winner: where two creates matched the
// deleted resource equally well, the detector's address tiebreak chose one, and
// the alphabet is not evidence. Those refuse, out loud, with both candidates
// named.
//
// Getting that wrong is not a near miss. A moved block naming the wrong
// resource adopts a decommissioned object's state under a live address, so
// terraform stops managing the real thing and starts managing a ghost - which
// is worse than the missing block it set out to fix.

// Proposal is one moved block, or one refusal to write it.
type Proposal struct {
	From string `json:"from"`
	To   string `json:"to"`

	// Matched and Compared are the evidence, carried so the confidence is
	// attached to the proposal rather than left behind in the report.
	Matched  int `json:"matched"`
	Compared int `json:"compared"`

	// CrossModule pairings are real renames - moving a resource into or out
	// of a module is the case HashiCorp's own docs lead with - but they are
	// weaker evidence than a same-module match, and the output says so.
	CrossModule bool `json:"cross_module"`

	// Rivals is non-empty when the tool refuses. It names the other creates
	// that fitted exactly as well, and when it is set From and To describe
	// the pairing that was NOT proposed.
	Rivals []string `json:"rivals,omitempty"`
}

// Refused reports whether this proposal is a refusal rather than a block.
func (p Proposal) Refused() bool { return len(p.Rivals) > 0 }

// Proposals collects one entry per missed moved block the report found,
// ordered by source address so the output is deterministic.
//
// Deduplicated on the way: the detector files the same annotation under both
// the delete's address and the create's, so walking findings naively would
// propose every block twice.
func Proposals(r Report) []Proposal {
	seen := map[string]bool{}
	var out []Proposal

	for _, f := range r.Findings {
		for _, a := range f.Annotations {
			if a.Code != AnnMissedMoved || a.Moved == nil {
				continue
			}
			m := a.Moved
			key := m.From + " -> " + m.To
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, Proposal{
				From:        m.From,
				To:          m.To,
				Matched:     m.Matched,
				Compared:    m.Compared,
				CrossModule: m.CrossModule,
				Rivals:      m.Rivals,
			})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].From < out[j].From })
	return out
}

// RenderMoved writes the proposals as HCL, ready to be redirected into a .tf
// file.
//
// Everything that is not a moved block is a comment, so the whole output
// parses as HCL and can be redirected straight into a file without editing -
// including the refusals, which is the point of putting them in the same
// stream rather than on stderr. A reader who redirects this and never looks at
// their terminal still gets told what was not proposed and why.
func RenderMoved(ps []Proposal) string {
	var b strings.Builder

	b.WriteString("# Proposed by terraken from a plan file.\n")
	b.WriteString("#\n")
	b.WriteString("# VERIFY EACH PAIRING BEFORE APPLYING. These are inferred from attribute\n")
	b.WriteString("# similarity, not from any record of what you intended. A moved block naming\n")
	b.WriteString("# the wrong resource adopts a decommissioned object's state under a live\n")
	b.WriteString("# address, which is worse than the missing block it fixes.\n")

	if len(ps) == 0 {
		b.WriteString("#\n# No missed moved blocks were found in this plan.\n")
		return b.String()
	}

	var proposed, refused []Proposal
	for _, p := range ps {
		if p.Refused() {
			refused = append(refused, p)
		} else {
			proposed = append(proposed, p)
		}
	}

	var unwritable []Proposal
	for _, p := range proposed {
		// AN ADDRESS THAT IS NOT AN ADDRESS IS NOT WRITTEN OUT. This output is
		// HCL, meant to be redirected into somebody's configuration, and it is
		// the one place in the tool where escaping would be the wrong answer:
		// a display escape would change the address the block targets, and a
		// block that targets the wrong resource is worse than no block. So a
		// proposal whose addresses are not plausible Terraform addresses is
		// refused, loudly, with the reason.
		//
		// It is not hypothetical. Before this, an address carrying an escape
		// sequence put a real one on standard output, and a newline could open
		// a line of HCL the tool never wrote.
		if !plausibleAddress(p.From) || !plausibleAddress(p.To) {
			unwritable = append(unwritable, p)
			continue
		}
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("# %d of %d compared attributes identical", p.Matched, p.Compared))
		if p.CrossModule {
			// Said on the block itself rather than in the preamble: a reader
			// scanning thirty blocks for the risky ones should not have to
			// hold a list of exceptions in their head.
			b.WriteString(", across a module boundary, which is weaker evidence")
		}
		b.WriteString(".\n")
		b.WriteString("moved {\n")
		b.WriteString(fmt.Sprintf("  from = %s\n", p.From))
		b.WriteString(fmt.Sprintf("  to   = %s\n", p.To))
		b.WriteString("}\n")
	}

	// THE REFUSALS ARE IN THE OUTPUT, NOT OMITTED FROM IT. A silent omission
	// is indistinguishable from "nothing was found", and the reader would
	// never learn that the tool saw the rename and declined to guess.
	for _, p := range refused {
		if !plausibleAddress(p.From) || !plausibleAddress(p.To) {
			unwritable = append(unwritable, p)
			continue
		}
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("# NOT PROPOSED: %s\n", p.From))
		b.WriteString(fmt.Sprintf("# %d of %d attributes match %s, and equally well:\n",
			p.Matched, p.Compared, p.To))
		for _, r := range p.Rivals {
			if !plausibleAddress(r) {
				b.WriteString("#   (an address this tool will not reproduce - see below)\n")
				continue
			}
			b.WriteString(fmt.Sprintf("#   %s\n", r))
		}
		b.WriteString("# More than one candidate fits, so the evidence does not say which was\n")
		b.WriteString("# intended. Pick one yourself, or leave the resource to be recreated.\n")
	}

	// Said, never dropped. The reader has to learn that the tool saw a rename
	// and would not write it down, or a silent omission reads as "there was
	// nothing here".
	if len(unwritable) > 0 {
		b.WriteString("\n# NOT PROPOSED: ")
		b.WriteString(fmt.Sprintf("%d rename(s) whose addresses this tool will not write into a\n",
			len(unwritable)))
		b.WriteString("# configuration file. They hold characters a Terraform address cannot\n")
		b.WriteString("# legitimately contain - a control character, an escape sequence, a line\n")
		b.WriteString("# break - so reproducing them here would either corrupt this file or\n")
		b.WriteString("# target a resource other than the one intended. Look at the plan.\n")
	}

	return b.String()
}

// plausibleAddress reports whether s could be a Terraform resource address.
//
// DELIBERATELY A REJECTION TEST, NOT A GRAMMAR. It is not trying to decide
// whether an address is valid - `for_each` keys are arbitrary strings and the
// real grammar is wide - only whether writing it into a configuration file
// could do something other than name a resource. A control character, an
// escape sequence or a line break is the whole of what that takes, and every
// one of them is a character no address from a working Terraform run contains.
//
// Being too strict here costs a refusal that says so. Being too lax costs
// somebody's configuration file.
func plausibleAddress(s string) bool {
	if s == "" || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return false
		}
		if unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}
