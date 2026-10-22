import { initializeDesktop } from './desktop.js'

async function start() {
  try {
    await initializeDesktop()
    await import('./main.js')
  } catch (error) {
    const root = document.querySelector('#app')
    root.textContent = `启动失败：${String(error?.message || error)}`
    const button = document.createElement('button')
    button.textContent = '重新启动'
    button.onclick = () => location.reload()
    root.append(button)
  }
}
start()
