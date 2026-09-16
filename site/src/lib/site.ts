// terraken.dbhq.uk - the structured copy for the site.
//
// WHY THIS SITE EXISTS, AND WHY IT IS SHAPED THE WAY IT IS.
//
// It is a search site, not a brochure. The information architecture comes from
// measured demand rather than from how the tool is described internally.
// Worldwide Google Ads volume, English, measured 16 September 2026 and recorded
// in full in docs/research/terraken-seo-worldwide.md:
//
//   terraform moved block                      1,300 / month
//   terraform moved                            1,300
//   terraform taint                            1,300
//   terraform state rm                         1,000
//   terraform replace                            880
//   terraform state mv                           590
//   terraform untaint                            210
//   terraform rename resource                    210
//   terraform forces replacement                  90
//   terraform force replacement                   90
//
// and, measured the same way and returning nothing at all:
//
//   terraform plan review          0
//   terraform plan risk            0
//   review terraform plan          0
//   terraform plan noise           0
//   terraform state mv vs moved    0
//
// So the anchor of this site is /terraform-moved-block/, and the four guides
// beside it are the terms the tool has a true claim on. "Plan review" is how
// Terraken is positioned internally and it is not a phrase anybody types, so it
// is not what the pages are built around. Do not restructure this site around
// the internal positioning: the numbers above are the reason it is not.
//
// TERMS DELIBERATELY NOT TARGETED, and they are the bigger ones: terraform
// import (5,400), terraform destroy (2,400), terraform lifecycle (1,900) and
// terraform for_each (1,600). The tool does not import, the destroy intent is
// mostly "how do I" rather than "why is it", and the last two are language
// features rather than problems Terraken solves. Ranking for a query the tool
// cannot help with is worse than not ranking.
//
// WHAT MAY BE SAID HERE. Every technical claim on this site is true of the
// SHIPPED tool - checked against ~/dbhq-uk/terraken, not against its README
// alone, and not assumed. The terminal output on these pages was produced by
// running the built binary against the fixtures in that repo's testdata/, not
// written by hand.
//
// THAT RULE USED TO END "and the open issues appear nowhere on this site".
// It no longer does (Dan, 16 Sep 2026): the tool's direction is a much larger
// thing than today's plan reader, and a site that hides it describes a
// different product from the one being built.
//
// The rule that replaces it is narrower and harder. Shipped and unshipped live
// in SEPARATE exports - `contracts`, `levels`, `flags` and `findings` are all
// true today; `roadmap` is not shipped, every entry carries its issue number,
// and it renders under a heading that says so. Nothing from `roadmap` may be
// written in the present tense, mixed into a shipped list, or used to describe
// what the tool does. tests/content.test.mjs asserts the separation.
//
// This matters more here than on most sites. The entire proposition is that
// this tool can be trusted about a change nobody has vetted. A site that
// oversells by one feature has spent exactly the thing it is selling.

/** The release the site documents. Bump this and the samples together. */
export const VERSION = "v0.5.0";

export const REPO = "https://github.com/dbhq-uk/terraken";

export const site = {
  url: "https://terraken.dbhq.uk",
  name: "Terraken",
  // The one line. No trailing full stop: it is a subtitle, not a sentence.
  //
  // It used to be "Read a Terraform plan and find out what it actually does",
  // which described the first command rather than the tool. The line below is
  // the tool's own stated direction, from issue #13, and it does the work a
  // tagline should: it says what you get AND why the tool is shaped the way it
  // is. Asking nobody's permission is why there are no credentials, no network
  // call and no apply - the constraint and the promise are the same sentence.
  tagline: "Everything you can know about a change, without asking permission",
  lead:
    "Terraken is a free, open-source command-line tool for Terraform and OpenTofu. Hand it a plan and it tells you what the change actually does, ranked by how much damage it can do. It takes a file and nothing else: no credentials, no network, no apply, and no attribute value in the output.",
  // The sibling cross-link block every DBHQ property carries in its footer.
  // dbhq.uk itself carries the siblings in its Explore navigation instead; the
  // subdomains each carry the block, and this site joins them rather than
  // inventing a different treatment. Each entry says what the site is for.
  also: [
    { label: "DBHQ", href: "https://dbhq.uk/", desc: "the practice that publishes this." },
    {
      label: "skills",
      href: "https://skills.dbhq.uk/",
      desc: "free agent skills for Claude Code and Codex.",
    },
    {
      label: "heliograph",
      href: "https://heliograph.dbhq.uk/",
      desc: "debug a remote machine through an operator, using Git.",
    },
  ],
} as const;

