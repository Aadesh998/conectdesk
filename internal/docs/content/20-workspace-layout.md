# Finding your way around

The parts of the workspace app that are on every screen: the left icon rail, your user menu, the inbox sidebar, notifications, the command palette and keyboard shortcuts.

> **How permissions work.** No page address is blocked on its own. Access is controlled in two places: the screen hides the buttons and menu items you lack the permission for, and the server checks the same permission on every request. If you type the address of a page you aren't allowed to use, it opens but its data or action fails with an error. The permission behind each item is listed in these docs; roles and permissions are set under Admin → Roles.

## Left icon rail

**Where:** The narrow bar on the far left of every page once you are logged in. Each icon shows its name as a tooltip. On mobile, the rail's items appear at the top of the slide-out drawer, and the bell and avatar at its bottom.

| Icon | Goes to | Shown when |
|---|---|---|
| **Inboxes** | The last inbox page you viewed (remembered), otherwise My Inbox (`/inboxes/assigned`). | Always |
| **Contacts** | `/contacts` | `contacts:read_all` |
| **Reports** | `/reports/overview` | `reports:manage` |
| **Admin** (shield) | Admin settings (see the Admin pages). | You have at least one admin permission |
| Bell (**Notifications**) | Opens the notification panel. | Always |
| Your avatar (with a status dot) | Opens the user menu. | Always |

## User menu

**Where:** Click your avatar at the bottom of the icon rail.

| Item | What it does | Typical use |
|---|---|---|
| Name and email header | Shows who is logged in. | — |
| **Dark Mode** switch | Switches between light and dark theme. | — |
| **Away** switch | Sets your status to Away (manual) or back to Online. | Going on a break. |
| **Reassign replies** switch | Sets your status to "away and reassigning", so customer replies on your conversations go to someone else. Turning it off returns you to plain Away. | A long absence or leave. |
| **Account** | Opens your profile (`/account/profile`). | — |
| **Keyboard shortcuts** | Opens the shortcuts dialog. | — |
| **Logout** | Signs you out. | — |

**Automatic idle status:** after 5 minutes without mouse or keyboard activity, an Online agent is set to Away. Activity in a visible tab sets them back to Online. A manual Away is never changed automatically.

## Page header, banners and tab title

Non-inbox pages show a title bar with a sidebar-toggle icon. The toggle collapses or expands the secondary sidebar, and the app remembers your choice.

| Banner | When it shows | Action |
|---|---|---|
| "No internet connection" (yellow pill) | The browser is offline. | — |
| "Connecting..." | The live connection has been down for more than 1.5 seconds. | — |
| "Connection failed. Please refresh." (red) | Reconnecting failed. | Reload the page. |
| "Platform support {admin} is logged in as {name}. Everything done here is done as this agent." | A platform admin used **Log in as** on this agent. | **End session** logs out. |
| "Set up your workspace" card ("{done} of {total} done", progress bar), bottom of the inbox sidebar | Users with `general_settings:manage`, until setup is complete or dismissed. | Click to open the setup checklist (Admin → Setup). |

**Browser tab title** shows the unread notification count, e.g. "(3) My inbox - ConnectDesk".

## Inbox sidebar

**Where:** The second column, shown on every `/inboxes/...`, `/call-backs` and `/leads` page.

**Who can use it:** All agents; each entry has its own permission, listed below.

| Item | What it does | Shown when / permission |
|---|---|---|
| "Inbox" heading + magnifier icon | The magnifier opens Search (`/inboxes/search`). | Always |
| **+ New conversation** | Opens the New conversation dialog. | Always; creating needs `conversations:write` on the server. |
| **Make a call** | Opens the Dialer. | Only if an enabled Phone (voice) inbox exists. |
| **Call-backs** + count badge | Opens `/call-backs`. The badge shows pending call-backs and turns red if any are overdue. | A voice inbox exists, or call-backs are pending. |
| **Leads** | Opens the admissions board (`/leads`). | `contacts:read_all` |
| **My Inbox** | `/inboxes/assigned`: conversations assigned to you. | `conversations:read_assigned` |
| **Mentions** | `/inboxes/mentioned`: conversations where you or your team were @-mentioned in a private note. | `conversations:read` |
| **Unassigned** | `/inboxes/unassigned`: conversations with no agent and no team. | `conversations:read_unassigned` |
| **All** | `/inboxes/all`: every conversation. | `conversations:read_all` |
| **Team Inboxes** (collapsible) | One entry per team you belong to (emoji + name) at `/inboxes/teams/{id}`. Lists conversations assigned to that team with no agent yet. | You are in at least one team; `conversations:read_team_inbox` |
| **Views** (collapsible) | Your personal saved filters, at `/inboxes/views/{id}`. | `view:manage` |
| – **+** on hover over "Views" | Opens the Create View dialog. | `view:manage` |
| – **⋮** on hover over a view → **Edit** / **Delete** | Edit the view, or delete it after confirming ("This action cannot be undone. This will permanently delete this view."). | `view:manage` |
| **Shared views** | Views an admin shared with everyone or with your team (Admin → Shared views). Read-only here. | At least one shared view exists. |

