# Platform admin dashboard

The platform admin dashboard is where the people who run the installation create and manage every workspace. This page covers the dashboard, the landing page on the base domain, and who can read these docs.

## 0. Before you start: how the installation is laid out

| Address | What it serves |
|---|---|
| `<base_domain>` (for example `yourapp.com`) | The landing page, where you find your workspace. It also carries the shared OAuth redirect (`/oauth/...`), the Meta webhook and callbacks (`/webhook/meta`, `/meta/...`) and static files. Everything else on this host returns "Workspace not found". |
| `admin.<base_domain>`, or whatever `app.tenancy.platform_host` is set to | The platform admin dashboard, and nothing else. Opening `/` redirects to `/platform`. |
| `docs.<base_domain>` (in production **docs.desk.arrlink.com**), or `app.tenancy.docs_host` | This internal documentation, and nothing else. Opening `/` redirects to `/docs`. Only docs viewers can log in (see [Docs access](#115-docs-access)). |
| `<slug>.<base_domain>` | One workspace (tenant), for example `acme.yourapp.com`. |
| A verified custom domain | The workspace that owns that domain, or one of its help centres. |

The platform dashboard exists only when **both** of these are true:

- `app.tenancy.enabled = true`
- `app.tenancy.base_domain` is set

If either is missing, the dashboard does not exist. Its paths (`/platform`, `/api/platform/*`) return "page not found" on every host except the platform host.

---

## 1. Platform admin dashboard

**Where:** `https://admin.<base_domain>/platform`. The address of each screen comes after a `#`, as shown in each section below.

**Who can use it:** Platform admins only. These are people added from the command line with `--platform-admin-add`. They are not users of any workspace, and there is no way to add them from the dashboard.

**General behaviour:**

- **Session length:** A login lasts 12 hours, kept in a session cookie.
- **Cookies:** They are "Secure" unless `app.server.disable_secure_cookies` is on, and "SameSite strict".
- **Protection:** Every change sends a CSRF token. The page can't be embedded in another site (`X-Frame-Options: DENY`) and is never cached.
- **Expired session:** If the session expires, the next action takes you back to the Log in screen.
- **Messages:** Success and error messages appear as a short popup ("toast") at the bottom centre. It disappears after about 4 seconds, and errors show in red.
- **Audit log:** Every meaningful action is recorded in the platform audit log (section 1.12).

### Layout: sidebar, on every page after you log in

| Element | What it does |
|---|---|
| Logo: **"C" ConnectDesk / Platform admin** | Takes you to the Overview. |
| **Overview** | Opens the Overview (`#/`). |
| **Workspaces** | Opens the workspace list (`#/workspaces`). |
| **Audit log** | Opens the audit log (`#/audit`). |
| **Docs access** | Opens the list of people who may read this documentation (`#/docs`). See 1.15. |
| **Settings** | Opens the platform mail server settings (`#/settings`). The page itself is titled "Email". |
| Your email (bottom) | Shows which admin is logged in. |
| **Log out** | Ends the session (see 1.13). |

On phones and narrow windows the sidebar becomes a bar across the top, and your email is hidden.

---

## 1.1 Log in

**Where:** `https://admin.<base_domain>/platform`, shown whenever you are not logged in.

**Who can use it:** Platform admins.

**Purpose:** Prove who you are before managing workspaces.

The screen shows the heading "Log in" and the line "Manage every workspace on <base_domain>."

| Field or button | What it does | Typical use |
|---|---|---|
| **Email** | The platform admin's email. Upper or lower case doesn't matter. | The email you were added with. |
| **Password** | The admin's password. | |
| **Log in** | Checks the email and password. If two-factor login is off for you, you go straight in. If it is on, you go to the "Two-factor code" screen. | |
| Error line | Shows "Wrong email or password" (or another error), and the button works again. | |

Notes:

- Login attempts are rate-limited per IP address. The default is 30 a minute (`[rate_limit.auth]`).
- Failed logins are written to the server log with the email and IP address.
- A successful login is audited as **Logged in**.

## 1.2 Two-factor code (second login step)

**Where:** Same address. It appears after a correct password when the admin has two-factor login turned on.

**Who can use it:** An admin who has set up two-factor login.

**Purpose:** The second step of login.

The screen shows the heading "Two-factor code" and the line "Enter the 6-digit code from your authenticator app, or a recovery code."

| Field or button | What it does |
|---|---|
| **Code** | Takes the 6-digit code from the authenticator app, or one of the recovery codes (up to 12 characters). |
| **Verify** | Checks the code. If it is right, you are logged in. |

Rules:

- **Time limit:** This step expires 5 minutes after the password step, and allows at most 5 attempts. After that you get "This login has expired. Enter your email and password again." and return to the Log in screen.
- **Wrong or reused code:** You get "That code isn't right, or was already used. Try the current one." Each app code works only once, and each recovery code works only once and is then used up.

## 1.3 Set up two-factor login (forced at first login)

**Where:** Same address, shown straight after login.

**When it appears:** Two-factor login is required (`app.tenancy.platform_require_2fa`, which is **on unless set to false**) and this admin hasn't set it up yet. Until they do, every other dashboard action is refused with "Your workspace requires two-factor login. Set it up to continue."

**Who can use it:** A logged-in admin without two-factor login.

**Purpose:** Connect an authenticator app.

**Step 1.** The screen shows the heading "Set up two-factor login" and the line "Scan this QR code with your authenticator app, or enter the key."

| Element | What it does |
|---|---|
| QR code | Scan it with Google Authenticator, 1Password, Authy or a similar app. The app labels the entry "Platform admin (<base_domain>)" with your email. |
| **Key: …** | The same secret as text, for typing in by hand. |
| **The 6-digit code the app shows** | Type the current code from the app. |
| **Turn on** | Checks the code and turns two-factor login on. It is audited as **Turned on two-factor login**. |

If more than 10 minutes pass between showing the QR code and pressing **Turn on**, you get "Setup took too long. Start again." A wrong code gives the invalid-code message.

**Step 2.** The screen shows the heading "Recovery codes" and the line "Each code logs you in once if you lose your phone. Keep them somewhere safe: they won't be shown again."

| Element | What it does |
|---|---|
| List of codes | One-time backup codes. They are shown only this once. |
| **I've saved them** | Continues into the dashboard. |

Notes:

- There is **no button anywhere in the dashboard** to turn two-factor login off, or to turn it on voluntarily when it isn't required. The setup screen appears only when it is required.
- If an admin loses their phone, an operator runs `--platform-admin-reset-2fa <email>` (section 3). The admin then sets it up again at their next login.

---

## 1.4 Overview

**Where:** `#/`, the default page after login.

**Who can use it:** Any logged-in platform admin.

**Purpose:** A snapshot of the whole installation.

| Element | What it shows or does |
|---|---|
| **New workspace** (top right) | Opens the Create workspace screen (1.7). |
| **Workspaces** | Total number of workspaces, the default one included. |
| **Active** | Workspaces whose status is active. |
| **Suspended** | Workspaces whose status is suspended. This includes workspaces scheduled for deletion, because scheduling a deletion suspends the workspace. |
| **Agents** | All agents across all workspaces, excluding deleted agents and the internal "System" user. |
| **Conversations, last 30 days** | Conversations created in the last 30 days, across all workspaces. |
| Panel **Workspaces by plan** | One bar per plan name, with the number of workspaces on it, largest first. It shows "No workspaces yet." when empty. |
| **All workspaces** (button on that panel) | Opens the workspace list. |

---

## 1.5 Workspaces list

**Where:** `#/workspaces`

**Who can use it:** Platform admins.

**Purpose:** Find a workspace and open it.

| Element | What it does | Typical use |
|---|---|---|
| **New workspace** | Opens Create workspace. | Onboarding a new customer. |
| Search box ("Search by name, address, domain or agent email") | Filters as you type, about 250 ms after you stop. It matches part of the workspace address (slug), name, custom domain, or **any agent's email** in the workspace. Upper or lower case doesn't matter. | "Which workspace is jane@school.com in?" |
| Status buttons **All / Active / Suspended** | Filter by status. **Suspended** also lists workspaces being deleted. | Review suspended accounts. |
| Table row (click, or press Enter on it) | Opens that workspace's detail page. | |
| **Previous / Next** | Move between pages of 25 workspaces. They are greyed out at the first or last page. | |
| Footer text | For example "52 workspaces, page 1 of 3". | |

Table columns:

| Column | Meaning |
|---|---|
| **Workspace** | The name, with its address underneath (the custom domain if one is set as primary, otherwise `<slug>.<base_domain>`). |
| **Plan** | The plan name. |
| **Status** | Active (green), Suspended (red), or Deleting (red, when a deletion is scheduled). |
| **Agents** | Agents in the workspace, excluding deleted agents and the System user. |
| **Inboxes** | Inboxes that haven't been deleted. |
| **Conversations, 30 days** | Conversations created in the last 30 days. |
| **Last active** | The most recent activity of any agent: "Just now", "5m ago", "3h ago", "4d ago", a date, or "Never". |
| **Created** | The creation date. |

The list is ordered newest first. With no results it shows either "No workspaces match. Try another search or status." or "No workspaces yet. Create the first one."

---

## 1.6 Workspace detail

**Where:** `#/workspaces/<id>`

**Who can use it:** Platform admins.

**Purpose:** Everything about one workspace, and every action on it.

### Header

| Element | What it does |
|---|---|
| **← Workspaces** | Back to the list. |
| Workspace name and status badge | Active, Suspended or Deleting. |
| **Open workspace ↗** | Opens the workspace's own address in a new tab. You still have to log in there. To enter as an agent, use "Log in as" instead. |

### Deletion banner (only when a deletion is scheduled)

The banner reads "**Scheduled for deletion.** It and all its data are deleted on <date>."

| Button | What it does | Confirmation |
|---|---|---|
| **Restore** | Cancels the deletion and makes the workspace **active** again. Audited as **Restored**. The toast says "Restored". | None |
| **Delete now** | Deletes the workspace and **all** its data and stored files immediately, without waiting. It then takes you back to the list with the toast "Deleted <slug>". Audited as **Deleted**. | A browser prompt: "Delete <name> and ALL its data NOW, without waiting? This can't be undone. Type its address, <slug>, to confirm:". The text must match the slug exactly, or you get "Type the workspace's address to confirm". Pressing Cancel does nothing. |

### Stats strip

| Stat | Meaning |
|---|---|
| **Contacts** | Contacts that haven't been deleted. |
| **Conversations** | All conversations ever. |
| **Conversations, last 30 days** | Conversations created in the last 30 days. |
| **Help centers** | Number of help centres. |

### Left column

**Usage panel** ("Against the limits of the <Plan> plan.") shows meters. Each bar turns orange at 80% of the limit and red when the limit is reached. With no limit it reads "N, no limit".

| Meter or fact | Source |
|---|---|
| **Agents** (used of `max_agents`) | Agent count against the plan. |
| **Inboxes** (used of `max_inboxes`) | Inbox count against the plan. |
| **File storage** (bytes used of `max_storage_mb`) | Attachments and avatars stored. |
| **AI requests this month** (of `ai_requests_per_month`) | AI completions and AI agent runs this month. |
| **API rate limit** | `api_requests_per_minute`, shown as "N a minute", or "No limit". |
| **Throttled today** | How many of the workspace's requests were refused today for going over the rate limit. |
| **Largest upload** | `max_file_upload_mb` in MB, or "Workspace setting" when the plan sets no cap. |

The default workspace (ID 1) always shows no limits.

**Agents panel** lists the workspace's agents (not the hidden System user or AI assistants) with the columns Name (with email), Roles, **Status** (Active or Suspended), Last active, Last login, and buttons. Use it to help a workspace whose admin is locked out, or to manage its people for them.

| Button | What it does | Typical use |
|---|---|---|
| **+ Add agent** (panel header) | Opens **Add an agent to <workspace>**: **First name** (required), **Last name**, **Email**, **Roles** (a switch per role in that workspace; Agent is on by default), **Password** (leave empty to generate one, or type one that meets the rules) and **Email the password to them** (on by default). **Add agent** creates the agent and shows the password once (see below). Counts against the plan's agent limit: "Your plan allows {max} agents. Upgrade your plan to add more". Audited as **Added an agent**. | Set up a workspace for a customer; add someone their admin can't. |
| **Log in as ↗** | Shown when the agent is active **and** the workspace is active. Opens the workspace logged in as this agent. See 1.11. | Support. |
| **New password** | Opens **New password for <name>** with the same **Password** and **Email the new password to <email>** fields. **Set password** replaces the password; the old one stops working at once. Audited as **New agent password**, and also written to the workspace's own activity log as "Platform support <your email> set password for <agent>". | An agent or workspace admin is locked out or forgot their password. |
| **Suspend** | After confirming ("They are logged out and can't log in until you activate them. Their conversations stay assigned to them."), disables the account. Their next click in the app logs them out, and logging in shows "Your account is disabled". Audited as **Suspended an agent**. | Someone is on leave or under review. |
| **Activate** | Shown for suspended agents; lets them log in again. Audited as **Activated an agent**. | They're back. |
| **Delete** | After confirming, deletes the agent the same way Admin → Agents does: they can't log in, they're logged out, and their open conversations are unassigned. Their past messages stay. Audited as **Deleted an agent**. | Someone has left. |

**The new password dialog.** After **Add agent** or **Set password**, a dialog shows the password **once**, with **Copy**, and either "Emailed to <email>." (green) or, if the email didn't go out, the mail server's error and "Share the password with them another way." The email ("Your ConnectDesk login") goes through the workspace's own mail server, or the platform's (Settings) when it has none, with the workspace's name as sender. It gives the workspace address, the email and the new password.

> A new password doesn't log out sessions already open with the old one; use **Suspend** then **Activate** if you need them logged out.

**Domains panel** is a table with the columns Domain, Status ("Verified <date>" or "Not verified") and "Used for" ("Workspace address" and/or "Help center <name>"). With no domains it reads "No custom domains. It is served on <slug>.<base_domain>." This panel is **read-only**. Domains are managed by the workspace's own admin (section 4.2).

**History panel** ("What platform admins did to this workspace.") shows the latest 50 audit entries for this workspace, with the columns When, Admin, Action and Details (see 1.12).

### Right column

**Details panel:**

| Field or button | What it does | Typical use |
|---|---|---|
| **Plan** dropdown | Lists every configured plan. If the workspace's current plan is no longer in the config, it is listed too. | |
| **Change** | Greyed out until you pick a different plan. It moves the workspace to that plan, and the limits apply straight away. The toast says "Plan changed to <Plan>". Audited as **Changed plan** ("Free to Growth"). There is no confirmation. | A customer upgrades or downgrades. |
| **Status** | A badge: Active, Suspended or Deleting. | |
| **Address** | A link to the workspace URL. | |
| **Created** | The creation date. | |
| **Workspace ID** | The internal number. | Support and debugging. |

**Channels panel** ("Turning one off switches off its inboxes until you turn it back on."):

| Control | What it does |
|---|---|
| **Every channel, including new ones** (master switch) | When on, the workspace may use every inbox channel, including channels added in future versions. The individual switches are greyed out. |
| **Email, Live chat, WhatsApp, Messenger, Instagram, Telegram, Phone calls (Exotel)** | The individual channel switches, used when the master switch is off. |
| **Save** | Greyed out until something changes. It saves the selection. |

What saving does:

- Every **enabled inbox** of a channel that is now off is switched off and reloaded. The audit entry records how many inboxes ("switched off N inboxes").
- From then on, the workspace can't create, enable or connect inboxes of a disallowed channel. They get "This channel isn't enabled for your workspace. Contact support to enable it".
- Turning a channel back on **does not** switch its inboxes back on. The workspace admin re-enables them.
- If you save with no channels at all, you are asked to confirm: "Leave <name> with no channels? All its inboxes will be switched off." Cancelling leaves things as they were and shows the toast "Not saved".
- The toast says "Channels saved". Audited as **Changed channels**.

**Integrations panel** ("Turning one off hides it and stops it sending. Its setup is kept."):

| Control | What it does |
|---|---|
| **Every integration, including new ones** (master switch) | Allows all integrations, now and in future. |
| **Webhooks, Slack, Microsoft Teams, Context links** | The individual switches. |
| **Save** | Saves the selection. Toast: "Integrations saved". Audited as **Changed integrations**. |

What turning one off does:

- Its admin screens and API refuse with "This integration isn't enabled for your workspace. Contact support to enable it".
- Webhooks stop being delivered, and Slack and Teams stop posting.
- The configuration is kept, and works again once the integration is turned back on.

**Danger zone panel.** It is hidden for the default workspace and while a deletion is scheduled (use the banner instead).

| Row and button | What it does | Confirmation |
|---|---|---|
| **Suspend** (shown when active): "Agents and customers see a suspended page until you activate it." | Sets the status to suspended. Every request to the workspace then gets "This workspace has been suspended. Contact support to restore access", including the agent app, live chat, help centre and webhooks. Toast: "Suspended". Audited as **Suspended**. | Browser confirm: "Suspend <name>? Its agents and customers get a "suspended" page until you activate it." |
| **Activate** (shown when suspended): "Agents and customers can use it again." | Sets the status back to active. Toast: "Activated". Audited as **Activated**. | None |
| **Delete**: "Suspends it now and deletes all its data after the grace period. You can restore it until then." | Suspends the workspace now and schedules permanent deletion in `deletion_grace_days`, 30 by default. Visitors then see "This workspace is being deleted, with all its data, on <date>. Until then support can restore it." Toast: "Scheduled for deletion". Audited as **Scheduled deletion** ("Deletes on <date>"). | Browser prompt: "Suspend <name> now and delete it with ALL its data after the grace period? Until then you can restore it. Type its address, <slug>, to confirm:". The slug must match exactly. |

Server-side rules:

- The default workspace can't be suspended or deleted: "The default workspace can't be suspended or deleted".
- A workspace scheduled for deletion can't be "activated", because that would leave the deletion pending. Use Restore: "This workspace is scheduled for deletion. Restore it instead."
- Restoring a workspace that isn't scheduled gives "This workspace isn't scheduled for deletion."
- Once the grace period ends, a background job that runs every hour deletes the workspace's data and stored files. It is audited with admin **system** and action **Deleted**.
- A workspace's own admin can also schedule its deletion from inside the workspace. This needs their password and the slug typed to confirm. It appears in the audit log as admin "<email> (workspace admin)", action **Scheduled deletion**, and platform admins can still restore it.

---

## 1.7 Create workspace

**Where:** `#/workspaces/new`. Reach it with the **New workspace** button on the Overview or Workspaces page.

**Who can use it:** Platform admins. Workspaces are created only here or from the command line. **There is no public signup.**

**Purpose:** Create a new customer workspace together with its first administrator.

The page has a **← Workspaces** back link and the heading "New workspace".

**Organization panel:**

| Field | Rules | Hint |
|---|---|---|
| **Name** | Required, up to 140 characters. Example: "Acme Tuition". | |
| **Address** (`____.<base_domain>`) | 3 to 63 characters: lowercase letters, numbers and dashes. It can't start or end with a dash and can't contain "--". Reserved words are refused: www, app, api, admin, mail, smtp, imap, ftp, static, cdn, assets, status, help, support, docs, blog, billing, signup, login, auth, dashboard, default, system, root. | "Lowercase letters, numbers and dashes." |
| **Plan** | A dropdown of the configured plans, preselected to `default_plan`. | |

**First admin panel** ("They log in with this email and password. Share the password securely; they can change it after logging in."):

| Field | Rules |
|---|---|
| **First name** | Required, up to 140 characters. |
| **Last name** | Optional, up to 140 characters. |
| **Email** | Required, a valid email address. |
| **Password** | 10 to 72 characters with an uppercase letter, a lowercase letter, a number and a symbol. Hint: "10 or more characters, with upper and lowercase letters, a number and a symbol." |

**Channels panel** ("The inbox channels this organization can set up. You can change this later."):

- It has the same switches as in 1.6.
- **By default every switch is off**, and you must turn on at least one channel or the master switch. Otherwise you get "Turn on at least one channel."

**Integrations panel** ("What this organization can set up under Admin, Integrations. You can change this later."):

- It has the same switches as in 1.6.
- **By default the master switch, "Every integration, including new ones", is on.**

**Buttons:**

| Button | What it does |
|---|---|
| **Create workspace** | Creates everything in a single step. If any part fails, nothing is kept. It creates the workspace on the chosen plan with its default data, sets the workspace's site name to the organisation name and its root URL and favicon to the workspace address, and creates the hidden "System" user and the first admin with the Admin role. On success the toast says "Created <url>" and the new workspace's detail page opens. Audited as **Created** (name, plan, admin email, channels, integrations). |
| **Cancel** | Back to the list, with nothing saved. |

Possible errors:

- "Enter the organization's name"
- "Workspace address must be 3 to 63 lowercase letters, numbers or dashes, and can't start or end with a dash"
- "That workspace address is taken or not allowed" (the slug is taken or reserved)
- "Enter the first admin's name"
- "Enter a valid email address for the first admin"
- "The first admin's password must be 10-72 characters…"
- "Unknown plan", "Unknown channel" or "Unknown integration"

**Typical use:** A new school signs up. Create `brightclasses` on Growth with Email, Live chat and WhatsApp, then send the admin their URL (`https://brightclasses.<base_domain>`) and password through a secure channel.

---

## 1.8 Plans (in the dashboard)

The dashboard has no plan editor. Plans are defined in the configuration file (section 4.1). In the dashboard you can only:

- pick a plan when creating a workspace (1.7)
- change a workspace's plan (Details panel in 1.6)
- see usage against the plan (Usage panel in 1.6)
- see how many workspaces are on each plan (Overview in 1.4)

## 1.9 Channels and integrations

See the Channels and Integrations panels in 1.6 and 1.7. Where each is enforced:

- **Channels** are checked when an inbox is created, when it is updated while enabled, when it is toggled on, when Gmail or Outlook is connected through OAuth, when WhatsApp, Messenger or Instagram is connected through Meta, when WhatsApp is set up manually, and when Telegram is connected.
- **Integrations** are checked by the routes for Webhooks (`/api/v1/webhooks*`), Slack (authorize and callback), Teams (package and code), Context links (`/api/v1/context-links*`), and the integration settings endpoints. Webhook delivery and Slack/Teams posting also check the switch before sending.

## 1.10 Suspend, activate, schedule delete, restore

Covered under Danger zone and the deletion banner in 1.6, and under the CLI in section 3.

## 1.11 Impersonate ("Log in as")

**Where:** The **Log in as ↗** button in the Agents panel of a workspace's detail page.

**Who can use it:** Platform admins.

**Purpose:** See and do exactly what an agent sees, for support or troubleshooting.

How it works:

1. Clicking the button creates a **one-time link valid for 60 seconds**. It works only on that workspace and only once.
2. The link opens in a new tab: `https://<workspace>/impersonate?token=…`.
3. If your browser blocks pop-ups, a toast shows "Allow pop-ups, or open the link within 60s: <url>".
4. Opening the link logs that browser into the workspace **as the agent**, then redirects to the workspace home.

What the workspace shows and records:

- A banner across the workspace app reads "Platform support <admin email> is logged in as <agent name>. Everything done here is done as this agent." Its **End session** link logs out.
- The workspace's own activity log gets the entry "Platform support <admin> logged in as <email> (#id)". Workspace admins can filter for it as "platform_impersonation".
- The platform audit log records **Logged in as an agent** ("Agent #<id>").

Errors:

- "That agent isn't an enabled agent of this workspace" means the agent is disabled or deleted.
- "This login link is invalid, used or expired. Create a new one from the platform dashboard" means the link was used, expired, or opened on the wrong workspace.
- Opening `/impersonate` is rate-limited the same way as login.

## 1.12 Audit log

**Where:** `#/audit`

**Who can use it:** Platform admins.

**Purpose:** An accountability record of everything platform admins did, plus automatic deletions and workspace admins' own deletion requests.

The page shows "Everything platform admins did, newest first. Hover a time or a detail for the full record." It lists the **latest 200 entries**, and there is no search or paging. Each workspace's detail page has its own History panel with the latest 50.

| Column | Meaning |
|---|---|
| **When** | Relative time. Hover for the full date and time. |
| **Admin** | Admin email, "system" for automatic deletion after the grace period, or "<email> (workspace admin)". |
| **Action** | See the table below. |
| **Workspace** | The slug. It is a link to the workspace if it still exists. Deleted workspaces show plain text. "—" means the action wasn't about a workspace. |
| **Details** | A plain-language summary. Hover for the raw record. |
| **IP** | The admin's IP address. |

**Every audit action:**

| Code | Shown as | When it is recorded | Details summary |
|---|---|---|---|
| `login` | Logged in | Successful dashboard login | — |
| `logout` | Logged out | **Log out** clicked | — |
| `enable_2fa` | Turned on two-factor login | Two-factor setup finished | — |
| `create` | Created | Workspace created from the dashboard | "<name>, <Plan> plan, admin <email>" |
| `change_plan` | Changed plan | Plan changed | "Free to Growth" |
| `set_channels` | Changed channels | Channels saved | The new list, or "Every channel" / "None", plus "; switched off N inboxes" |
| `set_integrations` | Changed integrations | Integrations saved | The new list, or "Every integration" / "None" |
| `suspend` | Suspended | Suspend | — (the record keeps the previous status) |
| `activate` | Activated | Activate | — |
| `schedule_delete` | Scheduled deletion | Delete (grace period), from the dashboard or by a workspace admin | "Deletes on <date>" |
| `restore` | Restored | Restore | — |
| `delete` | Deleted | Delete now, or automatic deletion after the grace period (admin "system") | The workspace name |
| `impersonate` | Logged in as an agent | Log in as | "Agent #<id>" |
| `update_mail` | Updated the mail server | Mail settings saved | "<host>", plus ", turned off" if disabled |
| `test_mail` | Sent a test email | Test email sent | "<to address>", plus ", failed" if sending failed |
| `add_docs_viewer` | Gave docs access | A docs viewer added, or their password reset, on Docs access | The viewer's email |
| `remove_docs_viewer` | Removed docs access | A docs viewer deleted on Docs access | The viewer's email |
| `suspend_docs_viewer` / `activate_docs_viewer` | Suspended / Restored docs access | Suspend / Restore on Docs access | The viewer's email |
| `reset_docs_password` | New docs password | New password on Docs access | The email, ", emailed" if it went out |
| `create_agent` | Added an agent | **Add agent** on a workspace | Email, roles, ", emailed" |
| `reset_agent_password` | New agent password | **New password** on an agent | Email, ", emailed" |
| `suspend_agent` / `activate_agent` | Suspended / Activated an agent | **Suspend** / **Activate** on an agent | The agent's email |
| `delete_agent` | Deleted an agent | **Delete** on an agent | The agent's email |

Command-line actions (`--tenant-*`, `--docs-viewer-*`) are **not** recorded in the audit log.

## 1.13 Log out

**Where:** The **Log out** button at the bottom of the sidebar.

**What it does:** Deletes the session on the server, clears the cookies, records **Logged out**, and returns to the Log in screen.

---

## 1.14 Settings → Email (platform mail server)

**Where:** `#/settings`. The sidebar says "Settings" and the page heading says "Email".

**Who can use it:** Platform admins.

**Purpose:** The platform's own SMTP server. The page explains: "It sends the email of every workspace that hasn't set up its own under Admin, Notifications, Email; those go out with the workspace's name on your From address."

Mail is chosen in this order:

1. The workspace's own mail settings, if it has any.
2. Otherwise, this platform server, using the workspace name as the sender name on this From address.
3. If neither is set up, email isn't sent.

The AI agent also uses this From address.

**Mail server panel:**

| Field | What it does | Default |
|---|---|---|
| **Send email through this server** (switch) | Turns the platform mailer on or off. When on, the host, a port from 1 to 65535 and a valid From address are required. Otherwise you get "Enter the mail server's host and port." or an invalid From address error. | Off |
| **SMTP host** | For example smtp.mailgun.org. | — |
| **Port** | The SMTP port. | 587 |
| **Security** | **STARTTLS (port 587)**, **TLS (port 465)** or **None**. | STARTTLS |
| **Authentication** | **Login**, **Plain**, **CRAM-MD5** or **None**. | Login |
| **Username** | The SMTP username. | — |
| **Password** | Never shown back. If one is saved, the placeholder reads "Saved; type to replace". **Leaving it empty keeps the saved password.** It is stored encrypted. | — |
| **From** | For example `YourApp <noreply@mg.yourapp.com>`. Hint: "An address on the domain the server sends for." | — |
| **Skip the TLS certificate check (only for testing)** | Accepts invalid certificates. | Off |
| **Save** | Saves the settings, takes effect immediately, shows the toast "Saved", and is audited as **Updated the mail server**. | |

**Send a test panel** ("Save first, then send a test to check the settings work."):

| Field or button | What it does |
|---|---|
| Email box (you@example.com) | The recipient. |
| **Send test email** | Sends "Test email from the platform" through the **saved** settings and waits for the mail server's answer. On success the toast says "Test email sent to …". On failure it shows the server's actual error message. Rate-limited, and audited as **Sent a test email**, noting whether it worked. |

**Using Mailgun panel:** A help note: "Host smtp.mailgun.org, port 587, STARTTLS, Login. The username and password are the domain's SMTP credentials, under Sending, Domain settings, SMTP credentials in Mailgun. From is an address on that domain."

---

## 2. Landing page (base domain)

**Where:** `https://<base_domain>/`, only when multi-tenancy is on.

**Who can use it:** Anyone, typically agents who don't remember their workspace URL.

**Purpose:** Find a workspace and go to its login. There is **no signup** here.

The page title is "Log in · <product_name>". It shows the product name (`app.tenancy.product_name`, "ConnectDesk" by default), the heading "Log in to your workspace" and the line "Enter your workspace address."

| Field or button | What it does |
|---|---|
| Address box (placeholder "brightclasses") with ".<base_domain>" after it | Type the workspace slug. Upper or lower case doesn't matter. |
| **Continue** | Looks up the slug. If it exists, the browser goes to that workspace's URL (its primary custom domain if it has one, otherwise `<slug>.<base_domain>`). If not, it shows "No workspace at <slug>.<base_domain>. Check the address, or ask your administrator." If the lookup fails, it shows "Something went wrong. Please try again." |

Notes:

- The lookup (`/api/v1/workspaces/lookup?slug=`) works only on the base domain and is rate-limited like login.
- Suspended workspaces are still found. The visitor then sees the suspended page on the workspace itself.
- The page is marked so search engines don't index it ("noindex").

---

## 1.15 Docs access

**Where:** `#/docs`, the **Docs access** item in the sidebar.

**Who can use it:** Platform admins.

**Purpose:** Decide who may read this internal documentation at `docs.<base_domain>` (in production **https://docs.desk.arrlink.com**). The people listed are **docs viewers**: their account opens the documentation and nothing else. It is not a platform admin, not a workspace user, and it can't log in to any workspace or to this dashboard. Likewise a platform admin's login doesn't open the docs, so admins who read them add themselves here too.

The page explains: "People listed here can log in to the internal documentation at … and nothing else: not this dashboard and no workspace."

| Element | What it does | Typical use |
|---|---|---|
| **Open the docs ↗** (top right) | Opens the documentation's address in a new tab. | Check what a viewer sees. |
| **Docs viewers** table | One row per viewer: **Email**, **Status** (Active or Suspended), **Added** (date), **Last login** ("3h ago", or "Never"). Shows "No one has docs access yet." when empty. | See who has access and whether they use it. |
| **New password** (on each row) | Opens **New password for <email>**: **Password** (leave empty to generate one) and **Email the password to <email>**. **Set password** replaces it; the old one stops working at once. Audited as **New docs password**. | They forgot their password. |
| **Suspend** (on each row) | After confirming, blocks the account and logs them out at once. Logging in then shows "This account is suspended. Ask a platform admin to let you back in". Audited as **Suspended docs access**. | Pause someone's access without losing their account. |
| **Restore** (suspended rows) | Lets them log in again. Audited as **Restored docs access**. | |
| **Delete** (on each row) | After confirming, deletes the account and logs them out at once. Audited as **Removed docs access**. | Someone leaves the team. |
| **Add a viewer** panel: **Email** | The person's email (upper or lower case doesn't matter). | |
| **Password** | Leave empty for a strong generated password, or type one with 10 to 72 characters, an uppercase letter, a lowercase letter, a number and a symbol. | |
| **Email the password to them** | On by default. Sends "Your ConnectDesk login" with the docs address, the email and the password, through the platform's mail server (Settings). | |
| **Add viewer** | Creates the viewer (or, for an email already listed, sets this new password) and shows the password once, with **Copy**, plus whether the email went out. Audited as **Gave docs access**. | Give a new teammate the manual. |

Viewers can't change their password themselves; use **New password**. The same can be done from the command line with `--docs-viewer-add`, `--docs-viewer-remove` and `--docs-viewer-list` (see Operations).

### What a docs viewer sees

1. Opening `https://docs.desk.arrlink.com` (any path) without being logged in shows **Log in to the documentation** with **Email**, **Password** and **Log in**. A wrong email or password shows "Wrong email or password". Attempts are rate-limited like every login (30 a minute per IP).
2. After logging in, the documentation opens. A login lasts **7 days** in that browser.
3. **Top bar:** the product name (click to go to the first page), the **Search the docs** box (press <kbd>/</kbd> to jump to it; results appear as you type, <kbd>↑</kbd>/<kbd>↓</kbd> to pick, <kbd>Enter</kbd> to open, <kbd>Esc</kbd> to close), the viewer's email and **Log out**.
4. **Left sidebar:** every page, numbered; the current page lists its sections. On phones it is behind the **☰** button.
5. **Previous / Next** links at the bottom of each page.
6. Every other address on the docs host answers "page not found", and the docs session cookie is useless anywhere else.
