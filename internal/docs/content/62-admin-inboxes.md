# Admin: inboxes and channels

An inbox is one channel connection: an email mailbox, a website chat widget, a WhatsApp number, a Facebook Page, an Instagram account, a Telegram bot or an Exotel phone line. Which channels a workspace may add is decided by the platform admin (Platform admin → workspace → Channels), and how many inboxes by its plan.

## Inboxes: list

**Where:** Admin → Channels → **Inboxes**. URL: `/admin/inboxes`.
**Who can use it:** `inboxes:manage`.
**Purpose:** Each inbox is one channel connection (an email mailbox, chat widget, WhatsApp number, Facebook Page, Instagram account, Telegram bot or phone line).
**Help panel:**
- Email: "Connect your Google or Microsoft email account, or configure IMAP and SMTP settings manually…"
- Live Chat: "Create live chat widgets that can be embedded on your website…"

| Element | What it does |
|---|---|
| **New inbox** | Opens the channel picker. |
| Columns | **Name** (click to edit), **Channel**, **Status** (Enabled/Disabled), **Created at**, **Updated at**. A ⚠ next to a WhatsApp inbox's name (amber or red, with a hover note) means Meta raised a warning about the number. |
| Row menu **Edit** | Opens the inbox settings. |
| Row menu **Disable** / **Enable** | Stops or restarts the inbox receiving and sending, without deleting it. |
| Row menu **Delete** | "This action cannot be undone. This will permanently delete this inbox." |
| Toasts after returning from Google, Microsoft or Facebook sign-in | Success: "Inbox connected successfully", "Inbox reconnected successfully", "Connected {count} account(s)." (plus how many were skipped). Errors include "Authorization denied", "An inbox with this email already exists. Use Reconnect…", "That account is already connected to another workspace…", "No accounts were shared…" and "Could not connect the account…". |

## New inbox (choose channel)

**Where:** Inboxes → **New inbox**. URL: `/admin/inboxes/new`.
**Who can use it:** `inboxes:manage`.
**Purpose:** Pick the type of channel to add.
- Breadcrumb: "Inboxes > New inbox".
- Heading **Choose channel** with one card per channel. Only the channels the platform enabled for the workspace are shown. If none are enabled: "No channels are enabled for your workspace yet. Contact support…"
- **← Back** returns to the cards.

| Card | Subtitle |
|---|---|
| **Email** | Create an email inbox for email-based customer support |
| **Live Chat** | Create a live chat inbox for real-time customer support |
| **WhatsApp** | Connect a WhatsApp Business number through Meta |
| **Messenger** | Connect a Facebook Page's Messenger |
| **Instagram** | Connect an Instagram professional account |
| **Telegram** | Connect a Telegram bot |
| **Phone** | Take and make phone calls through Exotel |

## Email inbox (new and edit)

**Where:** New inbox → **Email**, or Edit on an email inbox. URL: `/admin/inboxes/:id/edit`.
**Who can use it:** `inboxes:manage`.
**Purpose:** Receive customer email over IMAP and reply over SMTP.

**Step 1: Choose setup method** (new inboxes only), "Select how you want to connect your email account".

**Google** ("Connect with Google Workspace or Gmail") and **Microsoft** ("Connect with Microsoft 365 or Outlook") open a dialog titled **Connect {provider} account**:
- If the platform provides its own app: "You'll sign in with {provider} and allow access to the mailbox… Nothing else to set up." A link **Use your own {provider} app instead** switches to the next option.
- Using your own app:
  1. "Create OAuth app at" a link to **Google Cloud Console** or **Microsoft Azure Portal**.
  2. "Add this callback URL:" a read-only URL with a **Copy** button.
  3. "Enter your credentials below:" fields **Client ID** and **Client secret**. A link **Connect without an app of your own** switches back, when available.
- **Cancel** / **Continue** (shows "Connecting..."). Continue sends you to Google or Microsoft to sign in, then back to Inboxes.

**Other provider** ("Configure IMAP and SMTP manually") reveals the manual form.

**Main fields** (shown once a setup method is chosen, or when editing):

