<template>
  <div class="h-full max-w-xl space-y-10">
    <!-- Name and account details -->
    <form class="flex flex-col space-y-4" @submit.prevent="saveName">
      <div class="space-y-1">
        <span class="sub-title">{{ $t('account.yourDetails') }}</span>
        <p class="text-muted-foreground text-xs">{{ $t('account.yourDetailsDescription') }}</p>
      </div>
      <div class="grid gap-4 sm:grid-cols-2">
        <div class="space-y-1">
          <label class="text-sm text-muted-foreground" for="first_name">{{ $t('globals.terms.firstName') }}</label>
          <Input id="first_name" v-model.trim="firstName" maxlength="140" autocomplete="given-name" />
        </div>
        <div class="space-y-1">
          <label class="text-sm text-muted-foreground" for="last_name">{{ $t('globals.terms.lastName') }}</label>
          <Input id="last_name" v-model.trim="lastName" maxlength="140" autocomplete="family-name" />
        </div>
      </div>
      <div class="space-y-1">
        <label class="text-sm text-muted-foreground" for="phone_number">{{ $t('account.callPhone') }}</label>
        <Input id="phone_number" v-model.trim="phone" maxlength="20" autocomplete="tel" placeholder="+91 98765 43210" />
        <p class="text-xs text-muted-foreground">{{ $t('account.callPhone.description') }}</p>
      </div>
      <dl class="grid gap-3 text-sm sm:grid-cols-2">
        <div>
          <dt class="text-muted-foreground">{{ $t('globals.terms.email') }}</dt>
          <dd class="break-all">{{ userStore.email || '—' }}</dd>
        </div>
        <div v-if="workspace">
          <dt class="text-muted-foreground">{{ $t('globals.terms.workspace') }}</dt>
          <dd>{{ workspace }}</dd>
        </div>
        <div>
          <dt class="text-muted-foreground">{{ $t('globals.terms.role', 2) }}</dt>
          <dd>{{ userStore.roles.join(', ') || '—' }}</dd>
        </div>
        <div>
          <dt class="text-muted-foreground">{{ $t('globals.terms.team', 2) }}</dt>
          <dd>{{ teamNames || '—' }}</dd>
        </div>
      </dl>
      <p class="text-xs text-muted-foreground">{{ $t('account.emailChangeHint') }}</p>
      <Button type="submit" class="self-start" :isLoading="isSavingName" :disabled="(!nameChanged && !phoneChanged) || !firstName">
        {{ $t('globals.messages.saveChanges') }}
      </Button>
    </form>

    <div class="flex flex-col space-y-5">
      <div class="space-y-1">
        <span class="sub-title">{{ $t('account.publicAvatar') }}</span>
        <p class="text-muted-foreground text-xs">{{ $t('account.changeAvatar') }}</p>
      </div>
      <AvatarUpload
        :src="userStore.avatar"
        :initials="userStore.getInitials"
        :label="$t('globals.messages.upload')"
        :disabled="isSaving"
        @upload="onCropped"
        @remove="removeAvatar"
      />

      <Button
        class="self-start"
        @click="saveUser"
        :isLoading="isSaving"
        :disabled="!pendingFile"
      >
        {{ $t('globals.messages.saveChanges') }}
      </Button>
    </div>
  </div>
</template>

<script setup>
import { useUserStore } from '../../../stores/user'
import { Button } from '@shared-ui/components/ui/button'
import { AvatarUpload } from '@shared-ui/components/ui/avatar'
import { ref, computed, watch } from 'vue'
import { Input } from '@shared-ui/components/ui/input'
import { useAppSettingsStore } from '../../../stores/appSettings'
import { useEmitter } from '../../../composables/useEmitter'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { EMITTER_EVENTS } from '../../../constants/emitterEvents.js'
import { useI18n } from 'vue-i18n'
import api from '../../../api'

const emitter = useEmitter()
const { t } = useI18n()
const isSaving = ref(false)
const userStore = useUserStore()
const pendingFile = ref(null)
const settingsStore = useAppSettingsStore()

// Name, editable; the rest of the account is shown as it is.
const firstName = ref(userStore.firstName)
const lastName = ref(userStore.lastName)
watch(
  () => [userStore.firstName, userStore.lastName],
  ([f, l]) => {
    firstName.value = f
    lastName.value = l
  }
)
// The number the agent takes phone calls on, for the voice channel.
const phone = ref(userStore.phoneNumber || '')
watch(
  () => userStore.phoneNumber,
  (p) => {
    phone.value = p || ''
  }
)
const phoneChanged = computed(() => phone.value !== (userStore.phoneNumber || ''))
const nameChanged = computed(
  () => firstName.value !== userStore.firstName || lastName.value !== userStore.lastName
)
const teamNames = computed(() => (userStore.teams || []).map((t) => t.name || t).join(', '))
const workspace = computed(() => {
  const slug = settingsStore.public_config['app.workspace_slug']
  const site = settingsStore.public_config['app.site_name']
  return settingsStore.public_config['app.tenancy_enabled'] && slug ? `${site || slug} (${slug})` : ''
})
const isSavingName = ref(false)
const saveName = async () => {
  const formData = new FormData()
  formData.append('first_name', firstName.value)
  formData.append('last_name', lastName.value)
  formData.append('phone_number', phone.value)
  try {
    isSavingName.value = true
    const resp = await api.updateCurrentUser(formData)
    if (resp?.data?.data) userStore.setCurrentUser(resp.data.data)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('globals.messages.savedSuccessfully') })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  } finally {
    isSavingName.value = false
  }
}

const onCropped = (file) => {
  if (isSaving.value) return
  pendingFile.value = file
  userStore.setAvatar(URL.createObjectURL(file))
}

const saveUser = async () => {
  if (!pendingFile.value) return
  const formData = new FormData()
  formData.append('files', pendingFile.value, 'avatar.png')
  try {
    isSaving.value = true
    await api.updateCurrentUser(formData)
    pendingFile.value = null
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('globals.messages.savedSuccessfully')
    })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    isSaving.value = false
  }
}

const removeAvatar = async () => {
  if (isSaving.value) return
  try {
    await api.deleteUserAvatar()
    pendingFile.value = null
    userStore.clearAvatar()
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('account.avatarRemoved')
    })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  }
}
</script>
