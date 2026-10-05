# Conversations

Everything agents do with customer conversations: the conversation list, bulk actions, the conversation screen and reply box, the right-hand Details and Juno panels, starting new conversations, personal views and SLA indicators.

## Conversation list

**Where:** The left panel of every inbox, team and view page:

- `/inboxes/assigned`, `/inboxes/mentioned`, `/inboxes/unassigned`, `/inboxes/all`
- `/inboxes/teams/{teamID}`
- `/inboxes/views/{viewID}`

**Who can use it:** As listed for each list in the inbox sidebar (see Finding your way around).

**Purpose:** Browse, filter, sort and multi-select conversations. On desktop the list sits beside the open conversation; the divider is draggable (20–45% width) and remembered. On mobile the list and the conversation are separate screens.

**Header:** the sidebar toggle and the list title ("My inbox", "Mentions", "Unassigned", "All", "Team Inbox", "View").

| Control | What it does | Typical use |
|---|---|---|
| Status dropdown, e.g. "12 Open ▾" | Picks which status to list. The options are your workspace's statuses (by default Open, Snoozed, Resolved, Closed; Open is preselected). Hidden on Views, which have their own filters and show only the count. | Look at resolved or snoozed work. |
| Sort dropdown | "Newest activity" (default), "Oldest activity", "Started first", "Started last", "Waiting longest", "Next SLA target", "Priority first". | Triage by SLA or by how long customers have waited. |
| **Load more** ("Loading..." while working) | Loads the next page. "All conversations loaded" appears at the end. | — |
| Empty / error states | "No conversations found / Try adjusting filters"; "Could not fetch conversations". | — |

**Auto-refresh:** the list refreshes every 2 minutes while the tab is visible, and at once when you return to the tab. Changes also arrive live.

### Each conversation row

- **Avatar.** On hover it becomes a checkbox ("Select conversation") if you have any bulk permission.
- **Contact name.** Bold when unread; a tooltip shows the full name.
- **Channel icon.** The tooltip shows the inbox name.
- **Time** since the last message.
- **Subject**, unless an admin turned subjects off.
- **Preview line:** the last message (with a green reply arrow if an agent sent it), "Typing..." while the customer types, or "Draft: …" when you have an unsent draft.
- **Unread count** bubble, up to "9+".
- **SLA badges:** FRD (first response deadline), RD (resolution deadline), NRD (next response deadline). Shown only while running (clock + time left) or overdue (red alert icon).
- **Clicking a row** opens the conversation. From Mentions it scrolls to the mentioning message.

**Right-click (or long-press on touch):**

| Item | What it does | Shown when |
|---|---|---|
| **Select conversation** | Starts multi-selection (the only way to start it on touch screens). | You have a bulk permission and nothing is selected yet. |
| **Mark as unread** | Sets the conversation back to unread. | Always |

Shift+click a checkbox to select a whole range. Once anything is selected, every row shows its checkbox.

## Bulk actions

**Where:** Replaces the status and sort bar as soon as one or more conversations are selected.

**Who can use it:** Anyone with at least one of `conversations:update_user_assignee`, `conversations:update_team_assignee`, `conversations:update_status` or `conversations:update_tags`. Each button needs its own permission.

