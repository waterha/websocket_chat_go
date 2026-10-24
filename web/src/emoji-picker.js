import { insertTextAtCaret } from './composer-dom.js'

// 分类表情面板：按常用/笑脸/心情/手势/动物/符号分组，标签切换。
export const emojiCategories = [
  { name: '常用', emojis: ['😀', '😂', '🥰', '😍', '😎', '🤔', '😴', '😭', '😡', '👍', '👏', '🙏', '❤️', '💔', '🎉', '🔥', '💯', '🤝', '✅', '❌', '⭐', '🚀', '💡', '📌'] },
  { name: '笑脸', emojis: ['😀', '😃', '😄', '😁', '😆', '😅', '🤣', '😂', '🙂', '🙃', '😉', '😊', '😇', '🥰', '😍', '🤩', '😘', '😗', '😚', '😙', '😋', '😛', '😜', '🤪', '😝', '🤑', '🤗', '🤭', '🤫', '🤔', '🤐', '🤨', '😐', '😑', '😶', '😏'] },
  { name: '心情', emojis: ['😪', '😴', '😌', '😔', '🤤', '😷', '🤒', '🤕', '🤢', '🤮', '🥵', '🥶', '🥴', '😵', '🤯', '🤠', '🥳', '🥸', '😎', '🤓', '🧐', '😕', '😟', '🙁', '😮', '😯', '😲', '😳', '🥺', '😦', '😧', '😨', '😰', '😥', '😢', '😱'] },
  { name: '手势', emojis: ['👍', '👎', '👌', '🤌', '✌️', '🤞', '🤟', '🤘', '🤙', '👈', '👉', '👆', '👇', '☝️', '👏', '🙌', '👐', '🤲', '🤝', '🙏', '💪', '🦾', '✍️', '🤳'] },
  { name: '动物', emojis: ['🐱', '🐶', '🐭', '🐹', '🐰', '🦊', '🐻', '🐼', '🐨', '🐯', '🦁', '🐮', '🐷', '🐸', '🐵', '🙈', '🙉', '🙊', '🐔', '🐧', '🐦', '🦆', '🦉', '🦄'] },
  { name: '符号', emojis: ['❤️', '🧡', '💛', '💚', '💙', '💜', '🖤', '💔', '💯', '💢', '💥', '💫', '💦', '💨', '🎉', '🎊', '🔥', '⭐', '🌟', '✨', '⚡', '✅', '❌', '❓'] },
]

export function installEmojiPicker(root) {
  const toolbar = root.closest('.composer')?.querySelector(':scope > div:last-child')
  if (!toolbar || toolbar.querySelector('.emoji-picker')) return

  const picker = document.createElement('div')
  picker.className = 'emoji-picker'

  const toggle = document.createElement('button')
  toggle.type = 'button'
  toggle.className = 'icon-button emoji-picker-toggle'
  toggle.textContent = '☺'
  toggle.title = '选择表情'
  toggle.setAttribute('aria-label', '选择表情')
  toggle.setAttribute('aria-expanded', 'false')
  toggle.addEventListener('pointerdown', event => event.preventDefault())
  toggle.addEventListener('click', () => {
    picker.classList.toggle('open')
    toggle.setAttribute('aria-expanded', String(picker.classList.contains('open')))
  })

  const tabs = document.createElement('div')
  tabs.className = 'emoji-picker-tabs'
  tabs.setAttribute('role', 'tablist')
  tabs.setAttribute('aria-label', '表情分类')

  const grid = document.createElement('div')
  grid.className = 'emoji-picker-grid'
  grid.setAttribute('role', 'group')
  grid.setAttribute('aria-label', '表情')

  let active = emojiCategories[0].name
  const renderGrid = () => {
    grid.replaceChildren()
    const category = emojiCategories.find(entry => entry.name === active) || emojiCategories[0]
    for (const emoji of category.emojis) {
      const option = document.createElement('button')
      option.type = 'button'
      option.className = 'emoji-option'
      option.textContent = emoji
      option.setAttribute('aria-label', emoji)
      option.addEventListener('pointerdown', event => event.preventDefault())
      option.addEventListener('click', () => {
        insertTextAtCaret(root, emoji, window.getSelection())
        picker.classList.remove('open')
        toggle.setAttribute('aria-expanded', 'false')
      })
      grid.append(option)
    }
  }
  const renderTabs = () => {
    tabs.replaceChildren()
    for (const category of emojiCategories) {
      const tab = document.createElement('button')
      tab.type = 'button'
      tab.className = 'emoji-picker-tab' + (category.name === active ? ' active' : '')
      tab.textContent = category.name
      tab.setAttribute('role', 'tab')
      tab.setAttribute('aria-selected', String(category.name === active))
      tab.addEventListener('click', () => {
        active = category.name
        renderTabs()
        renderGrid()
      })
      tabs.append(tab)
    }
  }
  renderTabs()
  renderGrid()

  picker.append(toggle, tabs, grid)
  toolbar.prepend(picker)
}
