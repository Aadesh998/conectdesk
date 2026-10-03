<template>
  <form @submit="onSubmit" novalidate class="space-y-6 w-full">
    <FormField v-slot="{ componentField, handleChange }" name="enabled">
      <FormItem>
        <SwitchField
          :title="t('globals.terms.enabled')"
          :checked="componentField.modelValue"
          @update:checked="handleChange"
        />
      </FormItem>
    </FormField>

    <FormField v-slot="{ componentField }" name="name">
      <FormItem>
        <FormLabel>{{ t('globals.terms.name') }}</FormLabel>
        <FormControl>
          <Input type="text" v-bind="componentField" />
        </FormControl>
        <FormDescription>{{ t('admin.ai.assistant.nameHint') }}</FormDescription>
        <FormMessage />
      </FormItem>
    </FormField>

    <div class="space-y-2">
      <div class="text-sm font-medium leading-none">{{ t('admin.ai.assistant.avatar') }}</div>
      <AvatarUpload
        :src="avatarPreview"
        :initials="avatarInitials"
        :label="t('globals.messages.upload')"
        @upload="onAvatarUpload"
        @remove="onAvatarRemove"
      />
    </div>

    <FormField v-slot="{ componentField }" name="description">
      <FormItem>
        <FormLabel>{{ t('globals.terms.description') }}</FormLabel>
        <FormControl>
          <Textarea
            rows="3"
            :placeholder="t('admin.ai.assistant.descriptionPlaceholder')"
            v-bind="componentField"
          />
        </FormControl>
        <FormDescription>{{ t('admin.ai.assistant.descriptionHint') }}</FormDescription>
        <FormMessage />
      </FormItem>
    </FormField>

    <FormField v-slot="{ componentField }" name="expectation">
      <FormItem>
        <FormLabel>{{ t('admin.ai.assistant.expectation') }}</FormLabel>
        <FormControl>
          <Textarea
            rows="2"
            :placeholder="t('admin.ai.assistant.expectationPlaceholder')"
            v-bind="componentField"
          />
        </FormControl>
        <FormDescription>{{ t('admin.ai.assistant.expectationHint') }}</FormDescription>
        <FormMessage />
      </FormItem>
    </FormField>

    <div class="grid gap-6 md:grid-cols-3">
      <FormField v-slot="{ componentField }" name="tone">
        <FormItem>
          <FormLabel>{{ t('admin.ai.assistant.tone') }}</FormLabel>
          <FormControl>
            <Select v-bind="componentField">
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem v-for="tone in tones" :key="tone" :value="tone">
                    {{ t(`admin.ai.assistant.toneOption.${tone}`) }}
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </FormControl>
          <FormMessage />
        </FormItem>
      </FormField>

      <FormField v-slot="{ componentField }" name="response_length">
        <FormItem>
          <FormLabel>{{ t('admin.ai.assistant.responseLength') }}</FormLabel>
          <FormControl>
            <Select v-bind="componentField">
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem v-for="length in responseLengths" :key="length" :value="length">
                    {{ t(`admin.ai.assistant.responseLengthOption.${length}`) }}
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </FormControl>
          <FormMessage />
        </FormItem>
      </FormField>

      <FormField v-slot="{ componentField }" name="max_turns">
        <FormItem>
          <FormLabel>{{ t('admin.ai.assistant.maxTurns') }}</FormLabel>
          <FormControl>
            <Input type="number" min="1" max="20" v-bind="componentField" />
          </FormControl>
          <FormDescription>{{ t('admin.ai.assistant.maxTurnsHint') }}</FormDescription>
          <FormMessage />
        </FormItem>
      </FormField>
    </div>

    <FormField v-slot="{ componentField, handleChange }" name="languages">
      <FormItem>
        <FormLabel>{{ t('globals.terms.language', 2) }}</FormLabel>
        <FormControl>
          <TagsInput :modelValue="componentField.modelValue" @update:modelValue="handleChange">
            <TagsInputItem v-for="item in componentField.modelValue" :key="item" :value="item">
              <TagsInputItemText />
              <TagsInputItemDelete />
            </TagsInputItem>
            <TagsInputInput :placeholder="t('admin.ai.assistant.languagesPlaceholder')" />
          </TagsInput>
        </FormControl>
        <FormDescription>{{ t('admin.ai.assistant.languagesHint') }}</FormDescription>
        <FormMessage />
      </FormItem>
    </FormField>

    <FormField v-slot="{ componentField, handleChange }" name="handoff_enabled">
      <FormItem>
        <SwitchField
          :title="t('admin.ai.assistant.offerHandoff')"
          :description="t('admin.ai.assistant.offerHandoffHint')"
          :checked="componentField.modelValue"
          @update:checked="handleChange"
        />
      </FormItem>
    </FormField>

    <FormField v-slot="{ componentField }" name="fallback_team_id">
      <FormItem>
        <FormLabel>{{ t('admin.ai.assistant.fallbackTeam') }}</FormLabel>
        <FormControl>
          <Select v-bind="componentField">
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="none">{{ t('globals.terms.none') }}</SelectItem>
                <SelectItem v-for="team in teams" :key="team.id" :value="String(team.id)">
                  {{ team.name }}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </FormControl>
        <FormDescription>{{ t('admin.ai.assistant.fallbackTeamHint') }}</FormDescription>
        <FormMessage />
      </FormItem>
    </FormField>

    <FormField v-slot="{ componentField }" name="instructions">
      <FormItem>
        <FormLabel>{{ t('admin.ai.assistant.instructions') }}</FormLabel>
        <FormControl>
          <Textarea
            rows="6"
            :placeholder="t('admin.ai.assistant.instructionsPlaceholder')"
            v-bind="componentField"
          />
        </FormControl>
        <FormDescription>{{ t('admin.ai.assistant.instructionsHint') }}</FormDescription>
        <FormMessage />
      </FormItem>
    </FormField>

    <FormField v-slot="{ componentField }" name="guardrails">
      <FormItem>
        <FormLabel>{{ t('admin.ai.assistant.guardrails') }}</FormLabel>
        <FormControl>
          <Textarea
            rows="6"
            :placeholder="t('admin.ai.assistant.guardrailsPlaceholder')"
            v-bind="componentField"
          />
        </FormControl>
        <FormDescription>{{ t('admin.ai.assistant.guardrailsHint') }}</FormDescription>
        <FormMessage />
      </FormItem>
    </FormField>

    <!-- The Jev decision layer: decides when the LLM is called, checks
         replies before they go out, and keeps the lead stage current. -->
    <div class="box p-4 space-y-4">
      <div>
        <h3 class="font-semibold">{{ t('admin.ai.assistant.jev.title') }}</h3>
        <p class="text-sm text-muted-foreground">{{ t('admin.ai.assistant.jev.description') }}</p>
        <p v-if="!jevReady" class="text-xs text-amber-600 dark:text-amber-400 mt-1">
          {{ t('admin.ai.assistant.jev.off') }}
          <router-link :to="{ name: 'ai-providers' }" class="underline">{{ t('admin.ai.assistant.jev.setUp') }}</router-link>
        </p>
      </div>
      <FormField v-slot="{ componentField, handleChange }" name="jev_gate">
        <FormItem>
          <SwitchField
            :title="t('admin.ai.assistant.jev.gate')"
            :description="t('admin.ai.assistant.jev.gateHint')"
            :checked="componentField.modelValue"
            @update:checked="handleChange"
          />
        </FormItem>
      </FormField>
      <FormField v-slot="{ componentField, handleChange }" name="jev_guard">
        <FormItem>
          <SwitchField
            :title="t('admin.ai.assistant.jev.guard')"
            :description="t('admin.ai.assistant.jev.guardHint')"
            :checked="componentField.modelValue"
            @update:checked="handleChange"
          />
        </FormItem>
      </FormField>
      <FormField v-slot="{ componentField, handleChange }" name="jev_lead_stage">
        <FormItem>
          <SwitchField
            :title="t('admin.ai.assistant.jev.leadStage')"
            :description="t('admin.ai.assistant.jev.leadStageHint')"
            :checked="componentField.modelValue"
            @update:checked="handleChange"
          />
        </FormItem>
      </FormField>
      <FormField v-slot="{ componentField }" name="jev_confidence">
        <FormItem>
          <FormLabel>{{ t('admin.ai.assistant.jev.confidence') }}</FormLabel>
          <FormControl>
            <Input type="number" min="0.5" max="0.99" step="0.05" class="w-28" v-bind="componentField" />
          </FormControl>
          <FormDescription>{{ t('admin.ai.assistant.jev.confidenceHint') }}</FormDescription>
          <FormMessage />
        </FormItem>
      </FormField>
    </div>

    <!-- Following up when the customer goes quiet. -->
    <div class="box p-4 space-y-4">
      <FormField v-slot="{ componentField, handleChange }" name="followups_enabled">
        <FormItem>
          <SwitchField
            :title="t('admin.ai.assistant.followups.title')"
            :description="t('admin.ai.assistant.followups.description')"
            :checked="componentField.modelValue"
            @update:checked="handleChange"
          />
        </FormItem>
      </FormField>
      <template v-if="form.values.followups_enabled">
        <div class="space-y-1.5">
          <Label>{{ t('admin.ai.assistant.followups.hours') }}</Label>
          <Input v-model="followupHours" class="w-48" placeholder="4, 24" />
          <p class="text-xs text-muted-foreground">{{ t('admin.ai.assistant.followups.hoursHint') }}</p>
        </div>

        <div class="space-y-3">
          <div>
            <p class="text-sm font-medium">{{ t('admin.ai.assistant.followups.templates') }}</p>
            <p class="text-xs text-muted-foreground">{{ t('admin.ai.assistant.followups.templatesHint') }}</p>
          </div>
          <div class="space-y-1.5">
            <Label class="text-xs">{{ t('admin.ai.assistant.followups.templateInbox') }}</Label>
            <select v-model.number="templateInboxId" class="w-full max-w-sm rounded-md border bg-background px-2 py-2 text-sm">
              <option :value="0">{{ t('admin.ai.assistant.followups.chooseInbox') }}</option>
              <option v-for="inb in whatsappInboxes" :key="inb.id" :value="inb.id">{{ inb.name }}</option>
            </select>
          </div>
          <div v-for="(tmpl, i) in followupTemplates" :key="i" class="flex items-start gap-2 rounded-md border p-3">
            <AutoTemplateField
              v-model="followupTemplates[i]"
              :inbox-id="templateInboxId"
              :label="t('admin.ai.assistant.followups.template', { n: i + 1 })"
              :hint="t('voice.missedCalls.template.hint')"
              class="flex-1"
            />
            <Button type="button" variant="ghost" size="sm" class="mt-6" @click="followupTemplates.splice(i, 1)">
              {{ t('globals.terms.remove') }}
            </Button>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            :disabled="!templateInboxId || followupTemplates.length >= 10"
            @click="followupTemplates.push({ name: '', language: '', body: [] })"
          >
            {{ t('admin.ai.assistant.followups.addTemplate') }}
          </Button>
        </div>
      </template>
    </div>

    <div class="space-y-2">
      <div class="text-sm font-medium leading-none">{{ t('admin.ai.assistant.tools') }}</div>
      <p class="text-sm text-muted-foreground">{{ t('admin.ai.assistant.toolsHint') }}</p>
      <div class="space-y-3 mt-3">
        <div v-for="tool in tools" :key="tool.id" class="flex items-center space-x-2">
          <Checkbox
            :id="`tool-${tool.id}`"
            :checked="selectedToolIds.includes(tool.id)"
            @update:checked="(checked) => toggleTool(tool.id, checked)"
          />
          <label :for="`tool-${tool.id}`" class="text-sm font-medium leading-none cursor-pointer">
            {{ tool.name }}
          </label>
        </div>
        <p v-if="!tools.length" class="text-sm text-muted-foreground">
          {{ t('admin.ai.assistant.toolsEmpty') }}
        </p>
      </div>
    </div>

    <div class="flex justify-end mt-10">
      <Button type="submit" :isLoading="formLoading">
        {{ isEditing ? t('globals.messages.save') : t('globals.messages.create') }}
      </Button>
    </div>
  </form>
