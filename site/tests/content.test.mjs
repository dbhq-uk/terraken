// The contract for terraken.dbhq.uk, asserted against the BUILT output in dist/.
//
// Testing the build rather than the source is the point: the house rules this
// file enforces - no em dashes, the three contracts on every page about the
// tool, titles inside the SEO band - are rules about what is PUBLISHED, and a
// rule checked against a data file cannot see what a template did to it.
//
// `npm test` runs `npm run build` first (see the pretest script), so dist/ is
// always current when these run.

import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, existsSync } from "node:fs";
import {
  VERSION,
  actionInputs,
  annotations,
  contracts,
  flags,
  levels,
  movedFaqs,
  nav,
  renameFaqs,
  replacementFaqs,
  roadmap,
  roadmapIntro,
  samples,
  site,
  stateMvFaqs,
  taintFaqs,
} from "../src/lib/site.ts";

const DIST = new URL("../dist/", import.meta.url);
const HOST = "https://terraken.dbhq.uk";

const read = (rel) => {
  const p = new URL(rel, DIST);
  assert.ok(existsSync(p), `missing built file: ${rel}`);
  return readFileSync(p, "utf8");
};

// The seven indexed pages. The 404 is deliberately not here: it is noindex, it
// carries no canonical worth asserting and it is checked on its own below.
const pages = [
  { path: "/", file: "index.html" },
  { path: "/terraform-moved-block/", file: "terraform-moved-block/index.html" },
  { path: "/terraform-rename-resource/", file: "terraform-rename-resource/index.html" },
  { path: "/terraform-state-mv/", file: "terraform-state-mv/index.html" },
  { path: "/terraform-taint/", file: "terraform-taint/index.html" },
  { path: "/terraform-forces-replacement/", file: "terraform-forces-replacement/index.html" },
  { path: "/docs/", file: "docs/index.html" },
];

// The five guides. Every one of them is a page about Terraform that mentions
// the tool, rather than a page about the tool, and the suite holds them all to
// the same standard: a TechArticle and an FAQPage, every published answer on
// the page in the words the structured data claims, a contents list of
// top-level sections only, an explicit verification step, and a cap on how
// often the tool may be named.
const guides = [
  { path: "/terraform-moved-block/", faqs: movedFaqs },
  { path: "/terraform-rename-resource/", faqs: renameFaqs },
  { path: "/terraform-state-mv/", faqs: stateMvFaqs },
  { path: "/terraform-taint/", faqs: taintFaqs },
  { path: "/terraform-forces-replacement/", faqs: replacementFaqs },
];

const html = new Map(pages.map((p) => [p.path, read(p.file)]));
const allHtml = [...html.values()].join("\n");

// Text with tags, script/style contents and HTML entities removed, for the
// checks that are about words a reader sees rather than markup.
function visibleText(doc) {
  return doc
    .replace(/<script[\s\S]*?<\/script>/g, " ")
    .replace(/<style[\s\S]*?<\/style>/g, " ")
    .replace(/<[^>]+>/g, " ")
    .replace(/&amp;/g, "&")
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    // AFTER the tags are stripped, never before: a flag like `--out <path>`
    // is published as &lt;path&gt; and is invisible to a check that does not
    // put the angle brackets back.
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&nbsp;/g, " ");
}

const flat = (doc) => visibleText(doc).toLowerCase().replace(/\s+/g, " ");

const graphOf = (doc) => {
  const blocks = [...doc.matchAll(/<script type="application\/ld\+json">([\s\S]*?)<\/script>/g)];
  assert.equal(blocks.length, 1, "expected exactly one JSON-LD block");
  return JSON.parse(blocks[0][1]);
};

const nodesOf = (doc, type) => graphOf(doc)["@graph"].filter((n) => n["@type"] === type);

test("every page is built - seven indexed pages, a real 404, and the edge files", () => {
  assert.equal(pages.length, 7);
  // The nav list is what the footer renders, and the footer is what keeps
  // every page one click from every other. A page that exists and is not on it
  // is a page only a search engine can reach.
  assert.deepEqual(
    nav.map((n) => n.path),
    pages.filter((p) => p.path !== "/").map((p) => p.path),
    "the nav list and the built pages disagree - a new page was added to one and not the other",
  );
  assert.ok(existsSync(new URL("404.html", DIST)), "a real 404 page, so Pages returns a 404 status");
  assert.ok(existsSync(new URL("sitemap-index.xml", DIST)));
  assert.ok(existsSync(new URL("sitemap-0.xml", DIST)));
  assert.ok(existsSync(new URL("copy.js", DIST)));
  assert.ok(existsSync(new URL("llms.txt", DIST)));
  assert.ok(existsSync(new URL("robots.txt", DIST)));
  assert.ok(existsSync(new URL("_headers", DIST)));
  assert.ok(existsSync(new URL("site.webmanifest", DIST)));
});

// ---------------------------------------------------------------------------
// House writing rules. These bind every DBHQ property, and a build that breaks
// one of them must not ship.
// ---------------------------------------------------------------------------

test("no em dashes and no en dashes anywhere in the output", () => {
  for (const [path, doc] of html) {
    const text = visibleText(doc);
    assert.equal(text.includes("—"), false, `${path}: em dash`);
    assert.equal(text.includes("–"), false, `${path}: en dash`);
    assert.equal(doc.includes("&mdash;"), false, `${path}: &mdash; entity`);
    assert.equal(doc.includes("&ndash;"), false, `${path}: &ndash; entity`);
  }
  const notFound = read("404.html");
  assert.equal(visibleText(notFound).includes("—"), false, "404: em dash");
  assert.equal(visibleText(notFound).includes("–"), false, "404: en dash");
  const llms = read("llms.txt");
  assert.equal(llms.includes("—"), false, "llms.txt: em dash");
  assert.equal(llms.includes("–"), false, "llms.txt: en dash");
});

test("no heading carries a trailing full stop", () => {
  for (const [path, doc] of html) {
    for (const m of doc.matchAll(/<h([1-3])[^>]*>([\s\S]*?)<\/h\1>/g)) {
      const text = visibleText(m[2]).trim();
      assert.equal(text.endsWith("."), false, `${path}: heading ends with a full stop - "${text}"`);
    }
  }
  // The FAQ questions are <dt>, not headings, and they are questions, so they
  // may not end in a full stop either - they end in a question mark.
  for (const { faqs } of guides) {
    for (const f of faqs) {
      assert.ok(f.q.endsWith("?"), `FAQ is not a question: "${f.q}"`);
    }
  }
  assert.equal(site.tagline.endsWith("."), false, "the tagline is a subtitle, not a sentence");
});

test("British English, not American", () => {
  // A short list of the spellings that actually turn up in this copy. Not a
  // dictionary: a full one produces false positives on quoted identifiers.
  //
  // "color" is NOT on it and cannot be: --no-color is a real flag the binary
  // accepts, NO_COLOR and FORCE_COLOR are real environment variables, and the
  // docs page documents all three. That is the tool's own surface, not this
  // site's prose. The British spelling of the word in prose is asserted
  // positively instead, below.
  const american = [
    "organize", "organized", "organization",
    "recognize", "recognized",
    "analyze", "analyzed",
    "behavior", "behaviors",
    "catalog ", "catalogs",
    "favorite", "favorites",
    "license plate", // n/a; the array is a list of forms, not a dictionary
    "unauthorized",
  ];
  for (const [path, doc] of html) {
    const text = flat(doc);
    for (const word of american) {
      if (word === "license plate") continue;
      assert.equal(text.includes(word), false, `${path}: American spelling "${word.trim()}"`);
    }
  }
  const docs = flat(html.get("/docs/"));
  assert.ok(docs.includes("colour"), "/docs/: the prose no longer spells colour the British way");
});

