// WhatsApp templates the app sends on its own, ready to submit to Meta for
// approval. body_values are what the app fills each {{n}} with.
export const SUGGESTED_TEMPLATES = {
  missedCall: {
    name: 'missed_call_followup',
    language: 'en',
    category: 'UTILITY',
    body: 'Hi {{1}}, sorry we missed your call to {{2}}. A counsellor will call you back shortly. You can also reply here with your question.',
    body_examples: ['Asha', 'Bright Classes'],
    body_values: ['{{name}}', '{{business}}']
  },
  missedCallAfterHours: {
    name: 'missed_call_after_hours',
    language: 'en',
    category: 'UTILITY',
    body: "Hi {{1}}, thanks for calling {{2}}. We're closed right now, and a counsellor will call you back as soon as we open. You can reply here with your question anytime.",
    body_examples: ['Asha', 'Bright Classes'],
    body_values: ['{{name}}', '{{business}}']
  },
  dailyReport: {
    name: 'daily_admissions_report',
    language: 'en',
    category: 'UTILITY',
    body: 'Your report for {{1}}\n\nNew enquiries: {{2}}\nCalls received: {{3}}\nMissed calls: {{4}}\nStill to call back: {{5}}\nAdmissions: {{6}}',
    body_examples: ['29 Sep 2026', '40', '25', '6', '3', '5'],
    body_values: []
  }
}
