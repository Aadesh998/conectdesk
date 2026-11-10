package main

import (
	"html/template"
	"strings"

	"github.com/conectdesk/conectdesk/internal/tenant"
	"github.com/zerodha/fastglue"
)

// The solution pages are the website's pages for what institutes search for:
// an omnichannel CRM, a WhatsApp CRM, missed-call replies and an admission
// CRM. Each one describes the same product from that search's side, so each
// says only what the product does.

// solutionsPrefix is where the solution pages are, on the base domain.
const solutionsPrefix = "/solutions/"

// solutionSection is one titled part of a solution page.
type solutionSection struct {
	H2      string
	Paras   []string
	Bullets []string
}

// solutionPage is one solution page. Its text may name the product with
// {product}.
type solutionPage struct {
	Slug string
	// Nav is the page's short name, in links to it.
	Nav string
	// Title is the browser and search result title, without the product.
	Title       string
	H1          string
	Description string
	Kicker      string
	Lede        string
	Sections    []solutionSection
	FAQs        []landingFAQ
}

var solutionPages = []solutionPage{
	{
		Slug:        "omnichannel-crm-for-coaching-institutes",
		Nav:         "Omnichannel CRM",
		Title:       "Omnichannel CRM for Coaching Institutes",
		H1:          "Omnichannel CRM for coaching institutes",
		Description: "Omnichannel software for coaching institutes: calls, WhatsApp, Instagram, Messenger and email in one shared inbox, with AI replies and lead tracking.",
		Kicker:      "Omnichannel inbox",
		Lede:        "Parents and students reach your institute on whichever channel is closest: a call, a WhatsApp message, an Instagram DM. {product} brings every one of those channels into one shared inbox, so your counsellors answer from one screen and every enquiry keeps one history.",
		Sections: []solutionSection{
			{
				H2:    "What omnichannel means for an admissions desk",
				Paras: []string{"Multichannel is having a phone, a WhatsApp number and an Instagram page. Omnichannel is when they share one record. A parent who calls in the morning, messages on WhatsApp in the afternoon and comments on Instagram at night is one contact in {product}, with one conversation history, one owner and one place in your admissions pipeline.", "That matters at a coaching institute because the same family usually enquires two or three times before visiting. Without a shared record, each counsellor starts from zero and the parent repeats themselves."},
			},
			{
				H2:      "Every channel your institute already uses",
				Bullets: []string{"Phone calls, with recordings, transcripts and a short summary of each call", "WhatsApp Business, including approved templates for messages after the 24-hour window", "Instagram direct messages and Facebook Messenger", "Telegram", "A chat widget for your website", "Email"},
				Paras:   []string{"Connect the channels you already use. Each one arrives in the same inbox, can be assigned to a counsellor or a team, and follows the same working hours and response targets."},
			},
			{
				H2:      "Built around admissions, not support tickets",
				Paras:   []string{"Generic omnichannel helpdesks are designed for complaint tickets. {product} is designed for enquiries that should become admissions:"},
				Bullets: []string{"Every missed call gets an automatic WhatsApp reply and a call-back task for your team", "An AI assistant answers fees, batches and timings in Hindi, English or Hinglish from your own details, and hands over to a counsellor when a person is needed", "A lead board moves each enquiry from New to Interested, Visit booked and Admitted", "The owner gets the day's enquiries, call-backs and admissions on WhatsApp every evening"},
			},
			{
				H2:    "Who it is for",
				Paras: []string{"Coaching institutes and training centres with an admissions team of roughly 3 to 30 people: JEE and NEET coaching, IELTS and study-abroad consultants, skill and computer courses, foundation classes and tuition centres that get most of their enquiries by phone and WhatsApp."},
			},
		},
		FAQs: []landingFAQ{
			{Q: "What is omnichannel software?", A: "Software that connects every channel customers use, such as calls, WhatsApp, Instagram and email, into one system with a shared history, so whoever replies sees the whole conversation."},
			{Q: "How is {product} different from an omnichannel helpdesk?", A: "Helpdesks are built for support tickets. {product} is built for admissions: missed-call replies, call-back tasks, an AI assistant that knows your fees and batches, a lead board from enquiry to admission, and a daily report for the owner."},
			{Q: "Do we need to change our phone number or WhatsApp number?", A: "No. You connect the WhatsApp Business number, Instagram account and phone line you already use."},
		},
	},
	{
		Slug:        "whatsapp-crm-for-coaching-institutes",
		Nav:         "WhatsApp CRM",
		Title:       "WhatsApp CRM for Coaching Institutes",
		H1:          "WhatsApp CRM for coaching institutes",
		Description: "Run admissions on WhatsApp with {product}: a shared inbox for counsellors, AI replies in Hindi and English, missed-call replies and a lead board.",
		Kicker:      "WhatsApp CRM",
		Lede:        "Most parents would rather send a WhatsApp message than fill in a form. {product} turns your institute's WhatsApp Business number into a shared inbox for the whole admissions team, with every chat tied to a contact, a counsellor and a stage in your pipeline.",
		Sections: []solutionSection{
			{
				H2:      "One WhatsApp number, the whole team on it",
				Paras:   []string{"Instead of one phone passed around the front desk, every counsellor works the same WhatsApp number from their own login."},
				Bullets: []string{"Chats are assigned to a counsellor or a team, and reassigned when someone is away", "Private notes and mentions let counsellors discuss a lead without the parent seeing it", "Approved WhatsApp templates reach parents after the 24-hour window, for follow-ups and reminders", "Every message, call and note about a contact stays in one history"},
			},
			{
				H2:    "An AI assistant that answers like your front desk",
				Paras: []string{"The assistant replies on WhatsApp in Hindi, English or Hinglish, using the fee structure, batch timings, eligibility and FAQs you give it. When a parent wants to talk to a person, or the question is outside what it knows, it hands the chat to a counsellor instead of guessing.", "It is not a menu bot: parents type the way they normally would, such as \"NEET weekend batch ki fees kitni hai?\", and get a straight answer."},
			},
			{
				H2:      "More than a WhatsApp inbox",
				Paras:   []string{"WhatsApp tools that stop at chat leave out how enquiries really arrive. {product} also handles:"},
				Bullets: []string{"Phone calls, with an automatic WhatsApp reply to every missed call", "Instagram, Messenger, Telegram, website chat and email in the same inbox", "Call-back tasks that close themselves when a counsellor calls that number", "A lead board from enquiry to admission, with course, class and city filled in by AI", "A daily admissions report sent to the owner on WhatsApp"},
			},
		},
		FAQs: []landingFAQ{
			{Q: "Is {product} a WhatsApp bot?", A: "No. It is a WhatsApp CRM for the whole admissions team. It includes an AI assistant, but its job is to get every enquiry answered and followed up, by the assistant or by a counsellor."},
			{Q: "Does it use the official WhatsApp Business API?", A: "Yes. You connect your WhatsApp Business number through Meta, so messages, templates and delivery receipts follow WhatsApp's rules."},
			{Q: "Can several counsellors reply from the same WhatsApp number?", A: "Yes. Everyone works from the shared inbox, and each chat shows who it is assigned to."},
		},
	},
	{
		Slug:        "missed-call-auto-reply",
		Nav:         "Missed-call replies",
		Title:       "Missed Call Auto Reply on WhatsApp",
		H1:          "Missed call auto reply on WhatsApp, for coaching institutes",
		Description: "Reply to every missed call on WhatsApp automatically, with a separate after-hours message and a call-back task for your counsellors.",
		Kicker:      "Missed-call replies",
		Lede:        "Most enquiries at a coaching institute arrive after the front desk has gone home, and a parent who isn't answered calls the next institute. {product} replies to every missed call on WhatsApp within seconds and puts a call-back on your team's list.",
		Sections: []solutionSection{
			{
				H2:      "How it works",
				Bullets: []string{"A call to your institute's number is missed", "Within seconds the caller gets a WhatsApp message from your business number, for example your batch timings and a promise to call back", "Outside business hours they get a separate after-hours message instead", "A call-back task is created and assigned, at the top of the next morning's list", "When a counsellor calls that number, the task closes itself", "Call-backs left too long are escalated"},
			},
			{
				H2:    "Polite, not spammy",
				Paras: []string{"Each number gets at most one automatic reply a day, so a parent who tries three times in ten minutes gets one message, not three. The reply comes from your own WhatsApp Business number, using a template you write and Meta approves."},
			},
			{
				H2:    "Every call, on the record",
				Paras: []string{"Calls, missed or answered, appear in the same inbox as the caller's WhatsApp and Instagram messages. Recorded calls are transcribed and summarised with the next step, so the next counsellor knows what was promised."},
			},
		},
		FAQs: []landingFAQ{
			{Q: "Does the caller pay for the WhatsApp reply?", A: "No. The message is sent from your WhatsApp Business number; the caller receives it like any other WhatsApp message."},
			{Q: "Can we send a different message at night?", A: "Yes. Messages inside and outside your business hours are set separately."},
			{Q: "What phone system does it work with?", A: "{product} works with cloud telephony numbers such as Exotel, so your existing business number can be connected."},
		},
	},
	{
		Slug:        "admission-crm",
		Nav:         "Admission CRM",
		Title:       "Admission CRM & Enquiry Management Software",
		H1:          "Admission CRM for coaching institutes and training centres",
		Description: "Track every admission enquiry from first call to admission. {product} is an admission CRM with a lead board, call-back tasks, AI replies and a daily report on WhatsApp.",
		Kicker:      "Admission CRM",
		Lede:        "An admission CRM should answer one question every evening: which enquiries became admissions, and which ones nobody followed up? {product} tracks every enquiry from the first call or message to the day the student joins.",
		Sections: []solutionSection{
			{
				H2:      "A board from enquiry to admission",
				Paras:   []string{"Every enquiry becomes a card on a lead board that counsellors move as the family decides."},
				Bullets: []string{"Stages your team already uses: New, Interested, Visit booked, Admitted, which you can rename", "Course, class, batch and city filled in by AI from the conversation", "Each lead tied to its calls, WhatsApp chats and Instagram messages", "Search and filters for course, stage and counsellor"},
			},
			{
				H2:      "Follow-ups that don't depend on memory",
				Bullets: []string{"Missed calls create call-back tasks automatically", "Tasks close when the counsellor actually calls the number", "Overdue call-backs are escalated to the owner", "Response targets and working hours per team"},
			},
			{
				H2:    "The owner's view, every evening",
				Paras: []string{"At the end of the day the owner gets a WhatsApp message with the day's enquiries by channel, missed calls and replies, call-backs still pending, visits booked and admissions closed. No dashboard to remember to open, although the full reports are there too."},
			},
		},
		FAQs: []landingFAQ{
			{Q: "Can we import our existing enquiries?", A: "Yes. Contacts can be imported, and every new call or message from that number joins the same contact."},
			{Q: "Can each counsellor see only their own leads?", A: "Yes. Roles and teams decide who sees which conversations and leads, and an activity log shows who did what."},
			{Q: "Is it only for large institutes?", A: "No. It is built for admissions teams of roughly 3 to 30 people."},
		},
	},
}

