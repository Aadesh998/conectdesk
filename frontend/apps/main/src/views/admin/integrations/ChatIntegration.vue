<template>
  <AdminSplitLayout>
    <template #content>
      <LoadingOverlay :loading="loading" reserve-height>
        <div v-if="setup" class="space-y-6">
          <!-- The platform's app isn't set up: say so, with what the operator needs. -->
          <div v-if="!configured" class="rounded-md border border-border bg-card p-5 space-y-3">
            <div class="font-medium">{{ $t(`admin.chat.${provider}.notConfigured`) }}</div>
            <p class="text-sm text-muted-foreground">{{ $t(`admin.chat.${provider}.notConfiguredHelp`) }}</p>
            <dl class="space-y-2 text-sm">
              <div v-for="row in setupUrls" :key="row.label">
                <dt class="text-muted-foreground">{{ row.label }}</dt>
                <dd class="font-mono text-xs break-all">{{ row.value }}</dd>
              </div>
            </dl>
          </div>

          <!-- Slack: install the app, picking the channel in Slack. -->
          <div v-else-if="provider === 'slack'" class="rounded-md border border-border bg-card p-5 space-y-3">
            <p class="text-sm text-muted-foreground">{{ $t('admin.chat.slack.connectHelp') }}</p>
            <Button :is-loading="connecting" @click="connectSlack">
              {{ $t('admin.chat.slack.connect') }}
            </Button>
          </div>

          <!-- Teams: add the app to a team, then link a channel with a code. -->
          <div v-else class="rounded-md border border-border bg-card p-5 space-y-5">
            <ol class="space-y-4 text-sm">
              <li class="space-y-2">
                <div class="font-medium">1. {{ $t('admin.chat.teams.step1') }}</div>
                <p class="text-muted-foreground">{{ $t('admin.chat.teams.step1Help') }}</p>
                <Button variant="outline" size="sm" :is-loading="downloading" @click="downloadPackage">
                  {{ $t('admin.chat.teams.download') }}
                </Button>
              </li>
              <li class="space-y-2">
                <div class="font-medium">2. {{ $t('admin.chat.teams.step2') }}</div>
                <p class="text-muted-foreground">{{ $t('admin.chat.teams.step2Help') }}</p>
              </li>
              <li class="space-y-2">
                <div class="font-medium">3. {{ $t('admin.chat.teams.step3') }}</div>
                <p class="text-muted-foreground">{{ $t('admin.chat.teams.step3Help') }}</p>
                <div v-if="code" class="flex flex-wrap items-center gap-2">
                  <code class="rounded bg-muted px-3 py-2 font-mono text-sm">{{ code.command }}</code>
                  <Button variant="outline" size="sm" @click="copyCommand">
                    {{ $t('admin.chat.teams.copy') }}
                  </Button>
                  <span class="text-xs text-muted-foreground">{{ $t('admin.chat.teams.codeExpires') }}</span>
                </div>
                <Button v-else size="sm" :is-loading="creatingCode" @click="createCode">
                  {{ $t('admin.chat.teams.getCode') }}
                </Button>
              </li>
            </ol>
          </div>

          <div class="space-y-3">
            <h3 class="font-medium">{{ $t('admin.chat.connected') }}</h3>
            <p v-if="connections.length === 0" class="text-sm text-muted-foreground">
              {{ $t(`admin.chat.${provider}.none`) }}
            </p>
            <ChatConnectionCard
              v-for="c in connections"
              :key="c.id + ':' + c.updated_at"
              :connection="c"
              :events="setup.events"
              @changed="loadConnections"
            />
          </div>
        </div>
      </LoadingOverlay>
    </template>

    <template #help>
      <div class="space-y-3">
        <p>{{ $t(`admin.chat.${provider}.help`) }}</p>
        <ul class="list-disc space-y-1 pl-4">
          <li>{{ $t(`admin.chat.${provider}.helpActions`) }}</li>
          <li>{{ $t(`admin.chat.${provider}.helpThread`) }}</li>
          <li>{{ $t('admin.chat.helpEmail') }}</li>
        </ul>
      </div>
    </template>
  </AdminSplitLayout>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AdminSplitLayout from '@/layouts/admin/AdminSplitLayout.vue'
