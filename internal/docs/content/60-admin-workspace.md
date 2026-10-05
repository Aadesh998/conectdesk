# Admin: workspace settings

The workspace-wide settings an admin manages: how to get into Admin, the setup checklist, General, Domains, Usage, Admissions, Privacy, Data export, Business hours and SLA policies.

> **How permissions work.** Admin pages carry no lock of their own. The Admin sidebar only shows the items you have the permission for, and the server checks the same permission on every request; typing an admin address you lack the permission for opens the page but its data is refused.

**On every admin screen:**

- **Help panel:** on wide screens most pages show a grey help text in the right-hand column; it is quoted under each screen below.
- **List tables** have a **Search** box above them that filters rows as you type.
- **Row menus:** the **⋮** at the end of a row opens its menu. **Delete** always opens a dialog titled **"Are you absolutely sure?"** with **Cancel** and **Delete**.
- **Saving** shows the toast "Saved successfully".

## Getting into Admin

**Where:** The **Admin** icon (a shield) in the far-left icon rail. On mobile it is in the nav drawer. URL: `/admin`.

**Who can use it:** The Admin icon appears only if the user has at least one permission that unlocks an admin page.

**Purpose:** This is the area for workspace-wide configuration.

| Element | What it does | Typical use |
|---|---|---|
| **Admin** icon (tooltip "Admin") | Opens Admin. If you have `general_settings:manage` you land on **General** (`/admin/general`). Otherwise you land on `/admin`, which shows "Select a section from the sidebar". | Daily entry into settings. |
| Admin sidebar header | Shows the word **Admin** and the app version in brackets, e.g. "(v2.x)". | Check the running version when talking to support. |
| Collapsible groups | Click a group to open it. Only one group is open at a time. The group holding the current page opens automatically. | |
| **Set up your workspace** card (rocket icon, "X of 5 done", progress bar) | Shown at the bottom of the main sidebar until setup is finished or an admin hides it. Opens `/admin/setup`. | Onboarding a new workspace. |

**Sidebar groups and items.** An item is hidden if you lack its permission or if the platform has switched off the feature.

| Group | Item (label) | URL | Permission | Extra condition |
|---|---|---|---|---|
| **Workspace** | General | /admin/general | general_settings:manage | |
| | Domains | /admin/domains | general_settings:manage | Only when multi-tenancy is on |
| | Usage | /admin/usage | general_settings:manage | |
| | Admissions | /admin/admissions | general_settings:manage | |
| | Privacy | /admin/privacy | general_settings:manage | |
| | Data export | /admin/export | general_settings:manage | |
| | Business hours | /admin/business-hours | business_hours:manage | |
| | SLA policies | /admin/sla | sla:manage | |
| **Help center** | Help center | /admin/help-center | help_center:manage | |
| **AI** | Providers / Snippets / Suggestions / Tools / Assistants | /admin/ai/... | ai:manage | |
| **Channels** | Inboxes | /admin/inboxes | inboxes:manage | |
| **Conversations** | Statuses | /admin/conversations/statuses | status:manage | |
| | Tags | /admin/conversations/tags | tags:manage | |
| | Custom attributes | /admin/custom-attributes | custom_attributes:manage | |
| | Shared views | /admin/conversations/shared-views | shared_views:manage | |
| **Productivity** | Macros | /admin/conversations/macros | macros:manage | |
| | Automations | /admin/automations | automations:manage | |
| **Teammates** | Agents | /admin/teams/agents | users:manage | |
| | Teams | /admin/teams/teams | teams:manage | |
| | Roles | /admin/teams/roles | roles:manage | |
| | Activity logs | /admin/teams/activity-log | activity_logs:manage | |
| **Notifications** | Email | /admin/notification | notification_settings:manage | |
| | Templates | /admin/templates | templates:manage | |
| **Security** | SSO | /admin/sso | oidc:manage | |
| **Integrations** | Webhooks, Slack, Microsoft Teams | ... | webhooks:manage | Only if the platform allows that integration |
| | Context links | /admin/context-links | context_links:manage | |

