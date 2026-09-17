package assess

import "testing"

func TestLevelString(t *testing.T) {
	cases := []struct {
		level Level
		want  string
	}{
		{Info, "info"},
		{Low, "low"},
		{High, "high"},
		{Critical, "critical"},
		{Unranked, "unranked"},
	}
	for _, c := range cases {
		if got := c.level.String(); got != c.want {
			t.Errorf("Level(%d).String() = %q, want %q", c.level, got, c.want)
		}
	}
}

func TestLevelOrdering(t *testing.T) {
	if !(Info < Low && Low < High && High < Critical) {
		t.Fatal("levels must order info < low < high < critical")
	}
}

// Unranked is not a fifth severity, it is the absence of one. It sits at
// the top of the ordering so that a finding the tool could not rank is read
// first and no display filter can hide it - and so that a comparison
// somebody adds later and forgets to guard errs towards showing it rather
// than hiding it. It is NOT a damage severity: it stays out of the counts
// and out of Max, so --fail-on never sees it.
func TestUnrankedSitsAboveEverySeverity(t *testing.T) {
	if Unranked <= Critical {
		t.Fatal("an unrankable finding must sort above critical, or --min-level can hide it")
	}
}

// A severity is something a person can ask for on the command line.
// "unranked" is not one: it is the tool saying it has no severity to give,
// so --fail-on unranked, --min-level unranked and a rule assigning it are
// all mistakes worth an error rather than a silent reading.
func TestParseLevelRejectsUnranked(t *testing.T) {
	if _, err := ParseLevel("unranked"); err == nil {
		t.Fatal("ParseLevel(\"unranked\") must error - it is the absence of a severity, not one of them")
	}
}

func TestParseLevel(t *testing.T) {
	for _, in := range []string{"critical", "high", "low", "info"} {
		got, err := ParseLevel(in)
		if err != nil {
			t.Fatalf("ParseLevel(%q) returned error: %v", in, err)
		}
		if got.String() != in {
			t.Errorf("ParseLevel(%q) round-tripped to %q", in, got.String())
		}
	}
}

func TestParseLevelRejectsMedium(t *testing.T) {
	if _, err := ParseLevel("medium"); err == nil {
		t.Fatal("ParseLevel(\"medium\") must error - there is deliberately no medium level")
	}
}
