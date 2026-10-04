import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useUserStore } from '@/stores/user'
import { useSoftphoneStore } from '@/stores/softphone'

// Where Exotel rings the agent first when they place a call: their browser,
// when they take calls there, or their own phone. Call buttons use it to tell
// whether the agent can call, and to have the browser pick up the call's leg
// to them when it arrives.
export function useCallLeg() {
  const { t } = useI18n()
  const userStore = useUserStore()
  const softphone = useSoftphoneStore()

  const inBrowser = computed(() => softphone.ringsBrowser)

  // Why the agent can't place a call now, or '' when they can.
  const notReadyReason = computed(() => {
    if (inBrowser.value) return softphone.ready ? '' : t('softphone.notReady')
    return userStore.phoneNumber ? '' : t('voice.callDisabled.noAgentPhone')
  })

  // Call before placing the call; then with its SID once the server returns it.
  const expectCall = (sid = '') => {
    if (inBrowser.value) softphone.expectCall(sid)
  }

  // The SID of a call the server placed, from a call message or a response.
  const callSID = (data) => data?.call_sid || data?.meta?.call?.sid || ''

  const startedMessage = computed(() => (inBrowser.value ? t('softphone.callStarted') : t('voice.callStarted')))

  return { inBrowser, notReadyReason, expectCall, callSID, startedMessage }
}
