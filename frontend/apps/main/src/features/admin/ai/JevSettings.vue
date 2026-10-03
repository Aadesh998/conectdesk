<template>
  <Card>
    <CardHeader>
      <CardTitle>{{ t('admin.ai.jev.title') }}</CardTitle>
      <CardDescription>{{ t('admin.ai.jev.description') }}</CardDescription>
    </CardHeader>
    <CardContent class="space-y-6">
      <SwitchField
        :title="t('admin.ai.jev.enabled')"
        :description="t('admin.ai.jev.enabledHint')"
        :checked="form.enabled"
        @update:checked="form.enabled = $event"
      />

      <div class="grid gap-6 md:grid-cols-2">
        <div class="space-y-1.5">
          <Label>{{ t('admin.ai.jev.apiKey') }}</Label>
          <Input
            v-model="form.api_key"
            type="password"
            autocomplete="new-password"
            :placeholder="hasKey ? '' : 'ts_…'"
          />
          <p class="text-xs text-muted-foreground">{{ t('admin.ai.jev.apiKeyHint') }}</p>
        </div>
        <div class="space-y-1.5">
          <Label>{{ t('globals.terms.model') }}</Label>
          <Input v-model="form.model" placeholder="jev-latest" autocomplete="off" />
          <p class="text-xs text-muted-foreground">{{ t('admin.ai.jev.modelHint') }}</p>
        </div>
      </div>

      <p v-if="testResult" class="text-sm text-emerald-600 dark:text-emerald-400">
        {{ t('admin.ai.jev.testOk', { model: testResult.model, ms: testResult.latency_ms }) }}
      </p>

      <div class="flex justify-end gap-2">
        <Button type="button" variant="outline" :is-loading="testing" :disabled="testing || (!hasKey && !form.api_key)" @click="test">
          {{ t('admin.ai.jev.test') }}
        </Button>
        <Button type="button" :is-loading="saving" :disabled="saving" @click="save">
          {{ t('globals.messages.save') }}
        </Button>
      </div>
    </CardContent>
  </Card>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button/index.js'
import { Input } from '@shared-ui/components/ui/input/index.js'
import { Label } from '@shared-ui/components/ui/label/index.js'
import SwitchField from '@shared-ui/components/SwitchField.vue'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@shared-ui/components/ui/card/index.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { useEmitter } from '@/composables/useEmitter.js'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import api from '@/api'

const { t } = useI18n()
const emitter = useEmitter()
const form = reactive({ enabled: false, api_key: '', model: 'jev-latest' })
const hasKey = ref(false)
const saving = ref(false)
const testing = ref(false)
const testResult = ref(null)

const toastError = (error) =>
  emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })

const apply = (data) => {
  form.enabled = !!data.enabled
  form.model = data.model || 'jev-latest'
  // The stored key never comes back; the field shows it's set.
  form.api_key = data.api_key || ''
  hasKey.value = !!data.has_key
}

onMounted(async () => {
  try {
    apply((await api.getJevSettings()).data.data)
  } catch (error) {
    toastError(error)
  }
})

const save = async () => {
  saving.value = true
  try {
    apply((await api.updateJevSettings({ ...form })).data.data)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('globals.messages.savedSuccessfully') })
  } catch (error) {
    toastError(error)
  } finally {
    saving.value = false
  }
}

const test = async () => {
  testing.value = true
  testResult.value = null
  try {
    testResult.value = (await api.testJevSettings({ api_key: form.api_key, model: form.model })).data.data
  } catch (error) {
    toastError(error)
  } finally {
    testing.value = false
  }
}
</script>