test("DBHQ never speaks as a team it does not have", () => {
  // DBHQ is one person and never writes "we". The tool's own README does in
  // one or two places; those sentences are recast on this site rather than
  // quoted.
  const banned = [
    " we wrote", " we built", " we would rather", " we kept",
    " our team", " our engineers", " we offer", " we can help",
    " we believe", " we think", " we deliver", " we recommend",
  ];
  for (const [path, doc] of html) {
    const text = flat(doc);
    for (const phrase of banned) {
      assert.equal(text.includes(phrase), false, `${path}: first-person plural "${phrase.trim()}"`);
    }
  }
  assert.equal(read("llms.txt").toLowerCase().includes(" we "), false, "llms.txt: first-person plural");
});

test("the denylist stays off this site", () => {
  // Mirrors the standing bans in the DBHQ repo's content plan
  // (docs/website/content-plan.md). The tool's README may name any of these;
  // this site may not.
  const banned = [
    "scentverdict",
    "security-cleared",
    "sc clearance",
    "security clearance",
    "ministry of defence",
    "fractional engineering lead",
    "fractional cto",
    "cto in all but name",
    "ai-accelerated",
    "outside ir35",
    "ir35-safe",
    "currently taking projects",
    "delivery blueprint",
    "azure production readiness review",
    "scoping pays for itself",
    "credited against the build",
  ];
  const surfaces = [...html, ["/404/", read("404.html")], ["llms.txt", read("llms.txt")]];
  for (const [path, doc] of surfaces) {
    const text = visibleText(doc).toLowerCase();
    for (const phrase of banned) {
      assert.equal(text.includes(phrase), false, `${path}: denylisted phrase "${phrase}"`);
    }
  }
});

test("no DBHQ price is published", () => {
  // Sterling is the tell: DBHQ prices in GBP, and nothing quoted on this site
  // is a figure in pounds. The GBP in the structured data is the free
  // SoftwareApplication offer, priced at zero, which is why this reads the
  // visible text rather than the document.
  const surfaces = [...html, ["/404/", read("404.html")], ["llms.txt", read("llms.txt")]];
  for (const [path, doc] of surfaces) {
    const text = visibleText(doc);
    assert.equal(/£\s?\d/.test(text), false, `${path}: a sterling figure`);
    assert.equal(/\bday rate\b/i.test(text), false, `${path}: a day rate`);
    assert.equal(/\bper day\b/i.test(text), false, `${path}: a per-day figure`);
    assert.equal(/\bfrom \$\d/i.test(text), false, `${path}: a price`);
  }
});

// ---------------------------------------------------------------------------
// Per-page SEO. This is the whole reason the site is four pages rather than a
// section of dbhq.uk, so it gets asserted rather than assumed.
// ---------------------------------------------------------------------------

test("every title is 50-60 characters and unique", () => {
  const seen = new Set();
  for (const [path, doc] of html) {
    const m = doc.match(/<title>([\s\S]*?)<\/title>/);
    assert.ok(m, `${path}: no <title>`);
    const t = m[1];
    assert.ok(t.length >= 50 && t.length <= 60, `${path}: title is ${t.length} characters, wanted 50-60: "${t}"`);
    assert.equal(seen.has(t), false, `${path}: duplicate title "${t}"`);
    seen.add(t);
  }
});

test("every meta description is written per page, 60-155 characters and unique", () => {
  const seen = new Set();
  const surfaces = [...html, ["/404/", read("404.html")]];
  for (const [path, doc] of surfaces) {
    const m = doc.match(/<meta name="description" content="([^"]*)"/);
    assert.ok(m, `${path}: no meta description`);
    const d = m[1];
    assert.ok(d.length > 60, `${path}: description is only ${d.length} characters`);
    assert.ok(d.length < 155, `${path}: description is ${d.length} characters, wanted under 155`);
    assert.equal(seen.has(d), false, `${path}: duplicate description`);
    seen.add(d);
  }
});

test("every page carries a canonical matching its own path", () => {
  for (const [path, doc] of html) {
    const m = doc.match(/<link rel="canonical" href="([^"]*)"/);
    assert.ok(m, `${path}: no canonical`);
    assert.equal(m[1], `${HOST}${path}`, `${path}: wrong canonical`);
    // og:url agrees with it, so a share and a crawl never disagree.
    const og = doc.match(/<meta property="og:url" content="([^"]*)"/);
    assert.ok(og, `${path}: no og:url`);
    assert.equal(og[1], `${HOST}${path}`, `${path}: og:url disagrees with the canonical`);
  }
});

test("the 404 is noindex, so a soft 404 never enters the index", () => {
  const doc = read("404.html");
  assert.match(doc, /<meta name="robots" content="noindex, nofollow"/);
});

test("every page emits one JSON-LD graph that parses", () => {
  for (const [path, doc] of html) {
    const graph = graphOf(doc);
    assert.equal(graph["@context"], "https://schema.org");
    const types = graph["@graph"].map((n) => n["@type"]);
    assert.ok(types.includes("Organization"), `${path}: no Organization node`);
    assert.ok(types.includes("WebSite"), `${path}: no WebSite node`);
    assert.ok(types.includes("BreadcrumbList"), `${path}: no BreadcrumbList`);
    // ONE COMPANY, ONE IDENTITY. The publisher keeps dbhq.uk's own canonical
    // @id rather than minting a second Organization for the same company on a
    // third hostname. skills.dbhq.uk asserts the same thing.
    const org = graph["@graph"].find((n) => n["@type"] === "Organization");
    assert.equal(org["@id"], "https://dbhq.uk/#organization");
    const web = graph["@graph"].find((n) => n["@type"] === "WebSite");
    assert.equal(web["@id"], `${HOST}/#website`);
    assert.deepEqual(web.publisher, { "@id": "https://dbhq.uk/#organization" });
  }
});

test("the index publishes a SoftwareApplication for the tool", () => {
  const [app] = nodesOf(html.get("/"), "SoftwareApplication");
  assert.ok(app, "index: no SoftwareApplication node");
  assert.equal(app.name, "Terraken", "the schema name is the product name, Title Cased");
  assert.equal(app.codeRepository, "https://github.com/dbhq-uk/terraken");
  // `url` is the URL of the ITEM, so it is this page. The repository is what
  // codeRepository and downloadUrl are for.
  assert.equal(app.url, `${HOST}/`);
  assert.notEqual(app.url, app.codeRepository);
  assert.equal(app.isAccessibleForFree, true);
  assert.equal(app.offers.price, "0");
  // The three contracts reach the machine-readable copy too. A page that
  // states a boundary to a reader and hides it from an engine is half honest.
  assert.ok(
    app.disambiguatingDescription.length > 40,
    "index: the contracts are missing from the structured data",
  );
  // And it is on the index alone. A SoftwareApplication node on a guide page
  // would say the page is about the software, and it is not.
  for (const { path } of guides) {
    assert.equal(
      nodesOf(html.get(path), "SoftwareApplication").length,
      0,
      `${path}: publishes a SoftwareApplication, but it is a page about Terraform`,
    );
  }
});

