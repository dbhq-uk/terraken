package plan

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"testing/iotest"
)

func TestLoadMinimal(t *testing.T) {
	p, _, err := Load("../../testdata/minimal.json")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if p.TerraformVersion != "1.9.8" {
		t.Errorf("TerraformVersion = %q, want %q", p.TerraformVersion, "1.9.8")
	}
	if len(p.ResourceChanges) != 0 {
		t.Errorf("expected no resource changes, got %d", len(p.ResourceChanges))
	}
}

func TestLoadMalformed(t *testing.T) {
	_, _, err := Load("../../testdata/malformed.json")
	if err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("error should explain the file is not valid JSON, got: %v", err)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, _, err := Load("../../testdata/does-not-exist.json")
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestLoadRejectsNonPlan(t *testing.T) {
	// A valid JSON document that is not a plan has no format_version.
	_, _, err := Load("../../testdata/notaplan.json")
	if err == nil {
		t.Fatal("expected an error for a JSON file that is not a plan")
	}
	if !strings.Contains(err.Error(), "does not look like a Terraform plan") {
		t.Errorf("error should say it is not a plan, got: %v", err)
	}
}

func TestReadParsesAStream(t *testing.T) {
	b, err := os.ReadFile("../../testdata/minimal.json")
	if err != nil {
		t.Fatalf("committed fixture is missing: %v", err)
	}
	p, _, err := Read(bytes.NewReader(b), "standard input")
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if p.TerraformVersion != "1.9.8" {
		t.Errorf("TerraformVersion = %q, want %q", p.TerraformVersion, "1.9.8")
	}
}

// TestReadErrorsNameTheStreamNotAFile checks a piped plan does not
// produce an error blaming a file nobody opened.
func TestReadErrorsNameTheStreamNotAFile(t *testing.T) {
	_, _, err := Read(strings.NewReader(`{"not":"a plan"}`), "standard input")
	if err == nil {
		t.Fatal("expected an error for JSON that is not a plan")
	}
	if !strings.Contains(err.Error(), "standard input") {
		t.Errorf("error should name standard input, got: %v", err)
	}
}

// TestEmptyInputIsItsOwnError covers the commonest piped failure: the
// upstream command produced nothing. "unexpected end of JSON input"
// would send someone looking at the plan rather than at terraform.
func TestEmptyInputIsItsOwnError(t *testing.T) {
	for _, in := range []string{"", "   \n\t "} {
		_, _, err := Read(strings.NewReader(in), "standard input")
		if err == nil {
			t.Fatalf("expected an error for empty input %q", in)
		}
		if !strings.Contains(err.Error(), "is empty") {
			t.Errorf("error should say the input is empty, got: %v", err)
		}
	}
}

func TestReadPropagatesAReaderFailure(t *testing.T) {
	_, _, err := Read(iotest.ErrReader(errors.New("pipe broke")), "standard input")
	if err == nil {
		t.Fatal("expected an error when the stream itself fails")
	}
	if !strings.Contains(err.Error(), "pipe broke") {
		t.Errorf("error should carry the underlying failure, got: %v", err)
	}
}
