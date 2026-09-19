package assess

import (
	"sort"
	"strconv"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
)

// Blast radius: what else in this plan depends on a resource, and how far away
// it is.
//
// WHY THIS IS POSSIBLE AT ALL WITHOUT CREDENTIALS, which is the whole point of
// the feature. Every other tool that answers "what breaks if I destroy this"
// either runs `terraform graph` or holds your cloud credentials and asks the
// provider. Neither is available here and neither is needed: the plan file's
// `configuration` object carries an `expressions` map for every resource, and
// each expression exposes a `references` list naming the addresses it depends
// on, ALREADY UNWRAPPED. The dependency graph is therefore sitting in the file
// the tool already reads.
//
// The graph is what the configuration DECLARES. Where that differs from what
// the cloud actually does - an implicit dependency nobody wrote down, a
// resource wired together outside Terraform - this cannot see it, and it must
// never imply otherwise. It is a lower bound on the blast radius, not a
// measurement of it.

// Reached is one resource that depends on another, and how far away it is.
//
// Depth is the SHORTEST path from the origin. A resource reachable both
// directly and through two hops is reported at 1, because the question a
// reviewer is asking is "how close is this to the thing being destroyed",
// not "what is the longest way round to it".
type Reached struct {
	Address string `json:"address"`
	Depth   int    `json:"depth"`
}

// graph is the dependency edges of one plan, pointing from a resource to the
// resources that depend on it - the reverse of how the configuration states
// them, because the question is always "what does destroying this reach".
type graph struct {
	edges map[string][]string

	// instances is every instance address this plan holds, by the
	// configuration address it expands from. See expand.go.
	instances map[string][]string

	// outputs is what each module output resolves to, and pending holds the
	// references to one that could not be joined when they were read. A module
	// may be walked after the caller that reads its output, so those edges are
	// made at the end - see resolvePending.
	outputs map[string][]string
	pending []pendingEdge
}

// pendingEdge is a reference to a module output, held until every module has
// been walked.
type pendingEdge struct {
	output string
	// exact distinguishes a reference to one named output from a reference to
	// the whole call, which stands for every output it has.
	exact     bool
	dependant string
}

// buildGraph reads the plan's configuration and inverts it.
//
// A plan with no configuration block gives an empty graph rather than an
// error. Most of this repository's fixtures carry `"configuration": {}` and a
// plan piped from an older terraform may too; that is a gap in what can be
// known, and the tool's answer to a gap is to say nothing rather than to fail.
func buildGraph(p *tfjson.Plan) *graph {
	g := &graph{edges: map[string][]string{}, outputs: map[string][]string{}}
	if p == nil || p.Config == nil || p.Config.RootModule == nil {
		return g
	}
	// The configuration names a resource once however many copies exist, and
	// the change set names every copy. Joining them is what stops the radius
	// naming addresses that are not in the plan - see expand.go.
	g.instances = instancesByConfig(p)
	g.walk(p.Config.RootModule, "", nil)
	g.resolvePending()
	// One pass to make every list unique and ordered. Doing it here rather
	// than on every read keeps `reach` free of allocation-heavy bookkeeping
	// and makes the ordering a property of the graph rather than of the
	// traversal.
	for k, v := range g.edges {
		g.edges[k] = dedupeSorted(v)
	}
	return g
}

