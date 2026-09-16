// Structured data (schema.org JSON-LD) for terrakit.dbhq.uk.
//
// ONE ORGANISATION ACROSS THE ESTATE. The publisher node keeps dbhq.uk's own
// canonical @id - `https://dbhq.uk/#organization` - rather than minting a
// second identity for the same company on a third hostname. It is emitted here
// in a short form (name, legal name, url, logo, sameAs) so a consumer that only
// ever crawls this host still has a resolvable node, and dedupes to the full one
// on dbhq.uk when it sees both. Do not give this site its own Organization @id:
// two @ids for one company is exactly the drift the anchors exist to prevent.
// skills.dbhq.uk does the same thing for the same reason.
//
// The WebSite node IS this site's own, because this is a different website with
// its own name.
//
// WHAT EACH PAGE PUBLISHES, and why it is not all the same node type:
//   /                              WebPage + SoftwareApplication - the tool itself
//   /terraform-moved-block/        TechArticle + FAQPage - a reference document
//   /terraform-rename-resource/    TechArticle + HowTo + FAQPage - a procedure
//   /terraform-state-mv/           TechArticle + FAQPage - a decision
//   /terraform-taint/              TechArticle + FAQPage - a reference document
//   /terraform-forces-replacement/ TechArticle + FAQPage - a reference document
//   /docs/                         TechArticle - the flag and format reference
// A guide page is not a page about the software, it is a page about Terraform
// that mentions the software, and the structured data says so.
//
// Only the rename page carries a HowTo, and that is deliberate rather than an
// omission: it is the one page whose subject is an ordered procedure with a
// single outcome. The other guides answer "what is this and what should I do
// about it", where the answer branches, and a HowTo that flattens a decision
// into numbered steps claims a certainty the page does not have.
//
// ONE NODE PER PAGE. Where a page is an article, the PAGE NODE IS the
// TechArticle - `pageProps` below merges the article's own properties onto it -
// rather than a WebPage carrying a second TechArticle beside it. The first
// version of this file did the second thing, and it published two nodes for one
// document with two different @ids, which is exactly the drift the shared
// Organization @id exists to prevent, committed one level down.

import {
  REPO,
  contracts,
  movedFaqs,
  renameFaqs,
  replacementFaqs,
  stateMvFaqs,
  taintFaqs,
  site,
  type Faq,
} from "./site";

const BASE = site.url;
const DBHQ = "https://dbhq.uk";

export const ORG_ID = `${DBHQ}/#organization`;
export const WEBSITE_ID = `${BASE}/#website`;
export const SOFTWARE_ID = `${BASE}/#software`;

const SITE_NAME = "terrakit";
const SITE_DESCRIPTION =
  "terrakit reads a Terraform or OpenTofu plan and ranks the change by how much damage it can do. Free, open source, offline and deterministic, and it never prints an attribute's value.";

function organizationNode() {
  return {
    "@type": "Organization",
    "@id": ORG_ID,
    name: "DBHQ",
    legalName: "DBHQ Consulting Ltd",
    url: `${DBHQ}/`,
    logo: {
      "@type": "ImageObject",
      url: `${DBHQ}/icon-512.png`,
      width: 512,
      height: 512,
    },
    email: "dan@dbhq.uk",
    sameAs: ["https://www.linkedin.com/company/dbhq-uk", "https://github.com/dbhq-uk"],
  };
}

function webSiteNode() {
  return {
    "@type": "WebSite",
    "@id": WEBSITE_ID,
    url: `${BASE}/`,
    name: SITE_NAME,
    description: SITE_DESCRIPTION,
    publisher: { "@id": ORG_ID },
    inLanguage: "en-GB",
  };
}

