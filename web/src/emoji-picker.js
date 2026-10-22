import { insertTextAtCaret } from './composer-dom.js'

const emojis = ['😀', '😃', '😄', '😁', '😆', '😂', '🤣', '😊', '😍', '🥰', '😘', '😉', '🙃', '😎', '🥳', '😭', '😅', '😴', '🤔', '🙄', '😡', '👍', '👎', '👏', '🙏', '❤️', '💔', '🎉', '🔥', '🌹', '💯', '🐱']

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

  const grid = document.createElement('div')
  grid.className = 'emoji-picker-grid'
  grid.setAttribute('role', 'group')
  grid.setAttribute('aria-label', '常用表情')

  for (const emoji of emojis) {
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

  picker.append(toggle, grid)
  toolbar.prepend(picker)
}
