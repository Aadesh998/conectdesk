# Admin: integrations

Webhooks, Slack, Microsoft Teams and Context links. Each appears only if the platform admin allowed that integration for the workspace (Platform admin → workspace → Integrations); turning one off there hides it and stops it sending, but keeps its setup. WhatsApp, Messenger, Instagram and Telegram are channels, not integrations: see Admin: inboxes and channels.

## Webhooks

**Where:** Admin > Integrations > Webhooks. URL `/admin/webhooks`. New: `/admin/webhooks/new`. Edit: `/admin/webhooks/:id/edit`
**Who can use it:** `webhooks:manage`, and the webhooks integration must be enabled for the workspace.
**Purpose:** Sends events from the desk to your own server, or posts them to a Slack or Teams channel through an incoming-webhook URL.
**Help panel:** "Configure webhooks to receive real-time notifications when events occur in your desk."

**List screen:**
- Buttons: **Add Slack**, **Add Microsoft Teams** (these open the form with that type chosen) and **New webhook**.
- Table columns: Name, URL, Events (count), Status (Active or Inactive).
- Row menu: **Edit**, **Enable** / **Disable**, **Send test** ("Webhook sent successfully"), **Delete** ("This action cannot be undone. This will permanently delete this webhook.").

**Form:**

| Field | What it does |
|---|---|
| **Active** (checkbox, edit only) | Turns the webhook on or off. |
| **Send to** (three cards) | **Custom endpoint**: "Your server receives each event as signed JSON." **Slack**: "Post events to a Slack channel as messages with a link back to the conversation." **Microsoft Teams**: "Post events to a Teams channel as cards with a link back to the conversation." |
| **Name** | Your own label. |
| **URL** | For Slack it must start with `https://hooks.slack.com/services/`. For Teams, use the URL from the channel's Workflows option "Post to a channel when a webhook request is received". Each type shows a matching hint. |
| **Events** (at least one) | **Conversation** group: Conversation created, Conversation status changed, Conversation tags changed, Conversation assigned, Conversation unassigned. **Message** group: Message created, Message updated. "Message updated" is hidden for Slack and Teams ("Message updates and activity notes aren't posted to chat, to keep the channel readable."). |
| **Secret** (Custom endpoint only) | "Optional secret key for webhook signature verification." |
| **Create** / **Save**, **Send test** (edit only) | |

**Technical details for developers (Custom endpoint):**
- Each event is sent as an HTTP POST with JSON `{event, timestamp, payload}`.
- Headers: `Content-Type: application/json`, `User-Agent: ConectDesk-Webhook/<version>`.
- If a secret is set: `X-ConectDesk-Signature: sha256=<HMAC-SHA256 of the body>`.
- Any 2xx response counts as success.
- The test event is `webhook.test`.
- The automation action "Trigger webhook" can also send custom event names to a chosen webhook.

## Slack (app integration)

**Where:** Admin > Integrations > Slack. URL `/admin/integrations/slack`
**Who can use it:** `webhooks:manage`, and the slack integration must be enabled.
**Purpose:** A two-way Slack app. Conversations are posted to channels, one thread per conversation. Buttons let agents act on them from inside Slack.
**Help panel:**
- "Post new conversations and their updates to Slack channels, each conversation in a thread of its own."
- "Buttons on each conversation let agents assign it to themselves, reply to the customer, add an internal note, or resolve it."
- "Anything an agent writes in a conversation's thread is added to it as an internal note."
- "Agents are matched to their Slack or Teams account by email, and can only do what their role allows in the desk."

**If the operator has not set up the Slack app:** the page says "The Slack app isn't set up on this installation yet." It lists the three addresses the operator needs (OAuth redirect URL, Event Subscriptions request URL, Interactivity request URL); the operator sets the app up in the `[slack]` section of the config.

**Otherwise:**
- **Add to Slack**: installs the app and lets you pick a channel inside Slack. Install again to add more channels.
- When you come back, a message confirms the result. If the channel is private: "...invite the app there with /invite before it can post". If something went wrong: cancelled, timed out, failed, or channel already connected to another workspace.

**Connected channels** (one card per channel):
- Channel name and Slack workspace.
- **Active / Inactive** toggle.
- **Post to this channel** checkboxes: New conversations; Status changes; Tag changes; Conversation assigned; Conversation unassigned; Messages, replies and notes.
- **Save** (needs at least one event checked).
- **Send test**: posts a test message ("Test message posted to {channel}.").
- **Disconnect**: asks for confirmation ("Nothing more is posted to this channel. The app stays installed in Slack until you remove it there.").

## Microsoft Teams (bot integration)

**Where:** Admin > Integrations > Microsoft Teams. URL `/admin/integrations/teams`
**Who can use it:** `webhooks:manage`, and the teams integration must be enabled.
**Purpose:** The same idea as Slack, for Teams channels.
**Help panel:**
- "Each conversation's card has a box to write in and buttons to send a reply, add a note, assign it to yourself or resolve it."
- "Mentioning the bot in a conversation's thread adds the message as an internal note."
- Plus the same email-matching note as Slack.

**If the operator has not set up the bot:** "The Teams bot isn't set up on this installation yet.", with the Bot messaging endpoint to configure.

**Otherwise, three steps:**
1. **Download the app**: the **Download app package** button downloads `conectdesk-teams.zip`. You only upload it once for your organization.
2. **Add it to a team**: in Teams, go to Apps > Manage your apps > Upload an app, and add it to the team that has the channel.
3. **Link the channel**: click **Get a link code**. It shows a message like `@<bot name> connect <code>` with a **Copy** button. The code works once, for 30 minutes. Send that message in the channel. The page checks every few seconds and shows "Teams channel connected." once it is linked.

Connected channel cards work the same as for Slack.

## Context links

**Where:** Admin > Integrations > Context links. URL `/admin/context-links`. New: `/admin/context-links/new`. Edit: `/admin/context-links/:id/edit`
**Who can use it:** `context_links:manage`, and the context_links integration must be enabled.
**Purpose:** Buttons in the agent's conversation sidebar that open external tools (CRM, billing, internal dashboards) with the contact's details passed along automatically.
**Help panel:** explains that `{{token}}` gives a fully encrypted payload, and that individual variables give plain links.

**List screen:**
- **New context link** button.
- Table columns: Name, URL template, Status, Created at.
- Row menu: **Edit**, **Enable** / **Disable**, **Delete**.

**Form:**

| Field | What it does |
|---|---|
| **Active** (edit only) | Turns the link on or off. |
| **Name** | Placeholder "My App". |
| **URL template** | E.g. `https://tools.example.com/lookup?token={{token}}`. `{{token}}` is an AES-256-GCM encrypted, base64 blob holding all contact and agent fields; it requires a secret. Plain variables: `{{email}}`, `{{phone}}`, `{{external_user_id}}`, `{{contact_id}}`, `{{first_name}}`, `{{last_name}}`, `{{conversation_uuid}}`. |
| **Secret** | Exactly 32 characters, shared with the external app. Only needed for `{{token}}`. |
| **Token expiry (seconds)** | Default 1200. Sent as `exp` inside the token. The external app should reject older tokens. |
| **Create** / **Save** | |