// One SoftwareApplication for the tool, on the index and nowhere else.
//
// `disambiguatingDescription` carries the three contracts. They are the reason
// the tool is safe to point at an unvetted plan, so they belong in the
// machine-readable copy as well as on the page: a boundary stated to a reader
// and hidden from an engine is only half stated.
export function softwareNode() {
  return {
    "@type": "SoftwareApplication",
    "@id": SOFTWARE_ID,
    name: "terrakit",
    description: SITE_DESCRIPTION,
    applicationCategory: "DeveloperApplication",
    applicationSubCategory: "Command-line tool",
    operatingSystem: "Linux, macOS",
    softwareRequirements: "A plan file from terraform show -json, or the same JSON on standard input",
    // `url` is the URL OF THE ITEM, which is this page. The repository belongs
    // in codeRepository and downloadUrl, and only there - the same split
    // skills.dbhq.uk settled on, for the same reason.
    url: `${BASE}/`,
    codeRepository: REPO,
    downloadUrl: `${REPO}/releases`,
    installUrl: `${BASE}/docs/`,
    publisher: { "@id": ORG_ID },
    isAccessibleForFree: true,
    license: "https://opensource.org/licenses/MIT",
    offers: { "@type": "Offer", price: "0", priceCurrency: "GBP" },
    featureList: [
      "Ranks a plan by how much damage each change can do",
      "Escalates a destroy to critical when the resource type holds data",
      "Names the attribute that forced a replacement, using Terraform's own stated reason",
      "Detects a rename that forgot its moved block, and shows its working",
      "Names attributes whose before and after are the same value written differently",
      "Says what cannot be known until apply",
      "Terminal, markdown, JSON and self-contained HTML output",
    ],
    disambiguatingDescription: contracts.map((c) => c.h).join(". "),
  };
}

function faqNode(id: string, faqs: readonly Faq[]) {
  return {
    "@type": "FAQPage",
    "@id": id,
    mainEntity: faqs.map((f) => ({
      "@type": "Question",
      name: f.q,
      acceptedAnswer: { "@type": "Answer", text: f.a },
    })),
  };
}

// Article properties, merged ONTO the page node rather than published beside
// it. See the note at the top of this file.
export const movedBlockArticle = {
  headline: "Terraform moved blocks, and renaming without destroying",
  about: [
    { "@type": "Thing", name: "Terraform" },
    { "@type": "Thing", name: "Terraform moved block" },
    { "@type": "Thing", name: "Infrastructure as code" },
  ],
  proficiencyLevel: "Beginner",
  publisher: { "@id": ORG_ID },
  mentions: { "@id": SOFTWARE_ID },
};

export const renameArticle = {
  headline: "Renaming a Terraform resource without destroying it",
  about: [
    { "@type": "Thing", name: "Terraform" },
    { "@type": "Thing", name: "Terraform refactoring" },
  ],
  proficiencyLevel: "Beginner",
  publisher: { "@id": ORG_ID },
  mentions: { "@id": SOFTWARE_ID },
};

export const stateMvArticle = {
  headline: "terraform state mv and state rm, and when a block is the better answer",
  about: [
    { "@type": "Thing", name: "Terraform" },
    { "@type": "Thing", name: "Terraform state" },
    { "@type": "Thing", name: "Infrastructure as code" },
  ],
  proficiencyLevel: "Beginner",
  publisher: { "@id": ORG_ID },
  mentions: { "@id": SOFTWARE_ID },
};

export const taintArticle = {
  headline: "Terraform taint and untaint, and the -replace option that succeeded them",
  about: [
    { "@type": "Thing", name: "Terraform" },
    { "@type": "Thing", name: "Terraform state" },
  ],
  proficiencyLevel: "Beginner",
  publisher: { "@id": ORG_ID },
  mentions: { "@id": SOFTWARE_ID },
};

export const replacementArticle = {
  headline: "Why a Terraform plan says forces replacement",
  about: [
    { "@type": "Thing", name: "Terraform" },
    { "@type": "Thing", name: "Terraform plan" },
  ],
  proficiencyLevel: "Beginner",
  publisher: { "@id": ORG_ID },
  mentions: { "@id": SOFTWARE_ID },
};

