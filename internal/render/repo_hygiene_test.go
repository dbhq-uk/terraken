package render

import (
	"os"
	"path/filepath"
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