test("each guide page publishes a TechArticle and its FAQPage", () => {
  assert.equal(guides.length, 5, "a guide was added or dropped - decide which, on purpose");
  for (const { path, faqs } of guides) {
    const doc = html.get(path);
    const articles = nodesOf(doc, "TechArticle");
    // ONE NODE PER PAGE. The page node IS the article, not a WebPage carrying a
    // second TechArticle beside it - two nodes for one document with two @ids
    // is the drift this whole graph is arranged to avoid.
    assert.equal(articles.length, 1, `${path}: ${articles.length} TechArticle nodes, expected exactly one`);
    const [article] = articles;
    assert.equal(article["@id"], `${HOST}${path}`, `${path}: the article is not the page`);
    assert.deepEqual(article.publisher, { "@id": "https://dbhq.uk/#organization" });
    assert.ok(article.headline, `${path}: the article has no headline`);

    const [faq] = nodesOf(doc, "FAQPage");
    assert.ok(faq, `${path}: no FAQPage node`);
    assert.equal(faq.mainEntity.length, faqs.length);

    // ONE DEFINITION, TWO SURFACES. Every published question and answer is on
    // the page in the words the structured data claims. An answer written for
    // an engine and not for a reader is a page nobody should rank.
    const text = flat(doc);
    for (const q of faq.mainEntity) {
      assert.equal(q["@type"], "Question");
      assert.ok(
        text.includes(q.name.toLowerCase()),
        `${path}: the FAQ question "${q.name}" is in the structured data and not on the page`,
      );
      const answer = q.acceptedAnswer.text;
      assert.ok(answer.length > 80, `${path}: the answer to "${q.name}" has no substance behind it`);
      assert.ok(
        text.includes(answer.slice(0, 70).toLowerCase()),
        `${path}: the answer to "${q.name}" is in the structured data and not on the page`,
      );
    }
  }
});

test("the rename page publishes a HowTo, because it is a procedure", () => {
  const [howTo] = nodesOf(html.get("/terraform-rename-resource/"), "HowTo");
  assert.ok(howTo, "no HowTo node");
  assert.equal(howTo.step.length, 4);
  for (const s of howTo.step) {
    assert.equal(s["@type"], "HowToStep");
    assert.ok(s.text.length > 40, `HowTo step "${s.name}" has no substance behind it`);
  }
});

test("the docs page publishes a TechArticle about the software", () => {
  const articles = nodesOf(html.get("/docs/"), "TechArticle");
  assert.equal(articles.length, 1, "/docs/: expected exactly one TechArticle node");
  assert.equal(articles[0]["@id"], `${HOST}/docs/`);
  assert.deepEqual(articles[0].about, { "@id": `${HOST}/#software` });
});

// ---------------------------------------------------------------------------
// THE SEARCH DECISION, ENCODED.
//
// This site exists because "terraform moved block" is measured at 390 searches
// a month and every phrasing of "terraform plan review" is measured at zero.
// The information architecture follows that and not the tool's internal
// positioning. The assertions below are what stops a future rewrite quietly
// reversing it: the anchor page has to keep the phrase in its title and its
// H1, and it has to keep the top sitemap priority.
// ---------------------------------------------------------------------------

test("the anchor page is built around the phrase people actually type", () => {
  const doc = html.get("/terraform-moved-block/");
  const title = doc.match(/<title>([\s\S]*?)<\/title>/)[1].toLowerCase();
  assert.ok(title.includes("moved block"), `the anchor page title lost the phrase: "${title}"`);
  const h1 = doc.match(/<h1[^>]*>([\s\S]*?)<\/h1>/);
  assert.ok(h1, "the anchor page has no H1");
  assert.ok(
    visibleText(h1[1]).toLowerCase().includes("moved block"),
    "the anchor page H1 lost the phrase",
  );

  const rename = html.get("/terraform-rename-resource/");
  const rTitle = rename.match(/<title>([\s\S]*?)<\/title>/)[1].toLowerCase();
  assert.ok(rTitle.includes("rename"), `the rename page title lost the phrase: "${rTitle}"`);

  const xml = read("sitemap-0.xml");
  const entry = xml.split("<url>").find((u) => u.includes(`${HOST}/terraform-moved-block/`));
  assert.ok(entry, "sitemap: the anchor page is missing");
  assert.ok(
    entry.includes("<priority>1</priority>") || entry.includes("<priority>1.0</priority>"),
    "sitemap: the anchor page is no longer top priority, which reverses the decision this site was built on",
  );

  // AND NOTHING OUTRANKS IT. Pinning the anchor at 1.0 is only half the
  // guarantee: a later page given 1.0 as well would leave the site with no
  // stated anchor at all, which is the same reversal arriving by addition
  // rather than by subtraction. Every page has a priority, and the anchor and
  // the index are the only two allowed to hold the top one.
  const priorities = new Map(
    xml
      .split("<url>")
      .slice(1)
      .map((u) => [
        u.match(/<loc>([^<]+)<\/loc>/)[1],
        Number(u.match(/<priority>([^<]+)<\/priority>/)[1]),
      ]),
  );
  assert.equal(priorities.size, pages.length, "sitemap: a page has no priority hint");
  const top = [...priorities].filter(([, p]) => p >= 1).map(([loc]) => loc).sort();
  assert.deepEqual(
    top,
    [`${HOST}/`, `${HOST}/terraform-moved-block/`].sort(),
    "sitemap: something other than the index and the anchor page holds the top priority",
  );
});

test("the guide pages are guides, not landing pages", () => {
  // terraken is mentioned on each because it detects the case, and it is not
  // the subject. A guide that turns into an advert stops ranking and deserves
  // to, so the tool's name is capped at a share of the page rather than banned.
  for (const { path } of guides) {
    const text = flat(html.get(path));
    const words = text.split(" ").length;
    const mentions = (text.match(/terraken/g) ?? []).length;
    assert.ok(words > 1200, `${path}: only ${words} words, which is not a complete answer to the query`);
    assert.ok(
      mentions / words < 0.01,
      `${path}: ${mentions} mentions of the tool in ${words} words - this is turning into a landing page`,
    );
  }
});

// A GUIDE HAS TO HAVE A STOPPING POINT.
//
// The gap this test exists for: every guide told a reader what to write and
// none of them told the reader how to know it had worked. Somebody mid-incident
// does not need to know whether their syntax is valid - Terraform will tell
// them that - they need to know whether the destroy has gone. A block with a
// typo in an address is valid configuration that does nothing at all, and
// Terraform reports nothing when it does nothing, so "it applied cleanly" is
// not evidence of anything.
//
// The three parts are the whole of it: what a right plan looks like, what a
// wrong one looks like, and what to do about a wrong one. A page carrying one
// or two of the three sends somebody back to the plan with no way to read it.
test("every guide says how to tell whether it worked", () => {
  for (const { path } of guides) {
    const doc = html.get(path);
    const text = visibleText(doc);

    const headings = [...doc.matchAll(/<h([23])[^>]*>([\s\S]*?)<\/h\1>/g)].map((m) =>
      visibleText(m[2]).trim(),
    );
    assert.ok(
      headings.some((h) => /check the plan|how to tell/i.test(h)),
      `${path}: no heading names the check, so the verification step cannot be found by scanning`,
    );
    assert.ok(
      /still[- ]wrong/i.test(text),
      `${path}: describes a correct plan and not a wrong one, which is the half a reader in trouble needs`,
    );
    assert.ok(
      /still there/i.test(text),
      `${path}: does not say what to do when the destroy is still in the plan`,
    );
  }
});