// solutionBySlug returns the solution page with the slug.
func solutionBySlug(slug string) (solutionPage, bool) {
	for _, p := range solutionPages {
		if p.Slug == slug {
			return p, true
		}
	}
	return solutionPage{}, false
}

// withProduct returns the page with {product} replaced in its text.
func (p solutionPage) withProduct(product string) solutionPage {
	r := strings.NewReplacer("{product}", product)
	p.Description, p.Lede = r.Replace(p.Description), r.Replace(p.Lede)
	secs := make([]solutionSection, len(p.Sections))
	for i, s := range p.Sections {
		s.Paras = replaceAll(r, s.Paras)
		s.Bullets = replaceAll(r, s.Bullets)
		secs[i] = s
	}
	p.Sections = secs
	faqs := make([]landingFAQ, len(p.FAQs))
	for i, f := range p.FAQs {
		faqs[i] = landingFAQ{Q: r.Replace(f.Q), A: r.Replace(f.A)}
	}
	p.FAQs = faqs
	return p
}

func replaceAll(r *strings.Replacer, in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = r.Replace(s)
	}
	return out
}

// handleSolutionPage serves a solution page on the base domain.
func handleSolutionPage(r *fastglue.Request) error {
	app := r.Context.(*App)
	slug, _ := r.RequestCtx.UserValue("slug").(string)
	page, ok := solutionBySlug(slug)
	if !ok || !app.tenant.Enabled() || !app.tenant.IsBaseDomain(string(r.RequestCtx.Host())) {
		serveWebsiteNotFound(app, r.RequestCtx, app.i18n.T("globals.messages.pageNotFound"))
		return nil
	}
	d := websiteData(app)
	product := d["Product"].(string)
	page = page.withProduct(product)
	d["Page"] = page
	d["Title"] = page.Title + " | " + product
	d["Description"] = page.Description
	d["Canonical"] = d["BaseURL"].(string) + solutionsPrefix + page.Slug
	d["DemoURL"] = ko.String("app.tenancy.demo_url")
	others := make([]solutionPage, 0, len(solutionPages)-1)
	for _, p := range solutionPages {
		if p.Slug != page.Slug {
			others = append(others, p)
		}
	}
	d["Others"] = others
	d["JSONLD"] = solutionJSONLD(d, page)
	setWebsiteHeaders(app, r.RequestCtx)
	return app.tmpl.RenderWebPage(tenant.DefaultID, r.RequestCtx, "solution", map[string]any{"Data": d})
}

