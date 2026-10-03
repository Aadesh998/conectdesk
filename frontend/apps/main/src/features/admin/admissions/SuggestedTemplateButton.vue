<template>
  <!-- Submits one of the app's suggested templates to Meta for review. -->
  <div class="flex flex-wrap items-center gap-2">
    <Button type="button" variant="outline" size="sm" :disabled="!inboxId" :is-loading="creating" @click="create">
      {{ $t('autoTemplate.createSuggested') }}
    </Button>
    <span class="text-xs text-muted-foreground">{{ status || $t('autoTemplate.createSuggested.hint', { name: template.name }) }}</span>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@main/api'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'

const props = defineProps({
  inboxId: { type: [Number, String], default: 0 },
  // One of SUGGESTED_TEMPLATES.
  template: { type: Object, required: true }
})
// Emitted with { name, language, body } once submitted, to select it.
const emit = defineEmits(['created'])

const { t } = useI18n()
const emitter = useEmitter()
const creating = ref(false)
const status = ref('')

const create = async () => {
  creating.value = true
  try {
    const { name, language, category, body, body_examples: examples } = props.template
    const resp = await api.createWhatsAppTemplate(props.inboxId, { name, language, category, body, body_examples: examples })
    status.value = t('autoTemplate.submitted', { status: resp.data.data?.status || '' })
    emit('created', { name, language, body: [...props.template.body_values] })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  } finally {
    creating.value = false
  }
}
</script>