// THE CONTENTS LIST IS TOP-LEVEL SECTIONS ONLY.
//
// A list that mirrors every heading on a 2,000-word article fills the first
// screen with navigation, and the first screen is where the answer is supposed
// to be. So a row may point at an h2 and never at a heading nested under one.
//
// Two rows point at a section wrapper rather than at a heading - #faq and
// #contracts, where the heading belongs to a component inside the section - so
// the rule is expressed as "the first heading at or after this id is an h2"
// rather than as a list of blessed names. A wrapper whose section opens on an
// h3 fails, which is the case worth catching.
test("a guide's contents list carries top-level sections only", () => {
  for (const { path } of guides.concat([{ path: "/docs/" }])) {
    const doc = html.get(path);
    const tocBlock = doc.match(/<ul class="toc-list"[\s\S]*?<\/ul>/);
    assert.ok(tocBlock, `${path}: no contents list`);
    const rows = [...tocBlock[0].matchAll(/href="#([^"]+)"/g)].map((m) => m[1]);
    assert.ok(rows.length > 0, `${path}: an empty contents list`);
    assert.ok(
      rows.length <= 11,
      `${path}: ${rows.length} rows in the contents list, which is a navigation panel rather than a contents list`,
    );

    const h2Ids = new Set(
      [...doc.matchAll(/<h2[^>]*\sid="([^"]+)"/g)].map((m) => m[1]),
    );
    const nestedIds = new Set(
      [...doc.matchAll(/<h[3-6][^>]*\sid="([^"]+)"/g)].map((m) => m[1]),
    );
    // The first heading at or after a wrapper id, for the rows that point at a
    // section rather than at a heading.
    const headingAfter = (id) => {
      const at = doc.indexOf(`id="${id}"`);
      if (at < 0) return null;
      const m = doc.slice(at).match(/<h([1-6])[^>]*>/);
      return m ? m[1] : null;
    };

    for (const id of rows) {
      assert.equal(
        nestedIds.has(id),
        false,
        `${path}: the contents list points at #${id}, which is a heading nested inside a section`,
      );
      assert.ok(
        h2Ids.has(id) || headingAfter(id) === "2",
        `${path}: the contents list points at #${id}, which is not a top-level section`,
      );
    }
  }
});

// THE THREE PAGES ADDED ON 16 SEP 2026, AND THE TERMS THEY WERE BUILT FOR.
//
// Same guard as the anchor page below: the phrase a page was built around is
// the one thing a rewrite must not quietly drop, because the page keeps
// reading well after it has stopped answering the query it exists for. The
// numbers are worldwide Google Ads volume, measured 16 September 2026 and
// recorded in docs/research/terraken-seo-worldwide.md.
test("each guide keeps the phrase it was built around", () => {
  const intents = [
    { path: "/terraform-moved-block/", phrase: "moved block" },
    { path: "/terraform-rename-resource/", phrase: "rename" },
    { path: "/terraform-state-mv/", phrase: "state mv" },
    { path: "/terraform-taint/", phrase: "taint" },
    { path: "/terraform-forces-replacement/", phrase: "forces replacement" },
  ];
  for (const { path, phrase } of intents) {
    const doc = html.get(path);
    const title = doc.match(/<title>([\s\S]*?)<\/title>/)[1].toLowerCase();
    assert.ok(title.includes(phrase), `${path}: the title lost "${phrase}" - "${title}"`);
    const h1 = doc.match(/<h1[^>]*>([\s\S]*?)<\/h1>/);
    assert.ok(h1, `${path}: no H1`);
    assert.ok(
      visibleText(h1[1]).toLowerCase().includes(phrase),
      `${path}: the H1 lost "${phrase}"`,
    );
  }
});

// ---------------------------------------------------------------------------
// THE THREE CONTRACTS. READ THIS BEFORE CHANGING ANY OF THEM.
//
// Every phrase below is a guarantee this site publishes about the tool, and
// each one is held by a test in the tool's own repository - AGENTS.md there
// calls them the constraints that must not be broken. They are pinned here
// because the change that erodes one does not look like vandalism, it looks
// like a copy-editing pass: "it never prints an attribute's value" softened
// into "values are handled carefully" reads better and promises nothing, and
// nothing else in this suite can tell the two apart.
//
// SO CHANGING THIS TABLE IS A DELIBERATE ACT, not maintenance. If a phrase
// here fails, the question is not how to get the test green - it is whether
// the guarantee itself has moved. If the tool genuinely changed, change the
// phrase and say why in the commit. If it did not, put the words back.
// ---------------------------------------------------------------------------
const PUBLISHED_CONTRACTS = [
  "it takes a file, and runs nothing",
  "never runs terraform, never reads a cloud credential, never makes a network call",
  "created mode 0600",
  "it never prints an attribute's value, in any format",
  "not masked, not redacted, not truncated",
  "a live credential was found in a real plan that terraform had not marked",
  "it is deterministic, with no model in the loop",
  "the same plan always produces the same verdict",
  "no ranking that cannot be read straight off the plan",
];

test("the three contracts are on every page about the tool, in the words they were published in", () => {
  assert.equal(contracts.length, 3, "a contract was added or dropped - decide which, on purpose");
  for (const c of contracts) {
    assert.equal(c.h.endsWith("."), false, `contract heading ends with a full stop: "${c.h}"`);
    assert.ok(c.p.length > 120, `the contract "${c.h}" has no substance behind it`);
  }
  // The index and the docs page are the two pages ABOUT the tool. The two
  // guides are about Terraform and carry the contracts in llms.txt only.
  for (const path of ["/", "/docs/"]) {
    const page = flat(html.get(path));
    for (const phrase of PUBLISHED_CONTRACTS) {
      assert.ok(
        page.includes(phrase),
        `${path}: the page no longer says "${phrase}". This is a published guarantee, not a phrasing choice - read the note above PUBLISHED_CONTRACTS before changing it here`,
      );
    }
  }
  const llms = read("llms.txt").toLowerCase().replace(/\s+/g, " ");
  for (const phrase of ["it never prints an attribute's value", "no model in the loop"]) {
    assert.ok(llms.includes(phrase), `llms.txt: lost the guarantee "${phrase}"`);
  }
});

// ---------------------------------------------------------------------------
// ACCURACY TO THE SHIPPED BINARY.
//
// Every flag, level, annotation code and Action input on this site was read
// out of dbhq-uk/terraken's source, not out of its README and not out of
// memory. The lists are pinned here so that adding one is a deliberate act
// with a check against the real binary behind it.
//
// THE TWELVE OPEN ISSUES ON THAT REPOSITORY ARE FUTURE WORK. A documented
// capability that does not exist is worse than an undocumented one that does,
// and the UNSHIPPED list below is the guard.
// ---------------------------------------------------------------------------

const SHIPPED_FLAGS = [
  "--format terminal|md|json|html",
  "--out <path>",
  "--fail-on critical|high|low|info",
  "--min-level critical|high|low|info",
  "--plain",
  "--no-colour, --no-color",
  "--version",
];

const SHIPPED_LEVELS = ["critical", "high", "low", "info"];

const SHIPPED_ANNOTATIONS = [
  "possible-missed-moved-block",
  "unverifiable-until-apply",
  "sensitive",
  "unrecognised-provider",
  "same-elements-reordered",
  "same-json-written-differently",
  "same-text-different-whitespace",
  "same-number-written-differently",
  "null-on-one-side-empty-on-the-other",
  "every-changed-attribute-written-differently",
];

test("the documented flags are the flags the binary has", () => {
  assert.deepEqual(
    flags.map((f) => f.flag),
    SHIPPED_FLAGS,
    "the flag list changed. Check it against cmd/terraken/main.go in dbhq-uk/terraken before updating this pin - a flag documented here that the binary does not accept is the worst kind of wrong",
  );
  const docs = html.get("/docs/");
  for (const f of flags) {
    assert.ok(visibleText(docs).includes(f.flag), `/docs/: the flag ${f.flag} is not on the page`);
    assert.ok(f.what.length > 40, `the flag ${f.flag} is documented in fewer words than it deserves`);
  }
});