import LoadingOverlay from '@/components/layout/LoadingOverlay.vue'
import { Button } from '@shared-ui/components/ui/button'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import ChatConnectionCard from '@/features/admin/integrations/ChatConnectionCard.vue'
import api from '@/api'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'

const props = defineProps({
  provider: { type: String, required: true }
})

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const emitter = useEmitter()

const loading = ref(true)
const setup = ref(null)
const all = ref([])
const connecting = ref(false)
const downloading = ref(false)
const creatingCode = ref(false)
const code = ref(null)
let poll = null

const configured = computed(() => setup.value?.[props.provider]?.configured)
const connections = computed(() => all.value.filter((c) => c.provider === props.provider))

const setupUrls = computed(() => {
  const s = setup.value?.[props.provider] || {}
  if (props.provider === 'slack') {
    return [
      { label: t('admin.chat.slack.redirectUrl'), value: s.redirect_url },
      { label: t('admin.chat.slack.eventsUrl'), value: s.events_url },
      { label: t('admin.chat.slack.interactionsUrl'), value: s.interactions_url }
    ]
  }
  return [{ label: t('admin.chat.teams.endpoint'), value: s.messaging_endpoint }]
})

const toast = (description, variant = 'success') =>
  emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant, description })

const loadConnections = async () => {
  const resp = await api.getChatIntegrations()
  all.value = resp.data.data || []
}

const connectSlack = async () => {
  connecting.value = true
  try {
    const resp = await api.authorizeSlack()
    window.location.href = resp.data.data
  } catch (error) {
    toast(handleHTTPError(error).message, 'destructive')
    connecting.value = false
  }
}

const downloadPackage = async () => {
  downloading.value = true
  try {
    const resp = await api.getTeamsPackage()
    const url = URL.createObjectURL(resp.data)
    const a = document.createElement('a')
    a.href = url
    a.download = 'conectdesk-teams.zip'
    a.click()
    URL.revokeObjectURL(url)
  } catch (error) {
    toast(handleHTTPError(error).message, 'destructive')
  } finally {
    downloading.value = false
  }
}

// Watches for the channel the code is sent in to show up.
const createCode = async () => {
  creatingCode.value = true
  try {
    const resp = await api.createTeamsCode()
    code.value = resp.data.data
    const known = connections.value.length
    clearInterval(poll)
    poll = setInterval(async () => {
      await loadConnections()
      if (connections.value.length > known) {
        clearInterval(poll)
        code.value = null
        toast(t('admin.chat.teams.linked'))
      }
    }, 4000)
    setTimeout(() => clearInterval(poll), code.value.expires_in * 1000)
  } catch (error) {
    toast(handleHTTPError(error).message, 'destructive')
  } finally {
    creatingCode.value = false
  }
}

const copyCommand = async () => {
  try {
    await navigator.clipboard.writeText(code.value.command)
    toast(t('admin.chat.teams.copied'))
  } catch {
    // The command is on screen to copy by hand.
  }
}

// The Slack install comes back here with how it went.
const showInstallResult = () => {
  const { success, error, invite } = route.query
  if (!success && !error) return
  if (success) {
    toast(invite ? t('admin.chat.slack.connectedInvite') : t('admin.chat.slack.connected'))
  } else {
    toast(t(`admin.chat.slack.error.${error}`), 'destructive')
  }
  router.replace({ query: {} })
}

onMounted(async () => {
  try {
    const [s] = await Promise.all([api.getChatSetup(), loadConnections()])
    setup.value = s.data.data
    showInstallResult()
  } catch (error) {
    toast(handleHTTPError(error).message, 'destructive')
  } finally {
    loading.value = false
  }
})

onUnmounted(() => clearInterval(poll))
</script>
