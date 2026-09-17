# What a plan file holds, and how much of it Terraken reads

A census of the plan JSON against the code, so the gap between what Terraform
records and what Terraken says is a measured fact rather than an impression.

Measured against `main` on 2026-09-17, at the release that flags unmarked
credentials. **Re-run the census before quoting the numbers**: they move every
time a capability ships, and a count typed out by hand is exactly the kind of
claim that quietly goes stale. Enumerate from the struct and diff; do not
enumerate by hand and grep.

## The top-level census

`internal/plan/load.go` decodes the whole plan with `hashicorp/terraform-json`,
whose `Plan` struct exposes fifteen top-level fields, and reads three more that
the pinned version of that library does not model at all. Terraken reads nine
fields in total.

| Field | Read | Where, or what it holds |
|---|---|---|
| `resource_changes` | yes | every finding in the tool |
| `configuration` | yes | blast radius, via `expressions[].references` |
| `terraform_version` | yes | carried into the report |
| `format_version` | yes | validated on load, carried into the report |
| `variables` | yes | credential detection |
| `output_changes` | yes | credential detection |
| `errored` | yes | plan status; not in the pinned library, decoded here |
| `applyable` | yes | plan status; not in the pinned library, decoded here |
| `resource_drift` | no | what Terraform found changed underneath the estate |
| `checks` | no | partial results for checkable objects |
| `complete` | yes | plan status, reported as metadata |
| `timestamp` | no | when the plan was created |
| `deferred_changes` | no | work Terraform knows it postponed |
| `prior_state` | no | the state the plan was computed against |
| `planned_values` | no | the resulting state if applied |
| `relevant_attributes` | no | the attributes the plan actually depended on |
| `action_invocations` | no | provider actions that will fire |

Inside `resource_changes` the picture is the opposite. All thirteen non-empty
`ActionReason` values are handled explicitly in `internal/assess/reason.go`,
and the fourteenth, `ActionReasonNone`, is covered by the empty-string
fallback. `replace_paths`, `after_unknown`, `before_sensitive`,
`after_sensitive`, `mode`, `provider_name`, `deposed`, `previous_address` and
`importing` are all read. Only `generated_config` and the identity pair are
untouched.

So the shape of the gap was precise: **Terraken was thorough about each change
and silent about the plan**. A reviewer could not tell a complete plan from a
partial one, and the report did not say that it could not tell.

**`errored`, `complete` and `applyable` are now read**, as plan status rather
than as findings - see `internal/plan/status.go`. The census above reflects
that. What remains unread is `resource_drift`, `checks`, `timestamp`,
`deferred_changes`, `prior_state`, `planned_values`, `relevant_attributes` and
the three undocumented fields below, and the sections that follow describe
those.

This was never theoretical. Six of the fixtures in `testdata/` already carry
`complete`, `timestamp` and `planned_values`, five carry `prior_state`, and
four carry `variables` and `relevant_attributes`. The fields are sitting in
files the tool already reads, and it steps over the ones still listed here.

## What the unread fields mean

Sourced from Terraform's own JSON output format specification, the relevant
CHANGELOGs and Terraform's source, not from a summary.

### `complete`, `applyable` and `errored`

Terraform v1.7.0 added `errored`; v1.8.0 added `applyable` and `complete`
because they "both summarize
characteristics of a plan that were previously only inferrable by consumers
replicating some of Terraform Core's own logic". The specification is directive
about `complete`: "wrapping automations should use this flag as their primary
condition to accommodate potential changes to the exact definition of
`complete` in future Terraform versions".

`errored` "indicates whether planning failed. An errored plan cannot be
applied, but the actions planned before failure may help to understand the
error".

**Two traps here, both load-bearing.**

`applyable: false` is not a problem signal. Terraform's own source defines
Applyable as true only if planning succeeded *and* the plan calls for some sort
of meaningful change, so a perfectly clean no-op plan is not applyable.
Anything that treated the flag as risk would penalise the plan that most
deserves to pass.

`complete: false` does not mean `-target` was used. The wrapper library says it
"will be false if there are DeferredChanges or if the `-target` flag is used",
but Terraform defines it structurally: complete means the plan includes a
planned action, even a no-op, for every resource instance object mentioned
across both the desired state and the prior state. The honest reading is that
the plan is not expected to converge and another round will be needed. It does
not say why, and no field in the plan says why. A finding announcing that
`-target` was used would be an inference the file does not support.

Note also a type disagreement that matters: Terraform emits `complete` as a
bare bool, always present since that release, while the wrapper types it as a
pointer, nil for older plans. Absent and false are different facts and must
stay different.

**Neither `applyable` nor `errored` exists in the pinned version of
`hashicorp/terraform-json`**, so reaching them needed its own decoding rather
than a version bump. `internal/plan/status.go` does that: a second pass over
the same bytes into a struct of three pointers, deliberately NOT by embedding
`tfjson.Plan` in a wrapper, whose promoted `UnmarshalJSON` would consume the
whole object and never look at the siblings.

All three are reported as plan status - in the header, and under a stable
`status` key in `--format json` and `--format gate`, where `null` is the third
state. None of them is a finding, none is counted, and `--fail-on` cannot see
any of them.

### `checks`