// ---------------------------------------------------------------------------
// Navigation. Six links, so it is still a row and not a navigation system: no
// dropdown, no burger, no focus management to get wrong. The labels are short
// because the row has to fit beside the lockup on a laptop, and it wraps below
// that. The footer renders this same list, which is what keeps every page one
// click from every other.
//
// The three pages added on 16 Sep 2026 come from measured worldwide demand
// (docs/research/terraken-seo-worldwide.md): taint and untaint at 1,510 a
// month, state mv and state rm at 1,590, and the replacement cluster at 1,150.
// Each is a question the tool genuinely answers rather than a keyword the site
// is reaching for.
// ---------------------------------------------------------------------------

export interface NavItem {
  path: string;
  label: string;
  /** Breadcrumb label and <title> crumb. */
  crumb: string;
}

export const nav: readonly NavItem[] = [
  { path: "/terraform-moved-block/", label: "moved blocks", crumb: "Terraform moved blocks" },
  { path: "/terraform-rename-resource/", label: "renaming", crumb: "Renaming a resource" },
  { path: "/terraform-state-mv/", label: "state mv", crumb: "state mv and state rm" },
  { path: "/terraform-taint/", label: "taint", crumb: "taint and untaint" },
  {
    path: "/terraform-forces-replacement/",
    // "replacement", not "forces replacement". It was half again the length of
    // every other label and pulled the centred row off balance; the page it
    // goes to still carries the full phrase, which is the term people search.
    label: "replacement",
    crumb: "Forced replacement",
  },
  { path: "/docs/", label: "docs", crumb: "Docs" },
];

// ---------------------------------------------------------------------------
// Install. Three routes, and the third is the one worth reading: piping the
// plan in means it never lands on disk at all.
// ---------------------------------------------------------------------------

export const install = {
  go: "go install github.com/dbhq-uk/terraken/cmd/terraken@latest",
  releases: `${REPO}/releases`,
  /** The release ships two binaries built from the same package. */
  binaries: "terraken and tken",
  // Every platform terraform ships for, which is the rule .goreleaser.yaml
  // states: this tool reads what `terraform show -json` emits, so it has no
  // business claiming a platform terraform does not support and no excuse for
  // missing one it does. Sixteen builds as of v0.3.0, up from four.
  platforms: "Linux, macOS, Windows, FreeBSD, OpenBSD and Solaris",
  use: ["terraform plan -out tfplan", "terraform show -json tfplan > plan.json", "terraken plan.json"],
  pipe: "terraform show -json tfplan | terraken -",
} as const;

// ---------------------------------------------------------------------------
// THE THREE CONTRACTS. These are the trust proposition, and they are the
// reason the tool is safe to point at a plan nobody has vetted. They are not
// marketing lines: each one is held by a test in the tool's own repository, and
// AGENTS.md there calls them the constraints that must not be broken.
// ---------------------------------------------------------------------------

export interface Contract {
  h: string;
  p: string;
}

