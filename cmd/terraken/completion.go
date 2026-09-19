package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/dbhq-uk/terraken/internal/assess"
	"github.com/dbhq-uk/terraken/internal/render"
)

// Shell completions, GENERATED FROM THE FLAG SET rather than hand-maintained.
//
// The flags that get mistyped are the ones with constrained values: --format
// takes a fixed list, --fail-on and --min-level take the levels, and --explain
// takes a finding code. A hand-written completion script is a second copy of
// all of those, and a second copy is a thing that drifts - a format added to
// the registry would complete as nothing until somebody remembered.
//
// So the script is built from the same registries the command validates
// against: render.Formats, assess.Levels and assess.ExplainableCodes. A value
// the tool accepts is a value it completes, by construction.

// positionalValues is what a flag takes as the NEXT WORD rather than as its
// value. --explain is a boolean and reads the code from the argument after it,
// so offering `--explain=blast-radius` produced "invalid boolean value" and
// exit 2 - a completion that cannot be run.
func positionalValues() map[string][]string {
	return map[string][]string{"explain": assess.ExplainableCodes()}
}

// completionValues is the constrained values of each flag that has any.
func completionValues() map[string][]string {
	levels := make([]string, 0, 4)
	for _, l := range assess.Levels() {
		levels = append(levels, l.String())
	}
	return map[string][]string{
		"format":    append([]string(nil), render.Formats...),
		"fail-on":   levels,
		"min-level": levels,
	}
}

// writeCompletion emits a completion script for one shell.
func writeCompletion(shell string, flags []string, w io.Writer) int {
	sort.Strings(flags)
	values := completionValues()
	after := positionalValues()

	switch shell {
	case "bash":
		fmt.Fprintf(w, "# terraken bash completion. Generated - do not edit.\n")
		fmt.Fprintf(w, "_terraken() {\n")
		fmt.Fprintf(w, "  local cur prev\n")
		fmt.Fprintf(w, "  cur=\"${COMP_WORDS[COMP_CWORD]}\"\n")
		fmt.Fprintf(w, "  prev=\"${COMP_WORDS[COMP_CWORD-1]}\"\n")
		fmt.Fprintf(w, "  case \"$prev\" in\n")
		for _, name := range sortedKeys(values) {
			fmt.Fprintf(w, "    --%s) COMPREPLY=( $(compgen -W %q -- \"$cur\") ); return ;;\n",
				name, strings.Join(values[name], " "))
		}
		// Flags whose argument is the next word rather than their value.
		for _, name := range sortedKeys(after) {
			fmt.Fprintf(w, "    --%s) COMPREPLY=( $(compgen -W %q -- \"$cur\") ); return ;;\n",
				name, strings.Join(after[name], " "))
		}
		fmt.Fprintf(w, "  esac\n")
		fmt.Fprintf(w, "  if [[ \"$cur\" == -* ]]; then\n")
		fmt.Fprintf(w, "    COMPREPLY=( $(compgen -W %q -- \"$cur\") ); return\n", dashed(flags))
		fmt.Fprintf(w, "  fi\n")
		fmt.Fprintf(w, "  COMPREPLY=( $(compgen -f -- \"$cur\") )\n")
		fmt.Fprintf(w, "}\ncomplete -F _terraken terraken tken\n")

	case "zsh":
		fmt.Fprintf(w, "#compdef terraken tken\n")
		fmt.Fprintf(w, "# terraken zsh completion. Generated - do not edit.\n")
		fmt.Fprintf(w, "_terraken() {\n  _arguments -s \\\n")
		for _, name := range flags {
			if vals, ok := values[name]; ok {
				fmt.Fprintf(w, "    '--%s=[%s]:value:(%s)' \\\n", name, name, strings.Join(vals, " "))
				continue
			}
			if vals, ok := after[name]; ok {
				// A boolean flag whose argument is the next word: no `=`.
				fmt.Fprintf(w, "    '--%s[%s]:code:(%s)' \\\n", name, name, strings.Join(vals, " "))
				continue
			}
			fmt.Fprintf(w, "    '--%s[%s]' \\\n", name, name)
		}
		fmt.Fprintf(w, "    '*:plan file:_files'\n}\n_terraken \"$@\"\n")

	case "fish":
		fmt.Fprintf(w, "# terraken fish completion. Generated - do not edit.\n")
		for _, name := range flags {
			if vals, ok := values[name]; ok {
				fmt.Fprintf(w, "complete -c terraken -l %s -x -a %q\n", name, strings.Join(vals, " "))
				continue
			}
			if vals, ok := after[name]; ok {
				fmt.Fprintf(w, "complete -c terraken -l %s -a %q\n", name, strings.Join(vals, " "))
				continue
			}
			fmt.Fprintf(w, "complete -c terraken -l %s\n", name)
		}

	default:
		return 2
	}
	return 0
}

func dashed(flags []string) string {
	out := make([]string, 0, len(flags))
	for _, f := range flags {
		out = append(out, "--"+f)
	}
	return strings.Join(out, " ")
}

func sortedKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