| Field | Meaning / use |
|---|---|
| **Name** | Internal inbox name. |
| **From email address** | e.g. "My inbox <support@example.com>". The From address for replies. |
| **From display name template (optional)** | Display name on outgoing replies. Variables: `{{ .Agent.FirstName }}`, `{{ .Agent.LastName }}`, `{{ .Agent.FullName }}`, `{{ .Inbox.Name }}`, e.g. "{{ .Agent.FirstName }} at {{ .Inbox.Name }}". Leave blank to use the From name. |
| **Reply-To address (optional)** | Where customer replies go. "Only needed when SMTP From and IMAP mailbox are on different domains." |
| **Enabled** | "Toggle scanning inbox and sending out messages." |
| **CSAT Surveys** | Sends a satisfaction survey when a conversation is resolved, once per conversation. Tip: turn this off and use an automation rule if you want more control over timing. |
| **Enable plus addressing** (on by default) | "Improves conversation threading but requires provider support (e.g., Gmail, Microsoft 365)." On Microsoft OAuth inboxes it is locked on: "Required for Microsoft inboxes to thread replies correctly." |
| **Prompt to tag before replying** | Warns agents before they reply on an untagged conversation. They can still confirm and send. |

**Connected via OAuth** (OAuth inboxes only)
- A green box shows "Connected via OAuth - {provider}", the mailbox email, and a shortened Client ID.
- **Reconnect** reopens the dialog titled "Reconnect {provider} account" ("Re-enter your credentials to refresh the OAuth connection"). The client secret must be typed again.
- **IMAP Configuration:** Host, Port, TLS (Off / SSL/TLS / STARTTLS), Mailbox, Scan Interval, Scan Inbox Since.
- **SMTP Configuration:** Host, Port, TLS, Max Connections, Max Retries, Idle Timeout, Wait Timeout.

**Manual IMAP Configuration**

| Field | Default | Meaning |
|---|---|---|
| Host | imap.gmail.com | IMAP server. |
| Port | 993 | |
| Mailbox | INBOX | The folder to scan. Usually no need to change it. |
| Username / Password | | Mailbox login. When editing, a stored password appears as dots; leave it unchanged to keep it. |
| TLS | Off | "Choose the encryption method for IMAP." Options: Off, SSL/TLS, STARTTLS. |
| Scan Interval | 5m | How often to check for new mail. Format: 120s, 1m, 1h. |
| Scan Inbox Since | 48h | Only scans mail received in this recent window, for speed. |
| Skip TLS Verification | off | Skips the hostname check on the server's certificate. |

**Manual SMTP Configuration**

| Field | Default | Meaning |
|---|---|---|
| Host / Port | smtp.gmail.com / 587 | Sending server. |
| Username / Password | | SMTP login. |
| Max Connections | 10 | Maximum concurrent connections. |
| Max Retries | 3 | Retries when a message fails. |
| Idle Timeout | 25s | How long an unused connection is kept open. |
| Wait Timeout | 120s (placeholder shows 60s) | How long to wait for a free connection before timing out. |
| Authentication Protocol | Login | Options: Login, CRAM, Plain, None. |
| TLS | Off | Options: Off, SSL/TLS, STARTTLS. |
| HELO Hostname | (blank means localhost) | The hostname sent in the HELO/EHLO command. |
| Skip TLS Verification | off | |

**Create** (new) or **Save** (edit).

## Live Chat inbox (chat widget)

**Where:** New inbox → **Live Chat**, or Edit on a live-chat inbox. On the edit page the top right shows the inbox **UUID** with a copy button.
**Who can use it:** `inboxes:manage`.
**Purpose:** Configure the chat widget customers see on your website. While you edit, the right-hand column shows a live **Preview** of the widget.

There are eight tabs. One **Create** or **Save** button at the bottom saves everything. If a field fails validation, the form jumps to the tab holding the error.

