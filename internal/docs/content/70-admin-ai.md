# Admin: AI

The AI group in the Admin sidebar (Providers, Snippets, Suggestions, Tools, Assistants). Every AI screen needs the **`ai:manage`** permission; `/admin/ai` opens Providers. AI requests count against the plan's monthly AI limit (Admin → Usage).

## AI Providers

**Where:** Admin > AI > Providers. URL `/admin/ai/providers`
**Who can use it:** `ai:manage`
**Purpose:** Connects ConnectDesk to the AI models it uses. One model writes text (drafting replies, Copilot, assistants, summaries). One model "embeds" text so articles and snippets can be searched. An optional decision layer called "Jev" sits on top.
**Help panel:** "Works with any OpenAI-compatible endpoint."

The page has three cards.

### Card 1: Completion
Description: "The model used to draft replies and power copilot."

| Field / button | What it does | Typical use |
|---|---|---|
| **Preset** (dropdown, placeholder "Custom") | Fills in Base URL and Model for a common provider. Options: OpenAI (`gpt-5.4-mini`), Anthropic (Claude) (`claude-sonnet-4-6`), OpenRouter, Groq, Together AI, Ollama (local) (`http://localhost:11434/v1`, `llama3.1`). Hint: "Pre-fills the base URL and model for a common provider. You can still edit any field." | Pick "OpenAI" so you only need to paste the key. |
| **Model** (required) | The exact model name to call. | `gpt-5.4-mini` |
| **Base URL** | The API address. Hint: "OpenAI-compatible API base URL. Leave blank for the OpenAI default." | Point at your own gateway or a local Ollama. |
| **API key** (hidden input) | The provider's secret key. Once a key is saved, the box shows dots and the placeholder says "Leave blank to remove the saved key". Saving with the box empty removes the key. | Paste the key from the provider's dashboard. |
| **Temperature** (0 to 2, blank by default) | How creative the model is. Blank uses the provider's default. Error message: "Temperature must be a number between 0 and 2." | Leave blank unless you need very consistent answers. |
| **Max tokens** (default 1024 for a new provider) | Upper limit on the length of each AI answer. | Raise it if replies get cut off. |
| **Reasoning effort** | Hint: "Leave blank for most models. Values: 'none', 'low', 'medium' or 'high'." | Only for reasoning models. |
| **Transcription model** | "Turns call recordings into text for call summaries, on the same API. Leave blank for whisper-1." | Used by "Summarise recorded calls" (see Admissions). |
| **Custom instructions** | "Added to AI prompts. Use it to set tone and format, e.g. keep replies under 3 sentences." | Set a house style for all AI drafts. |
| **Model supports image input** (toggle) | "Enable only if the completion model can read images. When on, image attachments from customers are sent to the model. When off, images are never sent." | Turn on for vision-capable models. |
| **Save** | Saves the card. | |
| **Test connection** | Sends a test request using what is typed (it does not need to be saved first). On success it shows "Connection successful. The provider accepted the request." | Check the key before saving. |

### Card 2: Embedding
Description: "The model used to embed articles and snippets for search."

| Field / button | What it does |
|---|---|
| **Preset** | Options: OpenAI text-embedding-3-small (1536 dimensions), OpenAI text-embedding-3-large (3072), Ollama (local) (`nomic-embed-text`, 768). |
| **Model**, **Base URL**, **API key** | Same as the Completion card. |
| **Dimensions** (default 1536) | "Must match the model's output size (e.g. 1536 for text-embedding-3-small). Existing snippets are re-embedded automatically after you change this." |
| **Save** / **Test connection** | Same as the Completion card. |

### Card 3: Jev decision layer (TypeSafe)
Description: "Jev is a fast decision model from TypeSafe. It doesn't write messages; it decides, in a fraction of a second, whether a customer's message needs an AI reply at all, checks replies before they're sent, picks the lead stage and decides when to follow up. Your assistants use it once it's switched on here."

