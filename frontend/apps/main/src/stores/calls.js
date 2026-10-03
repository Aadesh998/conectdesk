import { ref } from 'vue'
import { defineStore } from 'pinia'

// Incoming phone calls ringing this agent right now, shown as a pop-up.
export const useCallStore = defineStore('calls', () => {
  const ringing = ref([])

  const addIncoming = (call) => {
    if (!call?.call_sid || ringing.value.some((c) => c.call_sid === call.call_sid)) return
    ringing.value.push({ ...call, at: Date.now() })
    // Drop a call whose end we never heard about, e.g. across a reconnect.
    setTimeout(() => remove(call.call_sid), 2 * 60 * 1000)
  }

  const remove = (sid) => {
    ringing.value = ringing.value.filter((c) => c.call_sid !== sid)
  }

  return { ringing, addIncoming, remove }
})