export const contracts: readonly Contract[] = [
  {
    h: "It takes a file, and runs nothing",
    p: "Terraken reads a plan file, or the same JSON piped in on standard input. It never runs terraform, never reads a cloud credential, never makes a network call and never applies anything. The only file it writes is the one you name with --out, created mode 0600 because the report lists every resource in the plan.",
  },
  {
    h: "It never prints an attribute's value, in any format",
    p: "Not masked, not redacted, not truncated. Values are not in the output at all. Masking depends on Terraform having marked a value sensitive, and that marking is best-effort: a live credential was found in a real plan that Terraform had not marked. Paths, counts, levels and Terraken's own sentences are all it will ever show you.",
  },
  {
    h: "It is deterministic, with no model in the loop",
    p: "The same plan always produces the same verdict. Nothing is sent anywhere, there is no model to talk you round, and no ranking that cannot be read straight off the plan. Findings are sorted most severe first with ties broken on the resource address, so two runs of the same plan are byte for byte identical.",
  },
];

// ---------------------------------------------------------------------------
// THE SELECTION RULE, and the work it admits.
//
// This is the unusual thing about the roadmap and the reason it is worth
// printing: the capabilities below were not chosen because they were wanted.
// They were chosen because each one can be built without giving up one of the
// three contracts above. Of roughly a hundred distinct capabilities in this
// ecosystem, about ten survive that test. The rest need credentials, execution,
// a network call or state mutation, and taking any of them would end the first
// contract for every command rather than just the new one.
//
// NOTHING BELOW IS SHIPPED. Every entry carries its issue number, the heading
// above it says so, and a test asserts both. See the note at the top of this
// file about what may be said here.
// ---------------------------------------------------------------------------

export interface RoadmapItem {
  /** Issue number on dbhq-uk/terraken. */
  issue: number;
  h: string;
  p: string;
}

export const roadmap: readonly RoadmapItem[] = [
  {
    issue: 5,
    h: "The shape of a plan, before the findings",
    p: "How big this change is and what kind of change it is, in a line or two, before the list starts. A reviewer decides how much attention a plan deserves before reading any of it.",
  },
  {
    issue: 6,
    h: "Your own rules, evaluated over a plan",
    p: "Teams have rules that are theirs rather than everyone's - never destroy anything in this account, this tag is mandatory. A deterministic evaluation of rules you wrote, with no policy service and no account to sign up for.",
  },
  {
    issue: 7,
    h: "A gate an agent cannot talk its way past",
    p: "An exit code decided by the plan rather than by argument. As more changes are proposed by agents, the useful property is a check whose answer does not move because something articulate disagreed with it.",
  },
  {
    issue: 8,
    h: "Credentials Terraform did not mark sensitive",
    p: "Marking is best-effort, and a live credential was found in a real plan that Terraform had left unmarked. Finding them is a detection problem, not a printing one - it can say a value at this path looks like a credential without ever showing it.",
  },
  {
    issue: 9,
    h: "Two plans, and what actually resolved",
    p: "Re-plan after a fix and the question is which findings went away, which are new, and which are exactly as they were. That is a comparison of two files, which is still two files.",
  },
  {
    issue: 10,
    h: "One report across many roots",
    p: "Estates are split across many Terraform roots and a change often touches several. One ranked report over all of them, rather than a terminal window per directory.",
  },
  {
    issue: 11,
    h: "Cost delta, from a price sheet on disk",
    p: "What this change does to the bill, computed against a price file you supply. Every other tool in this space asks for an API key; a price sheet is a file, and a file is inside the contract.",
  },
  {
    issue: 12,
    h: "Evidence, for teams that must show their working",
    p: "Regulated change control needs an artefact saying what was reviewed, when, and what it said. Deterministic output is exactly what makes such an artefact worth anything.",
  },
];

/** Heading and framing for the roadmap section. Kept here so the page cannot
 *  render the list under a heading that fails to say it is unshipped. */
export const roadmapIntro = {
  kicker: "Not yet built",
  h: "Where this is going",
  p: "Terraken ships one command today and the rest of this page describes it accurately. This is the rest of the plan, and each item links to the issue tracking it. What makes the list worth reading is not its length but its edges: every capability here was picked because it can be built without giving up one of the three contracts above. Roughly a hundred things a Terraform tool could do were considered; about ten survive that test, and two of them - blast radius and the proposed moved block - have shipped and moved up the page.",
  outro:
    "Linting, formatting, security scanning, documentation and orchestration are all deliberately absent. Each is held by a good tool with years of accumulated rules, and aggregating them means inheriting the maintenance without earning the credibility. The aim is not to own your session - terraform already does that. It is to be the thing you hand a plan to when you need to know what it really says.",
} as const;

