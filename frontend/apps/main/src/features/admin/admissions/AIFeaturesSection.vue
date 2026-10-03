<template>
  <section class="space-y-4" :class="{ 'opacity-50 transition-opacity': !loaded }">
    <div class="space-y-1">
      <h3 class="font-medium">{{ $t('callAI.title') }}</h3>
      <p class="text-sm text-muted-foreground">{{ $t('callAI.description') }}</p>
    </div>
    <div class="flex items-start gap-3">
      <Switch id="ai-call-summaries" :checked="f['ai_features.call_summaries']" @update:checked="save('ai_features.call_summaries', $event)" />
      <div class="space-y-1">
        <Label for="ai-call-summaries">{{ $t('callAI.callSummaries') }}</Label>
        <p class="text-xs text-muted-foreground">{{ $t('callAI.callSummaries.description') }}</p>
      </div>
    </div>
    <div class="flex items-start gap-3">
      <Switch id="ai-lead-fields" :checked="f['ai_features.lead_extraction']" @update:checked="save('ai_features.lead_extraction', $event)" />
      <div class="space-y-1">
        <Label for="ai-lead-fields">{{ $t('callAI.leadFields') }}</Label>
        <p class="text-xs text-muted-foreground">{{ $t('callAI.leadFields.description') }}</p>
      </div>
    </div>
    <p class="text-xs text-muted-foreground">
      {{ $t('callAI.providerHint') }}
      <router-link to="/admin/ai/providers" class="link-style">{{ $t('callAI.providerLink') }}</router-link>
    </p>
  </section>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import api from '@main/api'
import { Label } from '@shared-ui/components/ui/label'
import { Switch } from '@shared-ui/components/ui/switch'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'

const emitter = useEmitter()
const f = reactive({ 'ai_features.call_summaries': false, 'ai_features.lead_extraction': false })
const loaded = ref(false)

const save = async (key, value) => {
  const before = f[key]
  f[key] = value
  try {
    await api.updateAIFeatures({ ...f })
  } catch (error) {
    f[key] = before
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  }
}

onMounted(async () => {
  try {
    const resp = await api.getAIFeatures()
    Object.assign(f, resp.data.data || {})
  } finally {
    loaded.value = true
  }
})
</script>