**Things common to every admin screen:**
- **Help panel:** On wide screens most pages show a grey help text in the right-hand column. The help text is quoted under each screen below.
- **List tables:** Every list table has a **Search** box above it that filters rows as you type.
- **Row menus:** The "⋮" button at the end of a row opens its menu. **Delete** always opens a dialog titled **"Are you absolutely sure?"** with **Cancel** and **Delete** buttons.
- **Saving:** A successful save shows a toast reading "Saved successfully" (`globals.messages.savedSuccessfully`).

## Set up your workspace (Setup checklist)

**Where:** Click the sidebar card, or go to `/admin/setup`.
**Who can use it:** Users with `general_settings:manage`.
**Purpose:** Walks a new workspace through its first five steps and tracks which are done.

| Element | What it does |
|---|---|
| "A few first steps to start answering customers." and "{done} of {total} done" with a progress bar | Shows overall progress. |
| Step **Connect an inbox** | "Bring in conversations from email, live chat, WhatsApp, Messenger or Instagram." **Start** opens New inbox. |
| Step **Invite a teammate** | "Add the agents who'll answer customers with you." **Start** opens New agent. |
| Step **Install the chat widget** | "Add a live chat inbox and put its widget on your website. Done when the first chat comes in." **Start** opens New inbox. |
| Step **Publish a help center article** | **Start** opens Help center. |
| Step **Reply to a customer** | "Send your first reply from a conversation." **Start** opens the Assigned inbox. |
| Done steps | Shown with a green tick and struck through. They have no **Start** button. |
| "Your workspace is set up." | Shown when all five steps are done. |
| **Hide checklist** | Dismisses the checklist for the workspace, which also removes the sidebar card. |

## General

**Where:** Admin → Workspace → **General**. URL: `/admin/general`.
**Who can use it:** `general_settings:manage`.
**Purpose:** Sets workspace-wide defaults: branding, language, time zone, default business hours, upload limits and login security. It also holds the **Delete workspace** area.
**Help panel:** "These are your desk-wide defaults. Team-level settings will override these where configured."

| Field / toggle | What it does / rules | Typical use |
|---|---|---|
| **Site name** (required, at least 1 character) | The workspace name. It appears in the browser tab title as "Page - Site name" and elsewhere. | Put your company or brand name. |
| **Language** (placeholder "Select language") | The default interface language. The list comes from the server's available languages. | Set Hindi or English for the whole desk. |
| **Timezone** (placeholder "Select timezone") | The workspace time zone. It is also used for the daily report time and the business-hours logic. | Asia/Kolkata. |
| **Business hours** (placeholder "Select business hours") | Picks the default business-hours schedule. SLA timers use it when a team has no schedule of its own. | Pick "Default" (Mon–Fri 09:00–17:00, seeded with every new workspace) or your own schedule. |
| **Root URL** (required, must be a valid URL) | "Root URL for the app. (No trailing slash)." It is used to build links, e.g. the OAuth callback URL and email links. | Change it after moving to a new address. |
| **Favicon URL** (required, valid URL) | The browser-tab icon. | Branding. |
| **Logo URL** (optional, valid URL or empty) | The logo shown in the app. | Branding. |
| **Max allowed file upload size** (number, 1–500) | "Max allowed file upload size in MB." | Raise it to 25 so agents can send larger PDFs. |
| **Allowed file upload extensions** (tags input, placeholder "jpg") | Type an extension and press Enter to add it; use the "x" on a tag to remove it. "Use `*` to permit all file types. For example: `jpg, png, pdf`". | Block risky file types. |
| **Show subject in conversation list** (switch) | "Display the conversation subject line in the conversation list when available." | Useful for email-heavy desks. |
| **Require single sign-on** (switch) | Agents can log in only through the SSO (OIDC) provider. Admins can still use their password, and API keys are not affected. The server refuses to save it with "Enable a single sign-on provider under Security → OIDC first." if no provider is enabled. | Enforce company login. |
| **Require two-factor login** (switch) | Every agent must log in with a code from an authenticator app, and anyone without one is made to set it up first. SSO logins and API keys are not affected. | Security compliance. |
| **Save** | Saves all of the above. | |

