package assess

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
)

// loadConfigured reads a fixture that carries a configuration block. The
// blast-radius work is the first thing in this package to read `configuration`
// rather than `resource_changes`, so it needs the whole plan rather than the
// change list the other tests use.
func loadConfigured(t *testing.T, name string) *tfjson.Plan {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var p tfjson.Plan
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return &p
}

func TestDependentsAreTheResourcesThatReferenceYou(t *testing.T) {
	g := buildGraph(loadConfigured(t, "blast-radius.json"))

	// subnet references network, so network's direct dependants include subnet.
	// The fixture's chain is network <- subnet <- {database, cache} and
	// database <- app, with app also referencing subnet directly.
	got := g.dependents("terraform_data.network")
	want := []string{"terraform_data.subnet"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("direct dependants of network:\n got %v\nwant %v", got, want)
	}
}

func TestBlastRadiusIsTransitiveAndCarriesDepth(t *testing.T) {
	g := buildGraph(loadConfigured(t, "blast-radius.json"))

	got := g.reach("terraform_data.network")

	// Everything downstream, each at its shortest distance from the root.
	// app is depth 3 by the subnet->database->app path and depth 2 by the
	// direct subnet->app edge; the shorter one wins, because depth is "how
	// far away is the nearest point this reaches", not "how long is the
	// longest path to it".
	want := []Reached{
		{Address: "terraform_data.subnet", Depth: 1},
		{Address: "terraform_data.app", Depth: 2},
		{Address: "terraform_data.cache", Depth: 2},
		{Address: "terraform_data.database", Depth: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reach of network:\n got %+v\nwant %+v", got, want)
	}
}

func TestReachIsSortedDeterministically(t *testing.T) {
	g := buildGraph(loadConfigured(t, "blast-radius.json"))

	// Same plan, same graph, same ordering - the third contract. Run it
	// repeatedly because a map iteration that leaked into the ordering would
	// pass a single comparison perhaps half the time.
	first := g.reach("terraform_data.network")
	for i := 0; i < 50; i++ {
		if got := g.reach("terraform_data.network"); !reflect.DeepEqual(got, first) {
			t.Fatalf("reach is not deterministic: run %d gave %+v, first gave %+v", i, got, first)
		}
	}

	// Shallowest first, then by address. Depth is the ranking a reviewer
	// wants; the address is only there to break the tie.
	for i := 1; i < len(first); i++ {
		a, b := first[i-1], first[i]
		if a.Depth > b.Depth || (a.Depth == b.Depth && a.Address >= b.Address) {
			t.Fatalf("not sorted by depth then address at %d: %+v then %+v", i, a, b)
		}
	}
}

func TestAResourceNothingDependsOnHasNoReach(t *testing.T) {
	g := buildGraph(loadConfigured(t, "blast-radius.json"))

	// The issue is explicit: a resource with no dependants is NOT reported as
	// having an empty blast radius, it is simply not annotated. An empty slice
	// here is what lets the caller tell "nothing depends on this" from
	// "something does", without a sentinel.
	if got := g.reach("terraform_data.isolated"); len(got) != 0 {
		t.Fatalf("isolated should reach nothing, got %+v", got)
	}
	// A leaf of the chain reaches nothing either.
	if got := g.reach("terraform_data.app"); len(got) != 0 {
		t.Fatalf("app is a leaf and should reach nothing, got %+v", got)
	}
}

func TestReferencesResolveToResourceAddressesNotAttributes(t *testing.T) {
	// Terraform emits BOTH forms for one dependency: the attribute that was
	// read ("terraform_data.subnet.output") and the bare resource
	// ("terraform_data.subnet"). Counting them separately would double every
	// edge and report a blast radius roughly twice its real size.
	g := buildGraph(loadConfigured(t, "blast-radius.json"))

	deps := g.dependents("terraform_data.subnet")
	seen := map[string]int{}
	for _, d := range deps {
		seen[d]++
	}
	for addr, n := range seen {
		if n != 1 {
			t.Fatalf("%s appears %d times in one dependant list - attribute and bare refs were not merged", addr, n)
		}
	}
	for _, d := range deps {
		if _, ok := g.edges[d]; !ok && d != "terraform_data.cache" && d != "terraform_data.app" && d != "terraform_data.database" {
			t.Fatalf("dependant %q is not a resource address", d)
		}
	}
}

func TestAGraphFromAPlanWithNoConfigurationIsEmptyRatherThanAnError(t *testing.T) {
	// Most fixtures here carry `"configuration": {}`, and a plan piped from an
	// older terraform may too. That is a gap in what can be known, not a
	// failure: the tool reports what it can and says nothing it cannot.
	g := buildGraph(loadConfigured(t, "critical.json"))
	if len(g.edges) != 0 {
		t.Fatalf("expected an empty graph, got %d edges", len(g.edges))
	}
	if got := g.reach("azurerm_postgresql_flexible_server.main"); len(got) != 0 {
		t.Fatalf("expected no reach from an empty graph, got %+v", got)
	}
}

func TestACycleTerminates(t *testing.T) {
	// Terraform rejects a dependency cycle, so a real plan cannot contain one.
	// This walks one anyway: the traversal must not be the reason the tool
	// hangs on a malformed or hand-edited file.
	g := &graph{edges: map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {"a"},
	}}
	got := g.reach("a")
	want := []Reached{
		{Address: "b", Depth: 1},
		{Address: "c", Depth: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cycle:\n got %+v\nwant %+v", got, want)
	}
}

func TestFindingsAtOneLevelAreRankedByReach(t *testing.T) {
	// The issue asks for this in as many words: "a replacement that thirty
	// resources depend on is a different event from one that nothing depends
	// on, and the report should say so."
	//
	// It says so by ORDERING, not by escalating a level. critical means the
	// resource type holds data and destroying it loses that data - that is the
	// only escalation in the tool and it must stay that way, or the word stops
	// meaning one thing. Reach breaks the tie WITHIN a level instead, which is
	// where the address used to break it and where nothing else was using the
	// signal.
	r := Assess(loadConfigured(t, "blast-radius-two-roots.json"))

	var order []string
	for _, f := range r.Findings {
		if f.Level == High {
			order = append(order, f.Address)
		}
	}
	if len(order) < 2 {
		t.Fatalf("fixture should have at least two high findings, got %v", order)
	}
	// terraform_data.network reaches four; terraform_data.lonely reaches none.
	// Alphabetically "lonely" sorts first, so if reach were not consulted this
	// would come back the other way round.
	if order[0] != "terraform_data.network" {
		t.Fatalf("expected the widest-reaching finding first, got %v", order)
	}
}
