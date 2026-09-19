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
	// outputs is what each module output stands for, unresolved. wants holds
	// every edge until each module has been walked, and moduleWide holds what
	// a module call's own count, for_each or depends_on makes everything
	// inside it depend on.
	// known is every instance address this plan holds, as a set.
	known map[string]bool

	outputs    map[string][]dep
	wants      []want
	moduleWide map[string][]dep
}

// buildGraph reads the plan's configuration and inverts it.
//
// A plan with no configuration block gives an empty graph rather than an
// error. Most of this repository's fixtures carry `"configuration": {}` and a
// plan piped from an older terraform may too; that is a gap in what can be
// known, and the tool's answer to a gap is to say nothing rather than to fail.
func buildGraph(p *tfjson.Plan) *graph {
	g := &graph{edges: map[string][]string{}, outputs: map[string][]dep{}, moduleWide: map[string][]dep{}}
	if p == nil || p.Config == nil || p.Config.RootModule == nil {
		return g
	}
	// The configuration names a resource once however many copies exist, and
	// the change set names every copy. Joining them is what stops the radius
	// naming addresses that are not in the plan - see expand.go.
	g.instances = instancesByConfig(p)
	g.known = knownInstances(p)
	g.walk(p.Config.RootModule, "", nil)
	g.resolveWants()
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
func (g *graph) walk(m *tfjson.ConfigModule, prefix string, vars map[string][]dep) {
	if m == nil {
		return
	}
	for _, r := range m.Resources {
		if r == nil {
			continue
		}
		dependant := prefix + r.Address
		for _, expr := range r.Expressions {
			g.fromExpression(expr, prefix, vars, dependant)
		}
		// COUNT AND FOR_EACH ARE EXPRESSIONS TOO, and Terraform exports them
		// outside `expressions` in their own fields - so walking that map
		// alone missed `count = length(terraform_data.base.input)` entirely.
		// A resource whose very existence depends on another is as dependent
		// as one that reads an attribute.
		g.fromExpression(r.CountExpression, prefix, vars, dependant)
		g.fromExpression(r.ForEachExpression, prefix, vars, dependant)

		// DEPENDS_ON IS A DEPENDENCY SOMEBODY WROTE DOWN. It is not in
		// `expressions`, so walking those never saw it.
		g.fromRefs(r.DependsOn, prefix, vars, dependant)
	}

	// What each of this module's outputs stands for, recorded UNRESOLVED. An
	// output can name another module's output - a wrapper forwarding a child's
	// - and that module may not have been walked, so nothing here can be
	// joined until every module has been read. See resolveDeps.
	for name, out := range m.Outputs {
		if out == nil {
			continue
		}
		key := prefix + "output." + name
		g.outputs[key] = append(g.outputs[key], g.depsOf(refsOf(out.Expression), prefix, vars)...)
		// An output can carry depends_on of its own, and it names resources
		// the output's expression does not mention.
		g.outputs[key] = append(g.outputs[key], g.depsOf(out.DependsOn, prefix, vars)...)
	}

	for name, call := range m.ModuleCalls {
		if call == nil {
			continue
		}
		inner := prefix + "module." + name + "."

		// WHAT THE CALL PASSES IN, resolved in THIS scope and bound to the
		// variable name the module knows it by. Without this a module is a
		// wall: every resource inside it references `var.something` and
		// nothing downstream of the call is connected to anything upstream.
		bound := map[string][]dep{}
		for varName, expr := range call.Expressions {
			bound[varName] = append(bound[varName], g.depsOf(refsOf(expr), prefix, vars)...)
		}

		// A module call's own count, for_each and depends_on apply to
		// EVERYTHING INSIDE IT, so they are bound as a dependency of every
		// resource the module declares rather than of any one of them.
		var whole []dep
		whole = append(whole, g.depsOf(refsOf(call.CountExpression), prefix, vars)...)
		whole = append(whole, g.depsOf(refsOf(call.ForEachExpression), prefix, vars)...)
		whole = append(whole, g.depsOf(call.DependsOn, prefix, vars)...)
		if len(whole) > 0 {
			g.moduleWide[inner] = append(g.moduleWide[inner], whole...)
		}

		g.walk(call.Module, inner, bound)
	}
}

// fromExpression records what one expression's references make this resource
// depend on.
func (g *graph) fromExpression(e *tfjson.Expression, prefix string, vars map[string][]dep, dependant string) {
	if e == nil {
		return
	}
	g.fromRefs(refsOf(e), prefix, vars, dependant)
}

// fromRefs records the dependencies a list of references stands for.
func (g *graph) fromRefs(refs []string, prefix string, vars map[string][]dep, dependant string) {
	for _, d := range g.depsOf(refs, prefix, vars) {
		g.wants = append(g.wants, want{dep: d, dependant: dependant})
	}
}

// dep is one thing a reference stands for: either a resource address, or a
// module output that cannot be resolved until every module has been walked.
type dep struct {
	addr string

	// output is a key into g.outputs. exact distinguishes a reference to one
	// named output from a reference to the whole call, which stands for every
	// output it has.
	output string
	exact  bool
}

// want is an edge waiting for its dependency to resolve.
type want struct {
	dep       dep
	dependant string
}

// depsOf turns the references of ONE expression into what they stand for.
//
// ONLY THE MOST SPECIFIC REFERENCE OF EACH CHAIN, which is the correction that
// matters most here. Terraform exports a reference AND its parents: reading
// `terraform_data.base["a.b"].output` gives both that and a bare
// `terraform_data.base`, and reading `module.apple.a` gives both that and a
// bare `module.apple`. Taking every entry meant the bare one was expanded to
// EVERY instance of the resource, or every output of the module - so selecting
// one instance depended on all of them, and reading one output depended on all
// of them. Terraform's own graph does neither, and a blast radius that claims
// dependencies Terraform does not have is not a floor any more.
//
// A bare reference that stands ALONE is different and is kept: a splat over an
// expanded call, `module.app[*].a`, is exported as `module.app` with nothing
// more specific beside it, and Terraform does connect that through the whole
// call.
func (g *graph) depsOf(refs []string, prefix string, vars map[string][]dep) []dep {
	var out []dep
	for _, ref := range mostSpecific(refs) {
		out = append(out, g.resolve(ref, prefix, vars)...)
	}
	return out
}

// mostSpecific drops any reference that is a proper prefix of another in the
// same list, compared segment by segment so `module.app` is a prefix of
// `module.app.handle` and `module.apple` is not.
//
// AN INDEX COUNTS AS THE SAME SEGMENT. Terraform exports three references for
// `terraform_data.keyed["one"].output`: that, `terraform_data.keyed["one"]`
// and a bare `terraform_data.keyed`. The bare one is not a segment-wise prefix
// of the indexed one - "keyed" and "keyed[\"one\"]" are different strings - so
// comparing them literally left it in, and it expanded to every instance. A
// segment matches when it matches with its index removed.
func mostSpecific(refs []string) []string {
	split := make([][]string, len(refs))
	for i, r := range refs {
		split[i] = splitRef(r)
	}
	var out []string
	for i, a := range split {
		covered := false
		for j, b := range split {
			if i == j || len(b) <= len(a) {
				continue
			}
			same := true
			for k := range a {
				if a[k] == b[k] || a[k] == configAddress(b[k]) {
					continue
				}
				same = false
				break
			}
			if same {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, refs[i])
		}
	}
	return out
}

// resolve turns one reference into what it stands for.
//
// A reference to a resource is itself. A reference to `var.x` is whatever the
// calling module passed in, which is how a dependency travels INTO a module. A
// reference to a module output is held unresolved, because that module may not
// have been walked - and because an output can name another module's output,
// which is how a wrapper forwards a child's.
func (g *graph) resolve(ref, prefix string, vars map[string][]dep) []dep {
	segs := splitRef(ref)
	if len(segs) == 0 {
		return nil
	}
	switch segs[0] {
	case "var":
		if len(segs) < 2 {
			return nil
		}
		return vars[segs[1]]
	case "local", "each", "count", "path", "terraform", "self", "data":
		return nil
	case "module":
		if len(segs) < 2 {
			return nil
		}
		base := prefix + "module." + configAddress(segs[1]) + ".output."
		if len(segs) == 2 {
			return []dep{{output: base}}
		}
		return []dep{{output: base + segs[2], exact: true}}
	}
	if len(segs) < 2 {
		return nil
	}
	return []dep{{addr: prefix + segs[0] + "." + segs[1]}}
}

// splitRef breaks a reference into its segments, keeping an index with the
// name it belongs to.
//
// IT CANNOT BE strings.Split ON ".". A for_each key is an arbitrary string
// written into the reference in quotes, so `terraform_data.base["a.b"].output`
// holds a dot that is not a separator - splitting on it produced the malformed
// graph key `terraform_data.base["a`.
func splitRef(ref string) []string {
	var out []string
	var b strings.Builder
	depth, inQuote, escaped := 0, false, false
	for _, r := range ref {
		switch {
		case escaped:
			escaped = false
			b.WriteRune(r)
		case inQuote && r == '\\':
			escaped = true
			b.WriteRune(r)
		case inQuote:
			b.WriteRune(r)
			if r == '"' {
				inQuote = false
			}
		case r == '"' && depth > 0:
			inQuote = true
			b.WriteRune(r)
		case r == '[':
			depth++
			b.WriteRune(r)
		case r == ']':
			if depth > 0 {
				depth--
			}
			b.WriteRune(r)
		case r == '.' && depth == 0:
			out = append(out, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

// resolveWants joins every held edge, once every module has been walked.
func (g *graph) resolveWants() {
	for _, w := range g.wants {
		for _, addr := range g.addresses(w.dep, map[string]bool{}) {
			g.edge(addr, w.dependant)
		}
	}
	// A module's count, for_each or depends_on applies to everything it
	// declares, so it becomes an edge to every resource inside it - including
	// inside nested calls, which the prefix match covers.
	for inner, deps := range g.moduleWide {
		for _, d := range deps {
			for _, addr := range g.addresses(d, map[string]bool{}) {
				for cfg := range g.instances {
					if strings.HasPrefix(cfg, inner) {
						g.edge(addr, cfg)
					}
				}
			}
		}
	}
	g.wants = nil
}

// addresses is what one dep finally stands for, following module outputs
// through as many forwards as they take.
//
// THE SEEN SET IS NOT OPTIONAL. An output naming an output can be made to
// refer to itself by a hand-edited plan, and a graph builder that hangs on one
// is worse than one that reports nothing.
func (g *graph) addresses(d dep, seen map[string]bool) []string {
	if d.addr != "" {
		return []string{d.addr}
	}
	if d.output == "" || seen[d.output] {
		return nil
	}
	seen[d.output] = true

	var out []string
	if d.exact {
		for _, inner := range g.outputs[d.output] {
			out = append(out, g.addresses(inner, seen)...)
		}
		return out
	}
	// A bare module reference stands for every output of that call.
	for key, deps := range g.outputs {
		if !strings.HasPrefix(key, d.output) {
			continue
		}
		for _, inner := range deps {
			out = append(out, g.addresses(inner, seen)...)
		}
	}
	return out
}

// edge records that every instance of dependant depends on every instance of
// dep, expanding both from configuration addresses to the instances this plan
// holds. A self-reference is not an edge.
//
// A CONFIGURATION WITH NO INSTANCES IS NOT IN THE PLAN. `count = 0` declares a
// resource and produces none, and a module called with `count = 0` declares
// everything inside it and produces none of that either. The blast radius says
// "N resources in this plan depend on it", so naming something the plan does
// not contain makes that sentence false - a reader looking it up finds nothing.
// Both sides are dropped when the plan holds no instance of them.
func (g *graph) edge(dep, dependant string) {
	if dep == "" || dep == dependant {
		return
	}
	deps, ok := g.expand(dep)
	if !ok {
		return
	}
	ons, ok := g.expand(dependant)
	if !ok {
		return
	}
	for _, d := range deps {
		for _, on := range ons {
			if d == on {
				continue
			}
			g.edges[d] = append(g.edges[d], on)
		}
	}
}

// expand turns one address into the instances this plan holds for it, and
// reports false when it holds none.
//
// AN ADDRESS THE PLAN ALREADY HAS IS ITSELF. A reference can name one
// instance - `terraform_data.base["a.b"]` - and stripping its index to look up
// the configuration would expand it back to every instance, which is precisely
// the false dependency this is here to avoid. Only an address the plan does
// NOT hold is treated as a configuration address and expanded.
func (g *graph) expand(addr string) ([]string, bool) {
	if g.known[addr] {
		return []string{addr}, true
	}
	got, ok := g.instances[addr]
	return got, ok
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
const graphNote = "It is read from the references, depends_on, count and for_each in the " +
	"configuration, so it is a floor rather than the whole graph: a dependency that travels " +
	"through a local or a data source is not in it, nor is one to a resource outside this plan, " +
	"nor one nobody wrote down."

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