// ---------------------------------------------------------------------------
// The four levels. There is deliberately no medium: a middle bucket is where
// findings go to be ignored. (internal/assess/level.go)
// ---------------------------------------------------------------------------

export interface LevelRow {
  name: string;
  what: string;
}

export const levels: readonly LevelRow[] = [
  {
    name: "critical",
    what: "A destroy or a replacement of a resource type that holds data, so destroying it loses that data. This is the only escalation in the tool, and it applies to destruction alone: updating a database in place does not lose data.",
  },
  {
    name: "high",
    what: "Any other destroy, or any replacement. The resource goes away and comes back, whatever is or is not inside it.",
  },
  {
    name: "low",
    what: "An update in place, or a resource that is being forgotten from state but left running.",
  },
  {
    name: "info",
    what: "A create, a data source read, an import and a no-op. Nothing existing is being taken away.",
  },
];

// ---------------------------------------------------------------------------
// Flags, checked against cmd/terraken/main.go rather than copied from the
// README. Nothing here is a flag the shipped binary does not have.
// ---------------------------------------------------------------------------

export interface Flag {
  flag: string;
  what: string;
}

export const flags: readonly Flag[] = [
  {
    flag: "--format terminal|md|json|html",
    what: "Output format. Default terminal. Any other value is an error rather than a fallback.",
  },
  {
    flag: "--out <path>",
    what: "Write the report to a file instead of standard output, and print one line naming it. Works for every format. The file is created mode 0600 and never contains colour.",
  },
  {
    flag: "--fail-on critical|high|low|info",
    what: "Exit 1 if any finding reaches this level. Off by default, and measured against every finding rather than against what was displayed.",
  },
  {
    flag: "--min-level critical|high|low|info",
    what: "Only show findings at this level or above. The counts stay complete and the report says how many were held back. It changes what you read, never what was found and never the exit code.",
  },
  {
    flag: "--plain",
    what: "No colour, and ASCII only: no box drawing anywhere in the output. For a pipeline, a log viewer, or a console that renders box characters badly.",
  },
  {
    flag: "--no-colour, --no-color",
    what: "Never colour terminal output. The British spelling is canonical and the American one is an alias, so guessing wrong does not cost you a run.",
  },
  {
    flag: "--moved",
    what: "Instead of the report, print the moved blocks this plan looks like it forgot, as HCL. Redirect it into a .tf file. Where more than one create matches the deleted resource equally well, it refuses and says so rather than guessing - the block is copy-pasteable, and naming the wrong resource adopts a decommissioned object's state under a live address.",
  },
  {
    flag: "--version",
    what: "Print the version and exit, without needing a plan.",
  },
];

// ---------------------------------------------------------------------------
// Annotation codes, from internal/assess/finding.go. These are the machine
// names a JSON consumer sees, so they are documented as data rather than as
// prose that could drift from the constant.
// ---------------------------------------------------------------------------

export interface Annotation {
  code: string;
  what: string;
}

