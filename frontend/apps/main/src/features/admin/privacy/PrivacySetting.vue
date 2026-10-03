<template>
  <AdminSplitLayout>
    <template #content>
      <div class="space-y-8" :class="{ 'opacity-50 transition-opacity duration-300': !loaded }">
        <section class="space-y-4">
          <div class="space-y-1">
            <h3 class="font-medium">{{ $t('admin.privacy.retention') }}</h3>
            <p class="text-sm text-muted-foreground">{{ $t('admin.privacy.retentionDescription') }}</p>
          </div>

          <RadioGroup v-model="mode" class="space-y-3">
            <div class="flex items-center gap-2">
              <RadioGroupItem id="retention-off" value="off" />
              <Label for="retention-off">{{ $t('admin.privacy.keepForever') }}</Label>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <RadioGroupItem id="retention-on" value="on" />
              <Label for="retention-on">{{ $t('admin.privacy.deleteAfter') }}</Label>
              <Input
                v-model="days"
                type="number"
                :min="limits.min"
                :max="limits.max"
                class="w-24"
                :disabled="mode !== 'on'"
                :aria-label="$t('admin.privacy.days')"
              />
              <span class="text-sm">{{ $t('admin.privacy.days') }}</span>
            </div>
          </RadioGroup>

          <div v-if="mode === 'on'" class="flex items-start gap-3 pl-6">
            <Switch id="retention-contacts" :checked="deleteContacts" @update:checked="deleteContacts = $event" />
            <div class="space-y-1">
              <Label for="retention-contacts">{{ $t('admin.privacy.deleteContacts') }}</Label>
              <p class="text-xs text-muted-foreground">{{ $t('admin.privacy.deleteContactsHint') }}</p>
            </div>
          </div>

          <p v-if="mode === 'on'" class="text-sm text-warning-600">{{ $t('admin.privacy.warning') }}</p>

          <Button :disabled="!loaded || !changed" :isLoading="saving" @click="confirmSave">
            {{ $t('globals.messages.save') }}
          </Button>
        </section>

        <section class="space-y-2">
          <h3 class="font-medium">{{ $t('admin.privacy.erasure') }}</h3>
          <p class="text-sm text-muted-foreground">{{ $t('admin.privacy.erasureAccess') }}</p>
          <p class="text-sm text-muted-foreground">{{ $t('admin.privacy.erasureDelete') }}</p>
        </section>

        <section v-if="legalLinks.length" class="space-y-2">
          <h3 class="font-medium">{{ $t('admin.privacy.legal') }}</h3>
          <ul class="space-y-1 text-sm">
            <li v-for="l in legalLinks" :key="l.url">
              <a :href="l.url" target="_blank" rel="noopener" class="link-style">{{ l.label }}</a>
            </li>
          </ul>
        </section>
      </div>

      <AlertDialog v-model:open="confirmOpen">
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{{ $t('globals.messages.areYouAbsolutelySure') }}</AlertDialogTitle>
            <AlertDialogDescription>{{ $t('admin.privacy.warning') }}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{{ $t('globals.messages.cancel') }}</AlertDialogCancel>
            <AlertDialogAction variant="destructive" @click="save">{{ $t('globals.messages.save') }}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </template>
    <template #help>
      <p>{{ $t('admin.privacy.help') }}</p>
    </template>
  </AdminSplitLayout>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import api from '@main/api'
import AdminSplitLayout from '@main/layouts/admin/AdminSplitLayout.vue'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { useEmitter } from '@main/composables/useEmitter'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { Switch } from '@shared-ui/components/ui/switch'
import { RadioGroup, RadioGroupItem } from '@shared-ui/components/ui/radio-group'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle
} from '@shared-ui/components/ui/alert-dialog'

const DEFAULT_DAYS = 365

const { t } = useI18n()
const emitter = useEmitter()
const loaded = ref(false)
const saving = ref(false)
const confirmOpen = ref(false)
const mode = ref('off')
const days = ref(DEFAULT_DAYS)
const deleteContacts = ref(false)
const saved = ref({ days: 0, deleteContacts: false })
const limits = ref({ min: 7, max: 3650 })
const legal = ref({})

// What saving would store: 0 days keeps everything.
const current = computed(() => {
  const on = mode.value === 'on'
  return { days: on ? Number(days.value) : 0, deleteContacts: on && deleteContacts.value }
})
const changed = computed(
  () => current.value.days !== saved.value.days || current.value.deleteContacts !== saved.value.deleteContacts
)

const legalLinks = computed(() =>
  [
    { url: legal.value.privacy_policy_url, label: t('admin.privacy.privacyPolicy') },
    { url: legal.value.dpa_url, label: t('admin.privacy.dpa') },
    { url: legal.value.subprocessors_url, label: t('admin.privacy.subprocessors') }
  ].filter((l) => l.url)
)

const toastError = (error) =>
  emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })

const load = async () => {
  try {
    const d = (await api.getPrivacySettings()).data.data
    const n = d['privacy.retention_days'] || 0
    saved.value = { days: n, deleteContacts: d['privacy.retention_delete_contacts'] && n > 0 }
    mode.value = n > 0 ? 'on' : 'off'
    days.value = n > 0 ? n : DEFAULT_DAYS
    deleteContacts.value = saved.value.deleteContacts
    limits.value = { min: d.min_days, max: d.max_days }
    legal.value = d.legal || {}
    loaded.value = true
  } catch (error) {
    toastError(error)
  }
}

// Deleting more than before is confirmed first: deleted data is gone.
const confirmSave = () => {
  const c = current.value
  const s = saved.value
  const deletesMore =
    (c.days > 0 && (s.days === 0 || c.days < s.days)) || (c.deleteContacts && !s.deleteContacts)
  if (deletesMore) {
    confirmOpen.value = true
    return
  }
  save()
}

const save = async () => {
  confirmOpen.value = false
  saving.value = true
  try {
    await api.updatePrivacySettings({
      'privacy.retention_days': current.value.days,
      'privacy.retention_delete_contacts': current.value.deleteContacts
    })
    saved.value = { ...current.value }
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('globals.messages.savedSuccessfully') })
  } catch (error) {
    toastError(error)
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
