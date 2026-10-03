// The setup checklist's steps, in order, keyed as the onboarding API reports
// them, with where each one is done.
export const onboardingSteps = [
  { key: 'inbox', to: { name: 'new-inbox' } },
  { key: 'teammate', to: { name: 'new-agent' } },
  { key: 'widget', to: { name: 'new-inbox' } },
  { key: 'article', to: { name: 'help-center-list' } },
  { key: 'reply', to: { name: 'inbox', params: { type: 'assigned' } } }
]
