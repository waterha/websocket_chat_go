export function appendPage(messages, page, conversationID) {
  let expected = (messages.at(-1)?.seq || 0) + 1
  for (const message of page.messages) {
    if (message.conversation_id !== conversationID || message.seq !== expected) throw new Error('消息序号不连续，请重新同步')
    expected++
  }
  if (page.next_seq !== expected - 1 || (page.has_more && page.messages.length === 0)) throw new Error('同步游标无效')
  return [...messages, ...page.messages]
}

export class SyncTasks {
  constructor() { this.pending = new Map() }

  run(id, lastSeq, synchronize) {
    const existing = this.pending.get(id)
    if (existing) {
      existing.target = Math.max(existing.target, lastSeq)
      return existing.promise
    }
    const state = { target: lastSeq }
    state.promise = (async () => {
      let received
      do { received = await synchronize() }
      while (received !== undefined && received < state.target)
    })()
    this.pending.set(id, state)
    state.promise.finally(() => {
      if (this.pending.get(id) === state) this.pending.delete(id)
    }).catch(() => {})
    return state.promise
  }

  clear() { this.pending.clear() }
}

export class Requests {
  constructor(socket, timeout = 8000) {
    this.socket = socket
    this.timeout = timeout
    this.pending = new Map()
  }

  send(event, data) {
    return new Promise((resolve, reject) => {
      if (this.socket.readyState !== 1) return reject(new Error('实时连接未就绪'))
      const requestID = crypto.randomUUID()
      const timer = setTimeout(() => {
        this.pending.delete(requestID)
        reject(new Error('确认超时，可安全重试'))
      }, this.timeout)
      this.pending.set(requestID, { resolve, reject, timer })
      try { this.socket.send(JSON.stringify({ event, request_id: requestID, data })) }
      catch (error) { clearTimeout(timer); this.pending.delete(requestID); reject(error) }
    })
  }

  receive(envelope) {
    const pending = this.pending.get(envelope.request_id)
    if (!pending) return false
    clearTimeout(pending.timer)
    this.pending.delete(envelope.request_id)
    if (envelope.error) {
      const error = new Error(envelope.error.message)
      error.application = true
      pending.reject(error)
    } else pending.resolve(envelope.data)
    return true
  }

  close() {
    for (const pending of this.pending.values()) {
      clearTimeout(pending.timer)
      pending.reject(new Error('连接断开，可安全重试'))
    }
    this.pending.clear()
  }
}

export class PendingSends {
  constructor(storage, userID) {
    this.storage = storage
    this.key = `im.pending.${userID}`
  }

  entries() {
    try {
      const value = JSON.parse(this.storage.getItem(this.key) || '{}')
      return value && typeof value === 'object' && !Array.isArray(value) ? value : {}
    } catch { return {} }
  }

  get(conversationID) {
    const item = this.entries()[conversationID]
    return item?.conversation_id === conversationID && typeof item.content === 'string' && typeof item.client_msg_id === 'string' && item.client_msg_id.length >= 8 ? item : null
  }

  save(conversationID, content) {
    const existing = this.get(conversationID)
    if (existing && existing.content !== content) throw new Error('上一条发送结果未确认，请先恢复原文重试或明确放弃')
    if (existing) return existing
    const item = { conversation_id: conversationID, client_msg_id: crypto.randomUUID(), content }
    this.storage.setItem(this.key, JSON.stringify({ ...this.entries(), [conversationID]: item }))
    return item
  }

  clear(conversationID, clientMsgID) {
    const items = this.entries()
    if (items[conversationID]?.client_msg_id !== clientMsgID) return
    delete items[conversationID]
    this.storage.setItem(this.key, JSON.stringify(items))
  }

  forget() { this.storage.removeItem(this.key) }
}