</template>

<script setup>
import { computed, ref, watch, onMounted } from 'vue'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import * as z from 'zod'
import { Button } from '@shared-ui/components/ui/button/index.js'
import { Input } from '@shared-ui/components/ui/input/index.js'
import { Textarea } from '@shared-ui/components/ui/textarea/index.js'
import { Checkbox } from '@shared-ui/components/ui/checkbox/index.js'
import { Label } from '@shared-ui/components/ui/label/index.js'
import AutoTemplateField from '@/features/admin/inbox/AutoTemplateField.vue'
import {
  TagsInput,
  TagsInputInput,
  TagsInputItem,
  TagsInputItemDelete,
  TagsInputItemText
} from '@shared-ui/components/ui/tags-input'
import { AvatarUpload } from '@shared-ui/components/ui/avatar'
import SwitchField from '@shared-ui/components/SwitchField.vue'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@shared-ui/components/ui/select/index.js'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage
} from '@shared-ui/components/ui/form/index.js'
import { useEmitter } from '@/composables/useEmitter.js'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { useI18n } from 'vue-i18n'
import api from '@/api'

const tones = ['friendly', 'professional', 'neutral', 'casual']
const responseLengths = ['concise', 'balanced', 'detailed']

const props = defineProps({
  initialValues: { type: Object, default: () => ({}) },
  isEditing: { type: Boolean, default: false },
  submitForm: { type: Function, required: true }
})

