// The site's description of the tool, checked against the tool.
//
// This is the reason the site lives in this repository rather than in the DBHQ
// monorepo. Every other test here asserts the site against itself: that the
// published HTML matches the lists in src/lib/site.ts. Those lists were typed
// by hand from the binary, so they were true once and nothing noticed when they
// stopped being.
//
// A docs page listing a flag the tool no longer accepts is exactly the kind of
// quiet wrongness this project exists to object to. So these tests read the Go
// source that defines the real surface and fail the build when the site and the
// binary disagree.
//
// They are deliberately shallow. The site's prose about what a flag MEANS is
// not checkable from a source file, and pretending otherwise would trade a real
// check for a comforting one. What is checkable is the set of names, and the
// set of names is what goes stale.

import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { flags, levels, annotations } from "../src/lib/site.ts";

const src = (rel) => readFileSync(new URL(`../../${rel}`, import.meta.url), "utf8");

test("every flag the site documents is one the binary declares, and none is missing", () => {
  const main = src("cmd/terraken/main.go");

  // fs.String("format", ...) and fs.Bool("plain", ...) - the name is the first
  // argument, and it is the flag without its dashes.
  const declared = new Set(
    [...main.matchAll(/fs\.(?:String|Bool|Int)\(\s*"([a-z-]+)"/g)].map((m) => m[1]),
  );
  assert.ok(declared.size >= 6, `only found ${declared.size} flags in main.go - the parse has broken`);

  // The site writes a row as "--format terminal|md|json|html", "--out <path>",
  // or "--no-colour, --no-color" where one row documents two spellings. Pull
  // every --name out of the row rather than assuming one per row: the alias
  // row is exactly the case a first-token parse gets wrong, and it is the case
  // where a real flag would go unchecked.
  const documented = new Set(
    flags.flatMap((f) => [...f.flag.matchAll(/--([a-z-]+)/g)].map((m) => m[1])),
  );

  for (const name of documented) {
    assert.ok(declared.has(name), `the site documents --${name}, which the binary does not declare`);
  }
  for (const name of declared) {
    assert.ok(documented.has(name), `the binary accepts --${name}, which the site does not document`);
  }
});

test("the four levels are the four the binary defines, in the same order", () => {
  const level = src("internal/assess/level.go");

  // The String() method is the authority: it is what the report prints and what
  // --fail-on parses.
  const names = [...level.matchAll(/return "(critical|high|low|info)"/g)].map((m) => m[1]);
  assert.equal(names.length, 4, "expected exactly four level names in level.go");
  assert.equal(new Set(names).size, 4, "level.go returns the same name twice");

  // There is deliberately no medium, and the binary has a test saying so. If
  // one ever appears, this site must not be the last thing to find out.
  assert.equal(level.includes('"medium"'), false, "a medium level has appeared in the binary");

  const documented = levels.map((l) => l.name.toLowerCase());
  assert.deepEqual(
    [...documented].sort(),
    [...names].sort(),
    "the site's levels and the binary's levels disagree",
  );
});

test("every annotation code the site documents is one the binary can emit", () => {
  const finding = src("internal/assess/finding.go");

  // Ann... = "some-code" - the constant block in finding.go is the full set.
  const declared = new Set(
    [...finding.matchAll(/Ann[A-Za-z]+\s*=\s*"([a-z-]+)"/g)].map((m) => m[1]),
  );
  assert.ok(declared.size >= 4, `only found ${declared.size} annotation codes - the parse has broken`);

  for (const a of annotations) {
    assert.ok(
      declared.has(a.code),
      `the site documents the annotation "${a.code}", which the binary cannot emit`,
    );
  }

  // The other direction is a warning rather than a failure: the binary may
  // gain an annotation before the site has written the page for it, and that
  // is a normal state for a day or two. Undocumented is untidy; documenting
  // one that does not exist is a lie, and only the lie fails the build.
  const documented = new Set(annotations.map((a) => a.code));
  const missing = [...declared].filter((c) => !documented.has(c));
  if (missing.length) {
    console.log(`  note: the binary emits ${missing.join(", ")}, not yet documented on the site`);
  }
});

test("the no-values contract the site claims is the one the binary tests", () => {
  // The site says, in its own words on every page, that no attribute value
  // reaches the output. That claim is only worth making because a test in this
  // repository holds it. Assert the test still exists, so the claim cannot
  // outlive its evidence.
  const guard = src("cmd/terraken/main_test.go");
  assert.ok(
    guard.includes("TestNoAttributeValueEverReachesAnyFormat"),
    "the test behind the site's central claim has been renamed or removed",
  );

  // The credential detector is the sharpest edge of the same claim. It is the
  // one part of the tool that knows which values are worth stealing, so the
  // site saying "never by value" about it needs its own evidence rather than
  // borrowing the general one.
  assert.ok(
    guard.includes("TestNoDetectedCredentialReachesAnyFormat"),
    "the test behind the site's claim about detected credentials has been renamed or removed",
  );
});