The specification defines `checks` as "the partial results for any checkable
objects, such as resources with postconditions, with as much information as
Terraform can recognize at plan time", with "`fail` means that the condition
evaluated successfully but returned false, while `error` means that the
condition expression itself was invalid".

This is the field that matters most, because of what a `check` block does:
"the check block is the only validation that does not block operations, if a
check block's assertion fails, Terraform reports a warning and continues
executing the current operation". The consequence is named in an upstream issue
open since May 2023 - assertion results do not affect the exit code, so a
successful apply returns 0 despite failing assertions. The result of the failed
check is sitting in the plan file the whole time.

**Two caveats.** The specification carries an explicit warning that "the JSON
representation of checks is experimental and some details may change in future
Terraform versions based on feedback, even in minor releases of Terraform CLI".
And its wording names "resources with postconditions" rather than `check`
blocks, with address kinds limited to resource and output value - so whether a
standalone `check` block appears in the plan-time array at all is **not
established by the documentation**, and must be proved against a real plan
before anything is built on it.

### `resource_drift`

"A description of the changes Terraform detected when it compared the most
recent state to the prior saved state", using the same object structure as
`resource_changes`. Drift is computed at plan time and recorded in the file, so
it is readable afterwards with no credentials at all.

But `terraform plan -refresh=false` "causes Terraform to ignore external
changes, which could result in an incomplete or incorrect plan", and nothing in
the plan JSON records whether refresh ran. **An absent or empty
`resource_drift` is ambiguous** between "nothing has drifted" and "nobody
looked", and rendering it as reassurance is quietly wrong.

### `action_invocations`

Terraform v1.14.0 introduced provider-defined actions - the CHANGELOG's own
example is invoking a Lambda or creating a CloudFront invalidation - which do
something imperative outside Terraform's normal CRUD model. This is a new class
of thing in a plan: not a create, update or destroy, but a side effect that
will fire. Ranking by what a change destroys does not cover it, because an
action invocation destroys nothing while potentially doing anything.

### The undocumented four

`timestamp`, `deferred_changes`, `deferred_action_invocations` and
`action_invocations` appear nowhere on Terraform's JSON output format page.
They exist only in the emitter's source. Anything built on them is built on
unspecified surface and can break on a minor release, so they are weaker ground
than everything above.

## Standing hazards

- **The library lags the emitter.** `hashicorp/terraform-json` says of itself
  that each version "represents a specific snapshot of the Terraform JSON
  output format, and it often slightly lags behind Terraform itself", and that
  it "drops unknown attributes". The skew is measurable: Terraform's emitter is
  on a later format version than the published specification page describes.
- **Terraform and OpenTofu have diverged here and neither flags it.**
  OpenTofu's JSON format page documents `timestamp` and a `deprecated`
  sub-field inside `variables`, and does not document `complete`, `applyable`,
  `deferred_changes` or `action_invocations` at all. Terraken serves both from
  one code path, so every plan-context capability needs an explicit answer for
  the OpenTofu case.
- **No fixture exercises any of this.** Not one fixture in `testdata/` contains
  `resource_drift`, `checks`, `deferred_changes` or `action_invocations`,
  because none was generated from a drifted estate, from a configuration
  carrying check blocks, or under Terraform 1.14 or later with actions. Every
  capability in this layer needs a fixture built first, and building those
  fixtures is a real part of the cost.

## The thread running through all of it

Each of these is the same shape as `unverifiable until apply`, which is already
the tool's most distinctive finding.

An empty `resource_drift` under `-refresh=false` is unknown, not clean. A check
with status `unknown` is undetermined, not passing. A plan with
`complete: false` is a partial account of the change. A plan that errored is
not a plan.

So reading the rest of the file is not a new idea bolted on. It is the existing
idea - say when something cannot be known - applied to the plan as an artefact
rather than to one resource inside it.

## Sources

- [JSON output format](https://developer.hashicorp.com/terraform/internals/json-format) - the plan representation, and the experimental warning on checks
- [check block reference](https://developer.hashicorp.com/terraform/language/block/check) - a failed assertion warns and does not block
- [terraform plan command reference](https://developer.hashicorp.com/terraform/cli/commands/plan) - `-refresh=false` may produce an incomplete or incorrect plan; `-target` in exceptional circumstances only
- [hashicorp/terraform#33174](https://github.com/hashicorp/terraform/issues/33174) - check assertion results do not affect the exit code, open since 2023-05-10
- [Terraform v1.8.0 CHANGELOG](https://github.com/hashicorp/terraform/blob/v1.8.0/CHANGELOG.md) - `applyable` and `complete` introduced
- [Terraform v1.14.0 CHANGELOG](https://github.com/hashicorp/terraform/blob/v1.14.0/CHANGELOG.md) - provider-defined actions
- [hashicorp/terraform `internal/plans/plan.go`](https://github.com/hashicorp/terraform/blob/main/internal/plans/plan.go) - the structural definitions of Complete and Applyable
- [hashicorp/terraform `internal/command/jsonplan/plan.go`](https://github.com/hashicorp/terraform/blob/main/internal/command/jsonplan/plan.go) - the emitter
- [hashicorp/terraform-json](https://github.com/hashicorp/terraform-json) - the `Plan` struct, and the library's own statement that it lags and drops unknown attributes
- [OpenTofu JSON output format](https://opentofu.org/docs/internals/json-format/) - the divergence
