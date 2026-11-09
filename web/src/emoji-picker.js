import { insertTextAtCaret } from './composer-dom.js'

// Keep the original inventory, including multi-codepoint emoji, in stable order.
export const originalEmojiCategories = [
  { name: '常用', emojis: ['😀', '😂', '🥰', '😍', '😎', '🤔', '😴', '😭', '😡', '👍', '👏', '🙏', '❤️', '💔', '🎉', '🔥', '💯', '🤝', '✅', '❌', '⭐', '🚀', '💡', '📌'] },
  { name: '笑脸', emojis: ['😀', '😃', '😄', '😁', '😆', '😅', '🤣', '😂', '🙂', '🙃', '😉', '😊', '😇', '🥰', '😍', '🤩', '😘', '😗', '😚', '😙', '😋', '😛', '😜', '🤪', '😝', '🤑', '🤗', '🤭', '🤫', '🤔', '🤐', '🤨', '😐', '😑', '😶', '😏'] },
  { name: '心情', emojis: ['😪', '😴', '😌', '😔', '🤤', '😷', '🤒', '🤕', '🤢', '🤮', '🥵', '🥶', '🥴', '😵', '🤯', '🤠', '🥳', '🥸', '😎', '🤓', '🧐', '😕', '😟', '🙁', '😮', '😯', '😲', '😳', '🥺', '😦', '😧', '😨', '😰', '😥', '😢', '😱'] },
  { name: '手势', emojis: ['👍', '👎', '👌', '🤌', '✌️', '🤞', '🤟', '🤘', '🤙', '👈', '👉', '👆', '👇', '☝️', '👏', '🙌', '👐', '🤲', '🤝', '🙏', '💪', '🦾', '✍️', '🤳'] },
  { name: '动物', emojis: ['🐱', '🐶', '🐭', '🐹', '🐰', '🦊', '🐻', '🐼', '🐨', '🐯', '🦁', '🐮', '🐷', '🐸', '🐵', '🙈', '🙉', '🙊', '🐔', '🐧', '🐦', '🦆', '🦉', '🦄'] },
  { name: '符号', emojis: ['❤️', '🧡', '💛', '💚', '💙', '💜', '🖤', '💔', '💯', '💢', '💥', '💫', '💦', '💨', '🎉', '🎊', '🔥', '⭐', '🌟', '✨', '⚡', '✅', '❌', '❓'] },
]

export const emojiCategories = [{
  name: 'emoji表情',
  emojis: [...new Set(originalEmojiCategories.flatMap(category => category.emojis))],
}]

// Root-owned state: repeat Vue updates refresh callbacks without resetting the UI.
const installations = new WeakMap()