export const annotations: readonly Annotation[] = [
  {
    code: "blast-radius",
    what: "What else in the plan depends on a resource being destroyed or replaced, transitively, with the nearest distance to each. Set on destructive changes only. A resource nothing depends on is not annotated rather than annotated with a zero.",
  },
  {
    code: "possible-missed-moved-block",
    what: "A destroy and a create that look like one resource renamed without a moved block. It carries the two addresses, the matched and compared attribute counts, and whether the pairing crossed a module boundary.",
  },
  {
    code: "unverifiable-until-apply",
    what: "Attributes Terraform cannot know until apply, named by path. No claim about them can be checked in review, and saying so is the feature.",
  },
  {
    code: "sensitive",
    what: "Attributes Terraform marked sensitive, named by path. The values are not printed, and neither is anything else.",
  },
  {
    code: "unrecognised-provider",
    what: "A resource being destroyed whose provider is not one Terraken has been curated against, so whether destroying it loses data has not been assessed. An unrecognised type is never assumed safe.",
  },
  {
    code: "same-elements-reordered",
    what: "A changed list holds the same elements in a different order. A fact, not a verdict: order is significant for a container command or an ordered rule list, so the call is yours.",
  },
  {
    code: "same-json-written-differently",
    what: "Both sides parse as JSON and hold the same data with the keys in a different order. Policy documents and anything a provider round-trips as a JSON blob.",
  },
  {
    code: "same-text-different-whitespace",
    what: "Both sides are the same text laid out differently: a trailing newline, an indent, a CRLF against an LF.",
  },
  {
    code: "same-number-written-differently",
    what: "Both sides are the same number written another way, such as 80 against the string form of 80.",
  },
  {
    code: "null-on-one-side-empty-on-the-other",
    what: "One side is null and the other is an empty list, object or string.",
  },
  {
    code: "every-changed-attribute-written-differently",
    what: "The roll-up: every attribute the plan shows as changed on this resource fell into one of the classes above. It is a strict claim, so one real change anywhere on the resource silences it.",
  },
];

// ---------------------------------------------------------------------------
// Real output, produced by running the built binary against the fixtures in
// dbhq-uk/terraken testdata/. NOT WRITTEN BY HAND, and not to be edited by
// hand either: regenerate it if the tool's rendering changes.
//
//   COLUMNS=78 terraken testdata/critical.json
//   COLUMNS=78 terraken testdata/rename-no-moved.json
//   COLUMNS=78 terraken testdata/written-differently.json
//   COLUMNS=78 terraken --min-level high testdata/demo.json
//   terraken --format json testdata/critical.json
//   terraken --format md testdata/critical.json
// ---------------------------------------------------------------------------

// Imported as raw HTML, generated by assets/_gen/samples.py in this repository
// by running the built binary with FORCE_COLOR and converting its ANSI to
// spans. They were plain text until 16 Sep 2026, which showed a monochrome
// version of a report whose colour is load-bearing: red means this change can
// destroy something.
//
// A generated .ts module, not .html files imported with ?raw: the tests load
// this file through plain Node, which cannot resolve a .html import at all.
// Regenerate them whenever the renderer changes - the capture must always be
// what the binary actually prints.
import { critical, heroCritical, missedMove, rewritten, minLevel } from "./samples.ts";

export const samples = {
  critical,
  // The same report at 64 columns, for the hero's narrower column. A real
  // width the tool prints at, not the 78 squeezed by CSS.
  heroCritical,
  missedMove,
  rewritten,
  minLevel,
  json: `{
  "terraform_version": "1.9.8",
  "format_version": "1.2",
  "findings": [
    {
      "address": "azurerm_postgresql_flexible_server.main",
      "type": "azurerm_postgresql_flexible_server",
      "provider": "registry.terraform.io/hashicorp/azurerm",
      "kind": "replace",
      "level": "critical",
      "reason": "an attribute changed that cannot be updated in place",
      "replace_paths": [
        "zone"
      ],
      "data_loss": true
    }
  ],
  "counts": {
    "critical": 1
  }
}`,

  // NOT terminal captures and deliberately still plain text: markdown and
  // JSON carry no colour, so there is nothing for the generator to convert
  // and nothing a reader loses by them being strings.
  markdown: `| Level | Change | Resource | Notes |
|---|---|---|---|
| CRITICAL | destroy and create | \`azurerm_postgresql_flexible_server.main\` | holds data, so destroying it loses that data; an attribute changed that cannot be updated in place; forces replacement: \`zone\` |

1 finding.`,
} as const;