test("there are four levels and deliberately no medium", () => {
  assert.deepEqual(levels.map((l) => l.name), SHIPPED_LEVELS);
  for (const path of ["/", "/docs/"]) {
    const doc = html.get(path);
    const text = flat(doc);
    for (const l of SHIPPED_LEVELS) {
      assert.ok(text.includes(l), `${path}: the level "${l}" is not on the page`);
    }
    // Not a word ban: the index heading says "no medium" out loud, and one of
    // the code samples names a db.t3.medium instance class. What must never
    // exist is a fifth level, so this looks for one being published rather
    // than for the word appearing.
    assert.equal(
      /lvl--medium|>\s*medium\s*</.test(doc),
      false,
      `${path}: a medium level, which the tool does not have`,
    );
  }
  assert.ok(flat(html.get("/")).includes("no medium"), "the index no longer says there is no medium level");
});

test("the documented annotation codes are the codes the binary emits", () => {
  assert.deepEqual(
    annotations.map((a) => a.code),
    SHIPPED_ANNOTATIONS,
    "the annotation list changed. Check it against internal/assess/finding.go in dbhq-uk/terraken before updating this pin",
  );
  const docs = visibleText(html.get("/docs/"));
  for (const a of annotations) {
    assert.ok(docs.includes(a.code), `/docs/: the annotation code ${a.code} is not on the page`);
  }
});

test("the GitHub Action is documented with the inputs it actually has", () => {
  assert.deepEqual(
    actionInputs.map((a) => a.name),
    ["plan", "fail-on", "summary", "version"],
    "the Action inputs changed. Check them against action.yml in dbhq-uk/terraken",
  );
  const docs = visibleText(html.get("/docs/"));
  assert.ok(docs.includes(`dbhq-uk/terraken@${VERSION}`), "/docs/: the Action example is not pinned to a tag");
  assert.equal(docs.includes("@latest\n  with"), false, "/docs/: the Action example is pinned to a moving ref");
  assert.match(VERSION, /^v\d+\.\d+\.\d+$/, "VERSION is not a release tag");
});

test("no unshipped capability is described as though it exists", () => {
  // These name the open issues on dbhq-uk/terraken. None of them is in the
  // binary today, so none may be described as a thing the tool does. Remove an
  // entry here in the same commit that ships the capability, not before.
  //
  // THE BAN USED TO BE ABSOLUTE AND IS NOW SCOPED (16 Sep 2026). The index
  // carries a roadmap section - labelled as unbuilt, every item linking its
  // open issue - so these phrases are expected there and nowhere else. The
  // section is cut out by its data-roadmap attribute before the sweep runs, and
  // the sweep is otherwise exactly as strict as it was: absolute on every guide
  // page, on /docs/, on llms.txt, and on the rest of the index including the
  // hero, the findings list and the contracts.
  //
  // Scoping by a marker attribute rather than by heading text is deliberate.
  // Deleting the roadmap heading to sneak a claim in would also delete the
  // exemption, so the copy cannot escape the check by losing its label.
  //
  // It does NOT ban the idea of generating a moved block. The shipped tool
  // already prints a suggested one, and the docs page says plainly that a tool
  // which writes them into your configuration is a different tool. Banning the
  // words would fail on that sentence, which is a correct disclaimer. The
  // write path is guarded on its own, in the next test.
  const unshipped = [
    "blast radius",
    "what depends on",
    "cost delta",
    "price sheet",
    "policy engine",
    "evaluates your own rules",
    "compare two plans",
    "many terraform roots",
    "across many roots",
  ];

  const withoutRoadmap = (doc) =>
    doc.replace(/<section[^>]*\bdata-roadmap\b[\s\S]*?<\/section>/g, " ");

  const surfaces = [...html, ["llms.txt", read("llms.txt")]];
  for (const [path, doc] of surfaces) {
    const text = flat(withoutRoadmap(doc));
    for (const phrase of unshipped) {
      assert.equal(text.includes(phrase), false, `${path}: describes unshipped work - "${phrase}"`);
    }
  }

  // The exemption has to actually be doing something, or a regex that stopped
  // matching would silently turn this back into the absolute ban and pass.
  const index = read("index.html");
  assert.ok(
    flat(index).includes("blast radius"),
    "the roadmap has lost its blast radius entry, or the section is no longer on the page",
  );
  assert.equal(
    flat(withoutRoadmap(index)).includes("blast radius"),
    false,
    "the data-roadmap cut is not matching - the exemption is wider than the section",
  );
});

test("the site never claims the tool writes to a configuration", () => {
  // It reads a plan. It has no write path into your Terraform at all, and the
  // one file it writes is the one --out names. Saying otherwise would be the
  // single most damaging inaccuracy this site could carry.
  for (const [path, doc] of html) {
    const text = flat(doc);
    assert.equal(text.includes("fixes it for you"), false, `${path}: claims it edits your configuration`);
    assert.equal(text.includes("applies the moved block"), false, `${path}: claims it applies a change`);
    assert.equal(text.includes("runs terraform for you"), false, `${path}: claims it runs terraform`);
  }
  const docs = flat(html.get("/docs/"));
  assert.ok(
    docs.includes("never writes to your configuration"),
    "/docs/: no longer states that it does not write to your configuration",
  );
});

test("the captured output is the tool's own, and has not been tidied up", () => {
  // Every output block on this site came from running the built binary against
  // a fixture in the tool's testdata/. The commands are recorded in
  // src/lib/site.ts. These pin the load-bearing lines so a hand edit shows up:
  // a sample that does not match what the tool prints is the first thing a
  // reader will check.
  const pins = {
    critical: ["holds data, so destroying it loses that data", "forces replacement   zone", "1 critical"],
    missedMove: [
      "possible missed moved block",
      "5 of 5 attributes match azurerm_subnet.application",
      "verify the pairing before using that block",
    ],
    minLevel: ["3 below high not shown", "1 critical  1 high  1 low  2 info"],
    rewritten: ["same JSON, written differently", "every attribute this plan shows as changed here"],
    json: ['"data_loss": true', '"counts": {', '"level": "critical"'],
    markdown: ["| Level | Change | Resource | Notes |"],
  };
  for (const [name, lines] of Object.entries(pins)) {
    for (const line of lines) {
      assert.ok(samples[name].includes(line), `samples.${name} no longer contains "${line}"`);
    }
  }
  // AND NO SAMPLE PRINTS A VALUE. The tool's one unbreakable guarantee is that
  // no attribute value reaches the output in any format, so no sample on this
  // site may contain an assignment of one. The moved-block suggestion is the
  // single exception and it assigns an address, not a value.
  for (const [name, text] of Object.entries(samples)) {
    if (name === "json" || name === "markdown") continue;
    for (const line of text.split("\n")) {
      if (line.includes("moved {")) continue;
      assert.equal(
        /=\s*\S/.test(line),
        false,
        `samples.${name} has a line that assigns something, which a terminal report never does: "${line}"`,
      );
    }
  }
});

// ---------------------------------------------------------------------------
// Links
// ---------------------------------------------------------------------------

test("every internal link carries the canonical trailing slash", () => {
  const surfaces = [...html, ["/404/", read("404.html")]];
  for (const [path, doc] of surfaces) {
    for (const m of doc.matchAll(/href="(\/[^"?]*)"/g)) {
      const href = m[1];
      // Real files are allowed to look like files, and a fragment attaches to
      // a path that has already been checked.
      if (/\.[a-z0-9]+$/i.test(href)) continue;
      const bare = href.split("#")[0];
      assert.ok(bare.endsWith("/"), `${path}: internal link without a trailing slash: ${href}`);
    }
  }
});