// walk adds every edge in a module and then recurses into its module calls.
//
// prefix is the module path as terraform writes it in an address, so a
// resource inside `module.db` comes out as `module.db.terraform_data.main`.
// Without it, two resources of the same name in different modules would
// collapse into one node and the reported radius would be wrong in both.
func (g *graph) walk(m *tfjson.ConfigModule, prefix string, vars map[string][]string) {
	if m == nil {
		return
	}
	for _, r := range m.Resources {
		if r == nil {
			continue
		}
		dependant := prefix + r.Address
		for _, expr := range r.Expressions {
			for _, ref := range refsOf(expr) {
				// A resource can reference the same thing from several
				// expressions; dedupeSorted collapses that later.
				g.refEdge(ref, prefix, vars, dependant)
			}
		}
		// DEPENDS_ON IS A DEPENDENCY SOMEBODY WROTE DOWN. It is not in
		// `expressions`, so walking those never saw it, and the standing
		// caveat only ever admitted to missing the ones nobody wrote.
		for _, ref := range r.DependsOn {
			g.refEdge(ref, prefix, vars, dependant)
		}
	}

	// What each of this module's outputs resolves to, so a reference to
	// `module.app.handle` in the caller reaches the resource behind it.
	// Recorded rather than joined now, because the caller may have been walked
	// already - see resolvePending.
	for name, out := range m.Outputs {
		if out == nil {
			continue
		}
		key := strings.TrimSuffix(prefix, ".")
		if key != "" {
			key += "."
		}
		for _, ref := range refsOf(out.Expression) {
			g.outputs[key+"output."+name] = append(g.outputs[key+"output."+name],
				g.resolve(ref, prefix, vars)...)
		}
	}

	for name, call := range m.ModuleCalls {
		if call == nil {
			continue
		}
		// WHAT THE CALL PASSES IN, resolved in THIS scope and bound to the
		// variable name the module knows it by. Without this a module is a
		// wall: every resource inside it references `var.something` and
		// nothing downstream of the call is connected to anything upstream of
		// it, which is how most real estates wire modules together.
		inner := map[string][]string{}
		for varName, expr := range call.Expressions {
			for _, ref := range refsOf(expr) {
				inner[varName] = append(inner[varName], g.resolve(ref, prefix, vars)...)
			}
		}
		g.walk(call.Module, prefix+"module."+name+".", inner)
	}
}

// refEdge records an edge from whatever a reference resolves to.
func (g *graph) refEdge(ref, prefix string, vars map[string][]string, dependant string) {
	// A reference to another module's output cannot be resolved yet: that
	// module may not have been walked. Held and joined at the end.
	if out, exact, ok := outputRef(ref, prefix); ok {
		g.pending = append(g.pending, pendingEdge{output: out, exact: exact, dependant: dependant})
		return
	}
	for _, dep := range g.resolve(ref, prefix, vars) {
		g.edge(dep, dependant)
	}
}

// resolve turns one reference into the resource addresses it stands for.
//
// A reference to a resource is itself. A reference to `var.x` is whatever the
// calling module passed in for x, which is how a dependency travels INTO a
// module. Anything else - a local, a data source this build does not follow,
// an input with no binding - resolves to nothing, which the standing caveat
// says out loud.
func (g *graph) resolve(ref, prefix string, vars map[string][]string) []string {
	if name, ok := strings.CutPrefix(ref, "var."); ok {
		// `var.x.y` is an attribute of the variable, and the binding is the
		// same either way.
		if i := strings.IndexByte(name, '.'); i >= 0 {
			name = name[:i]
		}
		return vars[name]
	}
	if addr := normaliseRef(ref, prefix); addr != "" {
		return []string{addr}
	}
	return nil
}

// outputRef reports whether a reference names another module's output, and
// returns the key it was recorded under and whether that key is exact.
//
// TWO SHAPES, AND THE SECOND IS THE COMMON ONE. `module.app.handle` names one
// output. A splat over an expanded call - `module.app[*].handle` - arrives as
// `module.app` with no output name at all, because Terraform records the
// reference against the call rather than the attribute. That is not "no
// output": it is every output, so the key is a prefix and every output of that
// module matches it.
func outputRef(ref, prefix string) (key string, exact, ok bool) {
	if !strings.HasPrefix(ref, "module.") {
		return "", false, false
	}
	parts := strings.Split(ref, ".")
	if len(parts) < 2 {
		return "", false, false
	}
	base := prefix + "module." + parts[1] + ".output."
	if len(parts) == 2 {
		return base, false, true
	}
	return base + parts[2], true, true
}

