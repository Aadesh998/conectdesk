# Admin: teammates, roles and security

Who can log in to the workspace and what they may do: agents, teams, roles and every permission, the activity log, and single sign-on.

## Agents: list

**Where:** Admin → Teammates → **Agents**. URL: `/admin/teams/agents`.
**Who can use it:** `users:manage`.
**Purpose:** Manage the people who log in and answer customers.
**Help panel:** "Manage support agents, roles, permissions and teams."

| Element | What it does |
|---|---|
| **Import** | Opens the import dialog (see below). |
| **New agent** | Opens the invite form. |
| Columns | **First name** (click to edit), **Last name**, **Status** (Enabled/Disabled), **Email**, **Created at**, **Updated at**. |
| Row menu **Edit** | Opens the agent. |
| Row menu **Delete** | "This will permanently delete the agent. Consider disabling the account instead." The system user cannot be deleted. |

**Import dialog**
1. Choose **Select CSV file**.
2. The dialog shows the **Required CSV format**: `first_name,last_name,email,roles,teams`. Multiple roles go in quotes, e.g. `"Agent,Admin"`. "Roles and teams must match exactly (case-sensitive)".
3. Click **Import**.
4. A **Logs** area reports each row, e.g. "Row 3: Created agent…", "email already exists", "invalid role(s)", and finally "Import completed: X of Y successful, Z failed". The list refreshes when the import finishes.
5. Buttons: **Cancel** / **Close**.

## New agent / Edit agent

**Where:** **New agent** (`/admin/teams/agents/new`) or Edit (`/admin/teams/agents/:id/edit`).
**Who can use it:** `users:manage`.
**Purpose:** Invite an agent, or change their details, access, password, API key and two-factor login.