| Field / button | What it does |
|---|---|
| **Use Jev** (toggle) | "When off, or if Jev can't be reached, assistants work as before: every customer message goes to the LLM." |
| **API key** (placeholder `ts_…`) | "From console.typesafe.ai. Stored encrypted." |
| **Model** (default `jev-latest`) | "jev-latest follows TypeSafe's stable release." |
| **Test connection** | Disabled until a key is saved or typed. On success it shows "Connected to {model} — answered in {ms} ms." |
| **Save** | Saves the Jev settings. |

## AI Snippets (knowledge base)

**Where:** Admin > AI > Snippets. URL `/admin/ai/snippets`
**Who can use it:** `ai:manage`
**Purpose:** Short, self-contained answers that the AI assistants and Copilot search when answering customers. Together with help-center articles marked for AI use (see Help Center), these are the "knowledge sources".
**Help panel:** "Snippets are the knowledge base the assistant searches to answer questions."

| Element | What it does | Typical use |
|---|---|---|
| **Import from URL** (button) | Opens a dialog with a URL box (placeholder `https://example.com/help/article`) and an **Import** button. ConnectDesk fetches the page and saves it as a snippet. In the table the snippet gets an "Imported" badge; hover the badge to see the source URL. | Pull an existing FAQ page into the knowledge base. |
| **New snippet** (button) | Opens a dialog with the fields below. | |
| Field: **Title** (required) | Placeholder "Refund policy". | |
| Field: **Content** (required, large box) | Hint: "The assistant searches this text to answer customer questions, so write it as a self-contained answer rather than a fragment." | |
| Field: **Enabled** (toggle, on by default) | Only enabled snippets are used. | Turn a snippet off without deleting it. |
| **Create** / **Save** | Saves the dialog. | |
| Table columns | **Title** (click it to edit), **Status** (Enabled/Disabled), **Updated at**. An "Auto-created" badge marks snippets that came from approved Suggestions. | |
| Row menu (three dots) | **Edit** opens the dialog. **Delete** asks for confirmation ("This will permanently delete the snippet."). | |

## AI Suggestions (learn from resolved conversations)

**Where:** Admin > AI > Suggestions. URL `/admin/ai/suggestions`
**Who can use it:** `ai:manage`
**Purpose:** ConnectDesk can draft new snippets from resolved conversations in which a human agent gave a reusable answer. An admin reviews each one before it is used.
**Help panel:** "Suggestions are drafted from resolved conversations where a human agent gave a reusable answer. Review each one; approving it adds a snippet the assistant can use."

| Element | What it does |
|---|---|
| **Learn from resolved conversations** (toggle) | "When on, new suggestions appear here for you to review." It saves as soon as you flip it. |
| Table (pending suggestions only) | Columns: **Question** (click to review), **Answer**, **Source** (link "#ref-number" to the original conversation, which opens in a new tab), **Created at**. |
| **Review suggestion** dialog | Editable **Question** and **Answer** boxes, plus a **View conversation** link. |
| **Approve** | Saves your edited question and answer as a snippet. Shows "Suggestion approved and added to snippets". |
| **Reject** | Throws the suggestion away. Shows "Suggestion rejected". |

## AI Tools

**Where:** Admin > AI > Tools. URL `/admin/ai/tools`. New tool: `/admin/ai/tools/new`. Edit: `/admin/ai/tools/:id/edit`
**Who can use it:** `ai:manage`
**Purpose:** Lets an AI assistant call your own APIs while it replies, for example to look up an order.
**Help panel:** "Tools let the assistant call your own APIs to fetch or update data while replying. A tool is shared across every assistant granted access to it, and the model decides when to call it from the name and description, so keep those precise."