const { t } = useI18n()
const emitter = useEmitter()
const formLoading = ref(false)
const tools = ref([])
const teams = ref([])
const selectedToolIds = ref([])
const avatarFile = ref(null)
const avatarPreview = ref('')
const removeAvatar = ref(false)
const jevReady = ref(true)
const followupHours = ref('4, 24')
const followupTemplates = ref([])
const templateInboxId = ref(0)
const whatsappInboxes = ref([])

// "4, 24" -> [4, 24]: whole hours within a week, at most five follow-ups.
const parseHours = (text) =>
  [...new Set(String(text).split(/[\s,]+/).map(Number).filter((h) => Number.isInteger(h) && h >= 1 && h <= 168))]
    .sort((a, b) => a - b)
    .slice(0, 5)

const avatarInitials = computed(() => (form.values.name || '').trim().charAt(0).toUpperCase() || 'A')

const onAvatarUpload = (file) => {
  if (!file) return
  avatarFile.value = file
  avatarPreview.value = URL.createObjectURL(file)
  removeAvatar.value = false
}

const onAvatarRemove = () => {
  avatarFile.value = null
  avatarPreview.value = ''
  removeAvatar.value = true
}

const form = useForm({
  validationSchema: toTypedSchema(
    z.object({
      name: z
        .string({ required_error: t('globals.messages.required') })
        .min(1, { message: t('globals.messages.required') }),
      description: z.string().optional(),
      expectation: z.string().optional(),
      tone: z.string().optional(),
      response_length: z.string().optional(),
      max_turns: z.coerce
        .number()
        .int()
        .min(1, { message: t('admin.ai.assistant.maxTurnsHint') })
        .max(20, { message: t('admin.ai.assistant.maxTurnsHint') }),
      fallback_team_id: z.string().optional(),
      handoff_enabled: z.boolean().optional(),
      languages: z.array(z.string()).optional(),
      instructions: z.string().optional(),
      guardrails: z.string().optional(),
      enabled: z.boolean().optional(),
      jev_gate: z.boolean().optional(),
      jev_guard: z.boolean().optional(),
      jev_lead_stage: z.boolean().optional(),
      jev_confidence: z.coerce
        .number()
        .min(0.5, { message: t('admin.ai.assistant.jev.confidenceHint') })
        .max(0.99, { message: t('admin.ai.assistant.jev.confidenceHint') }),
      followups_enabled: z.boolean().optional()
    })
  ),
  initialValues: {
    name: '',
    description: '',
    expectation: '',
    tone: 'friendly',
    response_length: 'balanced',
    max_turns: 6,
    fallback_team_id: 'none',
    handoff_enabled: true,
    languages: [],
    instructions: '',
    guardrails: '',
    enabled: true,
    jev_gate: true,
    jev_guard: true,
    jev_lead_stage: false,
    jev_confidence: 0.7,
    followups_enabled: false
  }
})