test("every internal link resolves to a page that exists, and every fragment to a real id", () => {
  const known = new Set(pages.map((p) => p.path));
  const idsByPath = new Map(
    pages.map((p) => [
      p.path,
      new Set([...html.get(p.path).matchAll(/\sid="([^"]+)"/g)].map((m) => m[1])),
    ]),
  );
  const surfaces = [...html, ["/404/", read("404.html")]];
  for (const [path, doc] of surfaces) {
    for (const m of doc.matchAll(/href="(\/[^"?]*)"/g)) {
      const href = m[1];
      if (/\.[a-z0-9]+$/i.test(href)) continue;
      const [bare, frag] = href.split("#");
      assert.ok(known.has(bare), `${path}: link to a page that does not exist: ${href}`);
      if (frag) {
        assert.ok(
          idsByPath.get(bare).has(frag),
          `${path}: link to an anchor that does not exist: ${href}`,
        );
      }
    }
    // Same-page anchors too: the table of contents on each guide page is only
    // useful if every row lands somewhere.
    if (known.has(path)) {
      for (const m of doc.matchAll(/href="#([^"]+)"/g)) {
        assert.ok(
          idsByPath.get(path).has(m[1]),
          `${path}: table of contents points at #${m[1]}, which is not on the page`,
        );
      }
    }
  }
});

// This test exists because it caught four real bugs on the day it was written,
// and seventy-five more on 16 September 2026 when it was widened.
//
// Astro strips the whitespace between a text node and an element that starts on
// the next source line, so a paragraph reading "download it from the <a>releases
// page</a>" ships as "from thereleases page". It is invisible in the source, it
// reads as a typo rather than a build artefact, and it is only ever noticed by
// someone looking at the rendered page. The fix is a literal `&#32;` at the end
// of the line that comes first. skills.dbhq.uk carries the same hazard and the
// same fix, in a comment rather than in a test.
//
// IT COVERS EVERY INLINE ELEMENT, NOT JUST LINKS. The first version watched
// <a> alone and passed a build carrying "two arguments, fromand to" and "under
// a new address.Where the resource holds something". <code> is the worst of
// them, because the chip's own padding leaves a gap that looks like a space
// and is not one: the words are joined in the accessible name, in a copied
// selection and in anything that reads the text rather than the pixels.
test("no inline element is glued to the word beside it", () => {
  const TAGS = "a|code|strong|em";
  const surfaces = [...html, ["/404/", read("404.html")]];
  for (const [path, doc] of surfaces) {
    const body = doc.replace(/<script[\s\S]*?<\/script>/g, " ").replace(/<style[\s\S]*?<\/style>/g, " ");
    // The character class is wider than \w on purpose. The bug is the same
    // whether the text before the element ends in a letter or in punctuation:
    // "resource change." followed by a link ships as "change.terraken", which
    // is just as broken and is invisible to a check that only looks for a word
    // character.
    //
    // A semicolon is NOT in the class and must not be added to it: the fix for
    // this bug is a literal `&#32;`, which ends in one, so a class holding `;`
    // fails on every line that has already been fixed.
    for (const m of body.matchAll(new RegExp(`[\\w.,:!?]<(?:${TAGS})\\b[^>]*>`, "g"))) {
      assert.fail(`${path}: no space before an inline element: ...${body.slice(Math.max(0, m.index - 45), m.index + 12)}`);
    }
    for (const m of body.matchAll(new RegExp(`</(?:${TAGS})>[A-Za-z]`, "g"))) {
      assert.fail(`${path}: no space after an inline element: ...${body.slice(Math.max(0, m.index - 45), m.index + 12)}`);
    }
  }
});

test("no page is a dead end - each links to every other", () => {
  for (const p of pages) {
    const doc = html.get(p.path);
    for (const other of pages.filter((o) => o.path !== p.path)) {
      assert.ok(
        doc.includes(`href="${other.path}"`),
        `${p.path}: no link to ${other.path}`,
      );
    }
  }
});

test("external links open in the same tab, and are annotated", () => {
  // Same-tab is the house decision (Dan, 10 Sep 2026): a link is navigation,
  // and a new window is a behaviour change the reader did not ask for. A
  // same-tab top-level navigation creates no opener relationship, so it needs
  // no rel="noopener" either - adding one to make it match a new-tab link is
  // exactly the cargo cult that rule exists to stop.
  assert.equal(/target="_blank"/.test(allHtml), false, "an external link opens in a new tab");
  assert.equal(/rel="noopener"/.test(allHtml), false, "a same-tab link carries rel=noopener, which does nothing");

  for (const [path, doc] of html) {
    const outbound = [...doc.matchAll(/<a[^>]*href="https?:\/\/[^"]*"[^>]*>([\s\S]*?)<\/a>/g)];
    assert.ok(outbound.length > 0, `${path}: no outbound links at all`);
    for (const m of outbound) {
      // An icon link has no text to annotate, so the glyph and the "(external
      // site)" span have nothing to attach to. Those carry their destination
      // in an aria-label instead, which is the whole accessible name rather
      // than a suffix on one - the DBHQ lockups in the masthead and footer,
      // and the GitHub mark.
      //
      // The exemption is deliberately narrow: no text content AND an explicit
      // label. A link that has text still has to be annotated, which is the
      // rule this test exists for.
      const iconOnly = /<(?:img|svg)\b/.test(m[0]) && !/>[^<>]*[A-Za-z]/.test(m[1].replace(/<[^>]*>/g, "><"));
      if (iconOnly) {
        assert.ok(
          /aria-label="[^"]+"/.test(m[0]),
          `${path}: icon-only outbound link with no aria-label: ${m[0].slice(0, 80)}`,
        );
        continue;
      }
      assert.ok(
        m[1].includes("8599") || m[1].includes("↗"),
        `${path}: outbound link without the north-east glyph: ${m[1].slice(0, 60)}`,
      );
      assert.ok(
        m[1].includes("(external site)"),
        `${path}: outbound link without the accessible annotation: ${m[1].slice(0, 60)}`,
      );
    }
  }
});

test("the sibling DBHQ sites are cross-linked, and this site is not listed on itself", () => {
  // Scoped to the footer block, not the whole document: the head carries this
  // page's own canonical and og:url on every page, which is not a self-link.
  for (const [path, doc] of html) {
    const m = doc.match(/<footer[\s\S]*?<\/footer>/);
    assert.ok(m, `${path}: no footer`);
    const foot = m[0];
    // Driven from site.also rather than a second copy of the list. A
    // hardcoded copy asserts that the footer matches what someone typed here
    // once, which is a different claim from matching the source of truth -
    // and it fails the day a sibling is added or dropped, for the wrong
    // reason. bbs and modem were dropped on 16 Sep 2026 and this test failed
    // on its own stale duplicate rather than on anything being wrong.
    for (const { href } of site.also) {
      assert.ok(foot.includes(href), `${path}: the footer does not link ${href}`);
    }
    assert.ok(site.also.length >= 3, "the sibling block has collapsed to almost nothing");
    assert.equal(foot.includes("terraken.dbhq.uk"), false, `${path}: the footer links this site to itself`);
  }
  assert.equal(
    site.also.some((a) => a.href.includes("terraken.dbhq.uk")),
    false,
    "the sibling list includes this site",
  );
  // Every sibling says what it is for. "Another DBHQ site" is not a reason to
  // click, and a list of bare names is what this block degrades into.
  for (const a of site.also) {
    assert.ok(a.desc.length > 20, `the sibling ${a.label} has no reason to click attached`);
  }
});

// ---------------------------------------------------------------------------
// Sitemap, llms.txt and edge configuration
// ---------------------------------------------------------------------------

