package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The completion scripts, RUN rather than read.
//
// Both defects this file exists for passed every test that looked at the text
// of the script. A generated script is a program in another language, and the
// only way to know what it offers is to ask the shell that runs it.
//
// Astra found both by running them:
//
//   - bash split `production plan.json` into two candidates, `production` and
//     `plan.json`, because the command substitution filling COMPREPLY was
//     unquoted and word-split on IFS. Neither candidate is a file.
//   - bash offered nothing for `--format=js`, because COMP_WORDBREAKS holds
//     `=` and the flag had already been split off into an earlier word.
//   - fish offered `--explain=blast-radius`, which the tool rejects with exit
//     2, and offered nothing for `--explain blast` - exactly backwards, since
//     --explain reads its code from the next word.

// completionScript writes the script for a shell to a temporary file.
func completionScript(t *testing.T, shell string) string {
	t.Helper()
	code, out, errb := explainRun(t, "--completion", shell)
	if code != 0 {
		t.Fatalf("%s: exit %d: %s", shell, code, errb)
	}
	path := filepath.Join(t.TempDir(), "completion."+shell)
	if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
		t.Fatalf("cannot write the script: %v", err)
	}
	return path
}

// bashComplete runs the generated bash function over a command line and
// returns what it offered, one candidate per line.
//
// The words are handed over as COMP_WORDS and COMP_CWORD directly, which is
// what bash itself does, so no readline and no terminal is needed.
func bashComplete(t *testing.T, dir string, words ...string) []string {
	t.Helper()
	script := completionScript(t, "bash")

	var quoted []string
	for _, w := range words {
		quoted = append(quoted, "'"+strings.ReplaceAll(w, "'", `'\''`)+"'")
	}
	prog := "source " + script + "\n" +
		"COMP_WORDS=(" + strings.Join(quoted, " ") + ")\n" +
		"COMP_CWORD=" + itoa(len(words)-1) + "\n" +
		"_terraken\n" +
		`printf '%s\n' "${COMPREPLY[@]}"` + "\n"

	cmd := exec.Command("bash", "--norc", "--noprofile", "-c", prog)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash failed: %v\n%s", err, out)
	}
	var got []string
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line != "" {
			got = append(got, line)
		}
	}
	return got
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// A filename with a space in it is one candidate, not two. Splitting it
// produced `production` and `plan.json`, and neither of them is a file that
// exists.
func TestBashKeepsAFilenameWithASpaceWhole(t *testing.T) {
	dir := t.TempDir()
	name := "production plan.json"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := bashComplete(t, dir, "terraken", "prod")
	if len(got) != 1 || got[0] != name {
		t.Errorf("offered %q, want exactly [%q]", got, name)
	}
}

// --format=js is an invocation the tool accepts, so it is one the completion
// has to finish. bash splits the word on the = in COMP_WORDBREAKS, which is
// why the flag has to be recovered from the earlier word.
func TestBashCompletesAValueWrittenWithAnEquals(t *testing.T) {
	dir := t.TempDir()
	for _, words := range [][]string{
		{"terraken", "--format=js"},
		{"terraken", "--format", "=", "js"},
	} {
		got := bashComplete(t, dir, words...)
		if len(got) != 1 || got[0] != "json" {
			t.Errorf("%v offered %q, want [json]", words, got)
		}
	}
}

// --explain reads its code from the NEXT WORD. --explain=blast-radius is
// rejected with exit 2, so it is not an invocation to offer.
func TestBashOffersNoEqualsFormForExplain(t *testing.T) {
	dir := t.TempDir()
	if got := bashComplete(t, dir, "terraken", "--explain=blast"); len(got) != 0 {
		t.Errorf("offered %q for --explain=, which the tool rejects", got)
	}
	got := bashComplete(t, dir, "terraken", "--explain", "blast")
	if len(got) != 1 || got[0] != "blast-radius" {
		t.Errorf("offered %q for --explain blast, want [blast-radius]", got)
	}
}

// The bash flag list still completes, which is what the IFS change could
// quietly have broken: compgen -W splits its word list on IFS too.
func TestBashStillCompletesFlags(t *testing.T) {
	got := bashComplete(t, t.TempDir(), "terraken", "--for")
	if len(got) != 1 || got[0] != "--format" {
		t.Errorf("offered %q for --for, want [--format]", got)
	}
}

// fish, if it is installed. It is not on the CI image, so this skips rather
// than fails - the structural test below holds either way.
func TestFishOffersTheCodeAsTheNextWord(t *testing.T) {
	fish, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish is not installed")
	}
	script := completionScript(t, "fish")

	run := func(line string) string {
		cmd := exec.Command(fish, "--no-config", "-c",
			"source "+script+"; complete -C "+shellQuote(line))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fish failed on %q: %v\n%s", line, err, out)
		}
		return string(out)
	}

	if got := run("terraken --explain blast"); !strings.Contains(got, "blast-radius") {
		t.Errorf("fish offers nothing for --explain blast: %q", got)
	}
	if got := run("terraken --explain=blast"); strings.Contains(got, "blast-radius") {
		t.Errorf("fish offers --explain=, which the tool rejects: %q", got)
	}
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// The fish declaration for --explain, checked without fish.
//
// A long option declared with -a and no -r or -x is a flag that takes no
// argument, and fish offers its candidates as --explain=value. The codes
// belong on a separate completion, conditional on the flag having been seen,
// so they are offered as the next word.
func TestFishDeclaresExplainWithoutAnArgument(t *testing.T) {
	_, out, _ := explainRun(t, "--completion", "fish")
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "-l explain") {
			continue
		}
		if strings.Contains(line, "commandline -opc") {
			continue // the conditional completion that offers the codes
		}
		if strings.Contains(line, " -a ") {
			t.Errorf("--explain is declared with candidates, so fish offers "+
				"--explain=code, which the tool rejects: %s", line)
		}
	}
	if !strings.Contains(out, "contains -- --explain (commandline -opc)") {
		t.Error("nothing offers the codes as the word after --explain")
	}

	// The condition uses fish builtins only. __fish_seen_argument is a
	// function fish autoloads from its data directory, and where that
	// directory is not where fish expects, sourcing the script printed
	// "Unknown command" into the terminal instead of completing.
	if strings.Contains(out, "__fish_") {
		t.Error("the fish completion depends on an autoloaded helper function")
	}
}
