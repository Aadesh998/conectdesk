# Calls, contacts, leads and search

Phone calling through Exotel (dialer, incoming calls, browser softphone, call-backs), the admissions Leads board, contacts and search.

## Dialer ("Make a call")

**Where:** Inbox sidebar → **Make a call**; Contacts → **Make a call**; or the link in the New conversation dialog.

**Who can use it:** Needs an enabled Phone inbox; the server checks `messages:write`.

**Purpose:** Call any number. Exotel first rings you (your phone or this browser), then the customer, who sees the line's caller ID.

| Control | What it does |
|---|---|
| **Number to call** | With country code. A 10-digit number is treated as Indian (+91). |
| **Name (Optional)** | Name for a new contact; hidden when calling a known contact. |
| **Caller ID** | Picker when there are several phone lines; otherwise just shown. "Set up a Phone inbox to call customers." if none. |
| "Rings your phone first: {number / this browser}" | Where you will be rung, or an error. |
| **Cancel** / **Call now** | Places the call and opens its conversation (in All). |

## Incoming calls and the browser softphone

**Where:** Bottom-right corner of every page.

- **Incoming call card:** the caller's name or number, the line and any IVR menu choice; the hint "Your phone is ringing…" or "Answer it in this browser."; **Open conversation**; ✕ dismisses.
- **Softphone card** (when "Take calls in this browser" is on in your profile): **Decline** / **Answer** while ringing; during a call **Mute/Unmute**, **Hold/Resume**, **Keypad** (0–9, *, # for phone menus), **Hang up**, and a timer.
- **Offline card:** "Browser calling is offline" with the reason (e.g. "Allow this site to use your microphone, then retry.") and **Retry**.

## Call-backs

**Where:** Inbox sidebar → **Call-backs** (`/call-backs`).

**Who can use it:** `conversations:read`. The link shows when a voice inbox exists or call-backs are pending.

**Purpose:** "Missed calls to return. Each closes by itself once a call with the caller connects."

| Control | What it does |
|---|---|
| **Only mine** | Only call-backs assigned to you. |
| **Waiting (N)** / **Done** | Switch between pending and completed call-backs. |
| Row information | Name and number; "Missed {n} times"; "Voicemail"; "Tried {n} times"; due text ("Due in {n} min" / "Due now" / "Overdue by …", red when overdue) · "missed {time} ago" · assignee, team or "Unassigned" · #reference. A WhatsApp follow-up status may also show ("WhatsApp message sent/delivered/read/failed…"), linking to that chat. |
| **Call back** | Calls the customer from that line (you are rung first). Disabled with a reason if your calling setup isn't ready. |
| **Open** | Opens the phone conversation. |
| **Mark done** | Closes the call-back by hand. |
| **Load more** | The next 50. |

Empty states: "No missed calls waiting. Nice work." / "No call-backs done yet." Done reasons: "Called back", "They called again and got through", "Marked done".

## Leads (admissions board)

**Where:** Inbox sidebar → **Leads** (`/leads`).

**Who can use it:** `contacts:read_all`. Moving cards needs `contacts:write`.

**Purpose:** "Where each enquiry is. Drag a card to move it." A Kanban board of contacts by lead stage. Stages are set under Admin → Admissions.

| Control | What it does |
|---|---|
| **Search name, email or number** | Filters the board. |
| **Edit stages** | Opens Admin → Admissions stage settings. Needs `general_settings:manage`. |
| Columns | One per stage, with a coloured dot (blue = open, green = won, grey = lost) and a count. |
| Card | Name (links to the contact), phone, details (course/class/grade/city/batch if set), "Active {time} ago" (links to the latest conversation), and a red phone icon when "A missed call is waiting to be returned". Clicking the card opens the latest conversation, or the contact if there's none. |
| Drag and drop | Moves a card to another stage; saved instantly, undone if saving fails. |
| **Move to** dropdown on each card | Changes stage without dragging (touch screens). |
| Trash (**Delete lead**) | Admin role only. Deletes the contact with all their conversations, after confirmation. |

"Nobody here" marks an empty column; "{n} more, search to find them" appears when a column has more than 100 cards. The board updates live when others move leads.

## Search

**Where:** The magnifier in the inbox sidebar (`/inboxes/search`).

**Who can use it:** Conversation results need `conversations:read`; message results need `messages:read`.

**Purpose:** "Search by reference number, contact email address or messages in conversations."

| Control | What it does |
|---|---|
| **Search** box | Searches automatically after 3+ characters ("Please enter at least 3 characters to search." otherwise). |
| **Conversations (n)** / **Messages (n)** tabs | Each result shows #reference, a status badge, the subject or message text and a date; click to open. |
| **Try again** | Shown on error. |

No results: "No results found for query `…`".

## Contacts list

**Where:** Icon rail → **Contacts** (`/contacts`; the sidebar calls it "All contacts").

**Who can use it:** `contacts:read_all`.

**Purpose:** Browse and find customers.

| Control | What it does |
|---|---|
| **Search by name, email or phone** | Matches 3+ characters against email, first name or part of the phone number. |
| Sort icon | Field ("Created at" / "Email") and direction ("Ascending" / "Descending"). |
| **Make a call** | Opens the Dialer (voice inbox only). |
| **Import** | Needs `contacts:write`; see below. |
| Contact card | Avatar, name, "Contact"/"Visitor" badge, email, phone, external ID. Click to open; **Call** calls the contact (voice inbox only). |
| Pagination | "Page X of Y", page size 15, 30, 50 or 100. |

Empty state: "No contacts found".

**Import dialog:** **Select CSV file**; the "Required CSV format" box lists the columns `first_name`\* (required), `last_name`, `phone_number`, `country_code`, `email`, `external_user_id`. Each row needs an email or a phone number. Existing contacts are updated, not duplicated. **Download a sample CSV** gives a template. **Import** / **Cancel**; progress shows under **Logs** (with a copy button), then **Close**.

## Contact detail

**Where:** Click a contact, or a customer's name in a conversation (`/contacts/{id}`).

**Who can use it:** `contacts:read`. The breadcrumb ("Contacts › Edit contact") shows only with `contacts:read_all`.

**Purpose:** View and edit one customer, and their notes.

| Control | What it does | Permission |
|---|---|---|
| Avatar (**Upload** on hover; red × removes) | Upload a PNG/JPG; a "Crop avatar" dialog opens (**Cancel** / **Save**). | — |
| Name, "Contact"/"Visitor" badge, "Blocked" badge | Identity and state. | — |
| **Call** | Calls the contact. | Voice inbox |
| Lead stage dropdown | Moves the contact on the Leads board ("Not a lead" / stages). Contacts only, not visitors. | `contacts:write` |
| **⋮ → Block** / **Unblock** | After confirming, blocks the contact: they can't chat, and email from their address is rejected (also for other contacts with that email). | `contacts:block` |
| **⋮ → Export data** | Downloads `contact-{id}-data.json` (for data-access requests). | `contacts:export` |
| **⋮ → Delete contact** | After confirming, permanently deletes the contact with all their conversations, messages, notes and attachments. | `contacts:delete` |
| External ID, "Created on {date}" | Information only. | — |
| Form: First name, Last name, Email, phone (with country code), Country | Editable fields. | — |
| **Update contact** | Saves the form. | `contacts:write` |
| **Notes** | **New note** / **Add note**, editor (<kbd>Ctrl</kbd>+<kbd>Enter</kbd> saves), **Save note** / **Cancel**, **⋮ → Delete note**. Empty: "No notes yet". | `contact_notes:read` / `write` / `delete` (own notes; Admin any) |
