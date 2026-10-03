<template>
  <!-- An approved WhatsApp template the app sends on its own, with fixed or
       placeholder values for its body. Templates with a media header, header
       values or button values need an agent, so they can't be picked. -->
  <div class="space-y-2">
    <div class="flex items-end gap-2">
      <div class="flex-1 space-y-1.5">
        <Label>{{ label }}</Label>
        <select
          :value="selectedKey"
          class="w-full rounded-md border bg-background px-2 py-2 text-sm"
          :disabled="!inboxId"
          @change="select($event.target.value)"
        >
          <option value="">{{ optional ? $t('autoTemplate.none') : $t('whatsapp.chooseTemplate') }}</option>
          <option v-for="tmpl in templates" :key="keyOf(tmpl)" :value="keyOf(tmpl)" :disabled="!usable(tmpl)">
            {{ tmpl.name }} · {{ tmpl.language }}{{ usable(tmpl) ? '' : ' · ' + $t('autoTemplate.needsAgent') }}
          </option>
          <!-- A saved template that's no longer approved still shows. -->
          <option v-if="model?.name && !selected" :value="`${model.name}:${model.language}`">
            {{ model.name }} · {{ model.language }}
          </option>
        </select>
      </div>
      <Button type="button" variant="outline" size="sm" :is-loading="loading" :disabled="!inboxId" @click="load(true)">
        {{ $t('whatsapp.refreshTemplates') }}
      </Button>
    </div>
    <p v-if="hint" class="text-xs text-muted-foreground">{{ hint }}</p>

    <template v-if="selected">
      <div v-for="(param, i) in selected.body_params" :key="param" class="space-y-1">
        <Label class="text-xs text-muted-foreground">
          {{ $t('whatsapp.templateBody') }} {{ '{' + '{' + param + '}' + '}' }}
          <span v-if="fixedValues[i]">· {{ fixedValues[i] }}</span>
        </Label>
        <Input v-if="!fixedValues[i]" v-model="body[i]" maxlength="1024" :placeholder="placeholder" />
      </div>
      <div class="rounded-lg border bg-muted/40 p-3 text-sm whitespace-pre-wrap">{{ preview }}</div>
    </template>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import api from '@main/api'

const props = defineProps({
  inboxId: { type: [Number, String], default: 0 },
  label: { type: String, required: true },
  hint: { type: String, default: '' },
  placeholder: { type: String, default: '' },
  optional: { type: Boolean, default: false },
  // Values the app fills in itself, in body order (e.g. the report's
  // numbers): shown as labels, not asked for.
  fixedValues: { type: Array, default: () => [] }
})

// { name, language, body: [] }, or null for none.
const model = defineModel({ type: Object, default: null })

const templates = ref([])
const loading = ref(false)
const body = ref([])

const keyOf = (tmpl) => `${tmpl.name}:${tmpl.language}`
const usable = (tmpl) =>
  tmpl.supported && !tmpl.header_media && !tmpl.header_params.length && !tmpl.button_params.length
const selectedKey = computed(() => (model.value?.name ? `${model.value.name}:${model.value.language}` : ''))
const selected = computed(() => templates.value.find((tmpl) => keyOf(tmpl) === selectedKey.value))

const preview = computed(() => {
  const tmpl = selected.value
  if (!tmpl) return ''
  return (tmpl.body || '').replace(/\{\{\s*([A-Za-z0-9_]+)\s*\}\}/g, (m, name) => {
    const i = tmpl.body_params.indexOf(name)
    return props.fixedValues[i] ? `[${props.fixedValues[i]}]` : body.value[i]?.trim() || m
  })
})

const load = async (refresh = false) => {
  if (!props.inboxId) {
    templates.value = []
    return
  }
  loading.value = true
  try {
    const resp = await api.getWhatsAppTemplates(props.inboxId, refresh)
    templates.value = resp.data.data || []
  } catch {
    templates.value = []
  } finally {
    loading.value = false
  }
}

const select = (key) => {
  if (!key) {
    model.value = props.optional ? { name: '', language: '', body: [] } : null
    body.value = []
    return
  }
  const [name, language] = key.split(':')
  const tmpl = templates.value.find((x) => keyOf(x) === key)
  body.value = (tmpl?.body_params || []).map(() => '')
  model.value = { name, language, body: [...body.value] }
}

// Keep the saved values while the template list loads.
watch(
  () => model.value,
  (v) => {
    if (v?.body && v.body.join('\u0000') !== body.value.join('\u0000')) body.value = [...v.body]
  },
  { immediate: true }
)
watch(
  body,
  (v) => {
    if (model.value?.name) model.value = { ...model.value, body: [...v] }
  },
  { deep: true }
)
watch(() => props.inboxId, () => load(), { immediate: true })
</script>
