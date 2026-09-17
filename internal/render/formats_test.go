package render

import (
	"bytes"
	"strings"
	"testing"
)

// The registry has to be one registry. `writers` decides what exists and
// `order` decides how it is presented; if the two come apart, a format is
// either unreachable from the command or missing from its help text, and the
// leak proof - which iterates Formats - stops covering it.
func TestFormatOrderCoversEveryWriter(t *testing.T) {
	if len(order) != len(writers) {
		t.Fatalf("order has %d formats and writers has %d", len(order), len(writers))
	}
	seen := map[string]bool{}
	for _, name := range order {
		if _, ok := writers[name]; !ok {
			t.Errorf("order names %q, which nothing writes", name)
		}
		if seen[name] {
			t.Errorf("order names %q twice", name)
		}
		seen[name] = true
	}
	for name := range writers {
		if !seen[name] {
			t.Errorf("%q can be written but is not in order, so it is missing from --format's help "+
				"and from the leak proof", name)
		}
	}
}

// Every format actually writes something, through the one door.
func TestWriteProducesOutputForEveryFormat(t *testing.T) {
	r := sample()
	for _, name := range Formats {
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			if err := Write(&b, name, r, Options{
				Terminal:  TerminalOptions{Width: testWidth},
				Threshold: "high",
			}); err != nil {
				t.Fatalf("Write(%q) returned error: %v", name, err)
			}
			if b.Len() == 0 {
				t.Errorf("Write(%q) wrote nothing", name)
			}
		})
	}
}

func TestWriteRejectsAFormatItCannotWrite(t *testing.T) {
	var b bytes.Buffer
	err := Write(&b, "yaml", sample(), Options{})
	if err == nil {
		t.Fatal("Write accepted a format it cannot write")
	}
	if !strings.Contains(err.Error(), FormatList()) {
		t.Errorf("the error does not say what does work: %v", err)
	}
	if b.Len() != 0 {
		t.Errorf("a rejected format still wrote %d bytes", b.Len())
	}
}

// The help text reads as a sentence rather than as a machine list. It is the
// first thing a person sees when they get --format wrong.
func TestFormatListReadsAsASentence(t *testing.T) {
	got := FormatList()
	if !strings.Contains(got, " or ") {
		t.Errorf("FormatList() = %q; the last format should be joined with \"or\"", got)
	}
	for _, name := range Formats {
		if !strings.Contains(got, name) {
			t.Errorf("FormatList() = %q, which leaves out %q", got, name)
		}
	}
}