test("the sitemap lists every page and never the 404", () => {
  const xml = read("sitemap-0.xml");
  for (const p of pages) {
    assert.ok(xml.includes(`${HOST}${p.path}`), `sitemap: missing ${p.path}`);
  }
  assert.equal(xml.includes("/404"), false, "sitemap: lists the 404 page");
  const count = [...xml.matchAll(/<loc>/g)].length;
  assert.equal(count, pages.length, `sitemap: ${count} entries, expected ${pages.length}`);
  assert.ok(read("sitemap-index.xml").includes("sitemap-0.xml"));
});

test("robots.txt points at the sitemap index on this host", () => {
  assert.ok(read("robots.txt").includes(`Sitemap: ${HOST}/sitemap-index.xml`));
});

test("llms.txt names every page and links it on this host", () => {
  const llms = read("llms.txt");
  for (const p of pages) {
    assert.ok(llms.includes(`${HOST}${p.path}`), `llms.txt: missing ${p.path}`);
  }
  assert.ok(llms.startsWith("# Terraken"), "llms.txt: no H1, or it is not the product name");
  assert.ok(llms.includes("DBHQ Consulting Ltd"), "llms.txt: does not name the publisher");
});

// llms.txt is a hand-written file listing the same siblings the footer renders
// from site.also, and it had already drifted once: it still named bbs and modem
// after both were dropped from the footer. A stale sibling list is worse here
// than in the footer, because this file exists to be read by something that
// cannot see the page and check.
test("llms.txt lists the same siblings the footer does, and no others", () => {
  const block = read("llms.txt").split("## Also from DBHQ")[1];
  assert.ok(block, "llms.txt: no Also from DBHQ section");

  // Compared with the trailing slash intact rather than normalised away. This
  // site is trailingSlash: 'always', so a sibling written without one is a
  // redirect the reader pays for, and normalising here would hide it.
  const listed = [...block.matchAll(/https:\/\/[^\s]+/g)].map((m) => m[0]);
  const expected = site.also.map((a) => a.href);

  assert.deepEqual(
    [...listed].sort(),
    [...expected].sort(),
    "llms.txt and site.also disagree about the siblings",
  );
});

test("the edge policy is tight, and stays tight", () => {
  const headers = read("_headers");
  assert.ok(headers.includes("Strict-Transport-Security"));
  // Read the directive off the CSP line itself. Matching the whole file also
  // reads the comment block above it, which discusses the directives by name.
  const cspLine = headers.split("\n").find((l) => l.trim().startsWith("Content-Security-Policy:"));
  assert.ok(cspLine, "_headers: no Content-Security-Policy");
  const directives = new Map(
    cspLine
      .replace(/^\s*Content-Security-Policy:\s*/, "")
      .split(";")
      .map((d) => d.trim())
      .filter(Boolean)
      .map((d) => {
        const [name, ...values] = d.split(/\s+/);
        return [name, values];
      }),
  );
  // GOOGLE TAG MANAGER IS THE ONLY THIRD-PARTY ORIGIN THIS SITE ALLOWS, added
  // 16 Sep 2026 with the consent-gated GA4 tag. The list is asserted exactly
  // rather than loosely, because "one analytics origin" is a decision and
  // "whatever accumulated" is not. A second vendor has to change this line,
  // which is where somebody gets asked what it sets.
  const GTM = "https://www.googletagmanager.com";
  assert.deepEqual(
    directives.get("script-src"),
    ["'self'", GTM],
    "_headers: script-src has gained or lost an origin",
  );
  assert.equal(
    directives.get("script-src").includes("'unsafe-inline'"),
    false,
    "_headers: script-src gained 'unsafe-inline' - analytics.js and consent.js exist to avoid exactly this",
  );
  assert.deepEqual(directives.get("default-src"), ["'self'"]);
  // connect-src carries the GA beacon endpoints. Losing one of them loses
  // measurement silently, so they are named rather than pattern-matched.
  assert.deepEqual(directives.get("connect-src"), [
    "'self'",
    GTM,
    "https://www.google-analytics.com",
    "https://*.google-analytics.com",
    "https://*.analytics.google.com",
  ]);
  assert.deepEqual(directives.get("frame-ancestors"), ["'none'"]);
  assert.deepEqual(directives.get("object-src"), ["'none'"]);
  assert.equal(
    /<script(?![^>]*\bsrc=)(?![^>]*type="application\/ld\+json")[^>]*>/.test(allHtml),
    false,
    "an inline script shipped, which the CSP will block at the edge",
  );
  for (const h of ["X-Content-Type-Options", "Referrer-Policy", "X-Frame-Options", "Permissions-Policy"]) {
    assert.ok(headers.includes(h), `_headers: no ${h}`);
  }
});

// ---------------------------------------------------------------------------
// How the name is written
//
// Dan, 16 Sep 2026: the product is **Terraken** in prose, one word, capital T.
// The K is never capitalised. But four other things share the string and every
// one of them must stay lowercase - the command, the module path, the hostname
// and the logotype - so this is not a spelling rule with one answer, and a
// blanket find-and-replace in either direction breaks something.
// ---------------------------------------------------------------------------

test("the name is never written as a compound of terra and Ken", () => {
  const surfaces = [...html, ["/404/", read("404.html")], ["llms.txt", read("llms.txt")]];
  for (const [path, doc] of surfaces) {
    assert.equal(/TerraKen|terraKen/.test(doc), false, `${path}: the K is capitalised`);
  }
});

test("the logotype stays lowercase, and prose does not", () => {
  const index = read("index.html");

  // THE LOGOTYPE IS IN THE MASTHEAD, NOT THE H1 (16 Sep 2026). The hero used to
  // repeat the mark and the wordmark directly under the identical pair in the
  // bar above, so the lockup now appears once and the h1 carries the claim.
  // A lowercase logotype beside a Title Cased name in prose is the adidas
  // pattern, and heliograph already does it in this estate.
  const lockup = index.match(/<(?:a|span)[^>]*class="hd-site"[^>]*>([\s\S]*?)<\/(?:a|span)>/);
  assert.ok(lockup, "no masthead lockup on the index");
  assert.equal(
    visibleText(lockup[1]).trim(),
    "terraken",
    "the masthead logotype has been Title Cased",
  );

  // And it appears exactly once on the page: putting it back in the hero is
  // the specific regression this guards.
  const lockups = [...index.matchAll(/class="hd-site"/g)];
  assert.equal(lockups.length, 1, "the wordmark lockup is on the page more than once");

  // The h1 is the claim, which is the tagline verbatim.
  const h1 = index.match(/<h1[^>]*>([\s\S]*?)<\/h1>/);
  assert.ok(h1, "no h1 on the index");
  assert.equal(visibleText(h1[1]).trim(), site.tagline, "the h1 is not the tagline");

  // Prose, by contrast, carries the capital. Checked against visibleText and
  // NOT against flat(), which lowercases everything it is given - an assertion
  // about casing run through flat() can never fail, which is how a casing test
  // ends up testing nothing.
  assert.match(site.lead, /^Terraken\b/, "the lead paragraph does not open with the product name");
  assert.equal(
    /\bterraken is a free\b/.test(visibleText(index)),
    false,
    "the lead still lowercases the product name",
  );
});

