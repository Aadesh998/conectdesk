# Admin: conversation settings

The building blocks agents use on conversations: statuses, tags, macros, notification email and templates, custom attributes and shared views.

## Statuses

**Where:** Admin → Conversations → **Statuses**. URL: `/admin/conversations/statuses`.
**Who can use it:** `status:manage`.
**Purpose:** Add custom conversation statuses on top of the built-in ones.
**Help panel:** "Create custom conversation statuses to extend default workflow."

| Element | What it does |
|---|---|
| **New status** | Opens a dialog ("Set status name. Click save when you're done."). |
| **Name** (1–25 characters) | e.g. "Waiting on vendor". |
| **Category**: **Open**, **Waiting** or **Resolved** | Decides how the status behaves. Open statuses count toward the auto-assign limit. Resolved statuses are what CSAT and data retention treat as resolved. |
| **Create** / **Save** | |
| Table | **Name** (click to edit, custom statuses only), **Category**, **Created at**. |
| Row menu **Edit** / **Delete** | Hidden for the four built-in statuses: Open (Open), Snoozed (Waiting), Resolved (Resolved) and Closed (Resolved). Delete confirms with "…permanently delete this status." |

## Tags

**Where:** Admin → Conversations → **Tags**. URL: `/admin/conversations/tags`.
**Who can use it:** `tags:manage`.
**Purpose:** Labels for conversations, used to filter and in automations.
**Help panel:** "Tags can be used to filter conversations and as conditions in automations."

| Element | What it does |
|---|---|
| **Import** | CSV import with a single `name` column, e.g. Bug / Feature Request / Billing. Same import dialog as for agents. Duplicates are reported in the log. |
| **New tag** | Opens a dialog with **Name** (at least 3 characters), then **Create**. |
| Table | **Name**, **Created at**, **Updated at**. |
| Row menu **Edit** | Rename the tag (dialog "Change the tag name…", then **Save**). |
| Row menu **Delete** | "Are you sure you want to delete this tag? This will also remove it from all conversations". |

## Macros

**Where:** Admin → Productivity → **Macros**. URLs: `/admin/conversations/macros`, `/new` and `/:id/edit`.
**Who can use it:** `macros:manage`.
**Purpose:** One-click bundles of a canned reply and/or conversation actions that agents apply while working.
**Help panel:** "Combine multiple conversation actions into single-click macros."

**List**
- **New macro** button.
- Columns: **Name**, **Visibility**, **Usage** (times used), **Created at**, **Updated at**.
- Row menu: **Edit**, or **Delete** ("…permanently delete this macro.").

**Form**

| Field | Meaning |
|---|---|
| **Name** (required) | |
| **Response to be sent when macro is used (optional)** | A rich-text reply inserted into the agent's editor. |
| **Actions (Optional)** | **Add action**, then choose an **Action type** and its **Value**. Types: Assign team, Assign agent, Set status, Set priority, Add tags, Set tags (replaces all tags), Remove tags. Remove an action with its ✕. Duplicate actions aren't allowed. |
| Rule | The macro needs either a response or at least one action ("Either message content or actions are required"). |
| **Visible when** (multi-select) | **Replying**, **Starting conversation**, **Adding private notes**: the editor modes in which the macro is offered. |
| **Visibility** | **All agents**, **Team** (then pick a **Team**) or **Agent** (then pick an **Agent**): who can see the macro. |
| **Create** / **Save** | |

Agents can only apply a macro's actions if they hold the matching permissions. Otherwise they see "Permission denied for some macro actions" or "Macro partially applied".

## Email notifications (Notifications → Email)

**Where:** Admin > Notifications > Email. URL `/admin/notification`
**Who can use it:** `notification_settings:manage`
**Purpose:** The mail server used to send notification emails to team members.
**Help panel:** "Configure SMTP server settings for sending email notifications to team members. Once configured, teammates receive automated alerts for conversation assignments, SLA breaches, and other important events."

| Field | Notes |
|---|---|
| **Enabled** | Turns email notifications on or off. |
| **SMTP Host**, **SMTP Port** | Port 1 to 65535, default 587. |
| **Username**, **Password** | Required. |
| **Authentication Protocol** | Plain, Login, CRAM-MD5, None. |
| TLS (placeholder "Select TLS type") | Off, SSL/TLS, STARTTLS. |
| **From email address** | e.g. `My inbox <support@example.com>`. |
| **HELO Hostname** | Defaults to localhost. |
| **Max Connections** | 1 to 1000. |
| **Max Retries** | 0 to 1000, default 2. |
| **Idle Timeout** | Default 15s. |
| **Wait Timeout** | Default 5s. |
| **Skip TLS Verification** | Toggle. |
| **Save** | |