// resolvePending joins the references that named a module output, once every
// module has been walked and every output is known.
func (g *graph) resolvePending() {
	for _, p := range g.pending {
		if p.exact {
			for _, dep := range g.outputs[p.output] {
				g.edge(dep, p.dependant)
			}
			continue
		}
		for key, deps := range g.outputs {
			if !strings.HasPrefix(key, p.output) {
				continue
			}
			for _, dep := range deps {
				g.edge(dep, p.dependant)
			}
		}
	}
	g.pending = nil
}

// edge records that every instance of dependant depends on every instance of
// dep, expanding both from configuration addresses to the instances this plan
// holds. A self-reference is not an edge.
func (g *graph) edge(dep, dependant string) {
	if dep == "" || dep == dependant {
		return
	}
	for _, d := range expand(g.instances, dep) {
		for _, on := range expand(g.instances, dependant) {
			if d == on {
				continue
			}
			g.edges[d] = append(g.edges[d], on)
		}
	}
}

// refsOf pulls every reference out of one expression, including the nested
// ones.
//
// An expression is not always a leaf. A block written as a list of objects
// arrives as NestedBlocks, and one written as a single block as NestedBlocks
// with a single entry - so a reference inside `dynamic` content or a nested
// `setting {}` block lives one or more levels down. Reading only the top level
// would silently miss those edges, which is the kind of gap that makes a blast
// radius look smaller than it is.
// The nil check is on the EMBEDDED POINTER, not only on e. tfjson.Expression
// is a struct wrapping *ExpressionData, so a non-nil Expression with a nil
// embed panics on e.References rather than returning empty - and the library
// produces exactly that for an expression it could not decode.
func refsOf(e *tfjson.Expression) []string {
	if e == nil || e.ExpressionData == nil {
		return nil
	}
	out := append([]string(nil), e.References...)
	for _, block := range e.NestedBlocks {
		for _, inner := range block {
			out = append(out, refsOf(inner)...)
		}
	}
	return out
}

// normaliseRef turns one reference into the address of the resource it names,
// or "" if it does not name a resource in this plan.
//
// TERRAFORM EMITS BOTH FORMS FOR A SINGLE DEPENDENCY. Referencing
// `terraform_data.subnet.output` produces two entries - the attribute read and
// the bare resource - and counting them separately would double every edge and
// report a blast radius about twice its true size. Both normalise to the same
// address here.
//
// What is deliberately dropped:
//
//	var.x, local.y, each.key, count.index, path.module   not resources
//	data.foo.bar                                         a read, not a thing
//	                                                     that can be destroyed
//
// A data source is the interesting exclusion. It genuinely depends on the
// resource, but it is not something a destroy takes down, and listing it among
// the casualties would overstate the damage.
func normaliseRef(ref, prefix string) string {
	parts := strings.Split(ref, ".")
	if len(parts) < 2 {
		return ""
	}
	switch parts[0] {
	case "var", "local", "each", "count", "path", "terraform", "self", "data":
		return ""
	case "module":
		// module.db.output_name -> the module call, not a resource. An edge
		// through a module output is real, but resolving it needs the output's
		// own expression, and reporting `module.db` as a casualty would name
		// something that is not a resource.
		return ""
	}
	// resource_type.name[...] - the index is part of the address terraform
	// prints, so `terraform_data.node[0]` stays distinct from `node[1]`.
	addr := parts[0] + "." + parts[1]
	return prefix + addr
}

// dependents returns the resources that directly depend on an address.
func (g *graph) dependents(addr string) []string {
	return g.edges[addr]
}

