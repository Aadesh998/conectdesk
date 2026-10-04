import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import sdkURL from '@exotel-npm-dev/webrtc-client-sdk/dist/exotelsdk.js?url'
import api from '@/api'

// Browser calling. An agent with a SIP account at Exotel takes calls in this
// tab over WebRTC instead of on their phone: their browser signs in to the
// SIP account, and Exotel rings it for the calls they place (click-to-call
// rings the agent first) and for the customers who call in.

// Only one tab signs in: Exotel rings one registration, so the others wait
// for this lock and take over when the tab holding it closes.
const LOCK = 'conectdesk-softphone'
// How long after placing a call its agent leg is answered without asking.
const OUTBOUND_WINDOW_MS = 60 * 1000

// The SDK's prebuilt bundle, loaded as a plain script only for agents who use
// it. Its source requires .wav files only its own build can load, and the
// bundle finds its files from document.currentScript, which a module import
// doesn't set.
let sdk = null
const loadSDK = () => {
  sdk ||= new Promise((resolve, reject) => {
    const el = document.createElement('script')
    el.src = sdkURL
    el.async = true
    el.onload = () =>
      window.exotelSDK?.ExotelWebClient ? resolve(window.exotelSDK.ExotelWebClient) : reject(new Error('no ExotelWebClient'))
    el.onerror = () => {
      sdk = null
      el.remove()
      reject(new Error('the Exotel SDK failed to load'))
    }
    document.head.appendChild(el)
  })
  return sdk
}

export const useSoftphoneStore = defineStore('softphone', () => {
  // The agent's SIP account, with its password, from the server.
  const account = ref(null)
  // off, waiting (another tab has it), connecting, ready, failed, mic_blocked.
  const status = ref('off')
  // The call in the browser: { sid, from, incoming, state: ringing | connected,
  // muted, held, connectedAt }.
  const call = ref(null)

  let client = null
  let releaseLock = null
  let abortLock = null
  // The call the agent just placed, whose leg to them is answered on arrival.
  let expected = null

  const configured = computed(() => !!account.value?.configured)
  const ringsBrowser = computed(() => configured.value && !!account.value.ring_browser)
  const ready = computed(() => status.value === 'ready')

  const load = async () => {
    try {
      const resp = await api.getOwnSoftphone()
      account.value = resp.data.data
    } catch {
      account.value = null
    }
    if (ringsBrowser.value) start()
    else stop()
  }

  const setRingBrowser = async (on) => {
    await api.updateOwnSoftphone({ ring_browser: on })
    account.value = { ...account.value, ring_browser: on }
    if (on) start()
    else stop()
  }

  const start = () => {
    if (status.value !== 'off' || !ringsBrowser.value) return
    if (!navigator.locks) {
      connect()
      return
    }
    status.value = 'waiting'
    abortLock = new AbortController()
    navigator.locks
      .request(LOCK, { signal: abortLock.signal }, () => {
        abortLock = null
        connect()
        // Held until stop() or the tab closes.
        return new Promise((resolve) => (releaseLock = resolve))
      })
      .catch(() => {})
  }

  const connect = async () => {
    status.value = 'connecting'
    // Ask for the microphone now, not when a call is already ringing.
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      stream.getTracks().forEach((t) => t.stop())
    } catch {
      status.value = 'mic_blocked'
      return
    }
    try {
      const ExotelWebClient = await loadSDK()
      const a = account.value
      client = new ExotelWebClient()
      const ok = await client.initWebrtc(
        {
          userName: a.username,
          authUser: a.username,
          secret: a.password,
          sipdomain: a.domain,
          domain: `${a.server || a.domain}:${a.port}`,
          displayname: a.username,
          port: String(a.port),
          security: 'wss',
          endpoint: 'wss'
        },
        onRegister,
        onCall,
        () => {}
      )
      if (!ok) throw new Error('initWebrtc refused the account')
      client.DoRegister()
    } catch (err) {
      console.error('browser calling:', err)
      status.value = 'failed'
    }
  }

  const stop = () => {
    abortLock?.abort()
    abortLock = null
    if (client) {
      try {
        client.UnRegister()
      } catch {
        // Already gone.
      }
      client = null
    }
    releaseLock?.()
    releaseLock = null
    call.value = null
    status.value = 'off'
  }

  const onRegister = (state) => {
    if (!client) return
    if (state === 'registered') status.value = 'ready'
    else if (state === 'unregistered') status.value = 'failed'
  }

  const onCall = (details, event) => {
    if (event === 'incoming') {
      const sid = details?.callSid || ''
      const placed = expected && Date.now() < expected.until && (!expected.sid || !sid || expected.sid === sid)
      call.value = {
        sid,
        from: details?.remoteDisplayName || details?.remoteId || '',
        incoming: !placed,
        state: 'ringing',
        muted: false,
        held: false,
        connectedAt: null
      }
      // The leg to the agent of a call they placed: pick it up, and Exotel
      // goes on to ring the customer.
      if (placed) {
        expected = null
        answer()
      }
    } else if (event === 'connected') {
      if (call.value) call.value = { ...call.value, state: 'connected', connectedAt: Date.now() }
    } else if (event === 'callEnded') {
      call.value = null
    }
  }

  // expectCall marks a call the agent is about to place, or has placed with
  // the given SID, so its leg to them is answered when it arrives.
  const expectCall = (sid = '') => {
    expected = { sid, until: Date.now() + OUTBOUND_WINDOW_MS }
  }

  const control = (fn) => {
    try {
      const c = client?.getCall()
      if (c) fn(c)
    } catch (err) {
      console.error('browser calling:', err)
    }
  }
  const answer = () => control((c) => c.Answer())
  const hangup = () => {
    control((c) => c.Hangup())
    call.value = null
  }
  const toggleMute = () =>
    control((c) => {
      c.MuteToggle()
      call.value = { ...call.value, muted: !call.value.muted }
    })
  const toggleHold = () =>
    control((c) => {
      c.HoldToggle()
      call.value = { ...call.value, held: !call.value.held }
    })
  const sendDigit = (d) => control((c) => c.sendDTMF(d))

  return {
    account,
    status,
    call,
    configured,
    ringsBrowser,
    ready,
    load,
    setRingBrowser,
    start,
    stop,
    expectCall,
    answer,
    hangup,
    toggleMute,
    toggleHold,
    sendDigit
  }
})