// ---------------------------------------------------------------------------
// The GitHub Action's inputs, from action.yml in the tool's own repository.
// ---------------------------------------------------------------------------

export const actionInputs: readonly { name: string; what: string }[] = [
  { name: "plan", what: "Path to the plan JSON, or - to read it from standard input. Required." },
  {
    name: "fail-on",
    what: "Exit 1 if any finding reaches this level. Empty by default, which is no gate at all.",
  },
  {
    name: "summary",
    what: "Append the markdown report to the job summary. True by default.",
  },
  {
    name: "version",
    what: "Which release to install. Defaults to the ref the action was pinned at, so pinning the action pins the binary and the two cannot disagree.",
  },
];

// ---------------------------------------------------------------------------
// FAQs. These are rendered on the page AND published as FAQPage structured
// data, from this one definition - a page that answers a question to a reader
// and hides it from an engine is only half the job, and two copies of an
// answer is how one of them goes stale.
// ---------------------------------------------------------------------------

export interface Faq {
  q: string;
  a: string;
}

export const movedFaqs: readonly Faq[] = [
  {
    q: "What is a moved block in Terraform?",
    a: "A moved block records that an object in your state used to live at one address and now lives at another. You write it in your configuration with a from address and a to address, Terraform reads it during the next plan, and it updates the state entry instead of planning a destroy and a create. It was added in Terraform 1.1 and OpenTofu carries it too.",
  },
  {
    q: "Why does Terraform destroy and recreate a resource when I rename it?",
    a: "Because the address is the identity. Terraform's state maps an address such as aws_db_instance.db to one real remote object, and nothing else in the entry ties it to your configuration. Rename it to aws_db_instance.database and Terraform sees a state entry with no configuration, which it plans to destroy, and a configuration with no state, which it plans to create. It is not looking at the name and deciding: it genuinely cannot tell the rename apart from a deletion and an unrelated addition.",
  },
  {
    q: "Where does a moved block go?",
    a: "In the module whose addresses it names. A rename inside one module goes in that module. A move into or out of a child module goes in the parent that calls it, because that is the only place where both addresses can be written. A moved block takes literal addresses, so it cannot be generated with for_each and cannot take a variable.",
  },
  {
    q: "Do I have to keep the moved block after applying?",
    a: "Keep it until everyone who runs the configuration has applied it, including CI and every other workspace. A moved block whose from address is not in the state does nothing at all, so leaving it in place costs nothing and removing it too early costs a destroy for whoever had not applied yet. In a module you publish, keep it until the next major version, because a consumer can upgrade from any earlier one.",
  },
  {
    q: "When should I use a moved block instead of terraform state mv?",
    a: "Almost always. A moved block is code: it is reviewed in the pull request, it is versioned with the change it belongs to, and it runs for everyone and every workspace on the next plan. terraform state mv is one command, run once, by one person, against one state, with nothing in the repository to show it happened, and it needs write access to that state. Reach for state mv only where the change cannot be expressed in configuration at all, such as moving an object between two separate state files.",
  },
  {
    q: "Can a moved block change a resource's type?",
    a: "No. Terraform can move an object to a different address of the same type, not to a different type. Changing the type means a genuinely different resource with a different schema, and there is nothing to carry the old state entry across. That case is an import, or a destroy and a create.",
  },
  {
    q: "What happens if a moved block points at the wrong resource?",
    a: "Terraform believes you. It files the old object's state entry under the new address, and the next plan compares the new configuration against whatever real object the old entry pointed at. You have adopted one resource's state under another resource's name, which is worse than the destroy you were trying to avoid, and it is why any tool that suggests a moved block should tell you to verify the pairing before you use it.",
  },
];