**Tab "General"**
- **Enabled**
- **CSAT Surveys**
- **Prompt to tag before replying**
- **AI assistant**: choose **Automatic — your workspace's AI assistant** (the default), **Off — new conversations follow your automations**, or a specific assistant. New conversations go to that assistant, which answers them and hands over to a person when needed.
- **Name**
- **Brand name**
- **Website URL**: where the widget is installed. Used in continuity emails to link back to the chat.
- **Language**: **Auto-detect (browser language)** or a fixed language. If you pick Auto-detect, a **Fallback language** field appears for visitors whose browser language isn't available.
- **Conversation continuity email inbox**: **None** or an email inbox. When a visitor goes offline, replies are emailed to them through this inbox, and they can answer by email or come back to the chat. Choosing an inbox reveals:
  - **Offline threshold** (default 10m, minimum 1m): how long after going offline before email is used.
  - **Max messages per email** (default 10, 1–100).
  - **Min email interval** (default 15m, minimum 1m).

**Tab "Appearance"**
- **Logo URL**
- **Dark mode**
- **Show powered by**: shows "Powered by ConectDesk" when a conversation is open. On by default.
- **Colors** → **Primary color** (default #2563eb). A warning appears if it is too close to the background.
- **Home screen**:
  - **Header text color**: Black or White, with a contrast warning if it is hard to read.
  - **Background**: Solid (**Background color**), Gradient (**Gradient start** / **Gradient end**) or Image (**Background image URL**).
  - **Fade background**: "Blend the header background into the content area below".
- **Home screen apps** (drag to reorder; X removes one):
  - **Add announcement**: Title, Description, Cover image URL, Link URL.
  - **Add external link**: link text and URL.
  - An incomplete app shows "Fill in or remove the incomplete home screen app."
- **Launcher**:
  - **Position**: Left or Right (default Right).
  - **Launcher logo**.
  - **Launcher color** (default #000000).
  - **Side spacing** / **Bottom spacing**: distance from the screen edge in pixels, default 20, range 0–200.

**Tab "Messages"**
- **Greeting message** (default "Hello {{.FirstName | there}}"). Variables: `{{.FirstName}}`, `{{.LastName}}`. Use `|` for a fallback.
- **Introduction message** (default "How can we help?").
- **Chat introduction**.
- **Notice banner** → **Enable notice banner**. Turning it on reveals **Notice banner text**. Typical use: "Our response times are slower than usual."

**Tab "Features"**
- **Office hours**:
  - **Show office hours in chat**: "Show when the team will be next available". Turning it on reveals **Chat reply expectation message** (default "We typically reply in 5 minutes."), shown during business hours.
  - **Show office hours after team assignment**.
- **Features**:
  - **File upload** (on by default).
  - **Emoji support** (on by default).
  - **Launch directly into conversation**: skips the home screen and opens the visitor's latest conversation, or a new one.

**Tab "Pre-chat form"**
- **Enable pre-chat form**: "Show a form to collect information before chat starts". Turning it on reveals:
- **Form title**.
- **Form fields** (drag to reorder). The built-in fields are **Full name** (on, required), **Email address** (on, required) and **Phone number** (off). Each field has:
  - An on/off switch.
  - **Label** and **Placeholder** boxes (when the field is on).
  - A **Required** checkbox (not offered for checkbox-type fields).
  - A delete button, for custom fields only.
- **Available custom attributes**: click any contact or conversation custom attribute to add it as a field.

**Tab "Users"** has two sub-tabs. **Visitors** means anonymous people; **Users** means logged-in people verified with a JWT. Each has:
- **Start conversation button text** (default "Start conversation").
- **Allow start conversation**.
- **Prevent multiple conversations**: no starting a second conversation while one is ongoing.
- **Prevent replying to closed conversations**.

**Tab "Security"**
- **Secret key**: used to sign the identity-verification JWT. A warning appears if it is shorter than 32 random characters.
- **Authenticated user session duration** (default 10h, minimum 1h; examples 24h, 168h). Sessions extend automatically while the user is active.
- **Domain list**: the trusted domains allowed to host the widget, one per line. `*.example.com` matches subdomains only, so add `example.com` separately. Empty means any domain.
- **IP block list**: IPs or CIDR ranges denied access to the widget, one per line.

**Tab "Installation"**
- **Installation instructions**: copy the snippet and paste it just before the closing `body` tag on every page where the widget should appear. The snippet has a copy button.
- **Identity verification**: your server signs a JWT with the Secret key using HS256. The tab shows an example payload and an example snippet that includes the JWT (both copyable). Warning: never put the secret key in client-side code.
- **JavaScript API**: copyable examples of `ConectDesk.show()`, `hide()`, `toggle()`, `setUser(jwt)`, `logout()`, `onShow`, `onHide` and `onUnreadCountChange`.

## WhatsApp, Messenger and Instagram: connect (new)

**Where:** New inbox → **WhatsApp**, **Messenger** or **Instagram**.
**Who can use it:** `inboxes:manage`.
**Purpose:** Connect accounts through Meta.

Each screen shows a per-channel explanation and the reminder "WhatsApp, Messenger and Instagram only allow replies within 24 hours of the customer's last message."

**Continue with Facebook**
- Redirects to Facebook sign-in.
- WhatsApp: choose or create the WhatsApp Business Account and verify a number.
- Messenger: choose Pages. Each Page becomes an inbox.
- Instagram: choose the Pages your Instagram professional accounts are linked to. Each account becomes an inbox.
- You return to Inboxes with a result toast.
- If the platform's Meta app isn't configured: "{channel} isn't set up on this installation yet…" (for Messenger and Instagram the button is disabled).

**WhatsApp only: "or connect manually" → Connect with your own Meta app**
- For numbers already on the Cloud API through your own Meta app.
- The screen explains where to find each ID in developers.facebook.com and Meta Business Settings.
- Fields:
  - **Name (Optional)**
  - **WhatsApp Business Account ID**
  - **Phone number ID**
  - **App ID**
  - **App secret**
  - **Permanent access token**: a system-user token. Temporary tokens stop after 24 hours.
  - **Two-step verification PIN (Optional)**: only if the number isn't on the Cloud API yet and already has a PIN.
- "Encrypted in your browser before it's sent, and stored encrypted." This needs HTTPS.
- **Connect number**.
- If Meta can't be pointed at the inbox automatically, the screen shows a **Webhook** block instead: **Callback URL** and **Verify token**, each with **Copy**, plus "Then subscribe to the messages webhook field." Click **Done** when finished.

## WhatsApp, Messenger, Instagram and Telegram inbox (edit)

**Where:** Click the inbox name, or row menu → **Edit**.
**Who can use it:** `inboxes:manage`.
**Purpose:** View the connected account, renew access and set inbox options.

| Section / element | What it does |
|---|---|
| **Connected account** | WhatsApp: the number, verified name and WhatsApp Business Account ID. Messenger: the **Facebook Page**. Instagram: @username and Page. Telegram: **Bot** @username and **Telegram Business** status. Red notes appear for "Disconnected…" or a number registration error. |
| **Reconnect with Facebook** (Facebook-connected inboxes) | Renews access, e.g. after the account was disconnected on Meta. |
| **Update credentials** (manually connected WhatsApp) | Shows the webhook details and re-opens the manual form ending in **Update credentials**. |
| Telegram connection status | "Receiving messages" or "Not receiving messages: the bot's webhook points elsewhere." Also shows the pending update count and the last delivery error. **Reconnect webhook** re-registers it. |
| **Number health** (WhatsApp) | **Quality** (High / Medium / Low / Not rated yet), **Messaging limit**, **Status**, **Display name** (with review status), and the latest alert from Meta. **Refresh** re-reads them. |
| **Message templates** (WhatsApp) | The list of templates with name, language, category and status, plus the rejection reason where there is one. **Refresh** reloads the list. **Delete** asks in a browser prompt: "Delete the template {name}? It can't be sent any more, and its name can't be reused for 30 days." |
| **New template** dialog (WhatsApp) | See the table below. |
| **Import past conversations** (Messenger and Instagram) | Brings in recent history with original times. Nothing is sent and no automations run. Fields: **Conversations** (1–500, default 50) and **Messages per conversation** (1–200, default 50). Checkbox **File imported conversations as resolved** (ticked by default). **Import** shows "Importing… {done} conversation(s) so far." |
| **Name** | Inbox name. |
| **AI assistant** | Automatic, Off, or a specific assistant (same as Live Chat). |
| **Max media size (MB)** (default 20) | Larger incoming media is skipped and replaced by a placeholder. |
| **Include group chats** (Telegram only) | Opens a conversation for each group the bot is in. The bot only sees mentions unless privacy mode is turned off in @BotFather (`/setprivacy`). |
| **Send read receipts** (WhatsApp only, on by default) | Shows blue ticks on WhatsApp once the message is received. |
| **Enabled**, **CSAT Surveys**, **Prompt to tag before replying** | Same as for email inboxes. |
| **Save** | |

**New template dialog fields (WhatsApp)**

| Field | Details |
|---|---|
| **Name** | Lowercase letters, numbers and underscores only; other characters are converted automatically. |
| **Category** | **Utility** (updates about an order or account) or **Marketing** (offers and announcements). |
| **Language** | |
| **Header** | None, Text, Image, Video or Document. A Text header takes up to 60 characters plus a sample value for a variable. A file header needs a sample file for Meta's review. |
| **Message** | The body, up to 1024 characters. **Add variable** inserts {{n}}; each variable needs a sample value. |
| **Footer (Optional)** | Up to 60 characters. |
| **Buttons (Optional)** | **Add button**: Quick reply, Link (URL, optionally with {{1}} and a sample) or Call (phone number). Each label is up to 25 characters. |
| **Preview** | Shows the message the way WhatsApp will, with your sample values, plus Meta's rejection rules. |
| **Submit for review** | Sends the template to Meta for approval. |

## Telegram inbox (connect)

**Where:** New inbox → **Telegram**.
**Who can use it:** `inboxes:manage`.
**Purpose:** People who message your bot become conversations, and agents reply as the bot.
- Steps shown on screen: message @BotFather, send /newbot, copy the token.
- **Bot token** (stored encrypted).
- **Name (Optional)**.
- A note about Telegram's limits: the customer must write first, and Telegram has no read receipts.
- **Connect bot** creates the inbox and returns to the list. Errors include an invalid token or a bot already connected here or in another workspace.

## Phone inbox (Exotel voice)

**Where:** New inbox → **Phone**, or Edit on a phone inbox.
**Who can use it:** `inboxes:manage`.
**Purpose:** Calls to your Exotel number become conversations, and agents can call customers back. Exotel rings the agent's own phone first, then connects the customer.

| Field | Default / rules | Meaning |
|---|---|---|
| **Name** | placeholder "Support line" | |
| **Exotel** section: **Account SID**, **Region**, **API key**, **API token** | Region options: India (Mumbai) (default) or Singapore | Found in Exotel under Settings > API Settings. When editing, leave the token blank to keep the stored one. |
| **ExoPhone** | must include the country code | Your Exotel number. It is also the caller ID on calls you make. |
| **Test connection** (edit only) | | "Exotel accepted the credentials." or the error. |
| **Record calls** | on | Recordings are attached to the call. Tell callers they are recorded. |
| **Ring online agents** | on | Rings every online agent who has a phone number or browser calling. |
| **Fallback numbers** | comma-separated | Rung when no agent is online. |
| **Ring for (seconds)** | 30 (10–120) | How long phones ring before voicemail. |
| **Enabled** | on | |
| **Phone menu**: **Add option** (up to 10), each with a key 0–9, a team, a label ("e.g. Sales") and ✕ to remove | | Routes "press 1 for sales" to a team. Only that team's online agents ring, and the conversation is assigned to it. |
| **Missed calls** → **Create call-backs** | on | Each missed caller becomes a call-back on the Call-backs page. It closes by itself when a call with them connects. |
| **Minutes to call back** | 15 (1–1440) | After this time the call-back is overdue and people who see reports are told. |
| **Send the caller a WhatsApp message from** | "Don't send a WhatsApp message" or a WhatsApp inbox | Sent at most once a day per caller. |
| **Template**, **Template outside business hours** (optional), each with **Create the suggested template** | | Approved templates. Values can use `{{name}}`, `{{phone}}` and `{{business}}`. The second is sent when business hours are closed. |
| **Set up incoming calls in Exotel** (after saving) | | Step-by-step Exotel App Bazaar flow instructions with copyable Passthru, Dial, Completed and Voicemail URLs. "These addresses contain this inbox's secret key. Don't share them." New inboxes show "Once the inbox is saved, this page shows the addresses…". |
| **Create** / **Save** | | |
