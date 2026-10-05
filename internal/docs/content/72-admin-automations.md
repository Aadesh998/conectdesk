# Admin: automations

Rules that act on conversations by themselves: when one is created, when it changes, or every hour. Typical uses: route WhatsApp chats to an AI assistant, assign by inbox, apply an SLA, escalate long-waiting conversations, send CSAT.

## Automations list

**Where:** Admin > Productivity > Automations. URL `/admin/automations`
**Who can use it:** `automations:manage`
**Purpose:** Rules that act on conversations automatically.
**Help panel:** "Automate actions when conversations are created, updated, or on an hourly schedule."

| Element | What it does |
|---|---|
| **New rule** | Opens the rule editor with the type of the tab you are on. |
| Tab **New conversation** | "Rules that run when a new conversation is created by a contact. Conversations initiated by agents do not trigger these rules. Drag and drop to reorder." |
| Tab **Conversation update** | "Rules that run when a conversation is updated." |
| Tab **Time triggers** | "Rules that run once an hour." Every hour the engine checks conversations created in the last 30 days. |
| Execution mode (New conversation tab only, shown once rules exist) | **Execute all matching rules** or **Execute first matching rule**. Saves immediately. |
| Drag to reorder (New conversation tab) | Sets which rule runs first. Saves immediately. |
| Rule card | Shows the name (click to edit), an Enabled/Disabled badge and the description. Menu: **Edit**, **Enable**/**Disable**, **Delete** (confirmation: "This action cannot be undone..."). |
| Empty tab | "No rules found". |

## Rule editor (New rule / Edit rule)

**Where:** `/admin/automations/new?type=...`, `/admin/automations/:id/edit`

| Field | What it does |
|---|---|
| **Enabled** (checkbox, edit only) | Turns the rule on or off. |
| **Name**, **Description** | Your own labels for the rule. |
| **Type** | New conversation / Conversation update / Time triggers. Changing the type clears the conditions. |
| **Events** (Conversation update only) | "Evaluate rule on these events." Options: **Agent assigned**, **Team assigned**, **Priority change**, **Status change**, **Outgoing message**, **Incoming message**. |
| **Match these rules** | Two condition groups. Each group has a radio choice, "Match **ANY** below" or "Match **ALL** below", and an **Add condition** button. |
| **AND / OR** buttons between the groups | How the two groups combine. |
| Each condition | Field, operator and value, a remove (x) button, and a **Case sensitive match** checkbox. |
| **Perform these actions** | **Add action**, then pick the type and its value. Each action has a remove (x). |
| **Create** / **Save** | Checks the rule before saving. It warns if: there is no condition ("Please add at least one condition."), a condition has no field, operator or value, there is no action, or an action has no value. |

### Condition fields by rule type

**New conversation:**
- Email (the contact's)
- Content (message text)
- Subject
- Status
- Priority
- Assign team (the assigned team)
- Assign agent (the assigned agent)
- Inbox

**Conversation update:**
- Status, Priority, Assign team, Assign agent
- Hours since created, Hours since first reply, Hours since last reply, Hours since resolved
- Inbox
- Previous status, Previous priority, Previous assigned user, Previous assigned team

**Time triggers:** the same as Conversation update, without the four "Previous ..." fields.

**All types:** every contact custom attribute is also listed, under a "Contact" heading.

### Operators

| Field kind | Operators |
|---|---|
| Choice fields (Status, Priority, Team, Agent, Inbox, list-type attributes) | equals, not equals, set, not set |
| Text fields | equals, not equals, set, not set, contains any of, contains none of, starts with |
| Number fields (hours) | equals, not equals, greater than, less than |
| Yes/No attributes | equals, not equals (value True or False) |
| Date attributes | equals, not equals, set, not set, greater than, less than, between |

- "contains any of" and "contains none of" take several values. Type each one and press Enter.
- "set" and "not set" need no value.

### Actions

| Label | What it does |
|---|---|
| **Assign team** | Assigns the conversation to a team. |
| **Assign agent** | Assigns it to an agent. AI assistants can be picked here too. |
| **Set status** | Changes the status (Snoozed is not offered; use Snooze instead). |
| **Set priority** | Changes the priority. |
| **Add private note** | Adds an internal note, written in a rich-text editor. |
| **Send reply** | Sends a rich-text reply to the contact only (CCs are not copied). It is skipped if the contact has no email address. |
| **Send CSAT** | Sends the satisfaction survey email. It needs no value, is skipped if the contact has no email, and is only sent once per conversation. |
| **Set SLA** | Applies an SLA policy. |
| **Add tags** / **Set tags** / **Remove tags** | Adds tags, replaces all tags, or removes tags. |
| **Notify agents** | Pick recipients (Assignee, Assigned team, any "Team: X" or "Agent: Y"), then a Subject and a Message. Sends an in-app notification plus an email. |
| **Snooze** | Snoozes for a duration, e.g. `30m`, `3h`, `2h30m`. |
| **Trigger webhook** | Pick a webhook and type an event name (e.g. `priority.escalated`). "Event name sent in the webhook payload." The webhook receives the conversation and the actor. |

When a rule runs, its actions are carried out by the system user.
