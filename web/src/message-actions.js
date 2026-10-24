import { ref, computed, reactive, nextTick, watch, onMounted, onUnmounted } from 'vue/dist/vue.esm-bundler.js'
import { api, uploadMedia } from './api.js'
import { mediaURL } from './media.js'
import { hasBlocks, messageSegments } from './composer-blocks.js'
import { canRecall, RECALL_WINDOW_MS } from './reply.js'

export const isActionableMessage = message => Boolean(message?.id && message.kind !== 'system' && !message.recalled_at)

export function messageCopyText(message) {
  return messageSegments(message).map(segment => segment.type === 'text'
    ? segment.text
    : `[${segment.type === 'image' ? '图片' : segment.type === 'video' ? '视频' : '文件'}]${segment.fileName ? ' ' + segment.fileName : ''}`).join('\n')
}

const escapeHTML = value => String(value).replace(/[&<>"']/g, character => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[character])

async function imageDataURL(segment) {
  const response = await fetch(mediaURL(segment.mediaId), { credentials: 'same-origin' })
  if (!response.ok) throw new Error('图片加载失败，请稍后重试')
  const blob = await response.blob()
  const bytes = new Uint8Array(await blob.arrayBuffer())
  let binary = ''
  for (let offset = 0; offset < bytes.length; offset += 0x8000) binary += String.fromCharCode(...bytes.subarray(offset, offset + 0x8000))
  return `data:${blob.type || 'image/png'};base64,${btoa(binary)}`
}

export async function messageCopyHTML(items) {
  const rows = await Promise.all(items.filter(isActionableMessage).map(async message => {
    const segments = await Promise.all(messageSegments(message).map(async segment => segment.type === 'text'
      ? `<p class="block-text">${escapeHTML(segment.text)}</p>`
      : segment.type === 'image'
        ? `<a class="block-image"><img src="${await imageDataURL(segment)}" alt="${escapeHTML(segment.fileName || '图片')}" /></a>`
        : `<span>[${segment.type === 'video' ? '视频' : '文件'}] ${escapeHTML(segment.fileName || '')}</span>`))
    return `<div><div class="block-stack">${segments.join('')}</div></div>`
  }))
  return `<html><body>${rows.join('')}</body></html>`
}

export async function prepareForward(message, conversationID, clientMsgID = crypto.randomUUID()) {
  if (!isActionableMessage(message)) throw new Error('该消息无法转发')
  const payload = { client_msg_id: clientMsgID, content: message.content || '', media_id: '', blocks: [], reply_to_id: '', mentions: [] }
  for (const segment of messageSegments(message)) {
    if (segment.type === 'text') {
      if (hasBlocks(message)) payload.blocks.push({ type: 'text', text: segment.text })
      continue
    }
    const response = await fetch(mediaURL(segment.mediaId), { credentials: 'same-origin' })
    if (!response.ok) throw new Error('原附件已撤回或无法下载，不能转发')
    const blob = await response.blob()
    const file = new File([blob], segment.fileName || 'attachment', { type: blob.type || 'application/octet-stream' })
    const media = await uploadMedia(conversationID, file)
    if (hasBlocks(message)) payload.blocks.push({ type: 'image', media_id: media.id })
    else payload.media_id = media.id
  }
  return payload
}

export function useMessageActions({ messages, threadRoot, threadReplies, active, me, current, conversations, startReply, recallMessage, syncConversation, showError }) {
  const menu = ref(null)
  const menuElement = ref(null)
  const menuNow = ref(Date.now())
  const selectionMode = ref(false)
  const selectedIDs = ref([])
  const forwardQueue = ref([])
  const forwardTarget = ref('')
  const forwardBusy = ref(false)
  const forwardError = ref('')
  const forwardLocked = ref(false)
  const forwardPayloads = new Map()
  let recallTimer
  let forwardVersion = 0
  const available = computed(() => new Map([...threadReplies.value, threadRoot.value, ...messages.value].filter(Boolean).map(item => [item.id, item])))
  const menuMessage = computed(() => {
    const item = available.value.get(menu.value?.id)
    return isActionableMessage(item) ? item : null
  })
  const selected = computed(() => [...available.value.values()].filter(item => selectedIDs.value.includes(item.id) && isActionableMessage(item)).sort((left, right) => left.seq - right.seq))
  const recallAllowed = computed(() => menuMessage.value?.sender_id === me.value?.id
    && Number.isFinite(Date.parse(menuMessage.value?.created_at))
    && canRecall(menuMessage.value, me.value?.id, current.value, menuNow.value))
  const forwardTargets = computed(() => conversations.value)

  function closeMenu() {
    clearTimeout(recallTimer)
    menu.value = null
  }

  async function openMenu(event, item) {
    closeMenu()
    if (!isActionableMessage(item)) return
    menuNow.value = Date.now()
    const opened = { id: item.id, x: event.clientX || 8, y: event.clientY || 8 }
    menu.value = opened
    if (item.sender_id === me.value?.id && recallAllowed.value) {
      recallTimer = setTimeout(() => { menuNow.value = Date.now() }, Math.max(0, Date.parse(item.created_at) + RECALL_WINDOW_MS - Date.now() + 1))
    }
    await nextTick()
    if (menu.value?.id !== opened.id || !menuElement.value) return
    const bounds = menuElement.value.getBoundingClientRect()
    menu.value.x = Math.max(8, Math.min(opened.x, window.innerWidth - bounds.width - 8))
    menu.value.y = Math.max(8, Math.min(opened.y, window.innerHeight - bounds.height - 8))
    menuElement.value.querySelector('button[role="menuitem"]')?.focus({ preventScroll: true })
  }

  function cancelSelection() {
    selectionMode.value = false
    selectedIDs.value = []
  }

  function toggleSelected(item) {
    if (!isActionableMessage(item)) return
    selectedIDs.value = selectedIDs.value.includes(item.id) ? selectedIDs.value.filter(id => id !== item.id) : [...selectedIDs.value, item.id]
  }

  function onMessageClick(event, item) {
    if (!selectionMode.value) return
    event.preventDefault()
    event.stopPropagation()
    toggleSelected(item)
  }

  async function copyMessages(items) {
    const valid = items.filter(isActionableMessage)
    const text = valid.map(messageCopyText).join('\n')
    if (!valid.length || !text) return
    try {
      const hasImages = valid.some(message => messageSegments(message).some(segment => segment.type === 'image'))
      if (hasImages) {
        if (!window.navigator.clipboard?.write || !window.ClipboardItem) throw new Error('当前浏览器不支持复制图片，请使用最新版 Chrome 或 Edge')
        await window.navigator.clipboard.write([new window.ClipboardItem({
          'text/plain': new Blob([text], { type: 'text/plain' }),
          'text/html': messageCopyHTML(valid).then(html => new Blob([html], { type: 'text/html' })),
        })])
      } else if (window.navigator.clipboard?.writeText) await window.navigator.clipboard.writeText(text)
      else {
        const previous = document.activeElement
        const area = document.createElement('textarea')
        area.value = text
        area.style.cssText = 'position:fixed;left:-9999px;top:0'
        document.body.append(area)
        try {
          area.select()
          if (!document.execCommand?.('copy')) throw new Error('复制失败，请允许剪贴板访问')
        } finally { area.remove(); previous?.focus({ preventScroll: true }) }
      }
    } catch (cause) { showError(cause) }
  }

  function openForward(items) {
    const eligible = items.filter(isActionableMessage)
    if (!eligible.length) return
    forwardQueue.value = eligible
    forwardTarget.value = ''
    forwardError.value = ''
    forwardLocked.value = false
    forwardPayloads.clear()
  }

  function closeForward() {
    if (forwardBusy.value) return
    forwardVersion++
    forwardQueue.value = []
    forwardPayloads.clear()
    forwardError.value = ''
  }

  async function confirmForward() {
    const target = forwardTarget.value
    if (forwardBusy.value || !forwardQueue.value.length || !forwardTargets.value.some(item => item.id === target)) return
    const version = ++forwardVersion
    forwardBusy.value = true
    forwardLocked.value = true
    forwardError.value = ''
    try {
      while (forwardQueue.value.length && version === forwardVersion) {
        const source = forwardQueue.value[0]
        if (!isActionableMessage(available.value.get(source.id) || source)) throw new Error('原消息已撤回，不能转发')
        let payload = forwardPayloads.get(source.id)
        if (!payload) {
          payload = await prepareForward(source, target)
          if (version !== forwardVersion) return
          forwardPayloads.set(source.id, payload)
        }
        await api(`/conversations/${target}/messages`, { method: 'POST', body: JSON.stringify(payload) })
        if (version !== forwardVersion) return
        forwardPayloads.delete(source.id)
        forwardQueue.value = forwardQueue.value.slice(1)
      }
      cancelSelection()
      forwardPayloads.clear()
      try { await syncConversation(target) } catch (cause) { showError(cause) }
    } catch (cause) {
      if (version === forwardVersion) forwardError.value = `${cause.message || '转发失败'}。已成功的消息不会重发，请重试剩余消息。`
    } finally { if (version === forwardVersion) forwardBusy.value = false }
  }

  function runMenu(action) {
    const item = menuMessage.value
    const allowed = recallAllowed.value
    closeMenu()
    if (!item) return
    if (action === 'copy') copyMessages([item])
    if (action === 'reply') startReply(item)
    if (action === 'forward') openForward([item])
    if (action === 'select') { selectionMode.value = true; selectedIDs.value = [item.id] }
    if (action === 'recall' && allowed) recallMessage(item)
  }

  function onOutsidePointer(event) {
    if (!menuElement.value?.contains(event.target)) closeMenu()
  }

  function onKeydown(event) {
    if (event.key === 'Escape') {
      if (menu.value) closeMenu()
      else if (forwardQueue.value.length) closeForward()
      else cancelSelection()
    }
    if (!menu.value || !['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const buttons = [...menuElement.value.querySelectorAll('button:not(:disabled)')]
    const index = buttons.indexOf(document.activeElement)
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : (index + (event.key === 'ArrowDown' ? 1 : -1) + buttons.length) % buttons.length
    buttons[next]?.focus({ preventScroll: true })
  }

  function reset() {
    closeMenu()
    cancelSelection()
    forwardBusy.value = false
    closeForward()
  }

  watch([active, me], reset)
  onMounted(() => {
    document.addEventListener('pointerdown', onOutsidePointer)
    document.addEventListener('keydown', onKeydown)
    document.addEventListener('scroll', closeMenu, true)
    window.addEventListener('resize', closeMenu)
  })
  onUnmounted(() => {
    reset()
    document.removeEventListener('pointerdown', onOutsidePointer)
    document.removeEventListener('keydown', onKeydown)
    document.removeEventListener('scroll', closeMenu, true)
    window.removeEventListener('resize', closeMenu)
  })

  return reactive({ menu, menuElement, menuMessage, recallAllowed, selectionMode, selectedIDs, selected, forwardQueue, forwardTarget, forwardBusy, forwardError, forwardLocked, forwardTargets, openMenu, closeMenu, runMenu, toggleSelected, onMessageClick, cancelSelection, copyMessages, openForward, closeForward, confirmForward })
}