export function installEmojiPicker(root, options = {}) {
  const existing = installations.get(root)
  if (existing?.picker.isConnected) {
    existing.update(options)
    return
  }
  const composer = root.closest('.composer')
  const toolbar = composer?.querySelector('.composer-toolbar') || composer?.querySelector(':scope > div:last-child')
  if (!toolbar || toolbar.querySelector('.emoji-picker')) return
  let currentOptions = options
  const userID = () => String(currentOptions.getUserID?.() ?? '')
  let owner = userID()
  let requestVersion = 0
  let selecting = false
  let selectionVersion = 0
  const syncOwner = () => {
    if (owner === userID()) return false
    owner = userID()
    selecting = false
    selectionVersion++
    return true
  }
  let active = 'emoji表情'
  const reportError = error => currentOptions.onError?.(error)

  const picker = document.createElement('div')
  picker.className = 'emoji-picker'

  const toggle = document.createElement('button')
  toggle.type = 'button'
  toggle.className = 'toolbar-icon emoji-picker-toggle'
  toggle.innerHTML = '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"><circle cx="12" cy="12" r="10" /><path d="M8 14s1.5 2 4 2 4-2 4-2M9 9h.01M15 9h.01" /></svg>'
  toggle.title = '选择表情'
  toggle.setAttribute('aria-label', '选择表情')
  toggle.setAttribute('aria-expanded', 'false')
  toggle.addEventListener('pointerdown', event => event.preventDefault())
  const close = () => {
    requestVersion++
    picker.classList.remove('open')
    toggle.setAttribute('aria-expanded', 'false')
  }
  toggle.addEventListener('click', () => {
    if (picker.classList.contains('open')) return close()
    syncOwner()
    picker.classList.add('open')
    toggle.setAttribute('aria-expanded', 'true')
    if (active === '表情') loadStickers()
  })

  const tabs = document.createElement('div')
  tabs.className = 'emoji-picker-tabs'
  tabs.setAttribute('role', 'tablist')
  tabs.setAttribute('aria-label', '表情分类')

  const grid = document.createElement('div')
  grid.className = 'emoji-picker-grid'
  grid.setAttribute('role', 'group')
  grid.setAttribute('aria-label', '表情')

  const renderGrid = () => {
    grid.classList.remove('emoji-picker-stickers')
    grid.removeAttribute('aria-busy')
    grid.replaceChildren()
    for (const emoji of emojiCategories[0].emojis) {
      const option = document.createElement('button')
      option.type = 'button'
      option.className = 'emoji-option'
      option.textContent = emoji
      option.setAttribute('aria-label', emoji)
      option.addEventListener('pointerdown', event => event.preventDefault())
      option.addEventListener('click', () => {
        insertTextAtCaret(root, emoji, window.getSelection())
        close()
      })
      grid.append(option)
    }
  }
  const renderTabs = () => {
    tabs.replaceChildren()
    for (const category of [emojiCategories[0], { name: '表情' }]) {
      const tab = document.createElement('button')
      tab.type = 'button'
      tab.className = 'emoji-picker-tab' + (category.name === active ? ' active' : '')
      tab.textContent = category.name
      tab.setAttribute('role', 'tab')
      tab.setAttribute('aria-selected', String(category.name === active))
      tab.addEventListener('pointerdown', event => event.preventDefault())
      tab.addEventListener('click', () => {
        if (active === category.name) return
        requestVersion++
        active = category.name
        syncOwner()
        renderTabs()
        if (active === '表情') loadStickers()
        else renderGrid()
      })
      tabs.append(tab)
    }
  }
  const status = (message, retry = false) => {
    grid.classList.add('emoji-picker-stickers')
    grid.replaceChildren()
    const text = document.createElement('div')
    text.className = 'emoji-picker-status'
    text.setAttribute('role', retry ? 'alert' : 'status')
    text.textContent = message
    if (retry) {
      const button = document.createElement('button')
      button.type = 'button'
      button.className = 'emoji-picker-retry'
      button.textContent = '重试'
      button.addEventListener('pointerdown', event => event.preventDefault())
      button.addEventListener('click', loadStickers)
      text.append(button)
    }
    grid.append(text)
  }
  const loadStickers = async () => {
    if (!picker.classList.contains('open') || active !== '表情') return
    const version = ++requestVersion
    const account = userID()
    syncOwner()
    const valid = () => version === requestVersion && account === userID()
      && root.isConnected && picker.isConnected && picker.classList.contains('open') && active === '表情'
    status('加载中…')
    grid.setAttribute('aria-busy', 'true')
    try {
      const stickers = await (currentOptions.getStickers?.() ?? [])
      if (!valid()) return
      grid.setAttribute('aria-busy', 'false')
      if (!stickers.length) return status('还没有保存的表情')
      grid.replaceChildren()
      for (const sticker of stickers) {
        const option = document.createElement('button')
        option.type = 'button'
        option.className = 'emoji-sticker-option'
        option.disabled = selecting
        option.setAttribute('aria-label', sticker.kind === 'emoji' ? sticker.text : (sticker.file_name || '私人表情'))
        if (sticker.kind === 'image') {
          const image = document.createElement('img')
          image.src = `/api/v1/stickers/${encodeURIComponent(sticker.id)}/media`
          image.alt = sticker.file_name || '私人表情'
          image.setAttribute('loading', 'lazy')
          image.setAttribute('decoding', 'async')
          option.append(image)
        } else {
          option.textContent = sticker.text
        }
        option.addEventListener('pointerdown', event => event.preventDefault())
        option.addEventListener('click', async () => {
          if (selecting || !valid() || !currentOptions.onSelectSticker) return
          selecting = true
          const selection = ++selectionVersion
          grid.setAttribute('aria-busy', 'true')
          grid.querySelectorAll('.emoji-sticker-option').forEach(button => { button.disabled = true })
          try {
            await currentOptions.onSelectSticker(sticker)
            if (valid()) close()
          } catch (error) {
            if (valid()) reportError(error)
          } finally {
            if (selection === selectionVersion) selecting = false
            if (selection === selectionVersion && root.isConnected && picker.isConnected && account === userID()) {
              grid.setAttribute('aria-busy', 'false')
              grid.querySelectorAll('.emoji-sticker-option').forEach(button => { button.disabled = false })
            }
          }
        })
        grid.append(option)
      }
    } catch (error) {
      if (!valid()) return
      grid.setAttribute('aria-busy', 'false')
      status('加载表情失败', true)
      reportError(error)
    }
  }
  renderTabs()
  renderGrid()

  // 标签和网格同住一个弹出面板，关闭时整体隐藏，不再有内容泄漏到输入框上。
  const panel = document.createElement('div')
  panel.className = 'emoji-picker-panel'
  panel.append(tabs, grid)
  picker.append(toggle, panel)
  toolbar.prepend(picker)
  const escape = event => {
    if (event.key === 'Escape' && picker.classList.contains('open')) {
      event.preventDefault()
      close()
    }
  }
  root.addEventListener('keydown', escape)
  picker.addEventListener('keydown', escape)
  root.addEventListener('pointerdown', close)
  installations.set(root, {
    picker,
    update(nextOptions) {
      currentOptions = nextOptions
      if (syncOwner()) {
        close()
        active = 'emoji表情'
        renderTabs()
        renderGrid()
      }
    },
  })
}
