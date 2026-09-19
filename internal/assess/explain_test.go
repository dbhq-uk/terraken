package assess

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// declaredCodes is every annotation code this package declares, PARSED out of
// every file in it.
//
// Reading one file was a register with a hole in it: Astra declared
// AnnReviewProbe in another file, emitted it, and both tests passed. Annotation
// codes live in finding.go by convention and a convention is not a check -
// the sequencing and changed-attribute codes could as easily have gone in their
// own files, and one of them nearly did.
func declaredCodes(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("cannot parse the package: %v", err)
	}

	var out []string
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.CONST {
					continue
				}
				for _, spec := range gen.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, name := range vs.Names {
						if !strings.HasPrefix(name.Name, "Ann") || i >= len(vs.Values) {
							continue
						}
						lit, ok := vs.Values[i].(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							continue
						}
						code, uerr := strconv.Unquote(lit.Value)
						if uerr != nil {
							t.Errorf("%s has a value this test cannot read: %s", name.Name, lit.Value)
							continue
						}
						out = append(out, code)
					}
				}
			}
		}
	}
	return out
}

// TestEveryCodeIsExplained is the "one source" the issue asks for, enforced.
//
// It reads the constants out of the source rather than listing them here,
// because a list here is a second register and two registers wearing one name
// is how they drift. A code added to finding.go without an explanation fails
// the build, which is the acceptance the issue states.
func TestEveryCodeIsExplained(t *testing.T) {
	declared := declaredCodes(t)
	if len(declared) < 10 {
		t.Fatalf("found %d annotation codes - the parse has broken", len(declared))
	}
	for _, m := range declared {
		if _, ok := Explain(m); !ok {
			t.Errorf("the annotation code %q has no explanation. Add one to explain.go: a "+
				"reader meeting it in a report has nowhere else to look.", m)
		}
	}

	// Every level too, including the one that is the absence of a level.
	for _, l := range []Level{Info, Low, High, Critical, Unranked} {
		if _, ok := Explain(l.String()); !ok {
			t.Errorf("the level %q has no explanation", l)
		}
	}
}

// TestNothingIsExplainedThatCannotBePrinted is the other direction. An
// explanation for a code the tool cannot emit is a lie in the documentation,
// and it is the failure the site's own test guards against for the same list.
func TestNothingIsExplainedThatCannotBePrinted(t *testing.T) {
	known := map[string]bool{}
	for _, c := range declaredCodes(t) {
		known[c] = true
	}
	for _, l := range []Level{Info, Low, High, Critical, Unranked} {
		known[l.String()] = true
	}
	for _, code := range ExplainableCodes() {
		if !known[code] {
			t.Errorf("%q is explained and nothing can emit it", code)
		}
	}
}

// TestAnExplanationSaysWhereItStops. The first sentence says what the finding
// is; the rest says what it does NOT claim. That second half is the reason the
// feature exists - a code a reader half-remembers is more dangerous than one
// they look up - so an explanation that is only a definition is not finished.
func TestAnExplanationSaysWhereItStops(t *testing.T) {
	limits := []string{
		"not", "never", "rather than", "cannot", "stops", "floor", "only",
		"incomplete", "heuristic", "yours to judge",
	}
	for _, code := range ExplainableCodes() {
		text, _ := Explain(code)
		if len(text) < 80 {
			t.Errorf("%q: the explanation is %d characters. It has to say what the finding is "+
				"AND where it stops.", code, len(text))
		}
		low := strings.ToLower(text)
		found := false
		for _, w := range limits {
			if strings.Contains(low, w) {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: the explanation states what the finding is and never what it does "+
				"not claim: %q", code, text)
		}
	}
}

// TestNoExplanationPrintsAValue. These are the tool's own sentences and
// --explain opens no file, so there is nothing a plan could have put here - but
// a future explanation quoting a fixture would be exactly the kind of example
// that reads well and breaks the guarantee.
func TestNoExplanationPrintsAValue(t *testing.T) {
	for _, code := range ExplainableCodes() {
		text, _ := Explain(code)
		for _, shape := range []string{"AKIA", "-----BEGIN", "ghp_", "xoxb-", "eyJ"} {
			if strings.Contains(text, shape) {
				t.Errorf("%q: the explanation contains %q", code, shape)
			}
		}
	}
}

// TestEveryEmittedCodeIsExplained is the registry checked by
// behaviour rather than by syntax, and it is the one that cannot be dodged.
//
// declaredCodes parses the source, so it answers for declarations it
// understands: Astra walked a concatenated value, a `var`, an alias and an
// iota-derived string past it, and each round of tightening the parse invited
// the next form. This asks the tool instead - assess every committed fixture
// and collect the codes that come out. However a code is declared, if a report
// can print it a reader can meet it, and a reader who meets it has nowhere
// else to look.
func TestEveryEmittedCodeIsExplained(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range fixtureNames(t) {
		r := Assess(loadFixture(t, name))
		for _, list := range [][]Finding{r.Findings, r.Drift} {
			for _, f := range list {
				for _, a := range f.Annotations {
					seen[a.Code] = true
				}
			}
		}
	}
	if len(seen) < 8 {
		t.Fatalf("only %d codes were emitted across every fixture - the walk has broken", len(seen))
	}
	for code := range seen {
		if _, ok := Explain(code); !ok {
			t.Errorf("a report can print %q and --explain cannot answer for it", code)
		}
	}
}

// fixtureNames is every committed plan fixture.
func fixtureNames(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir("../../testdata")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		switch e.Name() {
		case "malformed.json", "notaplan.json", "rules-example.json":
			continue
		}
		out = append(out, e.Name())
	}
	return out
}