const toggleTool = (id, checked) => {
  if (checked) {
    if (!selectedToolIds.value.includes(id)) selectedToolIds.value.push(id)
  } else {
    selectedToolIds.value = selectedToolIds.value.filter((t) => t !== id)
  }
}

watch(
  () => props.initialValues,
  (values) => {
    form.setValues(
      {
        name: values.name || '',
        description: values.description || '',
        expectation: values.expectation || '',
        tone: values.tone || 'friendly',
        response_length: values.response_length || 'balanced',
        max_turns: values.max_turns ?? 6,
        fallback_team_id: values.fallback_team_id ? String(values.fallback_team_id) : 'none',
        handoff_enabled: values.handoff_enabled ?? true,
        languages: [...(values.languages || [])],
        instructions: values.instructions || '',
        guardrails: values.guardrails || '',
        enabled: values.enabled ?? true,
        jev_gate: values.jev_gate ?? true,
        jev_guard: values.jev_guard ?? true,
        jev_lead_stage: values.jev_lead_stage ?? false,
        jev_confidence: values.jev_confidence ?? 0.7,
        followups_enabled: values.followups_enabled ?? false
      },
      false
    )
    followupHours.value = (values.followup_hours?.length ? values.followup_hours : [4, 24]).join(', ')
    followupTemplates.value = (values.followup_templates || []).map((x) => ({ ...x, body: [...(x.body || [])] }))
    selectedToolIds.value = [...(values.tool_ids || [])]
    avatarFile.value = null
    avatarPreview.value = values.avatar_url || ''
    removeAvatar.value = false
    form.setErrors({})
  },
  { immediate: true, deep: true }
)

