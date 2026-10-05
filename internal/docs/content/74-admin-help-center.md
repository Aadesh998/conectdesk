# Admin: help center

Public help-center websites with articles customers can browse and search. Published articles marked for AI use are also a knowledge source for Juno and the AI assistants. What customers see is described under Customer-facing pages.

## Help center list

**Where:** Admin > Help center > Help center. URL `/admin/help-center`
**Who can use it:** `help_center:manage`
**Purpose:** Create one or more public help-center websites.
**Help panel:** "Create public help centers where customers browse and search articles. Published articles can also be used by the AI agent to answer conversations."

| Element | What it does |
|---|---|
| **New** | Opens a side panel with these fields: **Name** (fills in the slug automatically), **Slug** ("Used in the help center URL."), **Page title**, **Template**. Template is either **Classic** ("Landing page layout with a hero banner, a large search bar and collection cards.") or **Docs** ("Documentation layout with a top navigation bar, a sidebar listing collections and quick search (Ctrl+K)."). **Cancel** / **Create** follow. After you create one, the Customize screen opens. |
| Table | Columns: Name (click to open the articles tree), Slug, Status (Active or Paused), Updated at. |
| Row menu | **Articles** (opens the tree), **Edit** (opens Customize), **Visit site** (opens `<root URL>/hc/<slug>` in a new tab), **Pause site** / **Resume site**, **Delete** ("This will permanently delete the help center, including all its collections and articles."). |

## Help center: Customize (edit)

**Where:** List > row menu > Edit, or Tree > three-dot menu > Edit. URL `/admin/help-center/:id/customize`
**Who can use it:** `help_center:manage`
**Purpose:** Branding, languages, layout and SEO settings, with a live preview on the right.

**Preview controls:**
- Page dropdown: **Landing page** / **Article page**.
- Device dropdown: **Desktop**, **Tablet**, **Mobile**.
- The preview mimics a browser tab with your favicon and title.

**General tab:**
- **Name**
- **Slug**
- **Custom domain (optional)**: "Serve this help center on its own domain instead of the app URL." Enter it as `https://help.example.com`. It must first be added and verified under Admin > Domains, and it cannot be one another help center already uses.
- **Page title**
- **Template**: Classic or Docs.
- **Meta description**: "Shown in search-engine results and social previews for the help center home page. Leave blank to use the header text."
- **Navigation links**: label and URL rows, with **Add** and remove.
- **Supported languages**: language rows, with **Add** and remove. "Add a language before authoring articles in it." You cannot remove a language that still has content.
- **Default language**: "Visitors land on this language unless they pick another."

**Appearance tab** (collapsible sections):
- **Brand**: Logo URL, Primary color (hex, e.g. `#1f93ff`), Favicon URL.
- **Header & hero**:
  - Header text
  - Tagline (Markdown allowed: bold, italic, links)
  - Background: Default (accent tint), Solid color, Gradient or Image. Depending on the choice you then set Background color, Gradient start and Gradient end, or Background image URL. Recommended image size is at least 1600x500.
  - Text color (blank uses the default)
- **Announcement**: Message, Label, URL. "A dismissible bar shown above the header on every page. Leave the text empty to hide it." A link without text is refused.
- **Landing page**:
  - Collection layout: Card grid or One per row
  - Cards per row: 2 or 3
  - Card icon position: Beside the title, Above the title or Centered
  - Toggles: Show popular articles (with a "Popular articles heading" box), Show collection descriptions, Show article counts, Show icon background tile, Show author avatars
- **Article page**: toggles Show table of contents, Show related articles, Show author.
- **Footer**: Background color, Text color, Tagline, Links (label/URL rows), Social links (Platform: Website, X (Twitter), GitHub, LinkedIn, Facebook, Instagram, YouTube, plus a URL).
- **Custom code**: Custom CSS and Custom JS, both added to every public page.

**Cancel** / **Save** at the bottom.

## Help center: Articles tree

**Where:** List > click a name, or row menu > Articles. URL `/admin/help-center/:id/tree/:locale?`
**Who can use it:** `help_center:manage`
**Purpose:** Organise collections (folders) and articles, write articles, and publish them.
**Help panel:** "Group articles into collections. Drag a row by its handle to change the order readers see."

**Top bar:**
- A "Paused" badge if the site is paused.
- Language dropdown (only shown with more than one language).
- **Expand all / Collapse all**.
- **Search insights**: opens a panel with **Top searches** and **Searches with no results** (search term and count). Use it to find missing articles.
- **Visit site**.
- **New collection**.
- Three-dot menu: **Edit** (Customize), **Pause site** / **Resume site**, **Delete**.

**Tree rows:**
- Collections show an article count. Articles show a "Draft" badge if unpublished.
- Drag the handle to reorder. Articles can be dragged into another collection.
- Row menu: **Edit**, **New collection** (a sub-collection; nesting goes up to 3 levels), **New article**, **Move up** / **Move down**, **Publish** / **Unpublish**, **Delete**.
- Deleting a collection also deletes all its articles.

**Article editor** (side sheet, "New article" / "Edit article"):

| Field | What it does |
|---|---|
| **Title** | The article heading. |
| Body | Rich-text editor ("Write your article..."). Images can be placed inline. |
| **Status** | Draft or Published. |
| **Collection** | Must be in the same language as the article. |
| **Written by** | Pick an agent or an AI assistant as the shown author. |
| **Allow AI assistants to use this article** (toggle) | Makes the article a knowledge source for the AI. "Takes effect only when the article is published. Enable it in one language only, not in every translation." |
| **Language** | The article's language. |
| **Excerpt** | "A one-line summary shown under the article title and in search results." |
| **SEO & metadata** | **Meta title** (defaults to the title), **Meta description** (defaults to the excerpt), **Social image URL**. |
| Read-only information (existing articles) | Created by, Feedback (helpful and not-helpful counts from readers), Created at, Updated at, Views. |
| **Cancel** / **Create** or **Update** | |

**Collection editor** (side sheet):
- **Name**, **Description**
- **Published** (toggle)
- **Icon** (searchable icon picker, or **Remove**)
- **Language**
- **Parent collection** (None or another collection)
- Read-only: Created at, Updated at, Articles count
- **Cancel** / **Create** or **Update**
