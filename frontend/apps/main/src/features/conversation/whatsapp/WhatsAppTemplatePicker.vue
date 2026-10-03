<template>
  <div class="space-y-4">
    <div class="flex items-end gap-2">
      <div class="flex-1 space-y-1.5">
        <Label>{{ $t('whatsapp.template') }}</Label>
        <Select :model-value="selectedKey" @update:model-value="selectTemplate">
          <SelectTrigger>
            <SelectValue :placeholder="loading ? $t('globals.messages.loading') : $t('whatsapp.chooseTemplate')" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem
              v-for="tmpl in templates"
              :key="keyOf(tmpl)"
              :value="keyOf(tmpl)"
              :disabled="!tmpl.supported"
            >
              {{ tmpl.name }}
              <span class="text-muted-foreground text-xs">
                · {{ tmpl.language }}
                <template v-if="!tmpl.supported">· {{ $t('whatsapp.templateUnsupported') }}</template>
              </span>
            </SelectItem>
          </SelectContent>
        </Select>
      </div>
      <Button type="button" variant="outline" size="sm" :is-loading="loading" @click="load(true)">
        {{ $t('whatsapp.refreshTemplates') }}
      </Button>
    </div>

    <p v-if="!loading && templates.length === 0" class="text-sm text-muted-foreground">
      {{ $t('whatsapp.noTemplates') }}
    </p>

    <template v-if="selected">
      <!-- A media header is the file the customer gets with the message. -->
      <div v-if="selected.header_media" class="space-y-1">
        <Label class="text-xs text-muted-foreground">
          {{ $t('whatsapp.headerFile', { kind: $t(`whatsapp.header.${selected.header_media}`) }) }}
        </Label>
        <input type="file" :accept="headerAccept" class="text-sm" @change="uploadHeader" />
        <p v-if="headerFile" class="text-xs text-muted-foreground">{{ headerFile.name }}</p>
      </div>

      <div v-for="group in groups" :key="group.key" class="space-y-2">
        <div v-for="(param, i) in selected[group.params]" :key="group.key + param" class="space-y-1">
          <Label class="text-xs text-muted-foreground">
            {{ $t(group.label) }} {{ '{' + '{' + param + '}' + '}' }}
          </Label>
          <Input v-model="values[group.key][i]" :maxlength="group.key === 'header' ? 60 : 1024" />
        </div>
      </div>

      <!-- What the contact will see. -->
      <div class="rounded-lg border bg-muted/40 p-3 text-sm space-y-2 whitespace-pre-wrap">
        <div v-if="selected.header_media" class="rounded bg-muted p-4 text-center text-xs text-muted-foreground">
          {{ headerFile?.name || $t(`whatsapp.header.${selected.header_media}`) }}
        </div>
        <p v-if="selected.header" class="font-semibold">{{ fill(selected.header, selected.header_params, values.header) }}</p>
        <p>{{ fill(selected.body, selected.body_params, values.body) }}</p>
        <p v-if="selected.footer" class="text-xs text-muted-foreground">{{ selected.footer }}</p>
        <div v-if="selected.buttons.length" class="flex flex-wrap gap-1.5 pt-1">
          <span v-for="b in selected.buttons" :key="b" class="rounded border px-2 py-0.5 text-xs">
            {{ b }}
          </span>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@shared-ui/components/ui/select'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@main/api'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'

const props = defineProps({
  inboxId: {
    type: [Number, String],
    required: true
  }
})

// The template request the server expects, or null until every value is filled.
const model = defineModel({ type: Object, default: null })

const emitter = useEmitter()
const templates = ref([])
const loading = ref(false)
const selectedKey = ref('')
// The uploaded file for a media header: { id, name }.
const headerFile = ref(null)
const uploading = ref(false)
const headerAccept = computed(
  () => ({ image: 'image/jpeg,image/png', video: 'video/mp4,video/3gpp' })[selected.value?.header_media] || ''
)

const uploadHeader = async (event) => {
  const file = event.target.files?.[0]
  if (!file) return
  uploading.value = true
  try {
    const resp = await api.uploadMedia({ files: file, inline: false, linked_model: 'messages' })
    headerFile.value = { id: resp.data.data.id, name: file.name }
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    uploading.value = false
  }
}
const values = reactive({ header: [], body: [], buttons: [] })

const groups = [
  { key: 'header', params: 'header_params', label: 'whatsapp.templateHeader' },
  { key: 'body', params: 'body_params', label: 'whatsapp.templateBody' },
  { key: 'buttons', params: 'button_params', label: 'whatsapp.templateButton' }
]

const keyOf = (tmpl) => `${tmpl.name}:${tmpl.language}`
const selected = computed(() => templates.value.find((tmpl) => keyOf(tmpl) === selectedKey.value))

const load = async (refresh = false) => {
  if (!props.inboxId) return
  loading.value = true
  try {
    const resp = await api.getWhatsAppTemplates(props.inboxId, refresh)
    templates.value = resp.data.data || []
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    loading.value = false
  }
}

const selectTemplate = (key) => {
  selectedKey.value = key
  const tmpl = selected.value
  values.header = tmpl ? tmpl.header_params.map(() => '') : []
  values.body = tmpl ? tmpl.body_params.map(() => '') : []
  values.buttons = tmpl ? tmpl.button_params.map(() => '') : []
  headerFile.value = null
}

// Mirrors the server's rendering so the preview matches what gets stored.
const fill = (text, names, vals) =>
  (text || '').replace(/\{\{\s*([A-Za-z0-9_]+)\s*\}\}/g, (m, name) => {
    const i = names.indexOf(name)
    return i >= 0 && vals[i]?.trim() ? vals[i].trim() : m
  })

watch(
  [selected, values, headerFile],
  () => {
    const tmpl = selected.value
    const filled = (list) => list.every((v) => v && v.trim())
    const hasFile = !tmpl?.header_media || !!headerFile.value
    // attachments isn't part of the template request: the sender sends it as
    // the message's attachments, which carry a media header's file.
    model.value =
      tmpl && hasFile && filled(values.header) && filled(values.body) && filled(values.buttons)
        ? {
            name: tmpl.name,
            language: tmpl.language,
            header: [...values.header],
            body: [...values.body],
            buttons: [...values.buttons],
            attachments: headerFile.value ? [headerFile.value.id] : []
          }
        : null
  },
  { deep: true, immediate: true }
)

watch(
  () => props.inboxId,
  () => {
    selectTemplate('')
    templates.value = []
    load()
  },
  { immediate: true }
)
</script>
