# FAQ and troubleshooting

## AI assistant

**How do I make the AI assistant?**
Connect an AI provider (*Admin → AI → Providers*), add snippets (*Admin → AI → Snippets*), then *Admin → AI → Assistants → New assistant*. Full walkthrough: [Create your AI assistant](/guide/ai-assistant).

**Where do I put the information the AI should answer from?**
In **Snippets** (*Admin → AI → Snippets*) — one snippet per topic, written as a complete answer. Published help-center articles with **Allow AI assistants to use this article** turned on are used too. See [Knowledge base](/guide/knowledge-base).

**The assistant isn't replying.**
Check, in order: the assistant is **Enabled**; the inbox's **AI assistant** setting isn't **Off**; the conversation isn't already assigned to a person; **Test connection** works on both the Completion and Embedding cards; you haven't reached your monthly AI limit (*Workspace → Usage*). Email inboxes need an [automation rule](/guide/ai-assistant#for-email-and-other-channels-use-an-automation).

**The assistant says "I don't know" about something in my snippets.**
Make sure the snippet is enabled and the **Embedding** card is set up, then rewrite the snippet using the words customers use. Check with the assistant's **Test** tab — **Knowledge used** should list it.

**Can I have different assistants for different channels?**
Yes. Create several and pick one in each inbox's **AI assistant** setting. They all share the same knowledge base, but each has its own name, tone, instructions, guardrails and tools.

**How do I stop the AI on one channel?**
Set that inbox's **AI assistant** to **Off**. To stop it everywhere, untick **Enabled** on the assistant.

**How do I take a chat over from the AI?**
Open the conversation and click **Take over**, or just reply.

**Does the AI cost extra?**
Each AI reply, Juno answer or summary is one AI request against your plan's monthly limit, and your AI provider bills you for the model usage on your own API key.

## Channels

**The chat bubble doesn't show on my website.**
Make sure the snippet from **Installation** is on the page just before `</body>`, the inbox is **Enabled**, and — if you filled in **Domain list** on the Security tab — your website's domain is on it (add both `example.com` and `*.example.com`).

**I can't reply on WhatsApp.**
More than 24 hours have passed since the customer's last message. Send an approved template; the customer's reply reopens the 24-hour window.

**Emails aren't arriving.**
Open the email inbox and check it's **Enabled**. For Google/Microsoft inboxes, click **Reconnect**. For other providers, check the IMAP host, port, username and password.

## Team and settings

**An agent can't see Admin.**
Their role has no admin permissions. Give them a role with the permissions they need (*Teammates → Roles*).

**An agent lost their phone and can't log in.**
*Teammates → Agents →* their name → **Reset two-factor login**.

**SLA times look wrong.**
Check the workspace **Timezone** and **Business hours** under [General](/guide/workspace-settings#general), and the team's own time zone and hours.

**How do I get all my data out?**
*Workspace → Data export → Export all data*. See [Data export](/guide/workspace-settings#data-export).

## Still stuck?

Contact ConnectDesk support with your workspace address and a screenshot of what you see.
