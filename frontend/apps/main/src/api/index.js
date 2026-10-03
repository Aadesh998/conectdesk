import axios from 'axios'
import qs from 'qs'

const http = axios.create({
  timeout: 10000,
  responseType: 'json'
})

// LLM calls can take 30-40s+, well past the default request timeout.
const AI_TIMEOUT = 120000

function getCSRFToken () {
  const name = 'csrf_token='
  const cookies = document.cookie.split(';')
  for (let i = 0; i < cookies.length; i++) {
    let c = cookies[i].trim()
    if (c.indexOf(name) === 0) {
      return c.substring(name.length, c.length)
    }
  }
  return ''
}

// Route-scoped abort, opt-in via { abortOnRoute: true }. Default no-abort protects in-flight saves.
let routeAbort = new AbortController()
export function abortRouteScope () {
  routeAbort.abort()
  routeAbort = new AbortController()
}

http.interceptors.request.use((request) => {
  const token = getCSRFToken()
  if (token) {
    request.headers['X-CSRFTOKEN'] = token
  }

  if ((request.method === 'post' || request.method === 'put') && !request.headers['Content-Type']) {
    request.headers['Content-Type'] = 'application/json'
  }

  if (request.headers['Content-Type'] === 'application/x-www-form-urlencoded') {
    request.data = qs.stringify(request.data)
  }

  if (request.abortOnRoute && request.signal === undefined) {
    request.signal = routeAbort.signal
  }

  return request
})

// The login page. Matches the router's '/' route, whose child is named `login`.
const LOGIN_PATH = '/'
// Where agents set up two-factor login.
const TWO_FACTOR_SETUP_PATH = '/account/security'

// Send an expired session back to the login page.
//
// Without this a 401 was swallowed by whichever view made the call: the route
// still changed, the component mounted, its fetch failed, and it rendered an
// empty shell. The result was a blank page with no error and no way back, which
// looked like the app had simply stopped loading.
http.interceptors.response.use(
  (response) => response,
  (error) => {
    const status = error?.response?.status
    const url = error?.config?.url || ''

    // The login call itself answers 401 for bad credentials; that belongs to the
    // form, not here. Same for the session probe the app makes before deciding
    // whether anyone is signed in.
    const isAuthCall = url.includes('/api/v1/auth/login') || url.includes('/api/v1/agents/me')

    // The workspace requires two-factor login the agent hasn't set up.
    if (
      status === 403 &&
      error?.response?.data?.data?.two_factor_setup_required &&
      window.location.pathname !== TWO_FACTOR_SETUP_PATH
    ) {
      window.location.assign(`${TWO_FACTOR_SETUP_PATH}?required=1`)
    }

    if (status === 401 && !isAuthCall && window.location.pathname !== LOGIN_PATH) {
      // Keep where they were so login can send them back.
      const next = window.location.pathname + window.location.search
      window.location.assign(`${LOGIN_PATH}?redirect=${encodeURIComponent(next)}`)
    }

    return Promise.reject(error)
  }
)

const getCustomAttributes = (appliesTo) =>
  http.get('/api/v1/custom-attributes', {
    params: { applies_to: appliesTo }
  })
