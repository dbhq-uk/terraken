package plan

import (
	"bytes"
	"encoding/json"
)

// What a plan says about ITSELF, as opposed to about the resources in it.
//
// Terraform v1.8 added `applyable` and `complete` specifically so consumers
// would stop inferring them, and the specification tells wrapping automations
// to use `complete` as their primary condition. `errored` says planning failed
// outright. Terraken ranked the changes in a plan while saying nothing about
// whether the plan converges, or whether it failed to compute at all, so a
// reviewer could not tell a whole change from a partial one.
//
// EVERY FIELD IS A POINTER, AND THAT IS THE POINT. Absent and false are
// different facts: "this plan does not converge" and "this build cannot tell
// whether it converges" are different things to tell a reviewer. A bool cannot
// hold both, and the pinned decoder already learned this the hard way - it
// types `complete` as a pointer for exactly this reason.
//
// PARTIAL IS A REAL CASE, not a theoretical one. Terraform added `errored` in
// v1.7 and `complete` and `applyable` in v1.8, so a plan from in between states
// one and not the others, and an OpenTofu plan may state none.
//
// NONE OF THIS IS A FINDING. A finding is one resource change, assessed, and a
// failed planning operation is not a resource change. See the wording in
// docs/design.md: critical means the resource type holds data and destroying
// it loses that data, and that is the only escalation in the tool.
type Status struct {
	// Errored: "indicates whether planning failed. An errored plan cannot be
	// applied, but the actions planned before failure may help to understand
	// the error."
	Errored *bool `json:"errored"`

	// Complete: "indicates that Terraform expects that after applying this
	// plan the actual state will match the desired state."
	//
	// False means another plan and apply round is expected. It does NOT say
	// why, and nothing in the file says why - `-target` and deferred changes
	// both produce it - so nothing here may infer the cause.
	Complete *bool `json:"complete"`

	// Applyable: "indicates that it would make sense for a wrapping automation
	// to try to apply this plan."
	//
	// NEVER READ THIS AS RISK. Terraform defines applyable as true only when
	// planning succeeded AND the plan calls for a meaningful change, so a
	// perfectly clean no-op plan is not applyable. Anything that flagged it
	// would penalise the plan that most deserves to pass.
	Applyable *bool `json:"applyable"`
}

// Any reports whether the plan stated any of the three.
//
// A renderer asks this before printing a status block at all, so a report over
// a plan from before these flags existed looks exactly as it did before this
// existed - which is what keeps the change invisible to everybody it has
// nothing to tell.
func (s Status) Any() bool {
	return s.Errored != nil || s.Complete != nil || s.Applyable != nil
}

// decodeStatus reads the three flags in their own pass over the bytes.
//
// SEPARATELY, NOT BY EMBEDDING tfjson.Plan IN A WRAPPER. That type has its own
// UnmarshalJSON, which would be promoted to the wrapper and consume the whole
// object without ever looking at the sibling fields - so the flags would
// silently come back nil on a plan that stated all three.
//
// EACH VALUE IS TAKEN RAW AND CHECKED, rather than unmarshalled straight into
// a *bool. Decoding `"errored":"true"` into one does something worse than
// fail: Go allocates the pointer, then hits the type mismatch, and what is
// left behind is a NON-NIL POINTER TO FALSE. The error was discarded, so a
// plan stating garbage came out as a confident "errored: false" - explicit
// reassurance, invented by the decoder, handed to a caller whose whole policy
// is `.status.errored == true`. A value that is not exactly true, false or
// null is not stated, which is the honest answer and the one that fails safe.
func decodeStatus(b []byte) Status {
	var raw struct {
		Errored   json.RawMessage `json:"errored"`
		Complete  json.RawMessage `json:"complete"`
		Applyable json.RawMessage `json:"applyable"`
	}
	// An error here cannot happen on bytes the caller has already decoded as a
	// plan, and if it somehow did, every field stays nil - "not stated" - which
	// is the same answer this function gives for anything it cannot read.
	if err := json.Unmarshal(b, &raw); err != nil {
		return Status{}
	}
	return Status{
		Errored:   asBool(raw.Errored),
		Complete:  asBool(raw.Complete),
		Applyable: asBool(raw.Applyable),
	}
}

// asBool returns a pointer for exactly `true` and `false`, and nil for
// absent, null, and anything else at all.
//
// NULL IS TESTED BEFORE UNMARSHALLING, not left to it. Decoding the four bytes
// `null` into a bool is a documented no-op in Go: it succeeds, returns no
// error, and leaves the variable at its zero value - so `"errored":null` came
// back as a pointer to false, which is precisely the invented reassurance this
// function exists to prevent.
func asBool(raw json.RawMessage) *bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	var b bool
	if err := json.Unmarshal(trimmed, &b); err != nil {
		return nil
	}
	return &b
}
