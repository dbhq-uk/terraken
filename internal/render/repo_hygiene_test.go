package render

import (
	"bytes"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// A tool that refuses to print an invisible character should not ship one.
//
// This is not a style rule. Three of this repository's own documents ended up
// carrying a REAL U+202E in the sentence explaining that terraken escapes
// U+202E - the paragraph about reviewer deception was itself unreadable in the
// way it was describing, and nobody could see it, which is the whole point of
// the character. A review caught it; a test is what stops it coming back.
//
// Source files are covered as well as documentation. A payload in a test is
// written as a Go escape - the six characters backslash-u-2-0-2-e - which is
// readable in a diff and cannot be lost in a copy and paste.
func TestTheRepositoryShipsNoInvisibleCharacters(t *testing.T) {
	root := filepath.Join("..", "..")

	banned := map[rune]string{
		0x200b: "zero-width space", 0x200c: "zero-width non-joiner",
		0x200d: "zero-width joiner", 0x200e: "left-to-right mark",
		0x200f: "right-to-left mark", 0x202a: "left-to-right embedding",
		0x202b: "right-to-left embedding", 0x202c: "pop directional formatting",
		0x202d: "left-to-right override", 0x202e: "right-to-left override",
		0x2066: "left-to-right isolate", 0x2067: "right-to-left isolate",
		0x2068: "first strong isolate", 0x2069: "pop directional isolate",
		0xfeff: "byte order mark",
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Not ours to police, and full of things that legitimately hold
			// anything at all.
			switch info.Name() {
			case ".git", "node_modules", "dist", "testdata", "assets":
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".go", ".md", ".yml", ".yaml":
		default:
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, r := range string(b) {
			if name, bad := banned[r]; bad {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s holds a real %s (U+%04X) at offset %d - write it as an escape instead:\n%s",
					rel, name, r, i, excerptAround(string(b), i))
			}
			// Everything else in the format category, without naming each one.
			if unicode.Is(unicode.Cf, r) {
				if _, known := banned[r]; !known {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s holds a Unicode format character U+%04X at offset %d", rel, r, i)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func excerptAround(s string, at int) string {
	lo, hi := at-60, at+60
	if lo < 0 {
		lo = 0
	}
	if hi > len(s) {
		hi = len(s)
	}
	return strings.TrimSpace(s[lo:hi])
}

// TestHCLStaysOutOfTheBinary is the decision in docs/design.md, enforced.
//
// hashicorp/hcl/v2 is a dependency of this repository and only from _test.go
// files: one test asserts that emitted `moved` blocks parse, and another reads
// the generating roots in testdata/_gen to check what a fixture claims about
// the configuration it came from. Neither is the tool reading configuration.
//
// The decision is that HCL may verify evidence about a fixture and may not
// become an input to the report - see "The configuration is not an input". A
// non-test import is how that would start, so it fails here instead.
func TestHCLStaysOutOfTheBinary(t *testing.T) {
	// FAILS RATHER THAN SKIPS. A guard that skips when it cannot look is a
	// guard that disappears the day the thing it checks would have failed -
	// and `go list` erroring is not evidence that the dependency is absent.
	cmd := exec.Command("go", "list", "-deps", "../../cmd/terraken")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("cannot list the command's dependencies, so this guard could not run: %v\n%s",
			err, stderr.String())
	}

	// AND THE OUTPUT HAS TO BE THE RIGHT OUTPUT. A wrapper that exits zero
	// with nothing to say would otherwise pass this, which is the same
	// failure one line up wearing a different hat.
	if !strings.Contains(string(out), "github.com/dbhq-uk/terraken/cmd/terraken") {
		t.Fatalf("the dependency list does not include the command itself, so it is not the "+
			"list this guard needs:\n%s", out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "github.com/hashicorp/hcl") {
			t.Errorf("%s is linked into the command. Terraken reads a plan file and nothing "+
				"else; HCL may verify evidence about a fixture and may not become an input "+
				"to the report. See docs/design.md.", line)
		}
	}

	// AND EVERY SOURCE FILE IN THE MODULE, PARSED. `go list -deps` answers for
	// ONE build, so a file behind a build tag is invisible to it, and so is a
	// package only a tagged file imports.
	//
	// PARSED RATHER THAN SEARCHED, because a substring check is a guess at Go
	// syntax and lost twice: a raw-string import path walked past a
	// double-quoted search, and `"\x67ithub.com/hashicorp/hcl/v2"` walks past
	// both. go/parser resolves the literal, so the guard checks the import
	// path the compiler will see rather than the characters somebody typed.
	fset := token.NewFileSet()
	err = filepath.WalkDir("../..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// ONLY WHAT CANNOT HOLD GO. testdata was skipped and a tagged
			// helper under it was imported by the command, which put HCL in
			// the binary with the guard green. A directory is skipped here
			// because walking it is pointless, never because its contents are
			// assumed safe.
			switch d.Name() {
			case ".git", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			t.Errorf("cannot parse %s, so this guard could not read its imports: %v", path, perr)
			return nil
		}
		for _, imp := range f.Imports {
			got, uerr := strconv.Unquote(imp.Path.Value)
			if uerr != nil {
				t.Errorf("%s has an import path this guard cannot read: %s", path, imp.Path.Value)
				continue
			}
			if strings.HasPrefix(got, "github.com/hashicorp/hcl") {
				t.Errorf("%s imports %s and is not a test file. Terraken reads a plan file "+
					"and nothing else; HCL may verify evidence about a fixture and may not "+
					"become an input to the report. See docs/design.md.", path, got)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("cannot walk the module: %v", err)
	}
}