### Delete workspace (danger zone at the bottom of General)

Shown only when multi-tenancy is on **and** this is not the "default" workspace.

| Element | What it does |
|---|---|
| **Delete workspace** heading and text | "Everyone loses access at once. The workspace and all its data are deleted after 30 days; until then support can restore it. Export your data first if you want to keep it." |
| Confirm field: "Type the workspace address, {slug}, to confirm" | You must type the workspace slug exactly. |
| "Your password" field | Your current password. |
| **Delete workspace** button (red) | Enabled only when the slug matches and a password is entered. It shows "The workspace is scheduled for deletion." and then sends you to `/` about 1.5 seconds later. |

## Domains (custom domains)

**Where:** Admin → Workspace → **Domains**. URL: `/admin/domains`. Shown only when multi-tenancy is enabled.
**Who can use it:** `general_settings:manage`.
**Purpose:** Serve the workspace and help centers on your own domain, such as support.example.com.
**Help panel:** "Serve your workspace and help centers on a domain of your own, such as support.example.com." and "A domain works once it is verified. Make it the workspace address here, or set it as a help center's custom domain."

| Element | What it does |
|---|---|
| Hostname box (placeholder "support.example.com") and **Add domain** | Registers a domain. It starts as **Not verified**. |
| "Your workspace is at {address}." | The current workspace address. |
| "No custom domains yet." | Empty state. |
| Badges on each domain | **Verified** / **Not verified**. **Workspace address** marks the primary domain. **Help center: {name}** appears for each help center using the domain. |
| DNS instructions (unverified domains only) | "1. Prove you own the domain: add this TXT record…" shows Type / Name / Value. "2. Point the domain at your workspace with this CNAME record." Click any value to copy it ("Copied"). A note says DNS changes "can take a few minutes to an hour". The last error is shown in red. |
| **Verify** | Checks DNS. On success it shows "{domain} is verified." |
| **Use as workspace address** (verified, non-primary domains) | Makes the domain the main address. The toast reads "Your workspace address is now {address}. Links in emails use it." |
| **Use {address} instead** (on the primary domain) | Switches back to the built-in subdomain. |
| **Delete** | Asks for confirmation: "{domain} will stop serving your workspace and help centers." |
| Re-check warning | Shown on a verified domain whose TXT record has gone missing: "The verification record is missing ({count} of 3 daily checks). Put it back, or the domain stops working." |

## Usage

**Where:** Admin → Workspace → **Usage**. URL: `/admin/usage`.
**Who can use it:** `general_settings:manage`.
**Purpose:** Read-only view of what the workspace uses against its plan limits.
**Help panel:** "Limits come from the workspace's plan. At a limit, agents and inboxes can't be added, uploads stop once storage is full, and AI features pause until the next month."

| Element | Meaning |
|---|---|
| "Plan: {plan}" badge | The plan name. |
| **Upgrade plan** | Opens the platform's upgrade page in a new tab. Shown only if one is configured. |
| Meters: **Agents**, **Inboxes**, **File storage**, **AI requests this month** | Each shows "{used} of {limit}" with a bar. **Near the limit** appears at 80% or more; **Limit reached** (red) at 100%. A limit of 0 shows **Unlimited**. The AI meter adds "Counts again from the 1st of each month." |
| **API requests per minute**, **Requests over the limit today**, **Largest upload** | Fair-use figures. |

## Admissions (business / lead settings)

**Where:** Admin → Workspace → **Admissions**. URL: `/admin/admissions`.
**Who can use it:** `general_settings:manage`. The two AI sections additionally need `ai:manage`.
**Purpose:** Settings for institutes that handle enquiries: lead stages, a daily WhatsApp report, AI for calls, and a one-click AI admissions setup.

