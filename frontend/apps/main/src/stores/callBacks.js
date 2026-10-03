import { ref } from 'vue'
import { defineStore } from 'pinia'
import api from '../api'

// The missed calls this agent may return: counts for the navigation badge, and
// a version the call-backs page watches to reload when the server says they
// changed.
export const useCallBackStore = defineStore('callBacks', () => {
  const counts = ref({ pending: 0, overdue: 0 })
  const version = ref(0)
  let timer = null

  const fetchCounts = async () => {
    try {
      const resp = await api.getCallBackCounts()
      counts.value = resp.data.data || { pending: 0, overdue: 0 }
    } catch {
      // The badge just keeps its last value.
    }
  }

  // Changes come in bursts (a missed call, then its WhatsApp message), so
  // reload once they settle.
  const changed = () => {
    clearTimeout(timer)
    timer = setTimeout(() => {
      version.value++
      fetchCounts()
    }, 400)
  }

  return { counts, version, fetchCounts, changed }
})