export const docsArticle = {
  headline: "terrakit reference",
  about: { "@id": SOFTWARE_ID },
  proficiencyLevel: "Expert",
  publisher: { "@id": ORG_ID },
};

export function movedBlockNodes(canonical: string) {
  return [faqNode(`${canonical}#faq`, movedFaqs)];
}

export function stateMvNodes(canonical: string) {
  return [faqNode(`${canonical}#faq`, stateMvFaqs)];
}

export function taintNodes(canonical: string) {
  return [faqNode(`${canonical}#faq`, taintFaqs)];
}

export function replacementNodes(canonical: string) {
  return [faqNode(`${canonical}#faq`, replacementFaqs)];
}

export function renameNodes(canonical: string) {
  return [
    {
      "@type": "HowTo",
      "@id": `${canonical}#howto`,
      name: "Rename a Terraform resource without destroying it",
      description:
        "Four steps, and the third is the one people skip: read the plan and confirm the move before applying it.",
      totalTime: "PT10M",
      step: [
        {
          "@type": "HowToStep",
          position: 1,
          name: "Rename the resource in the configuration",
          text: "Change the resource label to the new name, and update every reference to it.",
        },
        {
          "@type": "HowToStep",
          position: 2,
          name: "Add a moved block",
          text: "In the module whose addresses it names, write a moved block with the old address as from and the new address as to.",
        },
        {
          "@type": "HowToStep",
          position: 3,
          name: "Read the plan before applying it",
          text: "terraform plan should report the move and plan nothing. If a destroy and a create are still there, the addresses in the moved block do not match the ones in the plan: copy both out of the plan rather than retyping them, check the instance key, and check the block is in the module that can see both addresses.",
        },
        {
          "@type": "HowToStep",
          position: 4,
          name: "Apply, and leave the block in place",
          text: "Keep the moved block until everyone who runs this configuration has applied it, including CI. A moved block whose from address is not in the state does nothing.",
        },
      ],
    },
    faqNode(`${canonical}#faq`, renameFaqs),
  ];
}

interface PageInput {
  canonical: string;
  title: string;
  description: string;
  image: string;
  pageType?: string;
  mainEntityId?: string;
  /** Extra properties merged onto the page node, for a page that IS an article. */
  pageProps?: Record<string, unknown>;
  /** Breadcrumb label for a child page. Omitted on the index. */
  crumb?: string;
}

function webPageNode(p: PageInput) {
  const node: Record<string, unknown> = {
    "@type": p.pageType ?? "WebPage",
    "@id": p.canonical,
    url: p.canonical,
    name: p.title,
    description: p.description,
    isPartOf: { "@id": WEBSITE_ID },
    primaryImageOfPage: { "@type": "ImageObject", url: p.image },
    inLanguage: "en-GB",
    breadcrumb: { "@id": `${p.canonical}#breadcrumb` },
    ...(p.pageProps ?? {}),
  };
  if (p.mainEntityId) node.mainEntity = { "@id": p.mainEntityId };
  return node;
}

// The site is flat: every page is Home, or Home > <page>.
function breadcrumbNode(p: PageInput) {
  const items: Record<string, unknown>[] = [
    { "@type": "ListItem", position: 1, name: "terrakit", item: `${BASE}/` },
  ];
  if (p.crumb) {
    items.push({ "@type": "ListItem", position: 2, name: p.crumb, item: p.canonical });
  }
  return {
    "@type": "BreadcrumbList",
    "@id": `${p.canonical}#breadcrumb`,
    itemListElement: items,
  };
}

export function pageGraph(p: PageInput, extra: object[] = []) {
  return {
    "@context": "https://schema.org",
    "@graph": [organizationNode(), webSiteNode(), webPageNode(p), breadcrumbNode(p), ...extra],
  };
}
