import { ref } from 'vue/dist/vue.esm-bundler.js'
import { api } from '../api.js'

export function useContacts({ showSettings, friends, showError, getEpoch }) {
  // Contact management (remark, deletion, blocking) lives in one dialog.
  const showContacts = ref(false)
  const contactBusy = ref(false)
  const remarkEditingID = ref('')
  const remarkDraft = ref('')
  const blockedUsers = ref([])
  // ---- contact management ----
  async function openContacts() {
    showSettings.value = false
    showContacts.value = true
    const version = getEpoch()
    try {
      const list = await api('/blocks')
      if (version === getEpoch()) blockedUsers.value = list
    } catch (cause) { if (version === getEpoch()) showError(cause) }
  }
  function startRemark(person) {
    remarkEditingID.value = person.id
    remarkDraft.value = person.remark || ''
  }
  async function saveRemark(person) {
    if (contactBusy.value) return
    contactBusy.value = true
    try {
      const updated = await api(`/friends/${person.id}`, { method: 'PATCH', body: JSON.stringify({ remark: remarkDraft.value }) })
      const friend = friends.value.find(item => item.id === person.id)
      if (friend) friend.remark = updated.remark
      remarkEditingID.value = ''
    } catch (cause) { showError(cause) }
    finally { contactBusy.value = false }
  }
  async function removeFriend(person) {
    if (contactBusy.value) return
    if (!confirm(`确认删除好友 ${person.remark || person.name}？聊天记录不受影响。`)) return
    contactBusy.value = true
    try {
      await api(`/friends/${person.id}`, { method: 'DELETE' })
      friends.value = friends.value.filter(item => item.id !== person.id)
    } catch (cause) { showError(cause) }
    finally { contactBusy.value = false }
  }
  const isBlocked = id => blockedUsers.value.some(entry => entry.id === id)
  async function blockUser(person) {
    if (contactBusy.value || isBlocked(person.id)) return
    if (!confirm(`确认将 ${person.remark || person.name} 加入黑名单？拉黑后不再收到对方的私聊与好友申请。`)) return
    contactBusy.value = true
    try {
      await api('/blocks', { method: 'POST', body: JSON.stringify({ user_id: person.id }) })
      blockedUsers.value = await api('/blocks')
    } catch (cause) { showError(cause) }
    finally { contactBusy.value = false }
  }
  async function unblockUser(person) {
    if (contactBusy.value) return
    contactBusy.value = true
    try {
      await api(`/blocks/${person.id}`, { method: 'DELETE' })
      blockedUsers.value = await api('/blocks')
    } catch (cause) { showError(cause) }
    finally { contactBusy.value = false }
  }

  return {
    showContacts, contactBusy, remarkEditingID, remarkDraft, blockedUsers, openContacts, startRemark,
    saveRemark, removeFriend, isBlocked, blockUser, unblockUser,
  }
}
