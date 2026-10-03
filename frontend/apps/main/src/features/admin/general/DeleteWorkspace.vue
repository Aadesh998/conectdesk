<template>
  <div class="mt-10 space-y-3 rounded-lg border border-destructive/40 p-4">
    <p class="font-semibold text-destructive">{{ $t('admin.general.deleteWorkspace') }}</p>
    <p class="text-sm text-muted-foreground">{{ $t('admin.general.deleteWorkspaceDescription') }}</p>
    <div class="grid max-w-md gap-2">
      <Input v-model.trim="confirm" :placeholder="$t('admin.general.deleteWorkspaceConfirm', { slug })" />
      <Input v-model="password" type="password" autocomplete="current-password" :placeholder="$t('account.twoFactor.password')" />
      <Button variant="destructive" class="justify-self-start" :disabled="confirm !== slug || !password || busy" @click="remove">
        {{ $t('admin.general.deleteWorkspace') }}
      </Button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import api from '@main/api'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { useEmitter } from '@main/composables/useEmitter'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { useAppSettingsStore } from '@main/stores/appSettings'

const { t } = useI18n()
const emitter = useEmitter()
const confirm = ref('')
const password = ref('')
const busy = ref(false)
const settingsStore = useAppSettingsStore()
const slug = computed(() => settingsStore.public_config['app.workspace_slug'] || '')

const remove = async () => {
  busy.value = true
  try {
    await api.deleteWorkspace({ confirm: confirm.value, password: password.value })
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('admin.general.deleteWorkspaceDone') })
    // The workspace is suspended now; its page says when it goes.
    setTimeout(() => window.location.assign('/'), 1500)
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  } finally {
    busy.value = false
  }
}
</script>