The automation action "Notify agents" sends its emails through this server.

If a workspace sets up no mail server here, its email goes out through the platform's mail server (Platform admin → Settings), with the workspace's name on the platform's From address.

## Templates (email templates)

**Where:** Admin → Notifications → **Templates**. URLs: `/admin/templates`, `/new?type=email_outgoing` and `/:id/edit`.
**Who can use it:** `templates:manage`.
**Purpose:** Control the HTML of emails sent to customers and the notification emails sent to your team.
**Help panel:** "Customize the look and content of outgoing emails to customers and notification emails to your team."

| Element | What it does |
|---|---|
| Tab **Outgoing email templates** | The wrappers used for email replies to customers. Columns: Name, **Default** (marks the default template), Created at. |
| Tab **Email notification templates** | Built-in system emails: Conversation assigned, SLA breach warning, SLA breached, Mentioned in conversation, CSAT request and others. Columns: Name, Created at. |
| **New template** | Enabled only on the Outgoing tab. |
| Row menu **Edit** | Opens the template. |
| Row menu **Delete** | Outgoing templates only. Built-in templates can't be deleted. Confirms with "…permanently delete this template." |
| Form **Name** | Editable only for outgoing templates. |
| Form **Subject** | Notification templates only (not CSAT request). |
| Form **Body** | HTML code editor. Outgoing templates must contain `{{ template "content" . }}` exactly once; that is where the agent's reply goes. |
| Form **Is default** checkbox | Outgoing only. "You can have only one default outgoing email template." |
| **Create** / **Save** | |

## Custom attributes

**Where:** Admin → Conversations → **Custom attributes**. URL: `/admin/custom-attributes`.
**Who can use it:** `custom_attributes:manage`.
**Purpose:** Extra fields stored on contacts or conversations, e.g. subscription plan or first purchase date. They can be used in pre-chat forms, filters and views.
**Help panel:** "Custom attributes help you set additional details about your contacts or conversations…"

| Element | What it does |
|---|---|
| Tabs **Contact** / **Conversation** | Lists the attributes of each kind. Columns: Name, Key, Type, Applies to, Created at, Updated at. |
| **New custom attribute** | Opens a dialog. |
| **Applies to** | Contact or Conversation. |
| **Name** (3–140 characters) | The display name. Typing a name auto-fills the **Key**. |
| **Key** | Lowercase letters, numbers and underscore. It becomes read-only after creation and must not clash with built-in fields. |
| **Description** | |
| **Type** | Text, Number, Checkbox, Date, Link or List. It can't be changed after creation. |
| **List values** (List type only) | Type each option and press Enter. |
| **Regex (Optional)** and **Regex hint (Optional)** (Text type only) | Validate values against a pattern (e.g. `^[a-zA-Z]*$`). The hint is shown to agents. |
| **Create** / **Save** | |
| Row menu **Edit** / **Delete** | Delete confirms with "…permanently delete this custom attribute." |

## Shared views

**Where:** Admin → Conversations → **Shared views**. URLs: `/admin/conversations/shared-views`, `/new` and `/:id/edit`.
**Who can use it:** `shared_views:manage`.
**Purpose:** Saved conversation filters that appear in the sidebar of all agents or of one team.
**Help panel:** "Create shared views visible to all agents or specific teams."

**List**
- **New shared view** button.
- Columns: **Name**, **Visibility** (All agents / Team), **Created at**, **Updated at**.
- Row menu: **Edit**, or **Delete** ("…permanently delete this shared view.").

**Form**

| Field | Meaning |
|---|---|
| **Name** (2–140 characters) | "Enter an unique name for your view." |
| **Filters** (at least one) | **Add filter** adds a row of field, operator and value. **Add group** adds a group; **Remove group** removes it. Click the AND/OR connector to switch it. Fields include Status, Priority, Assign team, Assign agent, Inbox, Tags, Created at, Waiting since, Snoozed until, Last message at, Last interaction at, Next SLA deadline, contact email or external ID, Last interaction by (Contact/Agent), SLA policy, Channel (Email / Live Chat / WhatsApp / Messenger / Instagram / Telegram / Phone), message content, and custom attributes. |
| **Visibility** | **All agents**, or **Team** (then pick a **Team**). |
| **Create** / **Save** | |
