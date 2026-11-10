# Customer-facing pages

What customers and website visitors see: the live-chat widget, the public help center and the satisfaction (CSAT) survey.

## Live-chat widget (what a website visitor sees)

Everything here is configured per Live Chat inbox under Admin → Inboxes.

**How it appears:**
- `widget.js`, embedded on your site, shows a round launcher button. Its color, logo and position (left or right, with spacing) come from the live-chat inbox settings.
- A red unread-count badge appears on the launcher.
- Clicking the launcher opens the chat window (full screen on mobile).
- Developers can control it from the page: `show()`, `hide()`, `isVisible()`, `onShow`, `onHide`, `onUnreadCountChange`, `setUser(jwt)` (identify a logged-in user), `logout()`, `destroy()`.
- The widget tracks the page the visitor is on.

**Home tab:**
- Logo, greeting and introduction text, over the configured header background.
- If the visitor has a previous chat: a **Continue your conversation** card with the last message, its author, its time and unread count.
- Otherwise: a start button (default label "Send us a message", or your custom text). It is hidden if multiple conversations are not allowed and one already exists.
- "Home apps" cards: announcements (image, title, description) and external links. Each opens in a new tab.

**Messages tab:**
- A list of the visitor's conversations: avatar, sender, last message, time and unread badge. Empty state: "No messages yet".
- A floating **Start new conversation** button, shown only when starting a new conversation is allowed and multiple conversations are permitted.

**Bottom navigation:** **Home** and **Messages** tabs.

**Chat screen:**
- **Back** arrow.
- Agent or assistant avatar and name.
- Status line, which is one of:
  - the AI assistant's expectation message
  - the out-of-business-hours status, if office hours are shown
  - the reply-expectation message when no agent is online
  - Online / Away
  - "Active ... ago"
- **Expand / Collapse** button (desktop only).
- **Close chat** (x) on mobile.

**Pre-chat form** (if it is enabled and has at least one enabled field):
- Shown before an anonymous visitor's first message, or before a new conversation.
- Field types: text, email, number, date, link, checkbox, phone (with country code) and list. Required fields are marked with *.
- A **Message** box ("Type your message...") and a **Start chat** button.

**Inside the chat:**
- An introduction line, and an optional notice banner.
- Messages with attachments, "Sending…" and "Uploading…" states, and a typing indicator.
- A scroll-to-bottom button.
- CSAT bubbles (see Part 7).

**Message box:**
- "Type your message...". Enter sends, Shift+Enter adds a new line.
- **Attach file** (if file uploads are enabled), **Add emoji** (if enabled), **Send**.

**Other states:**
- Closed conversation, when replying to closed conversations is not allowed: "This conversation has been closed" (the input is hidden).
- Connection banners: "No internet connection", connection failed, connecting, connected.
- Blocked IP: "Access denied".
- "Powered by ConnectDesk" footer in the chat view, unless it is turned off.

## Public help center (what customers see)

**URL:** `/hc/<slug>`, which redirects to the default language, `/hc/<slug>/<locale>`. Other pages:
- Collection: `/collections/<collection-slug>`
- Article: `/articles/<article-slug>`
- Search: `/search?q=`
- Sitemap: `/hc/<slug>/<locale>/sitemap.xml`

A verified custom domain can also serve it.

**Classic template:**
- Top bar with your logo or name, navigation links and a language switcher (folds into a menu on small screens).
- A hero with the header text, the tagline and a large search box ("Search articles...", **Search** button). Results appear as you type.
- The home page shows "Popular articles" (if switched on) and collection cards. If nothing is published yet: "Nothing has been published here yet. Please check back later."

**Docs template:**
- Top navigation and a left sidebar listing the collections.
- A **Search** button with a **Ctrl K** (Cmd K on Mac) shortcut. It opens a search dialog where arrow keys move, Enter selects and Esc closes.

**Article page (both templates):**
- Breadcrumb, title, excerpt, optional author byline, and "Last updated" date.
- "On this page" table of contents that highlights the section you are reading.
- Copy buttons on code blocks.
- A **"Was this article helpful?"** box with Yes / No. After answering it shows "Thanks for your feedback." It only counts once per browser. These answers feed the Feedback counts in the article editor.
- Related articles.

**Every page:**
- Optional announcement bar, which the reader can dismiss.
- Footer with tagline, links, social icons and "Powered by ConnectDesk".
- A skip-to-content link for accessibility.

**Other states:**
- Missing page: "This page doesn't exist or may have moved..."
- Empty search: "No articles matched your search. Try fewer or different words."
- A paused help center is not served.

## CSAT surveys

**How surveys are sent:**
1. **Automatically on resolve**, per inbox. In the inbox settings (email, live chat, Meta inboxes) there is a **CSAT Surveys** toggle: "Send customer satisfaction surveys when conversation is marked as resolved." It fires on every way of resolving: an agent, an AI assistant or an automation. The inbox hint suggests: "For better control on when to send surveys, disable this option and create an automation rule to send surveys."
2. **With the automation action "Send CSAT".**

Either way:
- "CSAT surveys are only sent once per conversation."
- The survey is skipped if the contact has no email address.
- The message is built from the stored email template "CSAT request" (Admin > Notifications > Templates) and includes the survey link.

**Public survey page:** `/csat/<uuid>`
- Heading: "Rate your recent interaction".
- Five clickable face icons labelled **Poor**, **Fair**, **Good**, **Great**, **Excellent** (values 1 to 5).
- **Additional feedback (optional)** box, up to 1000 characters, with a live counter.
- **Submit** button. Submitting without a rating shows "Please select rating".
- If the link includes `?rating=N`, that face is already selected (useful for one-click links in emails).
- After submitting, a page says "Thank you!" and "We appreciate you taking the time to submit your feedback."
- A second submission is refused ("CSAT already submitted").

**Embeddable star widget:** `/csat/<uuid>/widget`. Five stars; one click submits the rating and shows "Thank you!". If the survey was already answered, it only shows "Thank you!".

**Inside the live-chat widget:** the survey appears as a chat bubble: "Please rate this conversation:", five face buttons, "Additional feedback (optional)" (up to 500 characters, placeholder "Tell us more..."), and **Submit feedback**. Afterwards it shows "Thank you for your feedback!" with the rating and comment the visitor gave.

Agents see responses in the conversation; AI assistant CSAT appears in the assistant's Performance tab.