onMounted(async () => {
  try {
    const [toolsResp, teamsResp, inboxesResp, jevResp] = await Promise.all([
      api.getAITools(),
      api.getTeamsCompact(),
      api.getInboxes(),
      api.getJevSettings().catch(() => null)
    ])
    tools.value = (toolsResp.data.data || []).filter((tool) => tool.enabled)
    teams.value = teamsResp.data.data || []
    whatsappInboxes.value = (inboxesResp.data.data || []).filter((inb) => inb.channel === 'whatsapp')
    if (!templateInboxId.value && whatsappInboxes.value.length) templateInboxId.value = whatsappInboxes.value[0].id
    const jev = jevResp?.data?.data
    jevReady.value = !!(jev?.enabled && jev?.has_key)
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  }
})

const onSubmit = form.handleSubmit(async (values) => {
  try {
    formLoading.value = true
    const payload = {
      name: values.name,
      description: values.description || '',
      expectation: values.expectation || '',
      tone: values.tone || 'friendly',
      response_length: values.response_length || 'balanced',
      max_turns: Number(values.max_turns),
      fallback_team_id:
        values.fallback_team_id && values.fallback_team_id !== 'none'
          ? Number(values.fallback_team_id)
          : null,
      handoff_enabled: !!values.handoff_enabled,
      languages: values.languages || [],
      instructions: values.instructions || '',
      guardrails: values.guardrails || '',
      enabled: !!values.enabled,
      remove_avatar: removeAvatar.value,
      tool_ids: [...selectedToolIds.value],
      jev_gate: !!values.jev_gate,
      jev_guard: !!values.jev_guard,
      jev_lead_stage: !!values.jev_lead_stage,
      jev_confidence: Number(values.jev_confidence),
      followups_enabled: !!values.followups_enabled,
      followup_hours: parseHours(followupHours.value),
      followup_templates: followupTemplates.value.filter((x) => x?.name)
    }
    const formData = new FormData()
    formData.append('data', JSON.stringify(payload))
    if (avatarFile.value) {
      formData.append('files', avatarFile.value)
    }
    await props.submitForm(formData)
  } finally {
    formLoading.value = false
  }
})
</script>