**List screen:**
- **New tool** button.
- Table columns: Name (click to edit), URL, Status, Updated at.
- Row menu: **Edit**, **Enable**/**Disable**, **Delete** (confirmation: "This will permanently delete the tool.").

**Tool form:**

| Field | What it does |
|---|---|
| **Name** | "Letters, numbers, hyphen and underscore only, up to 64 characters." Names of built-in tools are refused ("This name is reserved for a built-in tool..."). |
| **Description** | Placeholder "Looks up an order's status and delivery date by order ID." Hint: "The model reads this to decide when to call the tool, so describe what it does and when it should be used." |
| **URL** | The endpoint to call, e.g. `https://api.example.com/orders/lookup`. |
| **Method** | GET or POST. |
| **Headers** | Name/value rows (placeholders "Authorization" / "Bearer xxx"), with an **Add header** button and a remove (x) on each row. "Sent with every request to this tool's URL. Values are encrypted at rest." Renaming a header does not keep its stored secret; you have to enter the value again. |
| **Parameters (JSON schema)** | The inputs the model passes when it calls the tool. "Leave blank for a single text input." When the box is empty, an **Insert example** button fills in a sample schema with an `order_id` parameter. Invalid JSON shows "Invalid JSON. Check for a missing comma, quote, or bracket." |
| **Enabled** (on by default) | Turns the tool on or off. |
| **Require verified contact** (on by default) | "The assistant verifies the customer's identity by emailing them a one-time code before this tool can run." Turning it off opens a warning, "Turn off customer authentication?", explaining that anyone in the chat could then trigger the tool, and that every request carries an `X-ConectDesk-Contact-Verified` header. Buttons: **Keep authentication on** / **Turn off**. |
| **Create** / **Save** | Saves the tool. |

## AI Assistants (AI agents)

**Where:** Admin > AI > Assistants. URL `/admin/ai/assistants`. New: `/admin/ai/assistants/new`. Edit: `/admin/ai/assistants/:id/edit`
**Who can use it:** `ai:manage`
**Purpose:** Creates AI "agents" that answer customer conversations themselves and hand over to a human when they cannot help.
**Help panel:** "Assistants reply to conversations you route to them with an automation rule, and hand off to a human when they can't help. Each one has its own persona, instructions and tools, and they all read from the same knowledge base."
**How conversations reach an assistant:** an assistant is a special user. To route conversations to it, create an automation rule with the action **Assign agent** and pick the assistant. The Admissions setup creates exactly this kind of rule for you.

**List screen:**
- **New assistant** button.
- Table columns: Name, Description, Status, Updated at.
- Row menu: **Edit**, **Delete** (confirmation: "This will permanently delete the assistant.").

A new assistant only has the form. An existing assistant has three tabs: **Settings**, **Test**, **Performance**.

### Settings tab (the assistant form)

| Field | What it does / default |
|---|---|
| **Enabled** | On by default. |
| **Name** | "The display name customers see when they chat with this assistant." |
| **Avatar** | **Upload** an image or remove it. |
| **Description** | Placeholder "Handles order status and returns questions". "For your own reference only; customers never see this." |
| **Expectation message** | Placeholder "I'm an AI assistant. A human agent can join if you need one." "Shown to the customer while this assistant handles the chat. It disappears on handoff to a human." In the chat widget it appears under the assistant's name, in place of the business-hours line. |
| **Tone** | Friendly (default), Professional, Neutral, Casual. |
| **Response length** | Concise, Balanced (default), Detailed. |
| **Max turns** (1 to 20, default 6) | "Number of back-and-forth replies before the assistant hands off." |
| **Languages** (type a language and press Enter) | "Languages the assistant is allowed to reply in. It uses the customer's language when it is one of these; otherwise it replies in the first one. Leave empty to always match the customer's language." |
| **Offer handoff to a human** (on by default) | "When off, the assistant never offers to connect the customer with a human agent and tells them it cannot help with questions outside its knowledge." |
| **Fallback team** (None or a team) | "Team the conversation is handed off to when the assistant cannot help." |
| **Instructions** | "Custom system prompt that guides how this assistant responds." Example placeholder: "Always confirm the order ID before looking anything up..." |
| **Guardrails** | "Rules the assistant must never break." Example placeholder: "Never issue a refund without a human agent's approval..." |

**Jev decisions** box. Description: "Jev decides what happens before and after the LLM writes. The LLM still writes every message." If Jev is not set up, a warning reads "Jev isn't set up for this workspace, so these settings have no effect yet." with a **Set it up** link to AI Providers.

| Field | What it does |
|---|---|
| **Decide when to call the LLM** (on) | Messages that need no answer (e.g. "ok, thanks") get no reply, and the conversation closes once everything is answered. Customers who ask for a person or are clearly upset go straight to a human. Anything else, or anything Jev is unsure about, goes to the LLM. |
| **Check replies before sending** (on) | A reply that breaks your guardrails, states facts the knowledge base does not back up, or reads as rude is not sent. It goes to a person as a private note to send or fix. |
| **Update the lead stage** (off) | Moves the contact through your lead stages as the conversation goes. It never moves a contact out of a won or lost stage, and needs extra certainty to move one into it. |
| **Minimum confidence** (0.5 to 0.99, default 0.7) | Jev answers below this confidence are ignored. Use a higher value for chats that aren't in English. |

**Follow up when the customer goes quiet** (toggle, off by default). "If the customer stops replying to the assistant, it sends a follow-up, but only when Jev judges the conversation is waiting on the customer, and only within business hours." When it is on, these fields appear:

| Field | What it does |
|---|---|
| **Follow up after (hours)** (e.g. `4, 24`) | One number per follow-up, each counted from the assistant's previous message. Up to 5. |
| **WhatsApp templates for follow-ups** | After 24 hours WhatsApp only allows approved templates. Jev picks the one that fits from this list, or sends nothing. Without Jev, follow-ups are only sent inside the 24-hour window. |
| **Load templates from** | Choose a WhatsApp inbox. |
| **Template {n}** | Pick an approved template (templates that need an agent are greyed out "needs an agent"). **Refresh** reloads the list. Fill in the body values, check the preview, and use **Remove** to drop it. |
| **Add template** | Adds another template slot, up to 10. Needs an inbox to be chosen first. |

**Tools** checklist. One checkbox per AI Tool. "Tools this assistant is allowed to call... Knowledge snippets are shared across all assistants automatically." If there are none: "No tools yet. Create tools under AI to grant them here."

**Create** / **Save** saves the assistant.

### Test tab
- **Test assistant**: type a sample customer message ("Type a sample customer message") and click **Test**, or press Ctrl+Enter.
- **Drafted reply** shows what the assistant would answer.
- **Knowledge used** lists the snippets and articles it drew on, each with a match percentage.
- Nothing is sent to a customer.

### Performance tab
- Range buttons: **7 days / 30 days / 90 days**.
- Tiles: Conversations, Replies, Resolution rate, Handoff rate, Reopen rate, Replies per conversation, CSAT average, CSAT positive, Left without an LLM reply, Replies stopped, Follow-ups sent.
- Some tiles show an up or down arrow comparing with the previous period. A red arrow means the change is bad, e.g. handoff rate going up.

## Where agents meet the AI

These are not admin screens; they need the Completion provider above. Also: **Generate reply**, **AI ▾** rewrite and **Summarize with AI** in the reply box and conversation menu, **Suggest tags** and **Fill with AI** in the Details sidebar, call summaries, and the assistants that answer customers themselves.
- **Juno** is the built-in Copilot name. Agents see a **Juno** tab in the conversation sidebar. It offers presets such as "Summarize this conversation" and "What is the customer asking?", plus **Insert into reply**, **Add as private note** and **Clear chat**. Empty state: "Ask Juno anything. It can search your knowledge base to answer."
- **Ask Juno about SLA**: a floating **Ask Juno** button on Reports > Overview (`/reports/overview`, needs `reports:manage`). It opens a panel: "Answers come from your SLA data and use the dashboard's range ({period}) unless you name another period." Presets: "How are we doing on SLA?", "Which team breaches the most?", "What is about to breach in the next hour?", "Show the latest first response breaches".
