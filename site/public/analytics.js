// GA4, loaded from a file rather than an inline block.
//
// This site's Content-Security-Policy has no 'unsafe-inline' in script-src and
// should not gain one. Google's own snippet is inline, so it is rewritten here
// as an ordinary same-origin module and the tag itself is injected. Nothing
// about the measurement changes; only where the code lives. bbs and modem do
// the same thing for the same reason - see dbhq/docs/reference/analytics.md.
//
// ON BY DEFAULT, WITH A NOTICE AND A SIMPLE OPT-OUT, under the PECR
// statistical-purposes exception (Data (Use and Access) Act 2025). Until
// 30 Sep 2026 this denied everything and loaded nothing until the reader
// clicked Accept. The exception needs clear information and a simple, free way
// to object rather than prior consent, and it holds only while the measurement
// is statistics and nothing else - so ad storage is denied, and Google Signals
// and ad personalisation are off in the config. Do not relax either: that is
// the line between counting visits and something the exception does not
// cover. The estate-wide pattern and the reasoning are in
// dbhq/docs/reference/analytics.md, and this file ports dbhq.uk's gate
// (web/apex/src/layouts/Base.astro) as closely as a module allows.
//
// consent.js draws the notice and calls window.dbhqAnalytics. It loads after
// this file, so the object is always there by the time it looks.
//
// GA4 IS NOT LOADED AT ALL for a reader who opted out, for a likely bot, or
// off the live host.
//
// THE MEASUREMENT ID IS THE ESTATE'S, NOT THIS SITE'S, and that is the rule
// rather than a shortcut. Property 544327698 carries exactly one data stream.
// A stream of terraken's own would give this host its own _ga_<id> cookie on
// that same shared parent, so a reader arriving from dbhq.uk would start a
// fresh session here and the journey between the two would be lost. It would
// also strand this site outside GA4's Search Console reporting, because that
// link binds to exactly one data stream. Split the sites at reporting time
// with the Hostname dimension instead.
//
// DO NOT CREATE A DATA STREAM FOR THIS SITE. The three-stream layout this
// replaced looked tidier in the GA4 UI and quietly broke every cross-site
// journey. dbhq/docs/reference/analytics.md is the record.
const MEASUREMENT_ID = "G-3H3NFGSX85";

// Never measure a local preview or a Pages branch build - only the real host.
// terraken.pages.dev serves the same bytes and is not the site.
const PROD = location.hostname === "terraken.dbhq.uk";

// The choice is a cookie on .dbhq.uk, so opting out on one site opts out on
// every *.dbhq.uk site - the _ga cookie it stops is estate-wide too. A
// localStorage "dbhq-consent" left by the old opt-in prompt is read once and
// carried over: "denied" stays an opt-out.
function readChoice() {
  const m = document.cookie.match(/(?:^|; )dbhq_analytics=(on|off)(?:;|$)/);
  if (m) return m[1];
  try {
    const old = localStorage.getItem("dbhq-consent");
    if (old === "denied") return "off";
    if (old === "granted") return "on";
  } catch (e) {
    // localStorage throws rather than returning null in some privacy modes.
  }
  return null;
}

function writeChoice(v) {
  let c = "dbhq_analytics=" + v + "; Max-Age=31536000; Path=/; SameSite=Lax; Secure";
  if (/(^|\.)dbhq\.uk$/.test(location.hostname)) c += "; Domain=dbhq.uk";
  document.cookie = c;
  try {
    localStorage.removeItem("dbhq-consent");
  } catch (e) {
    // Nothing to remove if storage cannot be read either.
  }
}

// Bots that run JavaScript, and scrapers rotating desktop Chrome or Firefox
// about two years stale. Mobile, Win7/8 and Firefox ESR are exempt. Kept
// identical to ScentVerdict's svAnalytics.likelyBot and to dbhq.uk's copy.
function likelyBot() {
  try {
    if (navigator.webdriver) return true;
    var ua = navigator.userAgent || "";
    if (/bot|crawl|spider|headless/i.test(ua)) return true;
    if (/Android|Mobile|CrOS/.test(ua) || !/Windows NT 10\.0|Macintosh|X11/.test(ua)) return false;
    var n = Math.max(0, Math.floor((Date.now() - Date.UTC(2025, 8, 2)) / 2592e6));
    var c = /Chrome\/(\d+)\./.exec(ua);
    if (c) return +c[1] < 140 + n - 24;
    var f = /Firefox\/(\d+)\./.exec(ua);
    if (f) return [115, 128, 140, 153].indexOf(+f[1]) < 0 && +f[1] < 142 + n - 24;
  } catch (e) {}
  return false;
}

// GA4 sets _ga on the highest domain it can (.dbhq.uk), so expire the cookies
// on this host and on every parent domain.
function deleteGaCookies() {
  const parts = location.hostname.split(".");
  document.cookie.split("; ").forEach((c) => {
    const name = c.split("=")[0];
    if (name === "_ga" || name.indexOf("_ga_") === 0) {
      const expired = name + "=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/";
      document.cookie = expired;
      for (let i = 0; i < parts.length - 1; i++) {
        document.cookie = expired + "; domain=." + parts.slice(i).join(".");
      }
    }
  });
}

window.dataLayer = window.dataLayer || [];
function gtag() {
  // Deliberately `arguments`, not a rest parameter: gtag reads the live
  // Arguments object itself, and a real array does not behave the same way.
  window.dataLayer.push(arguments);
}
window.gtag = gtag;

const bot = likelyBot();
let loaded = false;

// THE ONLY PLACE THE TAG IS REFERENCED. Everything above it decides whether
// this runs; nothing below it loads GA any other way.
function load() {
  if (!PROD || bot || loaded) return;
  loaded = true;
  gtag("consent", "default", {
    analytics_storage: "granted",
    ad_storage: "denied",
    ad_user_data: "denied",
    ad_personalization: "denied",
  });
  gtag("js", new Date());
  gtag("config", MEASUREMENT_ID, {
    allow_google_signals: false,
    allow_ad_personalization_signals: false,
  });
  const tag = document.createElement("script");
  tag.async = true;
  tag.src = "https://www.googletagmanager.com/gtag/js?id=" + MEASUREMENT_ID;
  document.head.appendChild(tag);
}

window.dbhqAnalytics = {
  choice: readChoice,
  keepOn() {
    writeChoice("on");
    window["ga-disable-" + MEASUREMENT_ID] = false;
    if (loaded) gtag("consent", "update", { analytics_storage: "granted" });
    load();
  },
  // Denied consent alone still lets GA4 send cookieless pings, including the
  // user_engagement hit it flushes when the page is left. Google's ga-disable
  // flag stops every hit from this page; later pages do not load GA4 at all.
  optOut() {
    writeChoice("off");
    window["ga-disable-" + MEASUREMENT_ID] = true;
    if (loaded) gtag("consent", "update", { analytics_storage: "denied" });
    deleteGaCookies();
  },
};

// A choice carried over from the old localStorage key is written to the cookie
// on this first visit, so the old key is gone after one page.
const choice = readChoice();
if (choice && !/(?:^|; )dbhq_analytics=/.test(document.cookie)) writeChoice(choice);
if (choice !== "off") load();
