<template>
  <!-- Browser calling: the SIP account Exotel issued for this agent, which
       their browser signs in to so calls ring there. Kept apart from the
       agent form, which it sits inside, so it saves on its own. -->
  <div class="bg-muted/30 box p-4 space-y-4">
    <div>
      <p class="text-base font-semibold text-foreground">{{ $t('softphone.title') }}</p>
      <p class="text-sm text-muted-foreground">{{ $t('softphone.admin.description') }}</p>
    </div>

    <div class="grid gap-4 sm:grid-cols-2">
      <div class="space-y-1">
        <Label for="sip-username">{{ $t('softphone.admin.username') }}</Label>
        <Input id="sip-username" v-model.trim="form.username" autocomplete="off" placeholder="asha01" />
      </div>
      <div class="space-y-1">
        <Label for="sip-password">{{ $t('softphone.admin.password') }}</Label>
        <Input
          id="sip-password"
          v-model="form.password"
          type="password"
          autocomplete="new-password"
          :placeholder="configured ? $t('softphone.admin.passwordKept') : ''"
        />
      </div>
      <div class="space-y-1">
        <Label for="sip-domain">{{ $t('softphone.admin.domain') }}</Label>
        <Input id="sip-domain" v-model.trim="form.domain" autocomplete="off" placeholder="yourcompany.voip.exotel.com" />
      </div>
      <div class="space-y-1">
        <Label for="sip-server">
          {{ $t('softphone.admin.server') }} ({{ $t('globals.terms.optional') }})
        </Label>
        <Input id="sip-server" v-model.trim="form.server" autocomplete="off" placeholder="voip.in1.exotel.com:443" />
      </div>
    </div>
    <p class="text-xs text-muted-foreground">{{ $t('softphone.admin.hint') }}</p>

    <div class="flex items-center gap-2">
      <Button type="button" size="sm" :is-loading="saving" :disabled="!canSave" @click="save">
        {{ $t('globals.messages.save') }}
      </Button>
      <Button v-if="configured" type="button" size="sm" variant="outline" :disabled="saving" @click="remove">
        {{ $t('softphone.admin.remove') }}
      </Button>
      <span v-if="configured" class="ml-auto text-xs text-muted-foreground">
        {{ ringBrowser ? $t('softphone.admin.ringsBrowser') : $t('softphone.admin.ringsPhone') }}
      </span>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'
import { useEmitter } from '@/composables/useEmitter'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'

const props = defineProps({
  agentId: { type: Number, required: true }
})

const { t } = useI18n()
const emitter = useEmitter()
const form = reactive({ username: '', password: '', domain: '', server: '' })
const configured = ref(false)
const ringBrowser = ref(false)
const saving = ref(false)

const canSave = computed(() => form.username && form.domain && (configured.value || form.password))

// The saved account, without its password, which stays on the server.
const fill = (data) => {
  configured.value = !!data?.configured
  ringBrowser.value = !!data?.ring_browser
  form.username = data?.username || ''
  form.domain = data?.domain || ''
  form.server = data?.server ? `${data.server}:${data.port}` : data?.port && data.port !== 443 ? `${data.domain}:${data.port}` : ''
  form.password = ''
}

onMounted(async () => {
  try {
    const resp = await api.getAgentSoftphone(props.agentId)
    fill(resp.data.data)
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  }
})

const save = async () => {
  try {
    saving.value = true
    const resp = await api.updateAgentSoftphone(props.agentId, { ...form })
    fill(resp.data.data)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('globals.messages.savedSuccessfully') })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  } finally {
    saving.value = false
  }
}

const remove = async () => {
  try {
    saving.value = true
    await api.deleteAgentSoftphone(props.agentId)
    fill(null)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('softphone.admin.removed') })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  } finally {
    saving.value = false
  }
}
</script>