// reach returns everything that transitively depends on an address, each at
// its shortest depth, shallowest first and then by address.
//
// Breadth-first, so the first time a node is seen is by definition its
// shortest path - which is what makes Depth cheap and correct at the same
// time. The seen set also makes a cycle terminate: terraform rejects a
// dependency cycle so a real plan cannot contain one, but a hand-edited or
// malformed file must not be the reason this hangs.
func (g *graph) reach(addr string) []Reached {
	seen := map[string]int{addr: 0}
	queue := []string{addr}
	var out []Reached

	for depth := 1; len(queue) > 0; depth++ {
		var next []string
		for _, cur := range queue {
			for _, dep := range g.edges[cur] {
				if _, ok := seen[dep]; ok {
					continue
				}
				seen[dep] = depth
				out = append(out, Reached{Address: dep, Depth: depth})
				next = append(next, dep)
			}
		}
		queue = next
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Depth != out[j].Depth {
			return out[i].Depth < out[j].Depth
		}
		return out[i].Address < out[j].Address
	})
	return out
}

// blastAnnotation describes what a destructive change reaches, or returns
// false when it reaches nothing.
//
// ONLY DESTRUCTIVE CHANGES GET THIS. An update in place does not take its
// dependants down with it, so annotating one with a list of resources that
// merely reference it would be noise dressed up as a warning - and the whole
// register of this tool is that it says what is true and stops.
//
// A resource nothing depends on is NOT annotated with an empty radius. The
// issue is explicit about it, and it is right: "0 dependants" printed under
// thirty findings is thirty lines that teach a reader to skip the annotation
// entirely, which costs more than it gives on the one finding that matters.
func blastAnnotation(g *graph, addr string, kind Kind) (Annotation, bool) {
	switch kind {
	case KindDelete, KindReplace:
	default:
		return Annotation{}, false
	}

	reached := g.reach(addr)
	if len(reached) == 0 {
		return Annotation{}, false
	}

	direct := 0
	for _, r := range reached {
		if r.Depth == 1 {
			direct++
		}
	}

	paths := make([]string, 0, len(reached))
	for _, r := range reached {
		paths = append(paths, r.Address)
	}

	return Annotation{
		Code:    AnnBlastRadius,
		Summary: blastSummary(len(reached), direct),
		Detail:  blastSummary(len(reached), direct) + ". " + blastNote,
		Note:    blastNote,
		Paths:   paths,
		Reached: reached,
	}, true
}

func blastSummary(total, direct int) string {
	switch {
	case total == 1:
		return "1 resource in this plan depends on it"
	case direct == total:
		return resources(total) + " in this plan depend on it directly"
	default:
		return resources(total) + " in this plan depend on it, " + itoa(direct) + " directly"
	}
}

// resources names a count of resources, in the singular when there is one.
// Shared with sequence.go so the two cannot disagree about how to count.
func resources(n int) string {
	if n == 1 {
		return "1 resource"
	}
	return itoa(n) + " resources"
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

// The caveat, and it is not boilerplate. This graph is what the CONFIGURATION
// declares. An implicit dependency nobody wrote down, or two resources wired
// together outside Terraform, is invisible here - so the number is a floor on
// the blast radius rather than a measurement of it, and a reader who takes it
// for the latter has been misled by us rather than by the plan.
// graphNote is the boundary of the dependency graph, and it is shared by every
// annotation read off that graph.
//
// ONE SENTENCE IN ONE PLACE, because two annotations reading one graph must not
// be able to describe its limits differently - and they did once.
//
// IT IS A FLOOR RATHER THAN A LIST OF EXCEPTIONS. Enumerating the ways a
// dependency can be missed invites a reader to assume the list is complete.
//
// WHAT IT USED TO ADMIT AND NO LONGER HAS TO. depends_on is read, a resource
// expanded by count or for_each is joined to its instances, and a dependency
// that travels through a module - in through a call's inputs, out through its
// outputs - is followed. #57. What remains unread is a local, whose definition
// Terraform does not put in the exported configuration at all, and a reference
// that passes through a data source.
const graphNote = "It is read from the references and depends_on in the configuration, so it is a " +
	"floor rather than the whole graph: a dependency that travels through a local or a data source " +
	"is not in it, nor is one to a resource outside this plan, nor one nobody wrote down."

const blastNote = "This is what the configuration declares, counted within this plan. " + graphNote

func dedupeSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	sort.Strings(in)
	out := in[:1]
	for _, s := range in[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}
