import { ref } from 'vue/dist/vue.esm-bundler.js'

export function useVoice({ canRecord, getContext, isCurrent, onSend, showError, documentRef = document, windowRef = window, mediaDevices = navigator.mediaDevices, Recorder = globalThis.MediaRecorder }) {
  const enabled = ref(false), state = ref('idle'), seconds = ref(0)
  let session, held = false, generation = 0
  const supported = Boolean(mediaDevices?.getUserMedia && Recorder)
  function release(item) {
    clearInterval(item?.timer)
    for (const track of item?.stream?.getTracks?.() || []) track.stop()
    if (item) item.stream = null
  }
  function cancel() {
    generation++; held = false
    const item = session
    session = null
    if (item) {
      item.cancelled = true
      if (item.recorder?.state !== 'inactive') { try { item.recorder?.stop() } catch {} }
      release(item)
    }
    state.value = 'idle'; seconds.value = 0
  }
  function reset() { enabled.value = false; cancel() }
  function toggle() {
    if (enabled.value) { reset(); return }
    if (!supported) { showError(new Error('当前环境不支持麦克风录音，请使用 localhost、HTTPS 或桌面客户端')); return }
    enabled.value = true
    documentRef.activeElement?.blur?.()
  }
  async function start() {
    if (state.value !== 'idle' || !canRecord()) return
    const version = ++generation
    const item = { context: getContext(), chunks: [], cancelled: false }
    session = item; state.value = 'requesting'
    try {
      item.stream = await mediaDevices.getUserMedia({ audio: true })
      if (version !== generation || !held || !isCurrent(item.context) || !canRecord()) { release(item); if (session === item) { session = null; state.value = 'idle' } return }
      const mimeType = ['audio/webm;codecs=opus', 'audio/webm', 'audio/ogg;codecs=opus', 'audio/mp4'].find(type => Recorder.isTypeSupported?.(type))
      const recorder = item.recorder = new Recorder(item.stream, mimeType ? { mimeType } : undefined)
      recorder.ondataavailable = event => { if (event.data.size) item.chunks.push(event.data) }
      recorder.onerror = () => { if (session === item) { cancel(); showError(new Error('录音失败，请检查麦克风后重试')) } }
      recorder.onstop = async () => {
        release(item)
        if (item.cancelled || version !== generation || !isCurrent(item.context)) return
        session = null
        if (Date.now() - item.started < 300) { state.value = 'idle'; showError(new Error('录音太短，请按住空格至少半秒')); return }
        const type = (recorder.mimeType || item.chunks[0]?.type || 'audio/webm').split(';')[0]
        const blob = new Blob(item.chunks, { type })
        if (!blob.size || blob.size > 10 * 1024 * 1024) { state.value = 'idle'; showError(new Error('录音为空或超过 10 MB，请重新录音')); return }
        const extension = type === 'audio/mp4' ? 'm4a' : type === 'audio/ogg' ? 'ogg' : 'webm'
        state.value = 'sending'
        try { await onSend(new File([blob], `语音-${Date.now()}.${extension}`, { type }), item.context) }
        catch (cause) { if (version === generation) showError(cause) }
        finally { if (version === generation) state.value = 'idle' }
      }
      recorder.start(250)
      item.started = Date.now(); seconds.value = 0; state.value = 'recording'
      item.timer = setInterval(() => { seconds.value = Math.floor((Date.now() - item.started) / 1000); if (seconds.value >= 60) stop() }, 200)
    } catch (cause) {
      release(item)
      if (version !== generation) return
      session = null; state.value = 'idle'
      showError(new Error(cause.name === 'NotAllowedError' ? '麦克风权限未开启，请在浏览器或系统设置中允许麦克风后重试' : '无法打开麦克风，请检查设备是否连接或被占用'))
    }
  }
  function stop() {
    if (state.value === 'requesting') { cancel(); return }
    if (state.value !== 'recording' || !session?.recorder) return
    state.value = 'sending'
    session.recorder.stop()
    release(session)
  }
  function keydown(event) {
    if (!enabled.value) return
    if (event.key === 'Escape') {
      if (state.value === 'recording' || state.value === 'requesting') { cancel(); event.preventDefault(); event.stopPropagation() }
      return
    }
    if (event.code !== 'Space' && event.key !== ' ') return
    if (event.ctrlKey || event.metaKey || event.altKey || event.shiftKey || event.isComposing) return
    const target = event.target
    if (documentRef.querySelector('.modal-backdrop,dialog[open]') || target?.closest?.('input,textarea,select,button,a,audio,video,[role="dialog"]')) return
    event.preventDefault(); event.stopPropagation()
    if (event.repeat || held) return
    held = true
    start()
  }
  function keyup(event) {
    if (!held || (event.code !== 'Space' && event.key !== ' ')) return
    held = false; event.preventDefault(); event.stopPropagation(); stop()
  }
  const blur = () => { if (state.value === 'recording' || state.value === 'requesting') cancel() }
  const visibility = () => { if (documentRef.hidden) blur() }
  documentRef.addEventListener('keydown', keydown, true)
  documentRef.addEventListener('keyup', keyup, true)
  windowRef.addEventListener('blur', blur)
  documentRef.addEventListener('visibilitychange', visibility)
  function dispose() {
    reset()
    documentRef.removeEventListener('keydown', keydown, true)
    documentRef.removeEventListener('keyup', keyup, true)
    windowRef.removeEventListener('blur', blur)
    documentRef.removeEventListener('visibilitychange', visibility)
  }
  return { enabled, state, seconds, toggle, reset, cancel, dispose }
}