// solutionJSONLD returns a solution page's structured data: the page, its
// place under the home page, and its questions.
func solutionJSONLD(d map[string]any, p solutionPage) template.JS {
	base := d["BaseURL"].(string)
	url := d["Canonical"].(string)
	questions := make([]any, 0, len(p.FAQs))
	for _, f := range p.FAQs {
		questions = append(questions, map[string]any{
			"@type":          "Question",
			"name":           f.Q,
			"acceptedAnswer": map[string]any{"@type": "Answer", "text": f.A},
		})
	}
	graph := []any{
		map[string]any{
			"@type":       "WebPage",
			"@id":         url + "#webpage",
			"url":         url,
			"name":        d["Title"],
			"description": p.Description,
			"inLanguage":  "en-IN",
			"isPartOf":    map[string]any{"@id": base + "/#website"},
			"about":       map[string]any{"@id": base + "/#software"},
		},
		map[string]any{
			"@type": "BreadcrumbList",
			"itemListElement": []any{
				map[string]any{"@type": "ListItem", "position": 1, "name": d["Product"], "item": base + "/"},
				map[string]any{"@type": "ListItem", "position": 2, "name": p.Nav, "item": url},
			},
		},
	}
	if len(questions) > 0 {
		graph = append(graph, map[string]any{"@type": "FAQPage", "@id": url + "#faq", "mainEntity": questions})
	}
	return jsonLD(map[string]any{"@context": schemaOrgContext, "@graph": graph})
}

// isSolutionPath reports whether path is a solution page's.
func isSolutionPath(path string) bool {
	_, ok := solutionBySlug(strings.TrimPrefix(path, solutionsPrefix))
	return strings.HasPrefix(path, solutionsPrefix) && ok
}