| Element | New | Edit | What it does |
|---|---|---|---|
| Summary card | | ✓ | Avatar, name, current availability badge (Active / Away / Away and reassigning / Offline), **Last active**, **Last login**. |
| **Enabled** checkbox | | ✓ | Untick it to disable the account so the person can't log in. This keeps their history, unlike Delete. |
| **First name** (2–50), **Last name**, **Email** (valid email) | ✓ | ✓ | |
| **Phone for calls** | ✓ | ✓ | "+91…" format, with the country code. Exotel rings this number for phone-channel calls. |
| **Teams** | ✓ | ✓ | Team membership. Agents are added to teams here, not on the Team screen. |
| **Roles** (at least 1) | ✓ | ✓ | Decides the agent's permissions. |
| **Availability status** | | ✓ | Set the agent to **Active**, **Away** or **Away and reassigning** (away, and their conversations are handed to others). |
| **Set password** | | ✓ | Admin resets the password. It must be 10–72 characters with an uppercase letter, a lowercase letter, a number and a special character. |
| **API keys** section | | ✓ | "Generate API keys for this agent to access ConnectDesk programmatically." When there is no key: **Generate API key**. When there is one: **Regenerate** (replaces the key) and **Revoke** (deletes it), plus "Last used". A newly generated key opens a dialog titled **API key generated** showing **API key** and **Secret** with copy buttons. Warning: "This secret will only be shown once." Click **Close** when done. |
| **Two-factor login** → **Reset two-factor login** | | ✓, shown only if the agent has 2FA on | For someone who lost their phone. They log in with their password alone until they set it up again; if the workspace requires 2FA they are asked to do so at once. The toast reads "Two-factor login reset". |
| **Browser calling** section | | ✓ | Lets the agent take calls in the browser. Fields: **SIP username**, **SIP password** ("Saved. Leave empty to keep it."), **SIP domain**, **WebSocket server (Optional)**. **Save** and **Remove** (calls go back to ringing the agent's phone). A status line reads "Calls ring their browser" or "They switched calls to their phone". |
| **Send welcome email?** checkbox | ✓ | | Emails the agent a welcome message with a link to set their own password. This is the invitation. |
| **Create** / **Save** | ✓ | ✓ | |

## Teams

**Where:** Admin → Teammates → **Teams**. URLs: `/admin/teams/teams`, `/new` and `/:id/edit`.
**Who can use it:** `teams:manage`.
**Purpose:** Group agents for assignment, team inboxes, business hours and SLA.
**Help panel:** "Configure team settings including working hours and SLA policies." and "Manage agent auto-assignment limits and more."

**List**
- **New** button.
- Columns: **Name** (click to edit), **Created at**, **Updated at**.
- Row menu: **Edit**, or **Delete**, which opens "Delete Team" with "This action cannot be undone. This will permanently delete this team."

**Form fields**

| Field | Meaning |
|---|---|
| **Emoji** (required) | Click to open an emoji picker. The emoji is shown next to the team name. |
| **Name** (at least 2 characters) | |
| **Auto assignment type** | **Round robin**: conversations are spread across team members in turn. **Manual**: members pick conversations themselves. |
| **Maximum auto-assigned conversations** | The most open conversations auto-assigned to one agent. Only statuses in the "Open" category count. 0 means unlimited. |
| **Timezone** (required) | The team's time zone, used for SLA. |
| **Business hours** | **None** or a schedule. Used for this team's SLA calculations. |
| **SLA policy** | **None** or a policy, applied automatically when a conversation is assigned to the team. |
| **Create** / **Save** | |

Members are added on the agent form, in its **Teams** field.

## Roles

**Where:** Admin → Teammates → **Roles**. URLs: `/admin/teams/roles`, `/new` and `/:id/edit`.
**Who can use it:** `roles:manage`.
**Purpose:** Define sets of permissions and give them to agents.
**Help panel:** "Manage roles and their permissions for fine-grained control over your support desk."

**List**
- **New role** button.
- Columns: **Name**, **Description**, **Created at**.
- Row menu: **Edit**, or **Delete** ("This action cannot be undone. This will permanently delete this role.").

**Server rules**
- The built-in **Admin** role cannot be edited ("Cannot modify admin role, Please create a new role.").
- The built-in **Admin** and **Agent** roles cannot be deleted.
- A role must have at least one permission.
- Role names must be unique.

**Form**
- **Name** (2–50 characters, placeholder "Agent").
- **Description** (2–300 characters, placeholder "Role for all support agents").
- **Set permissions for this role**: one checkbox per permission, in three groups (below).
- **Create** / **Save**.

### Permissions explained

**Group "Conversations"**

| Label | Code | What it grants |
|---|---|---|
| View conversation | conversations:read | The base permission for opening any conversation. Without it none of the "View…" scopes below work. It also covers search, mentions, call-backs, view inboxes and marking as unread. |
| Create conversation | conversations:write | Start a new outbound conversation. |
| View conversations assigned to me | conversations:read_assigned | See conversations assigned to yourself (the "Assigned" inbox). |
| View all conversations | conversations:read_all | See every conversation in the workspace. |
| View all unassigned conversations | conversations:read_unassigned | See conversations assigned to no person and no team. |
| View conversations in team inbox | conversations:read_team_inbox | See conversations assigned to your team but not yet to a person. |
| View your team conversations | conversations:read_team_all | See every conversation assigned to your teams, including those assigned to teammates. |
| Assign conversations to users | conversations:update_user_assignee | Assign or unassign an agent. |
| Assign conversations to teams | conversations:update_team_assignee | Assign or unassign a team. |
| Change conversation priority | conversations:update_priority | |
| Change conversation status | conversations:update_status | Resolve, snooze or reopen, and reset a chat session. |
| Add or remove conversation tags | conversations:update_tags | |
| View conversation messages | messages:read | Read messages and attachments, and download transcripts. |
| Send messages in conversations | messages:write | Reply and send private notes, retry or delete messages, send WhatsApp templates, place, transfer and summarise calls, and use the AI summary. |
| Send messages as contact | messages:write_as_contact | Post a message as if the customer sent it, e.g. to log something they said by phone. |
| Create and manage conversation views | view:manage | Personal saved views in the sidebar. |

**Group "Admin"**

| Label | Code | What it grants |
|---|---|---|
| Manage general settings | general_settings:manage | General, Domains, Usage, Admissions, Privacy, Data export, the Setup checklist and Delete workspace. |
| Manage notification settings | notification_settings:manage | Admin → Notifications → Email. |
| Manage conversation statuses | status:manage | Statuses screen. |
| Manage SSO configuration | oidc:manage | SSO screen. |
| Manage tags | tags:manage | Tags screen and tag import. |
| Manage macros | macros:manage | Macros screen. |
| Manage users | users:manage | Agents screen: invite, import, edit, delete, API keys, 2FA reset, browser calling. |
| Manage teams | teams:manage | Teams screen. |
| Manage automations | automations:manage | Automations. |
| Manage inboxes | inboxes:manage | Inboxes, every channel connect flow, WhatsApp templates and health. |
| Manage roles | roles:manage | Roles screen. |
| Manage templates | templates:manage | Templates screen. |
| Manage reports | reports:manage | The Reports section (overview, calls, SLA). |
| Manage business hours | business_hours:manage | Business hours screen. |
| Manage SLA policies | sla:manage | SLA screen. |
| Manage AI features | ai:manage | The AI admin section and the AI parts of Admissions. |
| Manage help centers | help_center:manage | Help center. |
| Manage custom attributes | custom_attributes:manage | Custom attributes screen. |
| Manage activity log | activity_logs:manage | Activity logs screen. |
| Manage webhooks | webhooks:manage | Webhooks, plus the Slack and Microsoft Teams integration pages. |
| Manage shared views | shared_views:manage | Shared views screen. |
| Manage context links | context_links:manage | Context links. |

**Group "Contacts"**

| Label | Code | What it grants |
|---|---|---|
| View all contacts | contacts:read_all | The Contacts section, its list, and the lead board. It also shows the Contacts icon in the rail. |
| View contact details | contacts:read | Open or search a contact, view their lead stage, call a contact. |
| Edit contact details | contacts:write | Edit contacts, import contacts, set lead stage, AI fill. |
| Block contacts | contacts:block | |
| Delete contacts | contacts:delete | Erase a contact and their conversations. |
| Export contact data | contacts:export | Export one person's data. |
| View contact notes | contact_notes:read | |
| Add contact notes | contact_notes:write | |
| Delete contact notes | contact_notes:delete | |

**Built-in roles in a new workspace**
- **Admin**: every permission except "Send messages as contact".
- **Agent**: all the conversation view and update permissions, View/Send messages, and conversation views. No admin or contact permissions.

## Activity logs

**Where:** Admin → Teammates → **Activity logs**. URL: `/admin/teams/activity-log`.
**Who can use it:** `activity_logs:manage`.
**Purpose:** An audit trail of agent and security events.

| Element | What it does |
|---|---|
| **Filter** button (shows a count when active) | Opens a filter builder. Fields: **Actor** (the agent) and **Log entry type**. Buttons **Apply** / **Clear**. |
| Log entry types | Agent login, Agent logout, Agent away, Agent away reassigned, Agent online, Agent password set, Agent role permissions changed, Contact deleted, Contact data exported, Platform support login, Data retention changed, Data retention deletion. Two-factor login on/off and Workspace data exported also appear as entries. |
| Sort button (arrows icon) | Order by **Created at**, **Ascending** or **Descending**. |
| Table | **Name** (the event description), **Timestamp**, **IP Address**. Read-only. |
| Pagination bar | Page number and rows per page. |

Typical uses: checking who logged in, investigating a role change, proving that data was exported or deleted.

## SSO (OIDC)

**Where:** Admin → Security → **SSO**. URLs: `/admin/sso`, `/new` and `/:id/edit`.
**Who can use it:** `oidc:manage`.
**Purpose:** Let agents sign in with Google, Microsoft or another OpenID Connect provider.
**Help panel:** "Configure single sign-on with one or more OpenID Connect providers."

**List**
- **New SSO** button.
- Columns: **Name**, **Provider**, **Status** (Enabled/Disabled), **Created at**, **Updated at**.
- Row menu: **Edit**, or **Delete** ("…permanently delete this SSO.").

**Form**

| Field | Meaning |
|---|---|
| **Enabled** checkbox (edit only) | Turns the login button for this provider on or off. |
| **Provider** | Google, Microsoft or Custom. |
| **Name** | The label on the login button (e.g. "Google"). |
| **Provider URL** | The issuer URL, e.g. https://accounts.google.com. |
| **Logo URL** (Custom only) | "Custom logo URL to display on the login page." |
| **Client ID** / **Client secret** | From your identity provider's app. |
| **Callback URL** (edit only, read-only) | "Set this URI for callback." Copy it into the identity provider. |
| **Create** / **Save** | |

To force everyone to use SSO, turn on **Require single sign-on** on the General screen. The server refuses that setting unless an SSO provider is enabled.
