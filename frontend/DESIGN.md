# ConectDesk Design System

Reference for colors, typography, spacing, radius, elevation, and component conventions
across both apps (agent dashboard + livechat widget).

**Source of truth:**
- Tokens (CSS variables): `shared-ui/assets/styles/main.scss` - `:root, .light` and `.dark` blocks
- Tailwind mapping: `tailwind.config.cjs` - `theme.extend.colors` and `borderRadius`
- Primitives: `shared-ui/components/ui/` (shadcn-vue)

**Golden rule:** never hardcode a value that a token exists for. Use `bg-success`, not
`bg-green-600`. Use `rounded-lg`, not `rounded`. Every token has a light and a dark value;
if you add one, define both.

---

## 1. Color tokens

HSL channel triples in CSS variables, consumed via `hsl(var(--x))` (Tailwind classes like
`bg-primary` do it for you). Opacity modifiers work: `bg-success/10`, `text-primary/80`.

| Token | Light | Dark | Use for |
|---|---|---|---|
| `background` | `0 0% 100%` (white) | `240 10% 3.9%` (zinc-950) | app/content surface |
| `foreground` | `240 10% 3.9%` (zinc-950) | `0 0% 98%` | primary text |
| `foreground-lighter` | `240 3.8% 46.1%` (zinc-500) | `240 5% 64.9%` (zinc-400) | idle sidebar nav items |
| `card` / `card-foreground` | `0 0% 100%` / zinc-950 | `240 5.9% 10%` (zinc-900) / `0 0% 98%` | raised card surface |
| `popover` / `popover-foreground` | `0 0% 100%` / zinc-950 | zinc-900 / `0 0% 98%` | menus, popovers, dropdowns |
| `primary` / `primary-foreground` | `224.3 76.3% 48%` (blue-700) / blue-50 | `221.2 83.2% 53.3%` (blue-600) / white | brand, active state, unread badges, primary buttons |
| `secondary` / `secondary-foreground` | `240 4.8% 95.9%` (zinc-100) / zinc-900 | `240 3.7% 15.9%` (zinc-800) / `0 0% 98%` | secondary buttons, outgoing message bubbles |
| `muted` / `muted-foreground` | zinc-100 / zinc-500 | zinc-800 / zinc-400 | muted backgrounds, captions/meta text, secondary labels |
| `accent` / `accent-foreground` | zinc-100 / zinc-900 | zinc-800 / `0 0% 98%` | hover and selected states |
| `destructive` / `destructive-foreground` | `0 72.2% 50.6%` (red-600) | `0 91% 71.4%` (red-400) | errors, delete, SLA breached, overdue, offline |
| `success` / `success-foreground` | `142.1 76.2% 36.3%` (green-600) | `142.1 70.6% 45.3%` (green-500) | positive/met, verified, online, connected, delivered |
| `warning` / `warning-foreground` | `37.7 92.1% 50.2%` (amber-500) | `45.9 96.7% 64.5%` (amber-300) | away, pending, SLA approaching, connecting (as background) |
| `warning-600` | `26 90.5% 37.1%` (amber-700) | amber-300 | warning as TEXT/icon color. Plain `warning` fails 4.5:1 on light |
| `link` | blue-700 | `213.1 93.9% 67.8%` (blue-400) | links inside rendered email/message content only. UI links use `.link-style` |
| `border` | `240 5.9% 90%` (zinc-200) | zinc-800 | all borders/dividers |
| `input` | zinc-200 | `240 4% 19%` | form field borders |
| `ring` | `240 5% 64.9%` (zinc-400) | zinc-500 | focus rings |
| `private` | `48 100% 96.1%` (amber-50) | `32 40% 14%` | private-note background tint |
| `canvas` | zinc-100 | `240 10% 2.5%` | app gutter behind floating panels (deepest surface) |