**Lead stages**
- What it is: "The steps an enquiry goes through, in order. Admitted stages count as admissions in reports; lost stages are leads that won't join."
- ▲ / ▼ arrows reorder the stages.
- Label box: up to 50 characters.
- Kind dropdown: **In progress**, **Admitted** or **Lost**.
- Each row shows "{n} leads" currently on that stage.
- ✕ removes a stage. It is disabled when only one stage is left, or when the stage still has leads (tooltip "Move its leads to another stage first").
- **Add stage** allows up to 12 stages.
- Switch **Put new enquiries on the first stage**: "Someone who calls or writes for the first time goes to "{stage}"."
- **Save**: every stage needs a name, and there must be at least one In progress and one Admitted stage.

**Daily report on WhatsApp**
- What it is: "Sends the day's numbers to the owner every evening: new enquiries, calls, missed calls, call-backs still waiting and admissions."
- Switch **Send the daily report**.
- **Send at** (default 20:00): a time, "In the workspace's time zone ({tz})."
- **Send from WhatsApp inbox**: pick a WhatsApp inbox, or "—" for none.
- **Template** (appears once an inbox is picked): choose an approved template. The report fills its values in this order: Date, New enquiries, Calls received, Missed calls, Still to call back, Admissions. Templates that need an agent (media header, header or button values) appear greyed out with "· needs an agent". **Refresh** reloads the list.
- **Create the suggested template**: submits a ready-made template to WhatsApp for review.
- **Send to**: WhatsApp numbers separated by commas, up to 10.
- **Save**.
- **Send today's report now**: a test send ("Report sent" on success).
- **Recent reports**: each row shows a date and **Sent** / **Failed** / **Sending**, plus any error.

**AI for calls and leads** (needs ai:manage). Note: "Each of these sends a request to your AI provider every time it runs, so it counts towards your AI usage."
- Switch **Summarise recorded calls**: transcribes recorded calls (Hindi, English or both) and summarises them on the call card.
- Switch **Fill in lead details**: fills a contact's empty fields (course, class, city…) from chats and calls. It never overwrites a field someone already filled.
- These switches save immediately; there is no Save button.
- A link points to **AI > Providers**.

