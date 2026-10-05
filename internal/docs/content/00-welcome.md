# Welcome

This is the internal manual for **ConectDesk**, the customer-support and admissions desk we run at **desk.arrlink.com**. It explains every screen and button: what it does, why you'd use it, where to find it and who is allowed to use it. It covers the agent app, every Admin setting, the pages customers see, the platform admin dashboard and the operator tools.

Only docs viewers can read this site. Platform admins give and remove access under **Docs access** on the platform dashboard (see [Platform admin dashboard](/docs/platform-admin#115-docs-access)).

## What the product does

ConectDesk brings every customer conversation into one shared inbox, whatever channel it arrives on, so a team can answer, track and report on them:

- **Channels:** email (Gmail, Microsoft 365 or any IMAP/SMTP mailbox), a live-chat widget on your website, WhatsApp, Facebook Messenger, Instagram, Telegram, and phone calls through Exotel.
- **Agents** reply from one screen, with private notes, @mentions, macros, tags, statuses, SLAs and a built-in AI copilot called **Juno**.
- **AI assistants** can answer customers on their own from your knowledge base and hand over to a person.
- **Admissions tools** for coaching institutes and schools: a Leads board with stages, call-backs for missed calls, call recordings with AI summaries and a daily WhatsApp report.
- **Help centers**: public article sites customers can search.
- **Reports** on workload, SLA, CSAT and calls.

It is **multi-tenant**: each customer organisation gets its own **workspace** with its own agents, inboxes, settings and data, which other workspaces can't see.

## Who uses what

| Who | What they use | How they log in |
|---|---|---|
| **Customers / website visitors** | The chat widget, email, WhatsApp and other channels; the public help center; CSAT survey pages. | They don't; the widget can identify logged-in users of your own site. |
| **Agents** | The workspace app: inboxes, conversations, contacts, leads, call-backs, reports (with permission). | Email + password (optionally two-factor or SSO) on the workspace address. |
| **Workspace admins** | Everything agents use, plus the **Admin** section of their workspace. What each person may do is set by their **roles**. | Same as agents. |
| **Platform admins** (us) | The platform dashboard: create, suspend, delete and inspect workspaces, set plans, channels and integrations, log in as an agent for support, run the platform mail server, manage docs access. | Separate login on the platform host, with required two-factor login. Added only from the command line. |
| **Docs viewers** | This documentation, and nothing else. | Separate login on the docs host. Added by platform admins. |
| **Operators** (whoever runs the servers) | Command-line tools and the configuration file. | Server access. |

## Where everything lives

| Address | What is there |
|---|---|
| `https://desk.arrlink.com` | Landing page: type your workspace's name to go to its login. No signup. |
| `https://<slug>.desk.arrlink.com` | One workspace, e.g. `brightclasses.desk.arrlink.com`. The agent app, its Admin section, its chat widget and help centers. |
| A workspace's own domain, e.g. `support.example.com` | The same workspace, or one of its help centers, once the domain is verified under Admin → Domains. |
| `https://<workspace>/hc/<help-center-slug>` | A public help center. |
| `https://<workspace>/csat/<id>` | A customer's satisfaction survey. |
| `https://admin.desk.arrlink.com` | The platform admin dashboard. |
| `https://docs.desk.arrlink.com` | This documentation. |

## How to read these docs

- Pages follow the order you meet things: signing in, the layout, conversations, calls and contacts, reports, then each Admin area, the customer-facing pages, the platform dashboard and operations.
- Each screen starts with **Where** (how to get there, and its address), **Who can use it** (the permission), and **Purpose**, then lists every button and field.
- **Bold** text is the exact label you see on screen. Text in "quotes" is an exact on-screen message. `code` is an address, a permission or a value to type.
- Use the search box at the top (press <kbd>/</kbd>) to jump straight to any screen or button.

## Where do I find…

| I want to… | Go to |
|---|---|
| Reply to a customer, add a note, change status | [Conversations → Reply box](/docs/conversations#reply-box) |
| Snooze, apply a macro, resolve from the keyboard | [Command palette](/docs/workspace-layout#command-palette) |
| Ask the AI about a conversation | [Conversations → Juno](/docs/conversations#juno-ai-copilot) |
| Call a customer, see missed calls | [Calls, contacts, leads](/docs/calls-contacts-leads) |
| Move an enquiry through admission stages | [Leads](/docs/calls-contacts-leads#leads-admissions-board) |
| See SLA, CSAT and call numbers | [Reports](/docs/reports) |
| Turn on two-factor login for myself | [Account → Security](/docs/signing-in#account--security-two-factor-login) |
| Connect email, WhatsApp, a chat widget or a phone line | [Admin: inboxes](/docs/admin-inboxes) |
| Invite an agent, reset their password or 2FA | [Admin: teammates](/docs/admin-teammates#new-agent--edit-agent) |
| Decide what a role may do | [Roles → Permissions explained](/docs/admin-teammates#permissions-explained) |
| Make canned replies | [Macros](/docs/admin-conversation-settings#macros) |
| Route conversations automatically | [Admin: automations](/docs/admin-automations) |
| Set up an AI assistant | [Admin: AI → Assistants](/docs/admin-ai#ai-assistants-ai-agents) |
| Publish help articles | [Admin: help center](/docs/admin-help-center) |
| Post conversations to Slack or Teams, send webhooks | [Admin: integrations](/docs/admin-integrations) |
| Create a workspace for a new customer | [Platform admin → Create workspace](/docs/platform-admin#17-create-workspace) |
| Change a workspace's plan, channels or integrations | [Platform admin → Workspace detail](/docs/platform-admin#16-workspace-detail) |
| Log in as an agent to help them | [Platform admin → Log in as](/docs/platform-admin#111-impersonate-log-in-as) |
| Give someone access to these docs | [Platform admin → Docs access](/docs/platform-admin#115-docs-access) |
| Add a platform admin, run upgrades | [Operations → CLI](/docs/operations) |

## Words used in these docs

| Word | Meaning |
|---|---|
| **Workspace** (tenant) | One customer organisation's separate copy of the desk. Its short name, the **slug**, is in its address. |
| **Inbox** | One channel connection: a mailbox, a chat widget, a WhatsApp number, a Page, an Instagram account, a Telegram bot or a phone line. |
| **Conversation** | One thread with a customer, with a status (Open, Snoozed, Resolved, Closed or custom ones), an assignee, a team, a priority and tags. |
| **Contact** / **Visitor** | A known customer / an anonymous website chat visitor. |
| **Private note** | An internal message on a conversation that customers never see. |
| **Macro** | A saved reply and/or set of actions applied in one click. |
| **SLA** | Response and resolution time targets, with warnings and breach alerts. |
| **CSAT** | Customer satisfaction survey (1–5 rating). |
| **Juno** | The AI copilot agents chat with inside a conversation. |
| **AI assistant** | An AI that answers customers itself and hands over to a person. |
| **Jev** | An optional fast decision model that tells assistants when to reply, checks replies and follows up. |
| **Lead stage** | Where an enquiry is in the admissions pipeline (In progress, Admitted, Lost). |
| **Call-back** | A missed call waiting to be returned. |
| **Plan** | The limits a workspace runs under (agents, inboxes, storage, AI requests, API rate). |
| **Platform admin** | Someone who runs the whole installation; not a user of any workspace. |
