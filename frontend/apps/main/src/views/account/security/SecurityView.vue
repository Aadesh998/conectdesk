<template>
  <div class="h-full max-w-xl">
    <div class="flex flex-col space-y-5">
      <div class="space-y-1">
        <span class="sub-title">{{ $t('account.twoFactor.title') }}</span>
        <p class="text-muted-foreground text-sm">{{ $t('account.twoFactor.description') }}</p>
      </div>

      <div
        v-if="status.required && !status.enabled"
        class="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
      >
        {{ $t('auth.twoFactorSetupRequired') }}
      </div>

      <!-- Recovery codes, shown once. -->
      <div v-if="recoveryCodes.length" class="space-y-3 rounded-lg border p-4">
        <span class="font-medium">{{ $t('account.twoFactor.recoveryTitle') }}</span>
        <p class="text-sm text-muted-foreground">{{ $t('account.twoFactor.recoveryHelp') }}</p>
        <div class="grid grid-cols-2 gap-2 rounded-md bg-muted p-3 font-mono text-sm">
          <span v-for="c in recoveryCodes" :key="c">{{ c }}</span>
        </div>
        <div class="flex gap-2">
          <Button variant="outline" size="sm" @click="copyCodes">{{ $t('globals.terms.copy') }}</Button>
          <Button size="sm" @click="recoveryCodesSaved">{{ $t('account.twoFactor.saved') }}</Button>
        </div>
      </div>

      <template v-else-if="status.enabled">
        <p class="text-sm">
          <span class="font-medium text-success">{{ $t('account.twoFactor.on', { date: formatDate(status.enabled_at) }) }}</span>
          · {{ $t('account.twoFactor.recoveryLeft', { count: status.recovery_codes_left }) }}
        </p>
        <div class="space-y-2">
          <Input
            v-model="password"
            type="password"
            autocomplete="current-password"
            :placeholder="$t('account.twoFactor.password')"
            class="max-w-xs"
          />
          <div class="flex gap-2">
            <Button variant="outline" size="sm" :disabled="!password || busy" @click="newCodes">
              {{ $t('account.twoFactor.newCodes') }}
            </Button>
            <Button
              v-if="!status.required"
              variant="destructive"
              size="sm"
              :disabled="!password || busy"
              @click="disable"
            >
              {{ $t('account.twoFactor.disable') }}
            </Button>
          </div>
        </div>
      </template>

      <template v-else-if="setup">
        <p class="text-sm">{{ $t('account.twoFactor.scan') }}</p>
        <img :src="setup.qr" alt="QR code" class="h-48 w-48 rounded-md bg-white p-2" />
        <p class="text-sm">
          {{ $t('account.twoFactor.key') }}:
          <code class="break-all rounded bg-muted px-1 py-0.5">{{ setup.secret }}</code>
        </p>
        <form class="flex gap-2" @submit.prevent="enable">
          <Input
            v-model.trim="code"
            inputmode="numeric"
            autocomplete="one-time-code"
            maxlength="6"
            :placeholder="$t('account.twoFactor.enterCode')"
            class="max-w-xs"
          />
          <Button type="submit" :disabled="code.length !== 6 || busy">
            {{ $t('account.twoFactor.confirm') }}
          </Button>
        </form>
      </template>

      <template v-else>
        <p class="text-sm text-muted-foreground">{{ $t('account.twoFactor.off') }}</p>
        <Button class="self-start" :disabled="busy" @click="startSetup">
          {{ $t('account.twoFactor.enable') }}
        </Button>
      </template>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import api from '@main/api'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { useEmitter } from '@main/composables/useEmitter'
import { handleHTTPError } from '@shared-ui/utils/http.js'

const { t, locale } = useI18n()
const emitter = useEmitter()
const status = ref({ enabled: false, required: false, recovery_codes_left: 0, enabled_at: null })
const setup = ref(null)
const code = ref('')
const password = ref('')
const recoveryCodes = ref([])
const busy = ref(false)

const toast = (description, variant) =>
  emitter.emit(EMITTER_EVENTS.SHOW_TOAST, variant ? { variant, description } : { description })
const fail = (error) => toast(handleHTTPError(error).message, 'destructive')
const formatDate = (d) => (d ? new Date(d).toLocaleDateString(locale.value) : '')

const load = async () => {
  try {
    status.value = (await api.getTwoFactor()).data.data
  } catch (error) {
    fail(error)
  }
}

const run = async (fn) => {
  busy.value = true
  try {
    await fn()
  } catch (error) {
    fail(error)
  } finally {
    busy.value = false
  }
}

const startSetup = () =>
  run(async () => {
    setup.value = (await api.setupTwoFactor()).data.data
    code.value = ''
  })

const enable = () =>
  run(async () => {
    recoveryCodes.value = (await api.enableTwoFactor(code.value)).data.data.recovery_codes
    setup.value = null
    toast(t('account.twoFactor.enabled'))
    await load()
  })

const disable = () =>
  run(async () => {
    await api.disableTwoFactor(password.value)
    password.value = ''
    toast(t('account.twoFactor.disabled'))
    await load()
  })

const newCodes = () =>
  run(async () => {
    recoveryCodes.value = (await api.newRecoveryCodes(password.value)).data.data.recovery_codes
    password.value = ''
    await load()
  })

const copyCodes = async () => {
  try {
    await navigator.clipboard.writeText(recoveryCodes.value.join('\n'))
    toast(t('globals.messages.copied'))
  } catch {
    // The codes stay on screen to copy by hand.
  }
}

// Once set up under a workspace that requires it, the rest of the app opens.
const recoveryCodesSaved = () => {
  recoveryCodes.value = []
  if (status.value.required) {
    window.location.assign('/')
  }
}

onMounted(load)
</script>