**Set up the AI admissions assistant** (needs ai:manage). It "Adds what an institute needs… Only what's missing is added, so it's safe to run again."
- Checkbox: contact fields for admissions (student's name, class, course, preferred batch, city).
- Checkbox: knowledge base drafts (courses, fees, timings, admission process, scholarships, results). They stay off until you fill them in and switch them on.
- Checkbox: an AI assistant that answers in Hindi, English or Hinglish and hands over to a counsellor. Ticking it reveals:
  - **Assistant name**
  - **Hand over to team**
  - **Send new chats in these WhatsApp inboxes to the assistant** (shows "Connect a WhatsApp inbox first." if there are none)
- **Set up**: runs the setup and then shows what was added, with links **Fill them in** (Snippets) and **Open it** (the assistant). If nothing was missing it says "Everything was already set up."

## Privacy

**Where:** Admin → Workspace → **Privacy**. URL: `/admin/privacy`.
**Who can use it:** `general_settings:manage`.
**Purpose:** Automatic data retention and guidance for personal-data requests.
**Help panel:** "Each run of data retention that deletes something, and each change to it, is in the activity log. Files of deleted conversations are removed from storage within 12 hours."

| Element | What it does |
|---|---|
| **Data retention** description | Resolved conversations with no activity for the set time are deleted, together with their messages and files. Open and snoozed conversations are never deleted. |
| Radio **Keep conversations until they're deleted by hand** | Retention off. |
| Radio **Delete resolved conversations after [N] days** | Retention on. N must be between the server's minimum and maximum (7–3650 by default). The default is 365. |
| Switch **Also delete contacts left without conversations and inactive as long** (only when retention is on) | Also deletes such contacts. "Blocked contacts are kept, so they stay blocked." |
| Orange warning | "Deleted conversations and contacts can't be recovered. Retention runs every hour, so the first deletion happens within an hour of saving." |
| **Save** | Enabled only after a change. If the change would delete more data than before, a confirmation dialog repeats the warning (**Cancel** / **Save**). |
| **Requests about personal data** | Instructions only: export or delete a person's data from the contact's own page. |
| **Legal documents** | Links to **Privacy policy**, **Data processing agreement** and **Subprocessors**, if the platform has configured them. |

## Data export

**Where:** Admin → Workspace → **Data export**. URL: `/admin/export`.
**Who can use it:** `general_settings:manage`.
**Purpose:** Download the whole workspace as one ZIP file.
**Help panel:** "An export of a large workspace can take a while. Each is kept for 7 days, and one runs at a time."

| Element | What it does |
|---|---|
| Description | The ZIP contains conversations, contacts, agents, settings, help center articles and more, as JSON Lines files, together with the attached files. Passwords, keys and tokens are left out. |
| **Export all data** (shows **Exporting…** while running) | Starts an export: "Export started. It appears here when it's ready." The list refreshes every 3 seconds while an export runs. |
| Export rows | Each shows the date, a status (**Exporting…**, **Ready**, **Failed** or **Expired**), "By {email}", the size, and "Available until {date}". |
| **Download** (ready exports only) | Downloads the ZIP. |

## Business hours

**Where:** Admin → Workspace → **Business hours**. URLs: `/admin/business-hours`, `/new` and `/:id/edit`.
**Who can use it:** `business_hours:manage`.
**Purpose:** Define working hours and holidays. SLA clocks and "office hours" messages use them.
**Help panel:** "Business Hours allows you to set working hours for your entire helpdesk or for individual teams." and "SLA calculations are based on business hours. If a team has business hours set… Otherwise, it will fall back to the helpdesk's business hours."

**List**
- **New business hour** button.
- Columns: **Name**, **Created at**, **Updated at**.
- Row menu: **Edit**, or **Delete** ("…permanently delete this business hour.").

**Form**

| Field | Meaning |
|---|---|
| **Name**, **Description** | |
| **Set business hours** | **Always open (24/7)** or **Custom business hours**. |
| Weekday rows (custom only) | Tick a day, then set its open and close times ("09:00 to 17:00"). Unticked days are closed. |
| **New holiday** | Opens a dialog with **Name** and **Date** (**Pick a date** calendar). Click **Add**. Holidays appear in a Name/Date table, each with a delete icon. |
| **Create** / **Save** | |

## SLA policies

**Where:** Admin → Workspace → **SLA policies**. URLs: `/admin/sla`, `/new` and `/:id/edit`.
**Who can use it:** `sla:manage`.
**Purpose:** Set response and resolution targets and alert people before or after a breach.
**Help panel:** "Configure SLA policies to set response, resolution and next response time targets." and "…Breached SLAs trigger notifications to configured team members."

**List**
- **New SLA policy** button.
- Columns: Name, Created at, Updated at.
- Row menu: **Edit**, or **Delete** ("…permanently delete this SLA policy.").

**Form**

| Field | Meaning |
|---|---|
| **Name** and **Description** (1–255 characters each) | |
| **First response time** (e.g. 6h) | Target time for the first agent reply. |
| **Resolution time** (e.g. 24h) | Target time to resolve. |
| **Next response time** (e.g. 30m) | Target time for each following reply after the customer writes. |

Rules for the three targets: at least one is required, durations are written like 1h, 30m or 1h30m, the minimum is 1 minute, and the first response time can't be later than the resolution time.

**Alert configuration** ("Set up alert triggers and recipients")
- **Add breach alert** adds a **Breach Alert** (post-breach).
- **Add warning alert** adds a **Warning Alert** (pre-breach).
- With none added, the section shows "No alerts configured".

Each alert card has:
- **Trigger timing** (breach alerts only): **Immediately on breach** or **After specific duration**.
- **Advance warning** (warning alerts) or **Follow up delay** (breach alerts with a delay): a duration.
- **Alert recipients**: agents and/or **Assigned user**. At least one is required.
- **SLA metric**: **All**, First response time, Next response time or Resolution time.
- ✕ removes the alert.

**Create** / **Save**.

SLA policies are applied through a team's **SLA policy** setting, or by automations and macros ("Set SLA").