test("every command shown is the lowercase binary, never the product name", () => {
  // The single most likely casing regression: a find-and-replace that means to
  // fix prose and capitalises an invocation, so a reader copies a command that
  // is not on their PATH. Any "Terraken" followed by a flag, a plan or a
  // fixture is a command that has been wrongly capitalised.
  const surfaces = [...html, ["llms.txt", read("llms.txt")]];
  for (const [path, doc] of surfaces) {
    const text = visibleText(doc);
    // A flag, a plan or a fixture after the name means it is being invoked.
    // `-` alone is NOT in this list even though it is the real stdin form: the
    // page titles read "Terraken - everything you can know about a change",
    // and a hyphen used as punctuation is indistinguishable from the flag here.
    // The stdin invocation is covered by the pipe form, which is inside a code
    // element and lowercase by construction.
    const bad = [...text.matchAll(/\bTerraken\s+(--[a-z-]+|plan\.json|testdata\/)/g)];
    assert.equal(
      bad.length,
      0,
      `${path}: a command is capitalised - "${bad[0]?.[0]}". The binary is lowercase.`,
    );
    // The install line and the module path are identifiers all the way down.
    assert.equal(/github\.com\/dbhq-uk\/Terraken/.test(text), false, `${path}: module path capitalised`);
    assert.equal(/Terraken\.dbhq\.uk/.test(text), false, `${path}: hostname capitalised`);
  }
});

// ---------------------------------------------------------------------------
// Shipped against unshipped
//
// The site states an ambition much larger than the one command that exists.
// That is fine, and it is deliberate, but the whole proposition is that this
// tool can be trusted about a change nobody has vetted - so a site that
// oversells by one feature has spent exactly the thing it is selling.
//
// These tests hold the line: the roadmap is labelled, every item points at an
// open issue, and nothing from it leaks into a shipped list.
// ---------------------------------------------------------------------------

test("the roadmap is unmistakably labelled as not built", () => {
  const index = read("index.html");

  assert.ok(index.includes(roadmapIntro.kicker), "the roadmap kicker is missing from the page");
  assert.match(
    roadmapIntro.kicker,
    /not (yet )?built|coming|planned|unshipped/i,
    "the roadmap kicker no longer says the work is unbuilt",
  );

  // The kicker has to appear BEFORE the first roadmap item, or a reader meets
  // the list without the caveat.
  const kickerAt = index.indexOf(roadmapIntro.kicker);
  const firstItemAt = index.indexOf(roadmap[0].h);
  assert.ok(kickerAt > -1 && kickerAt < firstItemAt, "the roadmap list appears before its caveat");
});

test("every roadmap item links the open issue tracking it", () => {
  const index = read("index.html");
  assert.ok(roadmap.length >= 8, "the roadmap has collapsed to almost nothing");

  for (const r of roadmap) {
    assert.ok(Number.isInteger(r.issue) && r.issue > 0, `roadmap "${r.h}" has no issue number`);
    assert.ok(
      index.includes(`/issues/${r.issue}`),
      `roadmap "${r.h}" does not link issue #${r.issue}`,
    );
  }

  // Unique issues: the same number twice means one item was copied and not
  // re-pointed, which is how a link quietly starts describing the wrong thing.
  const ids = roadmap.map((r) => r.issue);
  assert.equal(new Set(ids).size, ids.length, "two roadmap items share an issue number");
});

test("no roadmap capability is written up as something the tool does", () => {
  // The shipped exports. If a roadmap heading turns up in one of these, an
  // unbuilt feature is being described in the present tense somewhere a reader
  // reads as fact.
  const shipped = [
    ...contracts.flatMap((c) => [c.h, c.p]),
    ...levels.map((l) => l.what),
    ...flags.map((f) => f.what),
    ...annotations.map((a) => a.what),
    site.tagline,
    site.lead,
  ].join(" ").toLowerCase();

  for (const r of roadmap) {
    assert.equal(
      shipped.includes(r.h.toLowerCase()),
      false,
      `the unshipped capability "${r.h}" appears in copy describing what the tool does`,
    );
  }
});

// ---------------------------------------------------------------------------
// Analytics, and the gate in front of it
//
// GA4 sets its cookie on the shared .dbhq.uk parent, so a cookie dropped by
// this site without consent is a cookie across the whole estate. modem shipped
// without a gate once and did exactly that. These tests hold the two facts
// that keep it from happening here: nothing loads before a choice, and the
// measurement ID is the estate's single stream rather than one of this site's
// own.
// ---------------------------------------------------------------------------

test("GA loads nothing until the reader accepts", () => {
  const analytics = read("analytics.js");

  // Consent Mode v2, all four signals denied, before anything else happens.
  for (const signal of ["ad_storage", "analytics_storage", "ad_user_data", "ad_personalization"]) {
    assert.match(
      analytics,
      new RegExp(`${signal}:\\s*"denied"`),
      `analytics.js: ${signal} is not denied by default`,
    );
  }

  // The tag URL must be reachable only from inside the enable function. If it
  // appears before that function is declared, something loads on page load.
  const enableAt = analytics.indexOf("__dbhqEnableGA = function");
  const tagAt = analytics.indexOf("googletagmanager.com/gtag/js");
  assert.ok(enableAt > -1, "analytics.js: no __dbhqEnableGA");
  assert.ok(tagAt > enableAt, "analytics.js: the GA tag is referenced outside the consent gate");

  // Only the real host is measured - not a local preview, not the pages.dev
  // build, which serves the same bytes.
  assert.match(analytics, /location\.hostname === "terraken\.dbhq\.uk"/);
});

test("the tag reports to the estate's one data stream, not a stream of its own", () => {
  // The rule, and the reasoning, are in dbhq/docs/reference/analytics.md: one
  // property, one stream, split by Hostname at reporting time. A stream of this
  // site's own would fragment every journey from dbhq.uk into a fresh session
  // and strand this host outside GA4's Search Console reporting.
  const ids = [...read("analytics.js").matchAll(/G-[A-Z0-9]{8,}/g)].map((m) => m[0]);
  assert.deepEqual([...new Set(ids)], ["G-3H3NFGSX85"], "analytics.js: wrong or extra measurement ID");
});

test("the consent prompt ships on every page, and refusing is no harder than accepting", () => {
  for (const [path, page] of html) {
    assert.ok(page.includes("data-consent-accept"), `${path}: no consent prompt`);
    assert.ok(page.includes("data-consent-decline"), `${path}: consent prompt has no decline`);
  }

  // consent.js calls __dbhqEnableGA, which analytics.js defines, and modules
  // execute in document order. Loading them the other way round silently breaks
  // Accept - it fails as "analytics never worked", which is hard to spot.
  const index = read("index.html");
  assert.ok(
    index.indexOf("/analytics.js") < index.indexOf("/consent.js"),
    "consent.js loads before analytics.js, so Accept will do nothing",
  );

  // Both buttons carry the same .btn sizing, so neither is the easy one. A
  // prompt that makes refusal harder is not consent, and the shared
  // dbhq-consent key would carry that across every *.dbhq.uk site.
  const accept = index.match(/<button[^>]*data-consent-accept[^>]*>/)[0];
  const decline = index.match(/<button[^>]*data-consent-decline[^>]*>/)[0];
  assert.ok(accept.includes("btn ") && decline.includes("btn "), "the consent buttons are styled differently");

  // Red on this site means exactly one thing - this change can destroy
  // something - and spending it on a cookie prompt is how that stops being
  // true. Read the component source rather than the built page: the styles are
  // inlined into every page, so searching the HTML cannot tell whose rule a
  // colour came from.
  const consent = readFileSync(
    new URL("../src/components/Consent.astro", import.meta.url),
    "utf8",
  );
  assert.equal(/--danger|#D92D20/i.test(consent), false, "the consent prompt uses the danger red");
});

test("the reader is told what is collected and can reach the policy", () => {
  const index = read("index.html");
  assert.ok(
    index.includes("https://dbhq.uk/privacy/"),
    "the consent prompt does not link the privacy policy",
  );
  assert.ok(/Google Analytics/.test(index), "the consent prompt does not name what it uses");
});
