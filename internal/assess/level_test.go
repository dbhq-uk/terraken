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
