package render

import (
	"encoding/json"
	"io"

	"github.com/dbhq-uk/terraverdict/internal/assess"
)

// JSON writes the report as indented JSON for machines.
func JSON(w io.Writer, r assess.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
