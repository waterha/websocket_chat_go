import { ref, computed, nextTick } from 'vue/dist/vue.esm-bundler.js'
import { api } from '../api.js'

export function useAnnouncements({ active, current, canEditGroupAnnouncement, refresh, showError, getEpoch }) {
  // The right-hand panel holds two independent blocks: the announcement block
  // on top and the member list below. Both are always visible for a group, so
  // they no longer live in modal dialogs.
  const groupAnnouncements = ref([])
  const announcementsBusy = ref(false)
  const announcementsError = ref('')
  const announcementPanelOpen = ref(false)
  const announcementEditorOpen = ref(false)
  const announcementEditingID = ref('')
  const announcementDraftText = ref('')
  const announcementSaving = ref(false)
  const announcementCount = computed(() => groupAnnouncements.value.length)
  // The panel block shows the newest notice as a teaser; opening it reveals all.
  const latestAnnouncement = computed(() => groupAnnouncements.value[0] || null)
  const canManageAnnouncements = computed(() => canEditGroupAnnouncement.value)
  async function loadAnnouncements() {
    const id = active.value
    const version = getEpoch()
    if (!id || current.value?.kind !== 'group') {
      groupAnnouncements.value = []
      return
    }
    announcementsBusy.value = true
    announcementsError.value = ''
    try {
      const items = await api(`/conversations/${id}/announcements`)
      if (version !== getEpoch()) return
      groupAnnouncements.value = items || []
    } catch (cause) {
      if (version === getEpoch()) announcementsError.value = cause.message || '加载群公告失败'
    } finally { if (version === getEpoch()) announcementsBusy.value = false }
  }
  function openAnnouncementPanel() {
    announcementPanelOpen.value = true
    announcementEditorOpen.value = false
    loadAnnouncements().catch(showError)
  }
  function closeAnnouncementPanel() {
    announcementPanelOpen.value = false
    announcementEditorOpen.value = false
    announcementEditingID.value = ''
    announcementDraftText.value = ''
  }
  function focusAnnouncementEditor() {
    nextTick(() => {
      const body = document.querySelector('.announcement-body')
      if (body) body.scrollTop = 0
      document.querySelector('.announcement-editor textarea')?.focus({ preventScroll: true })
    })
  }
  function startCreateAnnouncement() {
    if (!canManageAnnouncements.value) return
    announcementEditingID.value = ''
    announcementDraftText.value = ''
    announcementEditorOpen.value = true
    focusAnnouncementEditor()
  }
  function startEditAnnouncement(item) {
    if (!canManageAnnouncements.value || !item) return
    announcementEditingID.value = item.id
    announcementDraftText.value = item.content
    announcementEditorOpen.value = true
    focusAnnouncementEditor()
  }
  function cancelAnnouncementEditor() {
    announcementEditorOpen.value = false
    announcementEditingID.value = ''
    announcementDraftText.value = ''
  }
  async function saveAnnouncementText() {
    const text = announcementDraftText.value.trim()
    const id = active.value
    if (!text || !id || announcementSaving.value) return
    const version = getEpoch()
    const editing = announcementEditingID.value
    announcementSaving.value = true
    try {
      if (editing) {
        await api(`/conversations/${id}/announcements/${editing}`, { method: 'PATCH', body: JSON.stringify({ content: text }) })
      } else {
        await api(`/conversations/${id}/announcements`, { method: 'POST', body: JSON.stringify({ content: text }) })
      }
      if (version !== getEpoch()) return
      cancelAnnouncementEditor()
      await loadAnnouncements()
      await refresh()
    } catch (cause) { if (version === getEpoch()) showError(cause) }
    finally { if (version === getEpoch()) announcementSaving.value = false }
  }
  async function deleteAnnouncement(item) {
    if (!canManageAnnouncements.value || !item || announcementSaving.value) return
    if (!confirm('删除后群成员将看不到这条公告，确认删除？')) return
    const id = active.value
    const version = getEpoch()
    announcementSaving.value = true
    try {
      await api(`/conversations/${id}/announcements/${item.id}`, { method: 'DELETE' })
      if (version !== getEpoch()) return
      await loadAnnouncements()
      await refresh()
    } catch (cause) { if (version === getEpoch()) showError(cause) }
    finally { if (version === getEpoch()) announcementSaving.value = false }
  }

  return {
    groupAnnouncements, announcementsBusy, announcementsError, announcementPanelOpen,
    announcementEditorOpen, announcementEditingID, announcementDraftText, announcementSaving,
    announcementCount, latestAnnouncement, canManageAnnouncements, loadAnnouncements,
    openAnnouncementPanel, closeAnnouncementPanel, startCreateAnnouncement, startEditAnnouncement,
    cancelAnnouncementEditor, saveAnnouncementText, deleteAnnouncement,
  }
}
