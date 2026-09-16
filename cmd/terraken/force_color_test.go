package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestForceColorMakesColourSurviveAPipe(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	var out, errOut bytes.Buffer
	if code := run([]string{"../../testdata/critical.json"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "\x1b[") {
		t.Error("FORCE_COLOR must produce colour even though the writer is not a terminal")
	}
}

func TestNoColorBeatsForceColor(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "1")
	var out, errOut bytes.Buffer
	run([]string{"../../testdata/critical.json"}, strings.NewReader(""), &out, &errOut)
	if strings.Contains(out.String(), "\x1b[") {
		t.Error("NO_COLOR must win: turning colour off is never the setting that loses")
	}
}

func TestForceColorZeroIsNotOptingIn(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	var out, errOut bytes.Buffer
	run([]string{"../../testdata/critical.json"}, strings.NewReader(""), &out, &errOut)
	if strings.Contains(out.String(), "\x1b[") {
		t.Error("FORCE_COLOR=0 must not enable colour")
	}
}

var _ = os.Getenv
