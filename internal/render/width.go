package render

import (
	"io"
	"os"
	"strconv"

	"golang.org/x/term"
)

// detectWidth answers how wide the terminal report may be set.
//
// It asks the stream actually being written to, not the process's
// terminal. That distinction is the whole point: a report redirected to
// a file, or piped into less, must not be set to the width of whatever
// terminal happened to launch the command.
//
// COLUMNS is the fallback for the case where the stream cannot answer
// but the caller knows better - a CI log viewer, a pipeline that exports
// it deliberately. 80 is the answer when nothing knows, which is the
// width a terminal has had by default since the punch card.
func detectWidth(w io.Writer) int {
	if f, ok := w.(*os.File); ok {
		if n, _, err := term.GetSize(int(f.Fd())); err == nil && n > 0 {
			return n
		}
	}
	if n, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && n > 0 {
		return n
	}
	return defaultWidth
}
