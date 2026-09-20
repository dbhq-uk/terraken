// Real per-page content-change dates, from git history.
//
// The sitemap carried no <lastmod> until 20 Sep 2026, which left Google
// without the one sitemap hint it still acts on - it has said it largely
// disregards changefreq and priority. Publishing none left the only useful
// field empty.
//
// WHY NOT THE BUILD TIMESTAMP. It would move all eight dates together on every
// deploy, telling a crawler nothing except that the site was rebuilt. Google
// discounts a lastmod it cannot trust, so a date that moves on every deploy is
// worth no more than no date at all. dbhq.uk and skills.dbhq.uk both solved
// this the same way.
//
// HOW A PAGE IS DATED. Every page here is its own .astro file, so a page's
// date is simply the last commit touching that file. No blame ranges are
// needed - unlike skills.dbhq.uk, where seventeen pages share one data file.
//
// WHAT IS DELIBERATELY EXCLUDED: the layout and shared components. A spacing
// fix in the layout is a real change to every page's HTML but not a change to
// what any page is about, and letting it move all eight dates at once is the
// problem being solved rather than the behaviour wanted.
//
// REQUIRES FULL GIT HISTORY. On a shallow clone `git log` for a path returns
// only what the clone contains, so pages would share the deploy commit's date
// - the build stamp this replaces. deploy-site.yml checks out with
// fetch-depth: 0, and this refuses to build otherwise rather than publishing
// eight identical dates quietly.

import { execFileSync } from "node:child_process";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO = resolve(HERE, "../../..");

const git = (args) => execFileSync("git", args, { cwd: REPO, encoding: "utf8" });

function assertFullHistory() {
  if (git(["rev-parse", "--is-shallow-repository"]).trim() === "true") {
    throw new Error(
      "terraken site content-dates: shallow clone, so every page would get the " +
        "deploy date. Check out with fetch-depth: 0.",
    );
  }
}

/** The source file behind a route: / -> index.astro, /docs/ -> docs/index.astro. */
function sourceFor(pathname) {
  const clean = pathname.replace(/^\/|\/$/g, "");
  return clean === ""
    ? "site/src/pages/index.astro"
    : `site/src/pages/${clean}/index.astro`;
}

let checked = false;

export function lastmodFor(pathname) {
  if (!checked) {
    assertFullHistory();
    checked = true;
  }
  const out = git(["log", "-1", "--format=%cI", "--", sourceFor(pathname)]).trim();
  return out ? out.slice(0, 10) : null;
}
