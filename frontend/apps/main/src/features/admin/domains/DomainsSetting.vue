<template>
  <AdminSplitLayout>
    <template #content>
      <div class="space-y-6" :class="{ 'opacity-50 transition-opacity duration-300': isLoading }">
        <form class="flex gap-2" @submit.prevent="addDomain">
          <Input
            v-model="hostname"
            :placeholder="$t('admin.domains.placeholder')"
            class="max-w-sm"
            autocomplete="off"
          />
          <Button type="submit" :disabled="!hostname.trim() || isAdding" :isLoading="isAdding">
            {{ $t('admin.domains.add') }}
          </Button>
        </form>

        <p class="text-sm text-muted-foreground">
          {{ $t('admin.domains.address', { address }) }}
        </p>

        <p v-if="!isLoading && domains.length === 0" class="text-sm text-muted-foreground">
          {{ $t('admin.domains.empty') }}
        </p>

        <div v-for="d in domains" :key="d.id" class="rounded-lg border p-4 space-y-4">
          <div class="flex flex-wrap items-center gap-2">
            <span class="font-medium break-all">{{ d.hostname }}</span>
            <Badge v-if="d.verified">{{ $t('admin.domains.verified') }}</Badge>
            <Badge v-else variant="outline">{{ $t('admin.domains.pending') }}</Badge>
            <Badge v-if="d.primary" variant="secondary">{{ $t('admin.domains.primary') }}</Badge>
            <Badge v-for="hc in d.help_centers" :key="hc" variant="secondary">
              {{ $t('admin.domains.helpCenter', { name: hc }) }}
            </Badge>
          </div>

          <p v-if="d.verified && d.failed_checks > 0" class="text-sm text-destructive">
            {{ $t('admin.domains.recheckFailing', { count: d.failed_checks, max: 3 }) }} {{ d.last_error }}
          </p>

          <template v-if="!d.verified">
            <div class="space-y-2 text-sm">
              <p>{{ $t('admin.domains.txtStep') }}</p>
              <DnsRecord type="TXT" :name="d.record_name" :value="d.record_value" />
              <p>{{ $t('admin.domains.cnameStep') }}</p>
              <DnsRecord type="CNAME" :name="d.hostname" :value="target" />
              <p class="text-muted-foreground">{{ $t('admin.domains.propagation') }}</p>
              <p v-if="d.last_error" class="text-destructive">{{ d.last_error }}</p>
            </div>
          </template>

          <div class="flex flex-wrap gap-2">
            <Button
              v-if="!d.verified"
              size="sm"
              :disabled="busyID === d.id"
              :isLoading="busyID === d.id"
              @click="verify(d)"
            >
              {{ $t('admin.domains.verify') }}
            </Button>
            <Button
              v-if="d.verified && !d.primary"
              size="sm"
              variant="outline"
              :disabled="busyID === d.id"
              @click="setPrimary(d.id)"
            >
              {{ $t('admin.domains.makePrimary') }}
            </Button>
            <Button
              v-if="d.primary"
              size="sm"
              variant="outline"
              :disabled="busyID === d.id"
              @click="setPrimary(0)"
            >
              {{ $t('admin.domains.useSubdomain', { address: target }) }}
            </Button>
            <Button size="sm" variant="ghost" @click="toRemove = d">
              {{ $t('globals.messages.delete') }}
            </Button>
          </div>
        </div>
      </div>

      <AlertDialog :open="!!toRemove" @update:open="(open) => !open && (toRemove = null)">
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{{ $t('globals.messages.areYouAbsolutelySure') }}</AlertDialogTitle>
            <AlertDialogDescription>
              {{ $t('admin.domains.removeConfirm', { domain: toRemove?.hostname }) }}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{{ $t('globals.messages.cancel') }}</AlertDialogCancel>
            <AlertDialogAction variant="destructive" @click="remove">
              {{ $t('globals.messages.delete') }}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </template>

    <template #help>
      <p>{{ $t('admin.domains.help.description') }}</p>
      <p>{{ $t('admin.domains.help.detail') }}</p>
    </template>
  </AdminSplitLayout>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import api from '@main/api'
import AdminSplitLayout from '@main/layouts/admin/AdminSplitLayout.vue'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { useEmitter } from '@main/composables/useEmitter'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Badge } from '@shared-ui/components/ui/badge'
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
import DnsRecord from './DnsRecord.vue'

const { t } = useI18n()
const emitter = useEmitter()
const domains = ref([])
const address = ref('')
const target = ref('')
const hostname = ref('')
const isLoading = ref(false)
const isAdding = ref(false)
const busyID = ref(0)
const toRemove = ref(null)

const toast = (description, variant) =>
  emitter.emit(EMITTER_EVENTS.SHOW_TOAST, variant ? { variant, description } : { description })
const toastError = (error) => toast(handleHTTPError(error).message, 'destructive')

const apply = (data) => {
  domains.value = data.domains
  address.value = data.address
  target.value = data.target
}

const load = async () => {
  try {
    isLoading.value = true
    apply((await api.getDomains()).data.data)
  } catch (error) {
    toastError(error)
  } finally {
    isLoading.value = false
  }
}

const addDomain = async () => {
  try {
    isAdding.value = true
    await api.createDomain({ hostname: hostname.value })
    hostname.value = ''
    await load()
  } catch (error) {
    toastError(error)
  } finally {
    isAdding.value = false
  }
}

const verify = async (d) => {
  try {
    busyID.value = d.id
    const updated = (await api.verifyDomain(d.id)).data.data
    if (updated.verified) {
      toast(t('admin.domains.verifiedToast', { domain: d.hostname }))
    }
    await load()
  } catch (error) {
    toastError(error)
  } finally {
    busyID.value = 0
  }
}

const setPrimary = async (id) => {
  try {
    busyID.value = id
    apply((await api.setPrimaryDomain(id)).data.data)
    toast(t('admin.domains.primaryToast', { address: address.value }))
  } catch (error) {
    toastError(error)
  } finally {
    busyID.value = 0
  }
}

const remove = async () => {
  const d = toRemove.value
  toRemove.value = null
  try {
    await api.deleteDomain(d.id)
    await load()
  } catch (error) {
    toastError(error)
  }
}

onMounted(load)
</script>