Switching lists keeps your open conversation open on the right (desktop).

**Quirk:** "Unassigned", "All" and team inboxes are shown to every agent; agents without the matching permission get an error when the list loads.

## Notifications panel

**Where:** The bell icon at the bottom of the icon rail (or in the mobile drawer). A red badge shows the unread count (up to 99).

**Who can use it:** Every user.

| Control | What it does |
|---|---|
| Double-check icon (**Mark all as read**) | Marks everything read. |
| Trash (**Delete all**) | Clears all notifications. |
| Notification row | Click to mark it read and open the target: mentions open the conversation in Mentions at the message; assignments open it in My Inbox; missed-call notices open Call-backs. Icons: @ mention, person+ assignment, warning (SLA warning), alert (SLA breach), missed phone (call-back, red if overdue). |
| Hover ✓ / × | **Mark as read** / **Delete** for that row. |
| **Load more** | Loads older notifications. |

Empty state: "No notifications found".

## Command palette

**Where:** <kbd>Ctrl</kbd>+<kbd>K</kbd> (<kbd>⌘</kbd>+<kbd>K</kbd> on Mac) anywhere. <kbd>Ctrl</kbd>/<kbd>⌘</kbd>+<kbd>M</kbd> opens it straight in macro mode. Choosing "Snoozed" in a status dropdown also opens it, on the snooze options.

**Who can use it:** Everyone; each action keeps its own permission.

**Purpose:** Fast keyboard actions on the open conversation.

**Search box:** "Type a command or search..." If nothing matches: "No command available".

**"Conversations" group** (only when a conversation is open):

- **Apply macro.** Two panes: macros on the left, a preview on the right ("Reply Preview" and "Actions", e.g. "Assign agent: …", "Set status: …"). With nothing highlighted: "Select a macro to view details".
  - <kbd>Enter</kbd> applies the macro: its text goes into the reply box and its actions are queued; they run when you send.
  - Only macros visible to you appear: shared with everyone, shared with your team, or your own. They are also filtered by context (replying, private note, new conversation). Actions you lack permission for are dropped.
- **Snooze** (header "Snooze for"): "1 Hour", "3 Hours", "6 Hours", "12 Hours", "1 Day", "2 Days", "3 Days", "1 Week", or **Pick date and time**, which opens "Pick a snooze time" (calendar, **Time** field, **Snooze** button). The time must be in the future ("Select a future time"). A snoozed conversation reopens by itself at that time.
- **Resolve.** Sets the status to Resolved.

Footer hints: <kbd>Enter</kbd> Select, <kbd>↑</kbd><kbd>↓</kbd> Navigate, <kbd>Esc</kbd> Close, <kbd>Backspace</kbd> Back (on an empty box, leaves a sub-menu).

In the New conversation dialog, <kbd>Ctrl</kbd>+<kbd>K</kbd> opens macro mode and the macro applies to the new conversation.

## Keyboard shortcuts

**Where:** User menu → **Keyboard shortcuts**.

| Group | Shortcut | Action |
|---|---|---|
| General | <kbd>Ctrl</kbd>/<kbd>⌘</kbd> + <kbd>K</kbd> | Open command bar |
| Reply editor | <kbd>Ctrl</kbd>/<kbd>⌘</kbd> + <kbd>M</kbd> | Open macros |
| Reply editor | <kbd>Ctrl</kbd> + <kbd>Enter</kbd> | Send reply |
| Reply editor | <kbd>Ctrl</kbd>/<kbd>⌘</kbd> + <kbd>B</kbd> | Bold |
| Reply editor | <kbd>Ctrl</kbd>/<kbd>⌘</kbd> + <kbd>I</kbd> | Italic |

Other keys not listed in the dialog:

- <kbd>Shift</kbd>+<kbd>Enter</kbd>: new line in the reply editor.
- <kbd>Ctrl</kbd>+<kbd>Enter</kbd>: saves a contact note.
- <kbd>Enter</kbd>: sends in Juno.
- <kbd>←</kbd> / <kbd>→</kbd> / <kbd>Esc</kbd>: browse or close the image lightbox.
- <kbd>Esc</kbd>: leaves the full-screen editor.
