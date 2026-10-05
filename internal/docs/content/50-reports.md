# Reports

**Where:** Icon rail → **Reports**. The reports sidebar has **Overview** and **Calls**.

**Who can use it:** `reports:manage`.

## Overview

**Where:** `/reports/overview`.

**Purpose:** A live dashboard of workload, agents, customer satisfaction, SLA and tags.

| Control / card | What it shows or does |
|---|---|
| **Today so far** tiles | New enquiries, Calls received, Missed calls, Still to call back (red if any are overdue), Called back, Admissions. |
| "Last updated: {time}" | Refreshes every 5 minutes. After an hour it shows "Auto-refresh paused" with **Refresh**. |
| Range: **7 days**, **30 days**, **90 days**, **Custom** | Custom shows a days box (1–365; applies on <kbd>Enter</kbd> or when you leave the box). |
| **Open conversations** | Open, Awaiting Response, Unassigned, Pending. |
| **Agent status** | Online, Offline, Away, Reassigning. |
| **Customer satisfaction** | Avg Rating, Response Rate, Responses. |
| **Message volume** | Total, Incoming, Outgoing, Per Conversation. |
| **SLA performance** | First Response / Next Response / Resolution: compliance %, met, breached, average times. |
| **Tag distribution** | Tagged % (x / total) and the top 5 tags; "No tags found" if none. |
| **Conversation trends** | Line chart of new vs resolved conversations. |
| Floating **Ask Juno** button | Opens "Ask Juno about SLA". Answers come from your SLA data, for the dashboard's range unless you name another period. Presets: "How are we doing on SLA?", "Which team breaches the most?", "What is about to breach in the next hour?", "Show the latest first response breaches". Also **Ask anything…**, **Copy** and **Clear chat**; the history is saved. |

## Calls

**Where:** `/reports/calls`.

**Purpose:** Phone activity and recordings.

| Control | What it does |
|---|---|
| **Today** / **7 days** / **30 days** / **90 days** | Period. |
| Tiles | Calls, Incoming, Outgoing, Missed, Missed rate, Voicemails, Average call. |
| **Calls per day** | Stacked bars: answered incoming, missed, outgoing. |
| **By agent** | Agent, Calls, Answered, Talk time. |
| **Call log** + **Recorded only** | When, Contact (links to the conversation), call type, Agent, Length, Recording (player + **Download**, or "No recording"). 25 per page with **Previous** / **Next**. |

Empty state: "No calls in this period."

The owner can get the day's numbers on WhatsApp every evening; see Admin → Admissions → Daily report on WhatsApp.
