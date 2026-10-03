<template>
  <div class="box p-4 space-y-3">
    <div class="flex items-center justify-between gap-2">
      <h3 class="font-semibold">{{ $t('whatsapp.templates') }}</h3>
      <div class="flex gap-2">
        <Button type="button" variant="outline" size="sm" :is-loading="loading" @click="load">
          {{ $t('whatsapp.refreshTemplates') }}
        </Button>
        <Button type="button" size="sm" @click="openCreate">{{ $t('whatsapp.newTemplate') }}</Button>
      </div>
    </div>
    <p class="text-sm text-muted-foreground">{{ $t('whatsapp.templates.description') }}</p>

    <p v-if="!loading && templates.length === 0" class="text-sm text-muted-foreground">
      {{ $t('whatsapp.noTemplatesYet') }}
    </p>
    <div v-else class="divide-y rounded-md border">
      <div v-for="tmpl in templates" :key="tmpl.id" class="flex items-start justify-between gap-3 p-3">
        <div class="min-w-0 space-y-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="font-medium">{{ tmpl.name }}</span>
            <span class="text-xs text-muted-foreground">{{ tmpl.language }} · {{ tmpl.category }}</span>
            <span class="rounded px-1.5 py-0.5 text-xs font-medium" :class="statusClass(tmpl.status)">
              {{ tmpl.status }}
            </span>
          </div>
          <p class="text-sm text-muted-foreground line-clamp-2 whitespace-pre-wrap">{{ tmpl.body }}</p>
          <p v-if="tmpl.rejected_reason" class="text-xs text-destructive">
            {{ $t('whatsapp.rejectedReason', { reason: tmpl.rejected_reason }) }}
          </p>
        </div>
        <Button type="button" variant="ghost" size="sm" @click="remove(tmpl)">
          {{ $t('globals.messages.delete') }}
        </Button>
      </div>
    </div>
  </div>

  <Dialog :open="creating" @update:open="creating = $event">
    <DialogContent class="max-w-2xl max-h-[90vh] overflow-y-auto">
      <DialogHeader>
        <DialogTitle>{{ $t('whatsapp.newTemplate') }}</DialogTitle>
        <DialogDescription>{{ $t('whatsapp.newTemplate.description') }}</DialogDescription>
      </DialogHeader>

      <div class="grid gap-4 md:grid-cols-2">
        <div class="space-y-4">
          <div class="space-y-1.5">
            <Label>{{ $t('globals.terms.name') }}</Label>
            <Input v-model="draft.name" placeholder="order_update" @input="draft.name = draft.name.toLowerCase().replace(/[^a-z0-9_]/g, '_')" />
          </div>
          <div class="grid grid-cols-2 gap-2">
            <div class="space-y-1.5">
              <Label>{{ $t('whatsapp.category') }}</Label>
              <select v-model="draft.category" class="w-full rounded-md border bg-background px-2 py-2 text-sm">
                <option value="UTILITY">{{ $t('whatsapp.category.utility') }}</option>
                <option value="MARKETING">{{ $t('whatsapp.category.marketing') }}</option>
              </select>
            </div>
            <div class="space-y-1.5">
              <Label>{{ $t('whatsapp.language') }}</Label>
              <select v-model="draft.language" class="w-full rounded-md border bg-background px-2 py-2 text-sm">
                <option v-for="l in languages" :key="l.code" :value="l.code">{{ l.label }}</option>
              </select>
            </div>
          </div>

          <div class="space-y-1.5">
            <Label>{{ $t('whatsapp.templateHeader') }}</Label>
            <select v-model="draft.header_format" class="w-full rounded-md border bg-background px-2 py-2 text-sm">
              <option value="">{{ $t('globals.terms.none') }}</option>
              <option value="TEXT">{{ $t('whatsapp.header.text') }}</option>
              <option value="IMAGE">{{ $t('whatsapp.header.image') }}</option>
              <option value="VIDEO">{{ $t('whatsapp.header.video') }}</option>
              <option value="DOCUMENT">{{ $t('whatsapp.header.document') }}</option>
            </select>
            <template v-if="draft.header_format === 'TEXT'">
              <Input v-model="draft.header_text" maxlength="60" :placeholder="$t('whatsapp.header.textPlaceholder')" />
              <Input v-if="headerVars.length" v-model="draft.header_example" :placeholder="$t('whatsapp.sampleFor', { v: '{{1}}' })" />
            </template>
            <template v-else-if="draft.header_format">
              <input type="file" :accept="headerAccept" class="text-sm" @change="uploadSample" />
              <p class="text-xs text-muted-foreground">{{ $t('whatsapp.header.sampleHint') }}</p>
            </template>
          </div>

          <div class="space-y-1.5">
            <Label>{{ $t('whatsapp.templateBody') }}</Label>
            <Textarea v-model="draft.body" rows="5" maxlength="1024" :placeholder="$t('whatsapp.body.placeholder')" />
            <div class="flex justify-between text-xs text-muted-foreground">
              <button type="button" class="underline" @click="addVariable">{{ $t('whatsapp.addVariable') }}</button>
              <span>{{ draft.body.length }}/1024</span>
            </div>
            <Input
              v-for="(v, i) in bodyVars"
              :key="v"
              v-model="draft.body_examples[i]"
              :placeholder="$t('whatsapp.sampleFor', { v: `{{${v}}}` })"
            />
          </div>

          <div class="space-y-1.5">
            <Label>{{ $t('whatsapp.footer') }} ({{ $t('globals.terms.optional') }})</Label>
            <Input v-model="draft.footer" maxlength="60" />
          </div>

          <div class="space-y-2">
            <div class="flex items-center justify-between">
              <Label>{{ $t('whatsapp.buttons') }} ({{ $t('globals.terms.optional') }})</Label>
              <select class="rounded-md border bg-background px-2 py-1 text-xs" :value="''" @change="addButton($event.target.value); $event.target.value = ''">
                <option value="">{{ $t('whatsapp.addButton') }}</option>
                <option value="QUICK_REPLY">{{ $t('whatsapp.button.quickReply') }}</option>
                <option value="URL">{{ $t('whatsapp.button.url') }}</option>
                <option value="PHONE_NUMBER">{{ $t('whatsapp.button.phone') }}</option>
              </select>
            </div>
            <div v-for="(b, i) in draft.buttons" :key="i" class="space-y-1.5 rounded-md border p-2">
              <div class="flex items-center gap-2">
                <span class="text-xs text-muted-foreground w-20 shrink-0">{{ buttonLabel(b.type) }}</span>
                <Input v-model="b.text" maxlength="25" :placeholder="$t('whatsapp.button.label')" />
                <Button type="button" variant="ghost" size="sm" @click="draft.buttons.splice(i, 1)">✕</Button>
              </div>
              <template v-if="b.type === 'URL'">
                <Input v-model="b.url" placeholder="https://example.com/track/{{1}}" />
                <Input v-if="b.url.includes('{{1}}')" v-model="b.url_example" :placeholder="$t('whatsapp.button.urlExample')" />
              </template>
              <Input v-if="b.type === 'PHONE_NUMBER'" v-model="b.phone" placeholder="+91 99999 99999" />
            </div>
          </div>
        </div>

        <!-- Preview, the way WhatsApp shows it, with the sample values. -->
        <div class="space-y-2">
          <Label>{{ $t('whatsapp.preview') }}</Label>
          <div class="rounded-lg border bg-muted/40 p-3 text-sm space-y-2 whitespace-pre-wrap">
            <p v-if="draft.header_format === 'TEXT'" class="font-semibold">{{ sample(draft.header_text, [draft.header_example]) }}</p>
            <div v-else-if="draft.header_format" class="rounded bg-muted p-6 text-center text-xs text-muted-foreground">
              {{ sampleName || draft.header_format }}
            </div>
            <p>{{ sample(draft.body, draft.body_examples) || '…' }}</p>
            <p v-if="draft.footer" class="text-xs text-muted-foreground">{{ draft.footer }}</p>
            <div v-if="draft.buttons.length" class="flex flex-wrap gap-1.5 pt-1">
              <span v-for="(b, i) in draft.buttons" :key="i" class="rounded border px-2 py-0.5 text-xs">{{ b.text || '…' }}</span>
            </div>
          </div>
          <p class="text-xs text-muted-foreground">{{ $t('whatsapp.reviewNote') }}</p>
        </div>
      </div>

      <DialogFooter>
        <Button type="button" :is-loading="submitting" :disabled="submitting || uploading" @click="submit">
          {{ $t('whatsapp.submitForReview') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { Textarea } from '@shared-ui/components/ui/textarea'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@shared-ui/components/ui/dialog'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'

const props = defineProps({
  inboxId: {
    type: [Number, String],
    required: true
  }
})

const { t } = useI18n()
const emitter = useEmitter()
const templates = ref([])
const loading = ref(false)
const creating = ref(false)
const submitting = ref(false)
const uploading = ref(false)
const sampleName = ref('')

// The languages businesses use most; Meta supports more.
const languages = [
  { code: 'en_US', label: 'English (US)' },
  { code: 'en_GB', label: 'English (UK)' },
  { code: 'en', label: 'English' },
  { code: 'hi', label: 'Hindi' },
  { code: 'mr', label: 'Marathi' },
  { code: 'ta', label: 'Tamil' },
  { code: 'te', label: 'Telugu' },
  { code: 'bn', label: 'Bengali' },
  { code: 'gu', label: 'Gujarati' },
  { code: 'kn', label: 'Kannada' },
  { code: 'ml', label: 'Malayalam' },
  { code: 'pa', label: 'Punjabi' },
  { code: 'ur', label: 'Urdu' },
  { code: 'ar', label: 'Arabic' },
  { code: 'es', label: 'Spanish' },
  { code: 'pt_BR', label: 'Portuguese (BR)' },
  { code: 'fr', label: 'French' },
  { code: 'de', label: 'German' },
  { code: 'it', label: 'Italian' },
  { code: 'id', label: 'Indonesian' }
]

const emptyDraft = () => ({
  name: '',
  category: 'UTILITY',
  language: 'en_US',
  header_format: '',
  header_text: '',
  header_example: '',
  header_media_id: 0,
  body: '',
  body_examples: [],
  footer: '',
  buttons: []
})
const draft = reactive(emptyDraft())

const varsIn = (text) => [...new Set([...(text || '').matchAll(/\{\{\s*(\d+)\s*\}\}/g)].map((m) => m[1]))]
const bodyVars = computed(() => varsIn(draft.body))
const headerVars = computed(() => varsIn(draft.header_text))

watch(bodyVars, (vars) => {
  draft.body_examples = vars.map((_, i) => draft.body_examples[i] || '')
})

const headerAccept = computed(
  () => ({ IMAGE: 'image/jpeg,image/png', VIDEO: 'video/mp4,video/3gpp', DOCUMENT: '' })[draft.header_format] || ''
)

watch(
  () => draft.header_format,
  () => {
    draft.header_media_id = 0
    sampleName.value = ''
  }
)

const statusClass = (status) =>
  ({
    APPROVED: 'bg-green-500/15 text-green-700 dark:text-green-400',
    PENDING: 'bg-amber-500/15 text-amber-700 dark:text-amber-400',
    REJECTED: 'bg-destructive/15 text-destructive',
    PAUSED: 'bg-muted text-muted-foreground',
    DISABLED: 'bg-muted text-muted-foreground'
  })[status] || 'bg-muted text-muted-foreground'

const buttonLabel = (type) =>
  ({
    QUICK_REPLY: t('whatsapp.button.quickReply'),
    URL: t('whatsapp.button.url'),
    PHONE_NUMBER: t('whatsapp.button.phone')
  })[type]

const sample = (text, values) =>
  (text || '').replace(/\{\{\s*(\d+)\s*\}\}/g, (m, n) => values[Number(n) - 1]?.trim() || m)

const addVariable = () => {
  draft.body = `${draft.body}{{${bodyVars.value.length + 1}}}`
}

const addButton = (type) => {
  if (!type) return
  draft.buttons.push({ type, text: '', url: '', url_example: '', phone: '' })
}

const toast = (description, variant) =>
  emitter.emit(EMITTER_EVENTS.SHOW_TOAST, variant ? { variant, description } : { description })

const load = async () => {
  loading.value = true
  try {
    const resp = await api.listManagedTemplates(props.inboxId)
    templates.value = resp.data.data || []
  } catch (error) {
    toast(handleHTTPError(error).message, 'destructive')
  } finally {
    loading.value = false
  }
}

const openCreate = () => {
  Object.assign(draft, emptyDraft())
  sampleName.value = ''
  creating.value = true
}

const uploadSample = async (event) => {
  const file = event.target.files?.[0]
  if (!file) return
  uploading.value = true
  try {
    const resp = await api.uploadMedia({ files: file, inline: false, linked_model: 'messages' })
    draft.header_media_id = resp.data.data.id
    sampleName.value = file.name
  } catch (error) {
    toast(handleHTTPError(error).message, 'destructive')
  } finally {
    uploading.value = false
  }
}

const submit = async () => {
  submitting.value = true
  try {
    const resp = await api.createWhatsAppTemplate(props.inboxId, {
      ...draft,
      header_example: headerVars.value.length ? draft.header_example : '',
      buttons: draft.buttons.map((b) => ({ ...b, url_example: b.url.includes('{{1}}') ? b.url_example : '' }))
    })
    toast(t('whatsapp.templateSubmitted', { status: resp.data.data.status }))
    creating.value = false
    await load()
  } catch (error) {
    toast(handleHTTPError(error).message, 'destructive')
  } finally {
    submitting.value = false
  }
}

const remove = async (tmpl) => {
  if (!window.confirm(t('whatsapp.deleteTemplate.confirm', { name: tmpl.name }))) return
  try {
    await api.deleteWhatsAppTemplate(props.inboxId, tmpl.name, tmpl.id)
    await load()
  } catch (error) {
    toast(handleHTTPError(error).message, 'destructive')
  }
}

watch(() => props.inboxId, load, { immediate: true })
</script>