The palette follows `UI-GUIDELINES.md` (logchef's shadcn-vue theme): zinc neutrals and a
blue accent. The guideline's dark `primary` is blue-800; the app uses blue-600 instead
because `text-primary` (unread counts, active links) is unreadable in blue-800 on zinc-950.

**Sidebar tokens** (left nav chrome): `sidebar-background` matches `background` in both
themes, plus `sidebar-foreground`, `sidebar-primary`, `sidebar-accent`,
`sidebar-accent-foreground`, `sidebar-border`, `sidebar-ring`.

**Theme switching**: the user menu has a Light / Dark / System control
(`useColorMode().store`: `light | dark | auto`); VueUse writes the `dark` class on `<html>`
and keeps the choice in localStorage.

**Chart tokens** (unovis): `--vis-primary-color: var(--primary)`,
`--vis-secondary-color: var(--success)`, `--vis-text-color: var(--muted-foreground)`.

### Semantic status mapping

Color carries meaning; use the semantic token, never a raw palette color.

- **success (green):** SLA met, identity verified, agent online, widget connected, message delivered/read
- **warning (amber):** agent away, SLA approaching/remaining, widget connecting, no-internet banner
- **destructive (red):** error, delete, SLA breached, SLA overdue
- **primary (blue):** brand identity, active nav, unread count badges, primary actions
- **foreground / muted:** neutral data (counts, totals, timestamps). Do not color a number
  unless the color means something. On the reports dashboard numbers are neutral; only
  met=success and breached=destructive are colored.

### Deliberate exceptions

- `features/conversation/message/attachment/BubbleAttachmentItem.vue` colors attachment
  icons by file type (pdf=red, spreadsheet=green, doc=blue, archive=amber, audio=purple)
  with raw palette classes. The color is file-type identity, not status, and no token means
  "blue = document". Leave it.
- `components/editor/TextEditor.vue` - styles for rendered email HTML. Emails are
  standalone documents, not themed.
- `features/admin/inbox/LivechatInboxForm.vue` / `LivechatWidgetPreview.vue` - the
  customer-configurable widget brand color and its defaults.
- `features/conversation/ReplyBox.vue` - a `linear-gradient(#000 0 0)` CSS mask trick, not
  a color choice.

---

## 2. Surfaces and depth

Both themes use three tiers so panels read as floating, not flat:

```text
canvas (deepest gutter behind the panels)
  └─ background (app content + sidebar chrome, same value)
        └─ card / popover (lifted by border + shadow-sm)
```

In light mode `card` is pure white against a near-white background. In dark mode `card` and
`popover` are lighter than the background, so lift comes from the surface, not the shadow.

---

## 3. Typography

Font: Geist (Geist Mono for small uppercase labels on the website). The palette is zinc
neutrals with a blue brand (`#1d4ed8`); the website at the base domain uses the same
palette, with amber only for "pending" chips. Headlines are Geist 600 with tight tracking. Sizes: `text-xs` 12 · `text-sm` 14 · `text-base` 16 · `text-lg` 18 ·
`text-xl` 20 · `text-2xl` 24 · `text-3xl` 30. Weights: 400 body · 500 labels · 600 headings ·
700 rare emphasis.

| Role | Style |
|---|---|
| Page / panel title | `text-xl font-semibold` |
| KPI / stat value | `text-2xl` (or `text-3xl`) `font-bold tabular-nums` |
| Section label | `.sidebar-section-label` utility, or the classes it applies |
| Body | `text-sm` |
| Caption / meta / helper | `text-xs text-muted-foreground` |

Use `tabular-nums` for any numeric column, timer, or metric to stop width jitter.

---

## 4. Radius

`--radius` = `0.25rem` (4px), tight corners everywhere. The Tailwind scale derives from it:

| Class | Value | Use for |
|---|---|---|
| `rounded-xl` | radius + 4 (8px) | large surfaces, widget window preview, dashboard message bubbles |
| `rounded-lg` | radius (4px) | cards, containers, dialogs |
| `rounded-md` | radius - 2 (2px) | buttons, inputs, chips, badges, small interactive |
| `rounded-sm` | radius - 4 (0px) | tiny insets |
| `rounded-full` | - | avatars, status dots, count badges, pills |

**Never use bare `rounded`.** It is Tailwind's fixed 4px and ignores the token.

---

## 5. Elevation

| Class | Use for |
|---|---|
| `shadow-sm` | cards, `.box`, default buttons |
| `shadow-md` | popovers, dropdown menus, hover/floating elements |
| `shadow-lg` | dialogs, modals, the widget window |

**Never use bare `shadow`.** Depth comes from the surface tiers and borders, not heavy
shadows.

---

## 6. Spacing

Follow a 4 / 8px rhythm for padding and gaps. Vertical section spacing tiers: 16 / 24 / 32 /
48. Keep the dense-desk feel; this is a high-volume support tool, not a marketing page.

---

## 7. Utilities

Defined in `main.scss`. Prefer them over repeating the class list.

| Utility | Expands to | Use for |
|---|---|---|
| `.box` | `border shadow-sm rounded-lg` | the standard card surface |
| `.sidebar-section-label` | `text-xs font-medium uppercase tracking-wider text-muted-foreground` | sidebar group headers (Views, Team Inboxes) |
| `.link-style` | `text-muted-foreground underline underline-offset-4 hover:text-foreground` | UI links. Not brand-colored, so links in chrome stay quiet |

---

## 8. Components

Reuse `shared-ui/components/ui/` primitives; do not hand-roll a styled `<button>`/`<input>`
when one exists.

**Button** - use the `size` variant, never an ad-hoc `h-*`:

| size | height | notes |
|---|---|---|
| `default` | h-9 | standard |
| `sm` | h-8 | dense (text-xs) |
| `xs` | h-7 | very dense |
| `lg` | h-10 | prominent |
| `icon` | h-9 w-9 | icon-only |

Variants: `default` (primary), `destructive`, `outline`, `secondary`, `ghost`, `link`.
`h-8 w-8` ghost triggers for dense table row actions are an intentional pattern.

Every button inside a `<form>` that is not the submit button needs an explicit
`type="button"`. HTML defaults to `type="submit"`, so a typeless button submits the form on
click and gets activated when the user presses Enter in any input. This includes buttons in
child components rendered inside a parent's form.

**Badge** variants: `default`, `secondary`, `destructive`, `success`, `outline`.

**AlertDialogAction** takes a `variant` prop, so destructive confirms use
`variant="destructive"` instead of a hand-written class.

**Sidebar nav items** are `font-medium text-foreground-lighter`; hover and active states use
`bg-sidebar-accent` (hover at `/50`). Group headers use `.sidebar-section-label`.

**Tooltip** is `bg-foreground text-background`, not brand-colored.

**Table row actions** stay hidden until hover: `[@media(hover:hover)]:opacity-0` with
`group-hover/row:opacity-100`, plus `focus-within:!opacity-100` and
`[&:has([data-state=open])]:!opacity-100`. The media query keeps them visible on touch and
focus-within keeps them reachable by keyboard.

---

## 9. Checklist

Before merging UI work:

- [ ] No hardcoded palette colors (`bg-green-600`, `text-blue-500`, ...). Only exception: file-type icons.
- [ ] No bare `rounded` or `shadow`.
- [ ] Every non-submit button in a form has `type="button"`.
- [ ] Color on a number or element means something, and is not decoration.
- [ ] Both light and dark verified.
- [ ] New shared words go through i18n (`i18n/en-US.json`); reused nouns in `globals.terms`.
- [ ] Reused an existing `ui/` primitive rather than building a variant.