| Control | What it does |
|---|---|
| Header checkbox (**Select all conversations**) | Selects or clears every loaded row. |
| "N selected" | How many rows are selected. |
| Person+ icon (**Assign agent**) | Searchable agent picker; "None" removes the agent. Needs `update_user_assignee`. |
| People icon (**Assign team**) | Team picker; "None" removes the team. Needs `update_team_assignee`. |
| Tag icon (**Add tags**) | Adds one tag to every selected row, keeping existing tags. Needs `update_tags`. |
| Circle icon (**Set status**) | Sets a status on every selected row (Snoozed isn't offered). Needs `update_status`. |
| X (**Clear selection**) | Clears the selection. |

Result: "Conversations updated", or "Some conversations couldn't be updated".

## When no conversation is open

The right side normally reads "Select a conversation from the left panel." If the workspace has no inboxes or no agents yet, it shows **Complete your setup** instead, with **Create your first inbox → Set up** and **Invite teammates → Invite**, both of which open Admin pages.

## Conversation screen

**Where:** Click a conversation. Addresses:

- `/inboxes/{type}/conversation/{uuid}`
- `/inboxes/teams/{id}/conversation/{uuid}`
- `/inboxes/views/{id}/conversation/{uuid}`

**Who can use it:** `conversations:read`; the messages need `messages:read`.

**Purpose:** Read the thread, reply, add internal notes, and change status, assignment and other properties. The right-hand sidebar is resizable and collapsible (remembered); when collapsed, a "‹" tab on the right edge reopens it. On mobile it opens as a slide-in sheet.

### Header

| Control | What it does | Shown when / permission |
|---|---|---|
| "‹" (**Back**) | Returns to the list. | Mobile |
| Contact name | — | — |
| Phone icon (**Call**) | Calls the contact (see Calls). | A voice inbox exists |
| Panel icon (**Contact**) | Opens the contact/info sheet. | Mobile |
| Clock + time ("Snoozed until {time}") | When a snoozed conversation wakes up. | Snoozed |
| **Resolve** | One click sets the status to Resolved. | Status isn't Resolved/Closed; `conversations:update_status` |
| Status pill (e.g. "Open") | Dropdown of every status. Picking "Snoozed" opens the snooze picker in the command palette. | `update_status` |
| **…** → **Close chat** | Closes the conversation panel and returns to the list. **It does not change the status** (don't confuse it with the "Closed" status). | Always |
| **…** → **Download transcript** | Downloads `transcript-{ref}.txt`. | `messages:read` |
| **…** → **Summarize with AI** | AI writes a summary and adds it as a private note ("Summary added to the conversation as a private note."). | `messages:write` |
| **…** → **Reset WhatsApp session** (red) | After confirming, resolves the customer's open conversations on that number so their next message starts fresh and the inbox's AI assistant picks it up. | Admin role, WhatsApp conversation |

### Message thread

- **Load more** at the top loads older messages; day separators divide the dates; a **jump to bottom** button with a count of new messages appears when you have scrolled up; a typing indicator shows while the customer types.
- **Customer messages** (left): the name and avatar link to the contact page.
- **Agent and AI messages** (right): the name and avatar link to the agent's admin page (with `users:manage`) or the AI assistant's page (with `ai:manage`).
- **Private notes** have a different background and a lock icon.
- **Email messages** show a header with Subject / From / To / Cc / Bcc, and **Show quoted text** / **Hide quoted text** for quoted replies.
- **Long messages** (over 400px) are collapsed behind **Expand**.
- **Attachments:** images open in a lightbox (<kbd>←</kbd>/<kbd>→</kbd> browse, <kbd>Esc</kbd> closes); audio plays in a popup; other files open in a new tab. Hover shows name, size and a download icon.
- **Delivery ticks** (WhatsApp, Messenger, Instagram): one tick "Sent", double tick "Delivered", blue double tick "Read", red alert "Not delivered" with the error. Other channels show a single green tick once sent.
- **Envelope icon** ("Sent via email"): a chat reply that was also emailed to the customer.
- **↻ retry** re-sends your own failed message.
- Timestamps show the full date and time on hover. Activity lines (e.g. assignment changes) are centred grey text.
- **Hover … on a private note → Delete note** ("This permanently deletes the note. It can't be recovered."). Allowed for the note's author or an Admin.
- **CSAT messages** show "Awaiting Response", or the rating (emoji, label, "x/5") and feedback.
- **Call cards** (voice): "Incoming call", "Calling…", "Call ended", "Missed call", "No answer", "Busy", "Call failed" or "Voicemail"; duration, number, agent, IVR menu choice, "Transferred by {name}"; a recording player ("The recording couldn't be saved." if it failed); **Summarise with AI** (`messages:write`) produces "AI summary" bullets and a "Transcript" expander, with **Try again** on failure.
- **"Assign to me" nudge:** after you reply to a conversation without an agent, a toast says "This conversation isn't assigned to anyone." **Assign to me** assigns it to you (`update_user_assignee`); X dismisses it.

### AI handling banner

Above the reply box when an AI assistant is assigned: "{name} (AI) is handling this conversation · replying takes it over", with **Take over**, which assigns the conversation to you. Sending a reply also takes over. Afterwards: "You took this conversation over from {name}…", with **Hand back to AI**.

### Reply box

**Who can use it:** Sending needs `messages:write`.

| Control | What it does | Typical use |
|---|---|---|
| **Reply** / **Private note** tabs | A reply goes to the customer. A private note stays internal (yellow background) and supports @mentions. | — |
| Expand ⤢ / Minimize | Full-screen editor; <kbd>Esc</kbd> closes it. | Long emails. |
| **TO:** / **CC:** (email, Reply tab) | Comma-separated addresses, pre-filled from the conversation. Invalid ones: "Invalid email(s) in 'to': …". | — |
| **BCC** / **Remove BCC** | Shows, or hides and clears, the BCC field. | — |
| Editor | "Shift + Enter to add a new line. Ctrl + Enter to send. Ctrl + K to open command bar." Paste or drop files to attach them; email inboxes also take pasted inline images. | — |
| Formatting bubble (select text) | **AI ▾** (rewrite prompts an admin set up; the whole draft is replaced with the result), **Bold**, **Italic**, **Strikethrough**, **Underline**, **Bullet list**, **Numbered list**, **Link** ("Add link URL" / "Edit link URL", **Remove link**, **Save**). | Polishing a reply. |
| @mention (Private note only) | Type "@" to pick agents or teams; they are notified and the conversation shows in their Mentions. | Asking a colleague. |
| Macro chips | The actions of the applied macro (assign, tag, status…); each chip's × drops that action before sending. | — |
| Attachment chips | File name and size, with × (**Remove attachment**). | — |
| Paperclip | Attach files (several at once). | — |
| Smiley | Emoji picker. | — |
| Sparkles (**Generate reply**, not on Private note) | AI drafts a reply from the conversation; anything you typed is used as an instruction. The box glows while it works. | A quick first draft. |
| **Send** | Sends (also <kbd>Ctrl</kbd>+<kbd>Enter</kbd>). Disabled until there is text, an attachment or a macro action, and the addresses are valid. | — |
| **▾** next to Send → **Send and set as** [status] | Sends, then sets that status (e.g. Resolved). Snoozed isn't offered. | Reply and resolve in one step. |

**Warnings before sending:**

- **No tags**, when the inbox requires tags: "No tags on this conversation… You can still send without tagging." → **Cancel** / **Send anyway**.
- **Contact not in recipients** (email): "Contact email not in recipients – The contact's email ({email}) is not included in to, cc, or bcc…" → **Cancel** / **Send anyway**.
- **Empty To:** "At least one recipient is required in the To field."

**Drafts** are saved automatically per conversation and per tab, including attachments and macro state, and restored when you come back. On small screens a single **Reply** / **Private note** button shows the draft and attachment count and opens the full-screen editor.

### Channel-specific reply bars

**Phone (voice) conversation**

- Notice: "Phone conversation. Call the customer back, or add a note." The box is forced to Private note.
- **Transfer to…** agent picker + **Transfer**: gives the call to another agent and reassigns the conversation ("Transferred. Exotel is calling the customer from the other agent's phone.").
- **Call** button.

**WhatsApp**

- Inside the 24-hour window: "Free-form replies are allowed until {time}." with a small **Send template** button.
- After 24 hours: "It's been more than 24 hours since the customer's last message, so WhatsApp only allows a template." with a prominent **Send template** button.
- **Send template** dialog: **Template** picker ("Choose an approved template"; unsupported ones greyed out, "can't be sent from here yet"), **Refresh**, a header file upload if the template has a media header, one field per variable (Header / Message / Button link), a live preview, and **Send**. With no templates: "This WhatsApp Business Account has no approved templates…"

**Telegram** (Reply tab)

- **+ Button** adds up to 10 buttons under the next reply: **Button label** and an optional **Link (optional), https://…**, then **Add**; ✕ removes one. A button without a link is a quick reply the customer taps.

## Right sidebar: Details tab

**Where:** The right panel of the conversation, which has two tabs, **Details** and **Juno**; the last-used tab is remembered.

### Contact header

- **Avatar**; live-chat contacts also get an online/offline dot.
- **Name**, linking to the contact page (`contacts:read`).
- **Panel icon** collapses the sidebar.
- **Email** ("Not available" if empty). Live chat adds a shield: "Identity verified" / "Identity not verified".
- **Phone number** + **Call**. With several voice inboxes, a picker chooses the line. Disabled with a reason such as "Add a phone number to this contact to call them."
- **Lead stage** dropdown: "Not a lead" or a stage. Changing it moves the contact on the Leads board; the tooltip shows "On this stage for {time}". Needs `contacts:write`.
- **External ID**, if any.
- **Live chat only:** country, IP address, browser/device.
- **Context links:** external apps an admin configured (Admin → Context links); clicking one opens it in a new tab with this conversation's details.

### Sections (open/closed state is remembered)

| Section | Contents and controls | Permission |
|---|---|---|
| **Actions** (open by default) | Agent picker ("Select agent", "None" unassigns), team picker ("Select team"), priority picker ("Select priority"), tags ("Select tags"). **Suggest tags** asks AI for tags, shown as "+ tag" chips to click ("No tag suggestions for this conversation." if none). | `update_user_assignee`, `update_team_assignee`, `update_priority`, `update_tags` |
| **Information** | Inbox, Subject, Reference number, Initiated at, First reply at, Resolved at, Last reply at (each with an SLA badge: "SLA met", "Overdue by …" or time left), Closed at, SLA policy, conversation custom attributes (editable), CSAT rating and CSAT feedback (**Show more** / **Show less**). | — |
| **Contact attributes** | The contact's custom fields. Hover a field: pencil to edit (text, number, date, link, list, checkbox; ✓ or <kbd>Enter</kbd> saves, X cancels), trash to clear. **Fill with AI** fills empty fields from the conversation ("Filled in: …" / "Nothing new to fill in from this conversation."). | Shown only if contact attributes exist; Fill with AI needs `contacts:write` |
| **Last visited pages** | The visitor's recent pages with times; click to open. | Live chat only |
| **Notes** | Contact notes: **New note**, editor, **Save note** / **Cancel**, **⋮ → Delete note**. Newest 10, then **Load more (N)**. | `contact_notes:read` / `:write` / `:delete` (own notes; Admin any) |
| **Previous conversations** | The contact's other conversations (subject, last message, times); click to open. "No previous conversations" if none. | — |

## Juno (AI copilot)

**Where:** The **Juno** tab in the conversation's right sidebar.

**Who can use it:** Any logged-in agent.

**Purpose:** A private AI chat about the open conversation that can search your knowledge base (help center articles and AI knowledge sources). The chat is saved per conversation and per agent; customers never see it.

| Control | What it does | Typical use |
|---|---|---|
| Persona picker (top-left, default "Juno") | Borrow the tone and instructions of one of your AI assistants. Remembered on this device; shown only if assistants exist. | Answer in the brand's voice. |
| **Clear chat** | Erases this conversation's Juno history. | — |
| Panel icon (**Close sidebar**) | Collapses the sidebar. | — |
| Presets: **Summarize this conversation**, **What is the customer asking?** | One-click questions, shown on an empty chat ("Ask Juno anything. It can search your knowledge base to answer."). | Catching up on a long thread. |
| **Ask anything…** | <kbd>Enter</kbd> sends; <kbd>Shift</kbd>+<kbd>Enter</kbd> adds a line. | — |
| **Copy** (under each answer) | Copies the answer ("Copied to clipboard"). | — |
| **Insert into reply** | Replaces the reply draft with the answer and switches to the Reply tab. | Use Juno's answer as the reply. |
| **Add as private note** | Posts the answer as an internal note ("Added as a private note."). | Share findings with the team. |

**Quirk:** the Actions pickers, Juno, Generate reply and Suggest tags have no on-screen permission check; users without rights see an error when they try them.

## New conversation

**Where:** Inbox sidebar → **+ New conversation**.

**Who can use it:** `conversations:write`.

**Purpose:** Start an outbound conversation by email, WhatsApp template or phone call. Only enabled Email, WhatsApp and Voice inboxes are offered.

| Control | What it does |
|---|---|
| "Just want to call a number? **Make a call**" | Opens the Dialer instead (voice inbox only). |
| **Email** ("Search contact by email or type new email") | Searches contacts as you type (<kbd>↑</kbd><kbd>↓</kbd>, <kbd>Enter</kbd>). Picking an existing contact locks the name fields; a new email creates a contact. |
| **WhatsApp number** / **Phone number** | Replaces Email for WhatsApp or Phone inboxes (e.g. "+91 99999 99999"). |
| **First name** / **Last name** | First name is required. |
| **Subject** | Not shown for phone or WhatsApp. |
| **Inbox** ("Select inbox") | Decides the channel; the form adapts. |
| **Assign team (Optional)** / **Assign agent (Optional)** | Initial assignment. |
| Message editor with attachments and emoji | Email only; inline images and macros work. |
| WhatsApp template picker | WhatsApp only: "Choose a template: a WhatsApp conversation can only start with one." |
| Voice notice | Your phone (or this browser) rings first. Without a phone number on your profile: "Add your phone number on your profile…" with a **Profile** link. |
| **Submit** / **Call now** | Creates the conversation; for phone inboxes, places the call. |

## Personal views

**Where:** Inbox sidebar → **Views** → **+** (create), or a view's **⋮ → Edit**.

**Who can use it:** `view:manage`. Views shared with everyone or a team are made under Admin → Shared views.

**Purpose:** "Create and save custom filter views for quick access to your conversations."

| Control | What it does |
|---|---|
| **Name** | "Enter an unique name for your view." 2–140 characters. |
| **Filters** builder | One or more condition groups. **Add group** / **Remove group**; the **AND/OR** toggle ("Click to switch between and / or") links groups and rows. **Add condition** adds a row: field → operator → value. |
| Fields | Status, Priority, Assign team, Assign agent, Inbox, Tags, Created at, Waiting since, Snoozed until, Last message at, Last interaction at, Next SLA deadline, Contact email, Contact external ID, Last interaction by (Contact/Agent), SLA policy, Channel (Email, Live chat, WhatsApp, Messenger, Instagram, Telegram, Voice), plus contact custom attributes. |
| Operators | equals, not equals, set, not set, contains any of, contains none of, greater than, less than, between, starts with — depending on the field. |
| **Save** | Saves the view ("Changes saved"). At least one filter is required. |

## SLA indicators

- **List rows:** FRD / RD / NRD badges while running (amber clock + time left) or overdue (red).
- **Details → Information:** next to First reply at, Resolved at and Last reply at: "SLA met" (green), "Overdue by {time}" (red) or time remaining (amber).
- **Sorting:** the list's **Next SLA target** sort puts the nearest deadlines first.
- SLA policies themselves are set under Admin → SLA policies and applied by automation rules.
