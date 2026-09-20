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

// commandNames is what the completion is installed for: the tool and the
// short alias the release ships beside it.
var commandNames = []string{"terraken", "tken"}

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
		fmt.Fprintf(w, "  local cur prev eq=0\n")
		fmt.Fprintf(w, "  cur=\"${COMP_WORDS[COMP_CWORD]}\"\n")
		fmt.Fprintf(w, "  prev=\"${COMP_WORDS[COMP_CWORD-1]}\"\n")
		// --format=js is an invocation the tool accepts, so it is one to
		// finish. It reaches here two ways: as one word, or already split on
		// the = because COMP_WORDBREAKS holds one. Both are unwrapped to the
		// flag and the part typed, and eq records that the = was used - which
		// is what --explain must refuse.
		fmt.Fprintf(w, "  if [[ \"$cur\" == --*=* ]]; then\n")
		fmt.Fprintf(w, "    prev=\"${cur%%%%=*}\"; cur=\"${cur#*=}\"; eq=1\n")
		fmt.Fprintf(w, "  elif [[ \"$prev\" == \"=\" && $COMP_CWORD -ge 2 ]]; then\n")
		fmt.Fprintf(w, "    prev=\"${COMP_WORDS[COMP_CWORD-2]}\"; eq=1\n")
		fmt.Fprintf(w, "  fi\n")
		fmt.Fprintf(w, "  case \"$prev\" in\n")
		for _, name := range sortedKeys(values) {
			fmt.Fprintf(w, "    --%s) COMPREPLY=( $(compgen -W %q -- \"$cur\") ); return ;;\n",
				name, strings.Join(values[name], " "))
		}
		// Flags whose argument is the next word rather than their value. The
		// = form is not offered at all, because the tool exits 2 on it.
		for _, name := range sortedKeys(after) {
			fmt.Fprintf(w, "    --%s)\n", name)
			fmt.Fprintf(w, "      [[ $eq -eq 1 ]] && return\n")
			fmt.Fprintf(w, "      COMPREPLY=( $(compgen -W %q -- \"$cur\") ); return ;;\n",
				strings.Join(after[name], " "))
		}
		fmt.Fprintf(w, "  esac\n")
		fmt.Fprintf(w, "  if [[ \"$cur\" == -* ]]; then\n")
		fmt.Fprintf(w, "    COMPREPLY=( $(compgen -W %q -- \"$cur\") ); return\n", dashed(flags))
		fmt.Fprintf(w, "  fi\n")
		// A PLAN FILE MAY HAVE A SPACE IN ITS NAME. The unquoted substitution
		// filling COMPREPLY word-splits on IFS, so "production plan.json" was
		// offered as "production" and "plan.json" - two candidates, neither of
		// them a file. IFS is narrowed to a newline for this one line only,
		// because compgen -W splits its own word list on IFS as well and every
		// -W above would collapse into a single candidate. -o filenames is
		// what makes bash escape the space when it inserts the name.
		fmt.Fprintf(w, "  local IFS=$'\\n'\n")
		fmt.Fprintf(w, "  COMPREPLY=( $(compgen -f -- \"$cur\") )\n")
		fmt.Fprintf(w, "  compopt -o filenames 2>/dev/null\n")
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
		// Both names, as bash and zsh already do. fish had only terraken, so
		// the alias completed nothing.
		for _, cmd := range commandNames {
			for _, name := range flags {
				if vals, ok := values[name]; ok {
					fmt.Fprintf(w, "complete -c %s -l %s -x -a %q\n", cmd, name, strings.Join(vals, " "))
					continue
				}
				// A flag whose argument is the NEXT WORD is declared bare.
				//
				// EXACTLY BACKWARDS IN FISH, AND IT RAN. A long option
				// declared with -a and no -r or -x takes no argument, so
				// -a put the codes after an =: fish offered
				// `--explain=blast-radius`, which the tool rejects with exit
				// 2, and offered nothing at all for `--explain blast`. The
				// codes go on their own completion below instead.
				fmt.Fprintf(w, "complete -c %s -l %s\n", cmd, name)
			}
			// The condition is written in fish's own builtins rather than with
			// __fish_seen_argument, which is a function fish autoloads from
			// its data directory: a completion that depends on it stops
			// working wherever that directory is not where fish expects,
			// and prints an error into the user's terminal when it does.
			for _, name := range sortedKeys(after) {
				fmt.Fprintf(w, "complete -c %s -n %q -f -a %q\n", cmd,
					"contains -- --"+name+" (commandline -opc)", strings.Join(after[name], " "))
			}
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
