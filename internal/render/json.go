package render

import (
	"encoding/json"
	"io"

	"github.com/dbhq-uk/terraken/internal/assess"
)

// JSON writes the report as indented JSON for machines.
func JSON(w io.Writer, r assess.Report) error {
	// Plan content is untrusted input. See untrusted.go. The encoder would
	// keep the DOCUMENT valid on its own; this is about what a consumer gets
	// back after decoding and prints somewhere else.
	r = sanitise(r)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