export const renameFaqs: readonly Faq[] = [
  {
    q: "How do I rename a Terraform resource without destroying it?",
    a: "Rename it in the configuration, then add a moved block naming the old address as from and the new one as to. Run terraform plan and confirm it reports the move and no destroy. Apply, then leave the block in place until everyone who runs this configuration has applied it.",
  },
  {
    q: "How do I move a resource into a module?",
    a: "The new address gains a module prefix, so aws_subnet.app becomes module.network.aws_subnet.app. Write the moved block in the parent module that calls the child, because that is the only place both addresses exist. Moving a resource out of a module is the same move written the other way round, and renaming the module call itself moves everything inside it in one block.",
  },
  {
    q: "What happens when a for_each key changes?",
    a: "The key is part of the address, so changing it changes the address and Terraform plans a destroy and a create for every instance whose key moved. A moved block per key fixes it: from aws_subnet.this with the old key, to aws_subnet.this with the new one. There is no wildcard and no way to generate the blocks, so a large re-key is a long list written out by hand.",
  },
  {
    q: "Can I move from count to for_each without destroying anything?",
    a: "Yes, with one moved block per instance. Under count the instance key is a number, and under for_each it is a string, so aws_subnet.this[0] becomes aws_subnet.this with the name as its key. Both are ordinary addresses and a moved block moves between them. Doing it the other way, from for_each to count, works the same way.",
  },
];

// Every Terraform behaviour quoted in the three FAQ sets below was run against
// Terraform 1.16.1 before it was written down, not recalled: the plan header
// lines, the command output, the error text and the ordering symbols. Where a
// behaviour could not be produced and read, it is not on the page.

export const taintFaqs: readonly Faq[] = [
  {
    q: "Is terraform taint deprecated?",
    a: "Yes. HashiCorp's documentation has pointed at the -replace option on terraform apply instead of terraform taint since Terraform v0.15.2. The command still exists, still works and prints no deprecation warning when you run it, which is why it is still in so many runbooks. terraform untaint is a different case and is not deprecated, because Terraform still sets the tainted mark itself when a create fails partway through.",
  },
  {
    q: "What does terraform taint actually do?",
    a: "It sets a flag on one resource instance in the state file, and that is the whole of it. No API call is made and nothing in your cloud account changes. The effect arrives on the next plan, which sees a tainted instance and plans to destroy that object and create a new one in its place.",
  },
  {
    q: "How do I replace a resource without terraform taint?",
    a: "Run terraform apply -replace=ADDRESS, or terraform plan -replace=ADDRESS -out tfplan if you want the plan reviewed before it is approved. The option can be given more than once to replace more than one object. The difference that matters is that nothing is written to state until the plan is approved, where terraform taint writes to state first and shows you the consequence afterwards.",
  },
  {
    q: "What is the difference between terraform taint and terraform untaint?",
    a: "terraform taint sets the tainted mark on a state entry, so the next plan replaces that object. terraform untaint clears the mark, so the next plan leaves it alone. Both are state surgery: they take a lock, write to the state, and leave nothing in the repository to say it happened. Neither one touches your cloud account.",
  },
  {
    q: "Does terraform taint destroy anything immediately?",
    a: "No. It writes a flag to state and returns. The destroy happens at the next apply, and the plan before it says so: the resource is headed is tainted, so must be replaced, and the summary counts one to add and one to destroy. If that resource has prevent_destroy set in its lifecycle block, the plan fails with Instance cannot be destroyed and the mark stays where it is.",
  },
  {
    q: "Can I taint one instance of a count or for_each resource?",
    a: "Yes. The address is an ordinary instance address, so aws_instance.web[1] and module.app.aws_instance.web are both valid, and the mark lands on that one instance rather than on every instance of the resource. terraform apply -replace takes the same addresses.",
  },
];

