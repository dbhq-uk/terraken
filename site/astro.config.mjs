import { defineConfig } from 'astro/config';
import sitemap from '@astrojs/sitemap';
import { lastmodFor } from './src/lib/content-dates.mjs';

// Per-path sitemap hints.
//
// THE PRIORITIES ARE THE SEARCH DEMAND, NOT THE SITE'S OWN OPINION OF ITSELF.
// /terraform-moved-block/ is the page this site was built for - "terraform
// moved block" and "terraform moved" measure 1,300 a month each worldwide,
// where "terraform plan review" and every phrasing around it measure zero. So
// it ranks level with the index rather than below it, and tests/content.test.mjs
// fails the build if it ever stops doing so.
//
// The three pages added on 16 Sep 2026 sit at 0.9 on measured worldwide volume
// (docs/research/terraken-seo-worldwide.md): state mv with state rm at 1,590 a
// month, taint with untaint at 1,510, and the replacement cluster at 1,150.
//
// /terraform-rename-resource/ MOVED DOWN, from 0.9 to 0.8, and that is a
// deliberate re-ranking rather than a tidy-up. It was second when there were
// two guides; on worldwide numbers its cluster measures 210 a month against
// 1,150 for the smallest of the three new pages, so it now sits between them
// and /docs/. It is still a page worth having - it answers a real question the
// anchor page does not - and it is still ahead of reference material for
// somebody who already has the tool.
const HINTS = {
  '/': { changefreq: 'monthly', priority: 1.0 },
  '/terraform-moved-block/': { changefreq: 'monthly', priority: 1.0 },
  '/terraform-state-mv/': { changefreq: 'monthly', priority: 0.9 },
  '/terraform-taint/': { changefreq: 'monthly', priority: 0.9 },
  '/terraform-forces-replacement/': { changefreq: 'monthly', priority: 0.9 },
  '/terraform-rename-resource/': { changefreq: 'monthly', priority: 0.8 },
  '/docs/': { changefreq: 'monthly', priority: 0.7 },
};

export default defineConfig({
  site: 'https://terraken.dbhq.uk',
  // Trailing slash everywhere, matching dbhq.uk and skills.dbhq.uk: one
  // canonical form per page, so /docs and /docs/ never both look canonical.
  trailingSlash: 'always',
  // 'always', not 'auto': the pages are small and every stylesheet inlined is a
  // render-blocking request removed from the critical path. Same setting, and
  // the same reasoning, as the other two DBHQ Astro sites.
  build: { inlineStylesheets: 'always' },
  integrations: [
    sitemap({
      filter: (page) => !new URL(page).pathname.startsWith('/404'),
      serialize(item) {
        const { pathname } = new URL(item.url);
        const hint = HINTS[pathname] ?? { changefreq: 'monthly', priority: 0.7 };
        // Real per-page dates from git, not the build stamp - see
        // src/lib/content-dates.mjs for why that distinction matters.
        const lastmod = lastmodFor(pathname);
        return { ...item, ...hint, ...(lastmod ? { lastmod } : {}) };
      },
    }),
  ],
});