const createCustomAttribute = (data) =>
  http.post('/api/v1/custom-attributes', data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const getCustomAttribute = (id) => http.get(`/api/v1/custom-attributes/${id}`)
const updateCustomAttribute = (id, data) =>
  http.put(`/api/v1/custom-attributes/${id}`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const deleteCustomAttribute = (id) => http.delete(`/api/v1/custom-attributes/${id}`)
const searchConversations = (params) => http.get('/api/v1/conversations/search', { params })
const searchMessages = (params) => http.get('/api/v1/messages/search', { params })
const searchContacts = (params) => http.get('/api/v1/contacts/search', { params })
const getEmailNotificationSettings = () => http.get('/api/v1/settings/notifications/email')
const deleteWorkspace = (data) => http.post('/api/v1/workspace/delete', data)
const getWorkspaceUsage = () => http.get('/api/v1/workspace/usage')
const getOnboarding = () => http.get('/api/v1/workspace/onboarding')
const dismissOnboarding = () => http.put('/api/v1/workspace/onboarding/dismiss')
const getPrivacySettings = () => http.get('/api/v1/settings/privacy')
const updatePrivacySettings = (data) => http.put('/api/v1/settings/privacy', data)
const getDataExports = () => http.get('/api/v1/data-exports')
const startDataExport = () => http.post('/api/v1/data-exports')
const getDomains = () => http.get('/api/v1/domains')
const createDomain = (data) => http.post('/api/v1/domains', data)
const verifyDomain = (id) => http.post(`/api/v1/domains/${id}/verify`)
const setPrimaryDomain = (domainID) => http.put('/api/v1/domains/primary', { domain_id: domainID })
const deleteDomain = (id) => http.delete(`/api/v1/domains/${id}`)
const updateEmailNotificationSettings = (data) =>
  http.put('/api/v1/settings/notifications/email', data)
const getPriorities = () => http.get('/api/v1/priorities')
const getStatuses = () => http.get('/api/v1/statuses')
const createStatus = (data) => http.post('/api/v1/statuses', data)
const updateStatus = (id, data) => http.put(`/api/v1/statuses/${id}`, data)
const deleteStatus = (id) => http.delete(`/api/v1/statuses/${id}`)
const createTag = (data) => http.post('/api/v1/tags', data)
const updateTag = (id, data) => http.put(`/api/v1/tags/${id}`, data)
const deleteTag = (id) => http.delete(`/api/v1/tags/${id}`)
const getTemplate = (id) => http.get(`/api/v1/templates/${id}`)
const getTemplates = (type) => http.get('/api/v1/templates', { params: { type: type } })
const createTemplate = (data) =>
  http.post('/api/v1/templates', data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const deleteTemplate = (id) => http.delete(`/api/v1/templates/${id}`)
const updateTemplate = (id, data) =>
  http.put(`/api/v1/templates/${id}`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })

const getAllBusinessHours = () => http.get('/api/v1/business-hours')
const getBusinessHours = (id) => http.get(`/api/v1/business-hours/${id}`)
const createBusinessHours = (data) =>
  http.post('/api/v1/business-hours', data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const updateBusinessHours = (id, data) =>
  http.put(`/api/v1/business-hours/${id}`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const deleteBusinessHours = (id) => http.delete(`/api/v1/business-hours/${id}`)

const getAllSLAs = () => http.get('/api/v1/sla')
const getSLA = (id) => http.get(`/api/v1/sla/${id}`)
const createSLA = (data) =>
  http.post('/api/v1/sla', data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const updateSLA = (id, data) =>
  http.put(`/api/v1/sla/${id}`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const deleteSLA = (id) => http.delete(`/api/v1/sla/${id}`)
const createOIDC = (data) =>
  http.post('/api/v1/oidc', data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const getConfig = () => http.get('/api/v1/config')
const getAllOIDC = () => http.get('/api/v1/oidc')
const getOIDC = (id) => http.get(`/api/v1/oidc/${id}`)
const updateOIDC = (id, data) =>
  http.put(`/api/v1/oidc/${id}`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const deleteOIDC = (id) => http.delete(`/api/v1/oidc/${id}`)
const updateSettings = (key, data) =>
  http.put(`/api/v1/settings/${key}`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const getSettings = (key) => http.get(`/api/v1/settings/${key}`)
const login = (data) => http.post(`/api/v1/auth/login`, data, {
  headers: {
    'Content-Type': 'application/json'
  }
})
const loginTwoFactor = (data) => http.post('/api/v1/auth/login/2fa', data)
const getTwoFactor = () => http.get('/api/v1/agents/me/2fa')
const setupTwoFactor = () => http.post('/api/v1/agents/me/2fa/setup')
const enableTwoFactor = (code) => http.post('/api/v1/agents/me/2fa/enable', { code })
const disableTwoFactor = (password) => http.post('/api/v1/agents/me/2fa/disable', { password })
const newRecoveryCodes = (password) => http.post('/api/v1/agents/me/2fa/recovery-codes', { password })
const resetAgentTwoFactor = (id) => http.delete(`/api/v1/agents/${id}/2fa`)
const getAutomationRules = (type) =>
  http.get(`/api/v1/automations/rules`, {
    params: { type: type }
  })
const toggleAutomationRule = (id) => http.put(`/api/v1/automations/rules/${id}/toggle`)
const getAutomationRule = (id) => http.get(`/api/v1/automations/rules/${id}`)
const updateAutomationRule = (id, data) =>
  http.put(`/api/v1/automations/rules/${id}`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const createAutomationRule = (data) =>
  http.post(`/api/v1/automations/rules`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const deleteAutomationRule = (id) => http.delete(`/api/v1/automations/rules/${id}`)
const updateAutomationRuleWeights = (data) =>
  http.put(`/api/v1/automations/rules/weights`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const updateAutomationRulesExecutionMode = (data) =>
  http.put(`/api/v1/automations/rules/execution-mode`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const getRoles = () => http.get('/api/v1/roles')
const getRole = (id) => http.get(`/api/v1/roles/${id}`)
const createRole = (data) =>
  http.post('/api/v1/roles', data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const updateRole = (id, data) =>
  http.put(`/api/v1/roles/${id}`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const deleteRole = (id) => http.delete(`/api/v1/roles/${id}`)
const getContacts = (params) => http.get('/api/v1/contacts', { params })
const getContact = (id) => http.get(`/api/v1/contacts/${id}`)
const updateContact = (id, data) =>
  http.put(`/api/v1/contacts/${id}`, data, {
    headers: {
      'Content-Type': 'multipart/form-data'
    }
  })
const blockContact = (id, data) => http.put(`/api/v1/contacts/${id}/block`, data, {
  headers: {
    'Content-Type': 'application/json'
  }
})
const deleteContact = (id) => http.delete(`/api/v1/contacts/${id}`)
const exportContact = (id) => http.get(`/api/v1/contacts/${id}/export`, { responseType: 'blob' })
const getTeam = (id) => http.get(`/api/v1/teams/${id}`)
const getTeams = () => http.get('/api/v1/teams')
const updateTeam = (id, data) => http.put(`/api/v1/teams/${id}`, data, {
  headers: {
    'Content-Type': 'application/json'
  }
})
const createTeam = (data) => http.post('/api/v1/teams', data, {
  headers: {
    'Content-Type': 'application/json'
  }
})
const getTeamsCompact = () => http.get('/api/v1/teams/compact')
const deleteTeam = (id) => http.delete(`/api/v1/teams/${id}`)
const updateUser = (id, data) =>
  http.put(`/api/v1/agents/${id}`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const getUsers = () => http.get('/api/v1/agents')
const getUsersCompact = () => http.get('/api/v1/agents/compact')
const updateCurrentUser = (data) =>
  http.put('/api/v1/agents/me', data, {
    headers: {
      'Content-Type': 'multipart/form-data'
    }
  })
const getUser = (id) => http.get(`/api/v1/agents/${id}`)
const deleteUserAvatar = () => http.delete('/api/v1/agents/me/avatar')
const getCurrentUser = () => http.get('/api/v1/agents/me')
const getCurrentUserTeams = () => http.get('/api/v1/agents/me/teams')
const updateCurrentUserAvailability = (data) => http.put('/api/v1/agents/me/availability', data, {
  headers: {
    'Content-Type': 'application/json'
  }
})
const resetPassword = (data) => http.post('/api/v1/agents/reset-password', data, {
  headers: {
    'Content-Type': 'application/json'
  }
})
const setPassword = (data) => http.post('/api/v1/agents/set-password', data, {
  headers: {
    'Content-Type': 'application/json'
  }
})
const deleteUser = (id) => http.delete(`/api/v1/agents/${id}`)
const importAgents = (data) =>
  http.post('/api/v1/agents/import', data, {
    headers: {
      'Content-Type': 'multipart/form-data'
    }
  })
const getAgentImportStatus = () => http.get('/api/v1/agents/import/status')
const createUser = (data) =>
  http.post('/api/v1/agents', data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const getTags = () => http.get('/api/v1/tags')
const importTags = (data) =>
  http.post('/api/v1/tags/import', data, {
    headers: {
      'Content-Type': 'multipart/form-data'
    }
  })
const getTagImportStatus = () => http.get('/api/v1/tags/import/status')
const upsertTags = (uuid, data) => http.post(`/api/v1/conversations/${uuid}/tags`, data, {
  headers: {
    'Content-Type': 'application/json'
  }
})
const updateAssignee = (uuid, assignee_type, data) =>
  http.put(`/api/v1/conversations/${uuid}/assignee/${assignee_type}`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const removeAssignee = (uuid, assignee_type) =>
  http.put(`/api/v1/conversations/${uuid}/assignee/${assignee_type}/remove`)
const updateContactCustomAttribute = (uuid, data) =>
  http.put(`/api/v1/conversations/${uuid}/contacts/custom-attributes`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const updateConversationCustomAttribute = (uuid, data) =>
  http.put(`/api/v1/conversations/${uuid}/custom-attributes`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const createConversation = (data) =>
  http.post('/api/v1/conversations', data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const updateConversationStatus = (uuid, data) =>
  http.put(`/api/v1/conversations/${uuid}/status`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const updateConversationPriority = (uuid, data) =>
  http.put(`/api/v1/conversations/${uuid}/priority`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const updateAssigneeLastSeen = (uuid) => http.put(`/api/v1/conversations/${uuid}/last-seen`)
const markConversationAsUnread = (uuid) => http.put(`/api/v1/conversations/${uuid}/mark-unread`)
const getConversationMessage = (cuuid, uuid) =>
  http.get(`/api/v1/conversations/${cuuid}/messages/${uuid}`)
const retryMessage = (cuuid, uuid) =>
  http.put(`/api/v1/conversations/${cuuid}/messages/${uuid}/retry`)
const deleteMessage = (cuuid, uuid) =>
  http.delete(`/api/v1/conversations/${cuuid}/messages/${uuid}`)
const getConversationMessages = (uuid, params) =>
  http.get(`/api/v1/conversations/${uuid}/messages`, { params, abortOnRoute: true })
const sendMessage = (uuid, data) =>
  http.post(`/api/v1/conversations/${uuid}/messages`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const getConversation = (uuid) => http.get(`/api/v1/conversations/${uuid}`, { abortOnRoute: true })
const getConversationTranscript = (uuid) =>
  http.get(`/api/v1/conversations/${uuid}/transcript`, { responseType: 'blob' })
const getContactPageVisits = (uuid) => http.get(`/api/v1/conversations/${uuid}/page-visits`, { abortOnRoute: true })
const getAllMacros = () => http.get('/api/v1/macros')
const getMacro = (id) => http.get(`/api/v1/macros/${id}`)
const createMacro = (data) =>
  http.post('/api/v1/macros', data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const updateMacro = (id, data) =>
  http.put(`/api/v1/macros/${id}`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const deleteMacro = (id) => http.delete(`/api/v1/macros/${id}`)
const applyMacro = (uuid, id, data) =>
  http.post(`/api/v1/conversations/${uuid}/macros/${id}/apply`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const getTeamUnassignedConversations = (teamID, params) =>
  http.get(`/api/v1/teams/${teamID}/conversations/unassigned`, { params, abortOnRoute: true })
const getAssignedConversations = (params) =>
  http.get('/api/v1/conversations/assigned', { params, abortOnRoute: true })
const getUnassignedConversations = (params) =>
  http.get('/api/v1/conversations/unassigned', { params, abortOnRoute: true })
const getAllConversations = (params) =>
  http.get('/api/v1/conversations/all', { params, abortOnRoute: true })
const getMentionedConversations = (params) =>
  http.get('/api/v1/conversations/mentioned', { params, abortOnRoute: true })
const getViewConversations = (id, params) =>
  http.get(`/api/v1/views/${id}/conversations`, { params, abortOnRoute: true })
const uploadMedia = (data) =>
  http.post('/api/v1/media', data, {
    headers: {
      'Content-Type': 'multipart/form-data'
    }
  })
const getOverviewCounts = () => http.get('/api/v1/reports/overview/counts')
const getOverviewCharts = (params) => http.get('/api/v1/reports/overview/charts', { params })
const getOverviewSLA = (params) => http.get('/api/v1/reports/overview/sla', { params })
const getOverviewCSAT = (params) => http.get('/api/v1/reports/overview/csat', { params })
const getOverviewMessageVolume = (params) => http.get('/api/v1/reports/overview/messages', { params })
const getOverviewTagDistribution = (params) => http.get('/api/v1/reports/overview/tags', { params })
const getLanguage = (lang) => http.get(`/api/v1/lang/${lang}`)
const getAvailableLanguages = () => http.get('/api/v1/lang')
const createInbox = (data) =>
  http.post('/api/v1/inboxes', data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const getInboxes = () => http.get('/api/v1/inboxes')
const getInbox = (id) => http.get(`/api/v1/inboxes/${id}`)

// Phone calls through Exotel.
const getVoiceSetup = (id) => http.get(`/api/v1/inboxes/${id}/voice/setup`)
const testVoice = (id) => http.post(`/api/v1/inboxes/${id}/voice/test`)
const getPhoneAgents = () => http.get('/api/v1/voice/agents')
const transferCall = (uuid, agentID, inboxID) =>
  http.post(`/api/v1/conversations/${uuid}/call/transfer`, { agent_id: agentID, inbox_id: inboxID }, {
    headers: { 'Content-Type': 'application/json' }
  })
const getCallStats = (days) => http.get('/api/v1/reports/calls', { params: { days } })
// Calls, newest first, each with its recording.
const getCallLog = (params) => http.get('/api/v1/reports/calls/log', { params })
// Calls a typed number, or a contact: the agent's phone rings first.
const dial = (data) =>
  http.post('/api/v1/voice/dial', data, { headers: { 'Content-Type': 'application/json' } })
const callContact = (id, inboxID) =>
  http.post(`/api/v1/contacts/${id}/call`, { inbox_id: inboxID }, {
    headers: { 'Content-Type': 'application/json' }
  })
const importContacts = (data) =>
  http.post('/api/v1/contacts/import', data, { headers: { 'Content-Type': 'multipart/form-data' } })
const getContactImportStatus = () => http.get('/api/v1/contacts/import/status')
// AI on calls and contacts: summarise a recorded call, fill a contact's
// empty fields from a conversation, and the workspace's switches for both.
const summarizeCall = (uuid, messageUUID) =>
  http.post(`/api/v1/conversations/${uuid}/messages/${messageUUID}/call-summary`)
const fillContactWithAI = (id, conversationUUID) =>
  http.post(`/api/v1/contacts/${id}/ai-fill`, { conversation_uuid: conversationUUID }, { timeout: AI_TIMEOUT })
const getAIFeatures = () => http.get('/api/v1/settings/ai-features')
// Sets up the AI admissions assistant, its knowledge drafts and contact fields.
const runAdmissionsSetup = (data) => http.post('/api/v1/admissions/setup', data, { timeout: AI_TIMEOUT })
const updateAIFeatures = (data) => http.put('/api/v1/settings/ai-features', data)
// The owner's daily report, and today's numbers.
const getDailyReportSettings = () => http.get('/api/v1/settings/daily-report')
const updateDailyReportSettings = (data) => http.put('/api/v1/settings/daily-report', data)
const testDailyReport = (data) => http.post('/api/v1/settings/daily-report/test', data, { timeout: 60000 })
const getTodaySummary = () => http.get('/api/v1/reports/today')
// Lead stages, the leads board and a contact's stage.
const getLeadStages = () => http.get('/api/v1/leads/stages')
const getLeadSettings = () => http.get('/api/v1/leads/settings')
const updateLeadSettings = (data) => http.put('/api/v1/leads/settings', data)
const getLeadBoard = (params) => http.get('/api/v1/leads/board', { params })
const getContactLead = (id) => http.get(`/api/v1/contacts/${id}/lead`)
const setContactLead = (id, stage) => http.put(`/api/v1/contacts/${id}/lead`, { stage })
// Missed calls to return: waiting ones by due time, or done ones.
const getCallBacks = (params) => http.get('/api/v1/call-backs', { params })
const getCallBackCounts = () => http.get('/api/v1/call-backs/counts')
const markCallBackDone = (id) => http.put(`/api/v1/call-backs/${id}/done`)
// Calls a conversation's contact: the agent's phone rings first.
const startCall = (uuid, inboxID) =>
  http.post(`/api/v1/conversations/${uuid}/call`, { inbox_id: inboxID }, {
    headers: { 'Content-Type': 'application/json' }
  })
// Creates a Telegram inbox from a bot token.
const connectTelegram = (data) =>
  http.post('/api/v1/inboxes/telegram', data, { headers: { 'Content-Type': 'application/json' } })
const getTelegramWebhook = (id) => http.get(`/api/v1/inboxes/${id}/telegram/webhook`)
const setTelegramWebhook = (id) => http.post(`/api/v1/inboxes/${id}/telegram/webhook`)
// Starts connecting WhatsApp, Messenger or Instagram; returns the Facebook Login URL.
const metaAuthorize = (product) => http.post(`/api/v1/inboxes/meta/${product}/authorize`)
// A WhatsApp inbox's approved templates; refresh skips the server's short cache.
const getWhatsAppTemplates = (inboxID, refresh = false) =>
  http.get(`/api/v1/inboxes/${inboxID}/whatsapp/templates`, { params: refresh ? { refresh: true } : {} })
// Every template on a WhatsApp inbox's business account, with review status.
const listManagedTemplates = (inboxID) => http.get(`/api/v1/inboxes/${inboxID}/whatsapp/templates/manage`)
const createWhatsAppTemplate = (inboxID, data) =>
  http.post(`/api/v1/inboxes/${inboxID}/whatsapp/templates`, data, {
    headers: { 'Content-Type': 'application/json' }
  })
const deleteWhatsAppTemplate = (inboxID, name, id) =>
  http.delete(`/api/v1/inboxes/${inboxID}/whatsapp/templates/${encodeURIComponent(name)}`, {
    params: id ? { id } : {}
  })
// Imports a Messenger or Instagram inbox's past conversations in the background.
const importMetaHistory = (inboxID, data) =>
  http.post(`/api/v1/inboxes/${inboxID}/meta/import`, data, {
    headers: { 'Content-Type': 'application/json' }
  })
const getMetaHistoryImport = (inboxID) => http.get(`/api/v1/inboxes/${inboxID}/meta/import`)
// A WhatsApp number's quality rating, messaging limit and status with Meta.
const getWhatsAppHealth = (inboxID) => http.get(`/api/v1/inboxes/${inboxID}/whatsapp/health`)
// Whether a WhatsApp conversation can take free text or needs a template.
const getReplyWindow = (uuid) => http.get(`/api/v1/conversations/${uuid}/reply-window`)
const toggleInbox = (id) => http.put(`/api/v1/inboxes/${id}/toggle`)
const updateInbox = (id, data) =>
  http.put(`/api/v1/inboxes/${id}`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const deleteInbox = (id) => http.delete(`/api/v1/inboxes/${id}`)
const saveDraft = (uuid, type, data) =>
  http.post(`/api/v1/conversations/${uuid}/draft`, { ...data, type }, {
    headers: {
      'Content-Type': 'application/json'
    }
  })

const getAllDrafts = () => http.get('/api/v1/drafts')

const deleteDraft = (uuid, type) =>
  http.delete(`/api/v1/conversations/${uuid}/draft`, { params: { type } })
const getCurrentUserViews = () => http.get('/api/v1/views/me')
const createView = (data) =>
  http.post('/api/v1/views/me', data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const updateView = (id, data) =>
  http.put(`/api/v1/views/me/${id}`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const deleteView = (id) => http.delete(`/api/v1/views/me/${id}`)

const getSharedViews = () => http.get('/api/v1/views/shared')
const getAllSharedViews = () => http.get('/api/v1/shared-views')
const getSharedView = (id) => http.get(`/api/v1/shared-views/${id}`)
const createSharedView = (data) =>
  http.post('/api/v1/shared-views', data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const updateSharedView = (id, data) =>
  http.put(`/api/v1/shared-views/${id}`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const deleteSharedView = (id) => http.delete(`/api/v1/shared-views/${id}`)

const getAiPrompts = () => http.get('/api/v1/ai/prompts')
const aiCompletion = (data) => http.post('/api/v1/ai/completion', data, {
  timeout: AI_TIMEOUT,
  headers: {
    'Content-Type': 'application/json'
  }
})
const getAIConfig = (type) => http.get(`/api/v1/ai/config/${type}`)
const updateAIConfig = (type, data) => http.put(`/api/v1/ai/config/${type}`, data)
const testAIConfig = (type, data) =>
  http.post(`/api/v1/ai/config/${type}/test`, data, { timeout: AI_TIMEOUT })
const getAITools = () => http.get('/api/v1/ai/tools')
const getAITool = (id) => http.get(`/api/v1/ai/tools/${id}`)
const createAITool = (data) => http.post('/api/v1/ai/tools', data)
const updateAITool = (id, data) => http.put(`/api/v1/ai/tools/${id}`, data)
const deleteAITool = (id) => http.delete(`/api/v1/ai/tools/${id}`)
const getAIAssistants = () => http.get('/api/v1/ai/assistants')
const getAIAssistantsCompact = () => http.get('/api/v1/ai/assistants/compact')
const getAIAssistant = (id) => http.get(`/api/v1/ai/assistants/${id}`)
const createAIAssistant = (data) =>
  http.post('/api/v1/ai/assistants', data, {
    headers: { 'Content-Type': 'multipart/form-data' }
  })
const updateAIAssistant = (id, data) =>
  http.put(`/api/v1/ai/assistants/${id}`, data, {
    headers: { 'Content-Type': 'multipart/form-data' }
  })
const deleteAIAssistant = (id) => http.delete(`/api/v1/ai/assistants/${id}`)
const previewAIAssistant = (id, data) =>
  http.post(`/api/v1/ai/assistants/${id}/preview`, data, { timeout: AI_TIMEOUT })
const getAIAssistantStats = (id, range) =>
  http.get(`/api/v1/ai/assistants/${id}/stats`, { params: range ? { range } : {} })
const getAISnippets = () => http.get('/api/v1/ai/snippets')
const createAISnippet = (data) => http.post('/api/v1/ai/snippets', data)
const importAISnippetFromURL = (data) =>
  http.post('/api/v1/ai/snippets/import-url', data, { timeout: AI_TIMEOUT })
const updateAISnippet = (id, data) => http.put(`/api/v1/ai/snippets/${id}`, data)
const deleteAISnippet = (id) => http.delete(`/api/v1/ai/snippets/${id}`)
const getAIFaqSuggestions = (status) =>
  http.get('/api/v1/ai/faq-suggestions', { params: status ? { status } : {} })
const approveAIFaqSuggestion = (id, data) =>
  http.post(`/api/v1/ai/faq-suggestions/${id}/approve`, data)
const rejectAIFaqSuggestion = (id) => http.post(`/api/v1/ai/faq-suggestions/${id}/reject`)
const getAIFaqLearning = () => http.get('/api/v1/ai/faq-learning')
const updateAIFaqLearning = (data) => http.put('/api/v1/ai/faq-learning', data)
const aiGenerateReply = (data) => http.post('/api/v1/ai/generate-reply', data, { timeout: AI_TIMEOUT })
const aiSummarizeConversation = (data) => http.post('/api/v1/ai/summarize', data, { timeout: AI_TIMEOUT })
const aiSuggestTags = (data) => http.post('/api/v1/ai/suggest-tags', data, { timeout: AI_TIMEOUT })
const aiCopilot = (data) => http.post('/api/v1/ai/copilot', data, { timeout: AI_TIMEOUT })
const getCopilotMessages = (conversationUUID) =>
  http.get('/api/v1/ai/copilot/messages', { params: { conversation_uuid: conversationUUID } })
const clearCopilotMessages = (conversationUUID) =>
  http.delete('/api/v1/ai/copilot/messages', { params: { conversation_uuid: conversationUUID } })
const aiSLACopilot = (data) => http.post('/api/v1/ai/sla-copilot', data, { timeout: AI_TIMEOUT })
const getSLACopilotMessages = () => http.get('/api/v1/ai/sla-copilot/messages')
const clearSLACopilotMessages = () => http.delete('/api/v1/ai/sla-copilot/messages')
const getHelpCenters = () => http.get('/api/v1/help-centers')
const getHelpCenterLocales = () => http.get('/api/v1/help-centers/locales')
const getHelpCenter = (id) => http.get(`/api/v1/help-centers/${id}`)
const createHelpCenter = (data) => http.post('/api/v1/help-centers', data)
const updateHelpCenter = (id, data) => http.put(`/api/v1/help-centers/${id}`, data)
const deleteHelpCenter = (id) => http.delete(`/api/v1/help-centers/${id}`)
const toggleHelpCenter = (id) => http.put(`/api/v1/help-centers/${id}/toggle`)
const previewHelpCenter = (id, data, page) =>
  http.post(`/api/v1/help-centers/${id}/preview`, data, {
    responseType: 'text',
    params: page ? { page } : {}
  })
const getHelpCenterTree = (id, locale) =>
  http.get(`/api/v1/help-centers/${id}/tree`, { params: locale ? { locale } : {} })
const getCollections = (helpCenterId) => http.get(`/api/v1/help-centers/${helpCenterId}/collections`)
const createCollection = (helpCenterId, data) =>
  http.post(`/api/v1/help-centers/${helpCenterId}/collections`, data)
const updateCollection = (helpCenterId, id, data) =>
  http.put(`/api/v1/help-centers/${helpCenterId}/collections/${id}`, data)
const deleteCollection = (helpCenterId, id) =>
  http.delete(`/api/v1/help-centers/${helpCenterId}/collections/${id}`)
const toggleCollection = (id) => http.put(`/api/v1/collections/${id}/toggle`)
const updateCollectionSortOrders = (helpCenterId, data) =>
  http.put(`/api/v1/help-centers/${helpCenterId}/collection-sort-order`, data)
const moveArticleToCollection = (id, data) => http.put(`/api/v1/articles/${id}/collection`, data)
const updateArticleSortOrders = (collectionId, data) =>
  http.put(`/api/v1/collections/${collectionId}/article-sort-order`, data)
const getArticle = (collectionId, id) =>
  http.get(`/api/v1/collections/${collectionId}/articles/${id}`)
const createArticle = (collectionId, data) =>
  http.post(`/api/v1/collections/${collectionId}/articles`, data)
const updateArticle = (id, data) => http.put(`/api/v1/articles/${id}`, data)
const deleteArticle = (collectionId, id) =>
  http.delete(`/api/v1/collections/${collectionId}/articles/${id}`)
const updateArticleStatus = (id, data) => http.put(`/api/v1/articles/${id}/status`, data)
const getHelpCenterInsights = (id) => http.get(`/api/v1/help-centers/${id}/insights`)
const getContactNotes = (id) => http.get(`/api/v1/contacts/${id}/notes`)
const createContactNote = (id, data) => http.post(`/api/v1/contacts/${id}/notes`, data, {
  headers: {
    'Content-Type': 'application/json'
  }
})
const deleteContactNote = (id, noteId) => http.delete(`/api/v1/contacts/${id}/notes/${noteId}`)
const getActivityLogs = (params) => http.get('/api/v1/activity-logs', { params })
const getWebhooksCompact = () => http.get('/api/v1/webhooks/compact')
const getWebhooks = () => http.get('/api/v1/webhooks')
const getWebhook = (id) => http.get(`/api/v1/webhooks/${id}`)
const createWebhook = (data) =>
  http.post('/api/v1/webhooks', data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const updateWebhook = (id, data) =>
  http.put(`/api/v1/webhooks/${id}`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const deleteWebhook = (id) => http.delete(`/api/v1/webhooks/${id}`)
const toggleWebhook = (id) => http.put(`/api/v1/webhooks/${id}/toggle`)
const testWebhook = (id) => http.post(`/api/v1/webhooks/${id}/test`)

const getChatSetup = () => http.get('/api/v1/integrations/chat/setup')
const getChatIntegrations = () => http.get('/api/v1/integrations/chat')
const updateChatIntegration = (id, data) =>
  http.put(`/api/v1/integrations/chat/${id}`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
const deleteChatIntegration = (id) => http.delete(`/api/v1/integrations/chat/${id}`)
const testChatIntegration = (id) => http.post(`/api/v1/integrations/chat/${id}/test`)
const authorizeSlack = () => http.post('/api/v1/integrations/slack/authorize')
const getTeamsPackage = () => http.get('/api/v1/integrations/teams/package', { responseType: 'blob' })
const createTeamsCode = () => http.post('/api/v1/integrations/teams/code')

const getContextLinks = () => http.get('/api/v1/context-links')
const getContextLink = (id) => http.get(`/api/v1/context-links/${id}`)
const createContextLink = (data) =>
  http.post('/api/v1/context-links', data, {
    headers: { 'Content-Type': 'application/json' }
  })
const updateContextLink = (id, data) =>
  http.put(`/api/v1/context-links/${id}`, data, {
    headers: { 'Content-Type': 'application/json' }
  })
const deleteContextLink = (id) => http.delete(`/api/v1/context-links/${id}`)
const toggleContextLink = (id) => http.put(`/api/v1/context-links/${id}/toggle`)
const getActiveContextLinks = () => http.get('/api/v1/context-links/active')
const getContextLinkURL = (id, conversationUUID) =>
  http.get(`/api/v1/context-links/${id}/url`, { params: { conversation_uuid: conversationUUID } })

const generateAPIKey = (id) => 
  http.post(`/api/v1/agents/${id}/api-key`, {}, {
    headers: {
      'Content-Type': 'application/json'
    }
  })

const revokeAPIKey = (id) => http.delete(`/api/v1/agents/${id}/api-key`)

const initiateOAuthFlow = (provider, data) =>
  http.post(`/api/v1/inboxes/oauth/${provider}/authorize`, data, {
    headers: {
      'Content-Type': 'application/json'
    }
  })

// User notifications (in-app)
const getNotifications = (params) => http.get('/api/v1/notifications', { params })
const getNotificationStats = () => http.get('/api/v1/notifications/stats')
const markNotificationAsRead = (id) => http.put(`/api/v1/notifications/${id}/read`)
const markAllNotificationsAsRead = () => http.put('/api/v1/notifications/read-all')
const deleteNotification = (id) => http.delete(`/api/v1/notifications/${id}`)
const deleteAllNotifications = () => http.delete('/api/v1/notifications')

export default {
  loginTwoFactor,
  getTwoFactor,
  setupTwoFactor,
  enableTwoFactor,
  disableTwoFactor,
  newRecoveryCodes,
  resetAgentTwoFactor,
  login,
  deleteUser,
  importAgents,
  getAgentImportStatus,
  importTags,
  getTagImportStatus,
  resetPassword,
  setPassword,
  getTags,
  getTeam,
  getUser,
  getRoles,
  getRole,
  createRole,
  deleteRole,
  updateRole,
  getTeams,
  deleteTeam,
  getUsers,
  getInbox,
  metaAuthorize,
  connectTelegram,
  getVoiceSetup,
  testVoice,
  startCall,
  dial,
  callContact,
  importContacts,
  getContactImportStatus,
  getPhoneAgents,
  transferCall,
  getCallStats,
  getCallBacks,
  getLeadStages,
  getDailyReportSettings,
  summarizeCall,
  fillContactWithAI,
  getAIFeatures,
  runAdmissionsSetup,
  updateAIFeatures,
  updateDailyReportSettings,
  testDailyReport,
  getTodaySummary,
  getLeadSettings,
  updateLeadSettings,
  getLeadBoard,
  getContactLead,
  setContactLead,
  getCallBackCounts,
  markCallBackDone,
  getCallLog,
  getTelegramWebhook,
  setTelegramWebhook,
  getWhatsAppTemplates,
  getWhatsAppHealth,
  importMetaHistory,
  getMetaHistoryImport,
  listManagedTemplates,
  createWhatsAppTemplate,
  deleteWhatsAppTemplate,
  getReplyWindow,
  getInboxes,
  getLanguage,
  getAvailableLanguages,
  getConversation,
  getAutomationRule,
  getAutomationRules,
  getAllBusinessHours,
  getBusinessHours,
  createBusinessHours,
  updateBusinessHours,
  deleteBusinessHours,
  getAllSLAs,
  getSLA,
  createSLA,
  updateSLA,
  deleteSLA,
  getAssignedConversations,
  getUnassignedConversations,
  getAllConversations,
  getMentionedConversations,
  getTeamUnassignedConversations,
  getViewConversations,
  getOverviewCharts,
  getOverviewCounts,
  getOverviewSLA,
  getOverviewCSAT,
  getOverviewMessageVolume,
  getOverviewTagDistribution,
  getConversationMessage,
  getConversationMessages,
  getConversationTranscript,
  getCurrentUser,
  getCurrentUserTeams,
  getAllMacros,
  getMacro,
  createMacro,
  updateMacro,
  deleteMacro,
  applyMacro,
  updateCurrentUser,
  updateAssignee,
  updateConversationStatus,
  updateConversationPriority,
  upsertTags,
  updateConversationCustomAttribute,
  updateContactCustomAttribute,
  uploadMedia,
  updateAssigneeLastSeen,
  markConversationAsUnread,
  updateUser,
  updateCurrentUserAvailability,
  updateAutomationRule,
  updateAutomationRuleWeights,
  updateAutomationRulesExecutionMode,
  createAutomationRule,
  toggleAutomationRule,
  deleteAutomationRule,
  createConversation,
  sendMessage,
  retryMessage,
  deleteMessage,
  createUser,
  createInbox,
  updateInbox,
  deleteInbox,
  toggleInbox,
  createTeam,
  updateTeam,
  getSettings,
  updateSettings,
  createOIDC,
  getAllOIDC,
  getConfig,
  getOIDC,
  updateOIDC,
  deleteOIDC,
  getTemplate,
  getTemplates,
  createTemplate,
  updateTemplate,
  deleteTemplate,
  deleteUserAvatar,
  createTag,
  updateTag,
  deleteTag,
  getStatuses,
  getPriorities,
  createStatus,
  updateStatus,
  deleteStatus,
  getTeamsCompact,
  getUsersCompact,
  getEmailNotificationSettings,
  deleteWorkspace,
  getWorkspaceUsage,
  getOnboarding,
  dismissOnboarding,
  getPrivacySettings,
  updatePrivacySettings,
  getDataExports,
  startDataExport,
  getDomains,
  createDomain,
  verifyDomain,
  setPrimaryDomain,
  deleteDomain,
  updateEmailNotificationSettings,
  saveDraft,
  getAllDrafts,
  deleteDraft,
  getCurrentUserViews,
  createView,
  updateView,
  deleteView,
  getSharedViews,
  getAllSharedViews,
  getSharedView,
  createSharedView,
  updateSharedView,
  deleteSharedView,
  getAiPrompts,
  aiCompletion,
  getAIConfig,
  updateAIConfig,
  testAIConfig,
  getAITools,
  getAITool,
  createAITool,
  updateAITool,
  deleteAITool,
  getAIAssistants,
  getAIAssistantsCompact,
  getAIAssistant,
  createAIAssistant,
  updateAIAssistant,
  deleteAIAssistant,
  previewAIAssistant,
  getAIAssistantStats,
  getAISnippets,
  createAISnippet,
  importAISnippetFromURL,
  updateAISnippet,
  deleteAISnippet,
  getHelpCenters,
  getHelpCenterLocales,
  getHelpCenter,
  createHelpCenter,
  updateHelpCenter,
  deleteHelpCenter,
  toggleHelpCenter,
  previewHelpCenter,
  getHelpCenterTree,
  getCollections,
  createCollection,
  updateCollection,
  deleteCollection,
  toggleCollection,
  updateCollectionSortOrders,
  updateArticleSortOrders,
  moveArticleToCollection,
  getArticle,
  createArticle,
  updateArticle,
  deleteArticle,
  updateArticleStatus,
  getHelpCenterInsights,
  getAIFaqSuggestions,
  approveAIFaqSuggestion,
  rejectAIFaqSuggestion,
  getAIFaqLearning,
  updateAIFaqLearning,
  aiGenerateReply,
  aiSummarizeConversation,
  aiSuggestTags,
  aiCopilot,
  getCopilotMessages,
  clearCopilotMessages,
  aiSLACopilot,
  getSLACopilotMessages,
  clearSLACopilotMessages,
  searchConversations,
  searchMessages,
  searchContacts,
  removeAssignee,
  getContacts,
  getContact,
  updateContact,
  blockContact,
  deleteContact,
  exportContact,
  getCustomAttributes,
  createCustomAttribute,
  updateCustomAttribute,
  deleteCustomAttribute,
  getCustomAttribute,
  getContactNotes,
  createContactNote,
  deleteContactNote,
  getActivityLogs,
  getWebhooksCompact,
  getChatSetup,
  getChatIntegrations,
  updateChatIntegration,
  deleteChatIntegration,
  testChatIntegration,
  authorizeSlack,
  getTeamsPackage,
  createTeamsCode,
  getWebhooks,
  getWebhook,
  createWebhook,
  updateWebhook,
  deleteWebhook,
  toggleWebhook,
  testWebhook,
  getContextLinks,
  getContextLink,
  createContextLink,
  updateContextLink,
  deleteContextLink,
  toggleContextLink,
  getActiveContextLinks,
  getContextLinkURL,
  generateAPIKey,
  revokeAPIKey,
  initiateOAuthFlow,
  getNotifications,
  getNotificationStats,
  markNotificationAsRead,
  markAllNotificationsAsRead,
  deleteNotification,
  deleteAllNotifications,
  getContactPageVisits
}