export const stateMvFaqs: readonly Faq[] = [
  {
    q: "What is the difference between terraform state mv and a moved block?",
    a: "They end with the same state entry under a new address. A moved block is configuration: it is reviewed in the pull request, versioned with the change it belongs to, and applied by everyone on their next plan, in every workspace. terraform state mv is one command, run once, by one person, against the one state they pointed at, needing direct write access to it and leaving nothing in the repository to show it happened.",
  },
  {
    q: "What does terraform state rm do to the real resource?",
    a: "Nothing. It removes the entry from state and leaves the object running in your cloud account, unmanaged and still being paid for. The part people miss is what the next plan does: with the resource still in your configuration and no state entry to match it, Terraform plans to create a second one. Delete the configuration too and the plan is clean, and the object is orphaned with nothing tracking it.",
  },
  {
    q: "When do I still need terraform state mv?",
    a: "When the move crosses a state boundary. A moved block names two addresses inside one configuration, so it cannot express moving an object from one state file to another, which is what splitting a root module in two or merging two into one comes down to. That case is still the command's, and it is the only one where a block is not the better answer.",
  },
  {
    q: "How do I undo a terraform state mv?",
    a: "Move it back, with the source and destination the other way round. The command also writes a state backup before it saves anything and the backup cannot be disabled, so the previous state is on disk beside the current one if the move went somewhere you did not expect. Read the next plan either way: a state that is correct plans nothing.",
  },
  {
    q: "What is a removed block, and is it the same as terraform state rm?",
    a: "A removed block, added in Terraform 1.7, drops an object from state as a code change rather than as a command. With destroy set to false in its lifecycle block, the plan says the object will no longer be managed by Terraform but will not be destroyed, and it raises a warning naming everything it is about to forget. That is the same outcome as terraform state rm, arriving in a pull request where somebody can object to it.",
  },
  {
    q: "Does terraform state mv change anything in my cloud account?",
    a: "No. It rewrites an entry in the state file and makes no API call to any provider. What it changes is which configuration block Terraform matches that object against on the next plan, which is why moving the entry without renaming the resource in the configuration produces a plan that destroys the new address and creates the old one.",
  },
];

export const replacementFaqs: readonly Faq[] = [
  {
    q: "What does forces replacement mean in a Terraform plan?",
    a: "It means the attribute on that line cannot be changed on the existing object, so the provider has to destroy the object and build a new one to get the value you asked for. Terraform prints the comment against the attribute that did it, and the plan header above the resource says the resource must be replaced.",
  },
  {
    q: "How do I find which attribute forces replacement?",
    a: "Search the plan for the comment forces replacement, which Terraform prints at the end of the line holding the attribute responsible. In the JSON form of the plan the same information is the replace_paths field on the resource change, which is a list of attribute paths and is much easier to search when the plan runs to thousands of lines.",
  },
  {
    q: "Is a replacement the same as a destroy?",
    a: "No, and the difference is the create that follows. A destroy takes the object away and nothing comes back. A replacement takes it away and builds a new one from your configuration, which means a new identity: a new id, and new values for every attribute the provider generates. Anything the old object held that is not in your configuration does not come across.",
  },
  {
    q: "What is the difference between -/+ and +/- in a Terraform plan?",
    a: "The symbol is the ordering. -/+ is destroy and then create replacement, which is the default, and the resource is gone for the length of the apply. +/- is create replacement and then destroy, which is what lifecycle create_before_destroy asks for. The summary line at the end of the plan counts the same one to add and one to destroy either way, so only the symbol tells you which ordering you are getting.",
  },
  {
    q: "Does create_before_destroy avoid the outage?",
    a: "Sometimes, and only where the old object and the new one can exist at once. Anything with a uniqueness constraint that the two would both claim, such as a name, a fixed port or a single-attachment volume, fails to create while the old one is still there. Terraform also propagates the inverted ordering to that resource's dependencies, so one lifecycle block can change the ordering of a part of the graph you were not thinking about.",
  },
  {
    q: "How do I stop Terraform replacing a resource?",
    a: "Decide first whether the attribute change is worth it, because the replacement is the provider telling you it cannot be done any other way. If it is not, revert the attribute. If the value drifted outside Terraform and is not yours to correct, ignore_changes in a lifecycle block stops the plan acting on it. prevent_destroy is a guard rather than a fix: it fails the plan with Instance cannot be destroyed, and because it lives inside the resource block, deleting the resource deletes the guard with it.",
  },
];
