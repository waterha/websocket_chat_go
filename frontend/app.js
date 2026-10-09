const STORAGE_KEY = 'websocket_chat_go-demo-state';
const AUTH_STORAGE_KEY = 'websocket_chat_go-auth';

const people = {
  lin: { name: '林溪', avatar: '林', tone: 'clay', status: '正在读一本书', online: true },
  chen: { name: '陈默', avatar: '陈', tone: 'sage', status: '午后的咖啡很香', online: true },
  yue: { name: '小月', avatar: '月', tone: 'lavender', status: '周末见！', online: false },
  mu: { name: '木木', avatar: '木', tone: 'sand', status: '分享了一张照片', online: false },
  qi: { name: '齐远', avatar: '齐', tone: 'blue', status: '今天也要加油', online: true },
  me: { name: '余温', avatar: '余', tone: 'clay', status: '在线', online: true },
};

const conversations = [
  { id: 'sunset', title: '一起去看日落', type: 'group', avatar: '☀', tone: 'group', preview: '林溪：周六五点，老地方见。', time: '刚刚', unread: 2, pinned: true, notice: '把日常里的小美好，分享给在意的人。', subtitle: '4 位成员 · 3 位在线（演示）', members: ['lin', 'chen', 'yue', 'me'] },
  { id: 'lin', title: '林溪', type: 'direct', person: 'lin', preview: '等春风，也等你。', time: '10:42', unread: 0, pinned: false, subtitle: '在线 · 正在读一本书' },
  { id: 'weekend', title: '周末慢生活', type: 'group', avatar: '☕', tone: 'group', preview: '小月：有人想去新开的面包店吗？', time: '昨天', unread: 0, pinned: false, subtitle: '8 位成员 · 2 位在线（演示）', members: ['yue', 'mu', 'qi', 'me'] },
  { id: 'chen', title: '陈默', type: 'direct', person: 'chen', preview: '照片 · 城南的风还是这么温柔。', time: '周二', unread: 0, pinned: false, subtitle: '在线 · 午后的咖啡很香' },
  { id: 'reading', title: '读书会', type: 'group', avatar: '✦', tone: 'group', preview: '齐远：本周读《山茶文具店》。', time: '周一', unread: 0, pinned: false, subtitle: '12 位成员 · 4 位在线（演示）', members: ['qi', 'lin', 'chen', 'me'] },
  { id: 'yue', title: '小月', type: 'direct', person: 'yue', preview: '一切顺利，明天见。', time: '上周', unread: 0, pinned: false, subtitle: '离线 · 最后在线 2 小时前' },
];

const baseMessages = {
  sunset: [
    { id: 'm1', author: 'lin', time: '16:12', text: '周六去海边看日落吗？\n最近刚好有一点点想念海风。' },
    { id: 'm2', author: 'chen', time: '16:18', text: '好呀！我查了天气，周六是晴天 ☀️', image: 'assets/sunset.svg', imageCaption: '那就把这一刻，留给海。' },
    { id: 'm3', author: 'me', time: '16:22', text: '太好了，五点老地方见。\n我会带上相机和一些小零食。', outgoing: true },
    { id: 'm4', author: 'yue', time: '16:25', text: '记得带一件外套，海边晚上会有点凉。' },
    { id: 'm5', author: 'lin', time: '16:28', text: '收到！等春风，也等我们一起看见的日落。' },
  ],
  lin: [{ id: 'l1', author: 'lin', time: '10:36', text: '今天路过花店，看到一束很像你上次说喜欢的花。' }, { id: 'l2', author: 'me', time: '10:42', text: '等春风，也等你。🌷', outgoing: true }],
  weekend: [{ id: 'w1', author: 'yue', time: '昨天 18:03', text: '有人想去新开的面包店吗？听说可颂每天都不一样。' }, { id: 'w2', author: 'me', time: '昨天 18:10', text: '我报名！周日下午怎么样？', outgoing: true }],
  chen: [{ id: 'c1', author: 'chen', time: '周二 14:08', text: '照片 · 城南的风还是这么温柔。', image: 'assets/sunset.svg', imageCaption: '城南，下午四点。' }],
  reading: [{ id: 'r1', author: 'qi', time: '周一 20:20', text: '本周读《山茶文具店》，下周一分享各自最喜欢的一段。' }, { id: 'r2', author: 'me', time: '周一 20:36', text: '好的，我已经把书放在床头了。', outgoing: true }],
  yue: [{ id: 'y1', author: 'yue', time: '上周 09:16', text: '一切顺利，明天见。' }, { id: 'y2', author: 'me', time: '上周 09:20', text: '明天见，路上慢一点。', outgoing: true }],
};

const authState = loadAuthState();
let authMode = 'login';
const state = loadState();
const elements = {};
let toastTimer;
let pendingAttachment = null;
let attachmentReading = false;
let attachmentVersion = 0;
const drafts = {};
const initialConversations = structuredClone(conversations);

document.addEventListener('DOMContentLoaded', () => {
  cacheElements();
  bindEvents();
  restoreState();
  renderAuth();
  renderAll();
});

function cacheElements() {
  const ids = ['auth-screen', 'auth-form', 'auth-email', 'auth-password', 'auth-password-confirm', 'auth-name', 'auth-title', 'auth-subtitle', 'auth-error', 'auth-submit', 'auth-switch', 'auth-switch-text', 'password-toggle', 'workspace', 'pane-title', 'list-search', 'conversation-list', 'list-tabs', 'total-count', 'unread-count', 'chat-avatar', 'chat-title', 'chat-subtitle', 'message-stream', 'composer', 'message-input', 'send-button', 'message-search', 'message-query', 'message-match-count', 'emoji-picker', 'attachment-input', 'attachment-preview', 'conversation-menu', 'app-dialog', 'dialog-title', 'dialog-eyebrow', 'dialog-content', 'toast', 'nav-unread'];
  ids.forEach((id) => { elements[id.replace(/-([a-z])/g, (_, letter) => letter.toUpperCase())] = document.getElementById(id); });
  elements.dialogClose = document.getElementById('dialog-close');
  elements.authNameLabel = document.querySelector('.auth-name-label');
  elements.authConfirmLabel = document.querySelector('.auth-confirm-label');
}


function loadAuthState() {
  const defaults = { accounts: [], sessionEmail: '' };
  try {
    const saved = JSON.parse(localStorage.getItem(AUTH_STORAGE_KEY) || '{}');
    defaults.accounts = Array.isArray(saved.accounts) ? saved.accounts.filter((account) => account && typeof account.email === 'string' && typeof account.passwordHash === 'string').map((account) => ({ email: account.email.toLowerCase(), passwordHash: account.passwordHash, name: typeof account.name === 'string' ? account.name.slice(0, 30) : '' })) : [];
    defaults.sessionEmail = typeof saved.sessionEmail === 'string' ? saved.sessionEmail.toLowerCase() : '';
  } catch { return defaults; }
  return defaults;
}

function persistAuth() {
  try {
    localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(authState));
    return true;
  } catch {
    showToast('账号演示数据无法保存，请检查浏览器存储空间。');
    return false;
  }
}

function renderAuth() {
  const signedIn = Boolean(authState.sessionEmail && authState.accounts.some((account) => account.email === authState.sessionEmail));
  elements.authScreen.hidden = signedIn;
  elements.workspace.hidden = !signedIn;
  if (signedIn) {
    const account = authState.accounts.find((item) => item.email === authState.sessionEmail);
    if (account) applyAccountProfile(account);
  } else {
    elements.authEmail.focus();
  }
}

function setAuthMode(mode) {
  authMode = mode;
  const registering = mode === 'register';
  elements.authTitle.textContent = registering ? '创建账号' : '登录账号';
  elements.authSubtitle.textContent = registering ? '注册后即可进入你的聊天空间。' : '使用邮箱和密码继续聊天。';
  elements.authSubmit.textContent = registering ? '注册并进入' : '登录';
  elements.authSwitchText.firstChild.textContent = registering ? '已有账号？' : '还没有账号？';
  elements.authSwitch.textContent = registering ? '返回登录' : '立即注册';
  elements.authNameLabel.hidden = !registering;
  elements.authName.hidden = !registering;
  elements.authConfirmLabel.hidden = !registering;
  elements.authPasswordConfirm.hidden = !registering;
  elements.authName.required = registering;
  elements.authPasswordConfirm.required = registering;
  elements.authPassword.autocomplete = registering ? 'new-password' : 'current-password';
  clearAuthError();
  elements.authForm.reset();
  elements.authEmail.focus();
}

function togglePasswordVisibility() {
  const visible = elements.authPassword.type === 'text';
  elements.authPassword.type = visible ? 'password' : 'text';
  elements.passwordToggle.textContent = visible ? '显示' : '隐藏';
  elements.passwordToggle.setAttribute('aria-label', visible ? '显示密码' : '隐藏密码');
  elements.passwordToggle.setAttribute('aria-pressed', String(!visible));
}

function normalizeEmail(value) { return value.trim().toLowerCase(); }
function validEmail(value) { return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value); }

async function hashPassword(password) {
  if (globalThis.crypto?.subtle) {
    const buffer = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(password));
    return Array.from(new Uint8Array(buffer), (byte) => byte.toString(16).padStart(2, '0')).join('');
  }
  return btoa(unescape(encodeURIComponent(password)));
}

function showAuthError(message) {
  elements.authError.textContent = message;
  elements.authError.hidden = !message;
}
function clearAuthError() { showAuthError(''); }

async function handleAuthSubmit(event) {
  event.preventDefault();
  clearAuthError();
  const email = normalizeEmail(elements.authEmail.value);
  const password = elements.authPassword.value;
  if (!validEmail(email)) { showAuthError('请输入有效的邮箱地址。'); return; }
  if (password.length < 8) { showAuthError('密码至少需要 8 位。'); return; }
  const passwordHash = await hashPassword(password);
  if (authMode === 'register') {
    const name = elements.authName.value.trim();
    if (!name) { showAuthError('请输入昵称。'); return; }
    if (password !== elements.authPasswordConfirm.value) { showAuthError('两次输入的密码不一致。'); return; }
    if (authState.accounts.some((account) => account.email === email)) { showAuthError('该邮箱已注册，请直接登录。'); return; }
    authState.accounts.push({ email, passwordHash, name: name.slice(0, 30) });
    authState.sessionEmail = email;
    persistAuth();
    applyAccountProfile(authState.accounts.at(-1));
    renderAuth();
    renderAll();
    showToast('注册成功，欢迎进入 websocket_chat_go');
    return;
  }
  const account = authState.accounts.find((item) => item.email === email);
  if (!account || account.passwordHash !== passwordHash) { showAuthError('邮箱或密码错误，请重试。'); return; }
  authState.sessionEmail = email;
  persistAuth();
  applyAccountProfile(account);
  renderAuth();
  renderAll();
  showToast('登录成功');
}

function applyAccountProfile(account) {
  const name = account.name || account.email.split('@')[0] || '用户';
  state.profile.name = name;
  people.me.name = name;
  people.me.avatar = Array.from(name)[0] || '用';
  people.me.status = '在线';
}

function logout() {
  authState.sessionEmail = '';
  persistAuth();
  closeDialog();
  renderAuth();
  showToast('已退出当前账号');
}

function loadState() {
  const defaults = { selectedId: 'sunset', messages: {}, conversations: [], profile: { name: '余温', status: '把生活过成喜欢的样子。' }, preferences: { accent: 'clay', enterToSend: true }, filter: 'all', view: 'chats' };
  try {
    const saved = JSON.parse(localStorage.getItem(STORAGE_KEY) || localStorage.getItem('warmchat-demo-state'));
    if (!saved || typeof saved !== 'object') return defaults;
    defaults.selectedId = typeof saved.selectedId === 'string' ? saved.selectedId : 'sunset';
    defaults.conversations = Array.isArray(saved.conversations) ? saved.conversations : [];
    if (saved.messages && typeof saved.messages === 'object' && !Array.isArray(saved.messages)) {
      Object.entries(saved.messages).forEach(([id, messages]) => {
        if (/^[a-z0-9-]+$/.test(id) && Array.isArray(messages)) defaults.messages[id] = messages.filter((message) => message && typeof message.id === 'string' && message.author === 'me' && typeof message.time === 'string' && typeof message.text === 'string' && message.text.length <= 4000 && (!message.image || /^data:image\/(png|jpeg|webp|gif);base64,/.test(message.image)) && (!message.file || (typeof message.file.name === 'string' && typeof message.file.size === 'string')));
      });
    }
    if (typeof saved.profile?.name === 'string' && saved.profile.name.trim()) defaults.profile.name = saved.profile.name.slice(0, 30);
    if (typeof saved.profile?.status === 'string') defaults.profile.status = saved.profile.status.slice(0, 60);
    if (['clay', 'apricot', 'olive'].includes(saved.preferences?.accent)) defaults.preferences.accent = saved.preferences.accent;
    defaults.preferences.enterToSend = saved.preferences?.enterToSend !== false;
    return defaults;
  } catch { return defaults; }
}

function persist() {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ selectedId: state.selectedId, messages: state.messages, conversations, profile: state.profile, preferences: state.preferences }));
    return true;
  } catch {
    showToast('浏览器存储不可用或空间已满；内容仍在页面中，请导出备份或移除大图片。');
    return false;
  }
}

function restoreState() {
  state.conversations.forEach((saved) => {
    if (!saved || typeof saved.id !== 'string' || !/^[a-z0-9-]+$/.test(saved.id)) return;
    let conversation = conversations.find((item) => item.id === saved.id);
    if (!conversation && saved.type === 'direct' && Object.hasOwn(people, saved.person) && saved.person !== 'me') {
      conversation = { id: saved.id, title: people[saved.person].name, type: 'direct', person: saved.person, preview: '开始一段新的对话吧。', time: '现在', unread: 0, pinned: false };
      conversations.push(conversation);
    }
    if (!conversation && saved.type === 'group' && typeof saved.title === 'string' && Array.isArray(saved.members)) {
      const members = [...new Set(saved.members.filter((key) => Object.hasOwn(people, key)))];
      if (!members.includes('me')) members.push('me');
      conversation = { id: saved.id, title: saved.title.slice(0, 30), type: 'group', avatar: '☀', tone: 'group', members, preview: '群组已创建，打个招呼吧。', time: '现在', unread: 0, pinned: false };
      conversations.push(conversation);
    }
    if (conversation) {
      conversation.pinned = Boolean(saved.pinned);
      conversation.muted = Boolean(saved.muted);
      conversation.cleared = Boolean(saved.cleared);
      if (Number.isInteger(saved.unread) && saved.unread >= 0) conversation.unread = saved.unread;
    }
  });
  if (!conversations.some((conversation) => conversation.id === state.selectedId)) state.selectedId = conversations[0].id;
  people.me.name = state.profile.name;
  people.me.avatar = Array.from(state.profile.name)[0];
  applyAccent(state.preferences.accent, false);
}

function bindEvents() {
  elements.authForm.addEventListener('submit', handleAuthSubmit);
  elements.authSwitch.addEventListener('click', () => setAuthMode(authMode === 'login' ? 'register' : 'login'));
  elements.passwordToggle.addEventListener('click', togglePasswordVisibility);
  document.querySelectorAll('.rail-button[data-view]').forEach((button) => button.addEventListener('click', () => switchView(button.dataset.view)));
  document.getElementById('new-button').addEventListener('click', () => state.view === 'groups' ? openCreateGroup() : openNewConversation());
  document.getElementById('settings-button').addEventListener('click', openSettings);
  document.getElementById('profile-button').addEventListener('click', openProfile);
  document.getElementById('back-button').addEventListener('click', () => elements.workspace.classList.remove('mobile-chat'));
  document.getElementById('more-button').addEventListener('click', toggleMenu);
  document.getElementById('message-search-button').addEventListener('click', openMessageSearch);
  document.getElementById('close-message-search').addEventListener('click', closeMessageSearch);
  document.getElementById('emoji-button').addEventListener('click', toggleEmojiPicker);
  document.getElementById('image-button').addEventListener('click', () => chooseAttachment('image/*'));
  document.getElementById('file-button').addEventListener('click', () => chooseAttachment('*/*'));
  elements.attachmentInput.addEventListener('change', handleAttachment);
  elements.composer.addEventListener('submit', (event) => { event.preventDefault(); sendMessage(); });
  elements.messageInput.addEventListener('input', updateComposer);
  elements.messageInput.addEventListener('keydown', (event) => { if (event.key === 'Enter' && !event.isComposing && event.keyCode !== 229 && !event.shiftKey && (state.preferences.enterToSend || event.ctrlKey || event.metaKey)) { event.preventDefault(); sendMessage(); } });
  elements.listSearch.addEventListener('input', renderConversationList);
  elements.listTabs.addEventListener('click', (event) => { const button = event.target.closest('button[data-filter]'); if (button) setFilter(button.dataset.filter); });
  elements.messageQuery.addEventListener('input', highlightMessages);
  elements.dialogClose.addEventListener('click', closeDialog);
  elements.appDialog.addEventListener('click', (event) => { if (event.target === elements.appDialog) closeDialog(); });
  document.addEventListener('click', (event) => { if (!event.target.closest('#conversation-menu, #more-button')) closeMenu(); });
  document.addEventListener('keydown', (event) => { if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') { event.preventDefault(); elements.listSearch.focus(); } if (event.key === 'Escape') { closeMenu(); closeMessageSearch(); } });
}

function renderAll() {
  document.getElementById('profile-button').innerHTML = `${escapeHtml(people.me.avatar)}<span class="presence"></span>`;
  document.getElementById('send-hint').textContent = state.preferences.enterToSend ? 'Enter 发送 · Shift + Enter 换行' : 'Ctrl / ⌘ + Enter 发送';
  renderNavigation();
  renderConversationList();
  renderChat();
  updateComposer();
}

function currentConversation() { return conversations.find((conversation) => conversation.id === state.selectedId) || conversations[0]; }

function conversationMessages(conversationId) { const conversation = conversations.find((item) => item.id === conversationId); return [...(conversation?.cleared ? [] : baseMessages[conversationId] || []), ...(state.messages[conversationId] || [])]; }

function avatarMarkup(keyOrConversation, size = '') {
  const source = typeof keyOrConversation === 'string' ? people[keyOrConversation] : keyOrConversation;
  const isPerson = Boolean(source && source.name);
  const tone = source?.tone || 'group';
  const label = source?.avatar || (isPerson ? source.name.slice(0, 1) : '☀');
  const presence = isPerson && source.online ? '<span class="presence"></span>' : '';
  return `<span class="avatar ${tone} ${size}">${escapeHtml(label)}${presence}</span>`;
}

function renderNavigation() {
  document.querySelectorAll('.rail-button[data-view]').forEach((button) => { const active = button.dataset.view === state.view; button.classList.toggle('active', active); button.setAttribute('aria-pressed', String(active)); });
  elements.navUnread.hidden = unreadTotal() === 0;
}

function switchView(view) {
  state.view = view;
  const labels = { chats: '消息', contacts: '联系人', groups: '群组' };
  elements.paneTitle.textContent = labels[view];
  renderNavigation(); renderConversationList();
  elements.workspace.classList.remove('mobile-chat');

}

function setFilter(filter) { state.filter = filter; renderConversationList(); }

function renderConversationList() {
  const query = elements.listSearch.value.trim().toLowerCase();
  document.querySelectorAll('#list-tabs button').forEach((button) => { const active = button.dataset.filter === state.filter; button.classList.toggle('active', active); button.setAttribute('aria-pressed', String(active)); });
  const list = conversations.filter((conversation) => {
    if (state.view === 'contacts' && conversation.type !== 'direct') return false;
    if (state.view === 'groups' && conversation.type !== 'group') return false;
    if (state.view === 'chats' && state.filter === 'unread' && conversation.unread === 0) return false;
    if (state.view === 'chats' && state.filter === 'group' && conversation.type !== 'group') return false;
    if (!query) return true;
    return `${conversation.title} ${conversationPreview(conversation)}`.toLowerCase().includes(query);
  }).sort((first, second) => Number(second.pinned) - Number(first.pinned));
  elements.totalCount.textContent = conversations.length;
  elements.unreadCount.textContent = unreadTotal();
  elements.listTabs.hidden = state.view !== 'chats';
  elements.conversationList.innerHTML = list.length ? list.map(conversationCard).join('') : '<div class="empty-state"><svg class="icon"><use href="#i-search"/></svg>没有找到匹配的内容<br>换个关键词试试吧。</div>';
  elements.conversationList.querySelectorAll('[data-conversation-id]').forEach((card) => card.addEventListener('click', () => selectConversation(card.dataset.conversationId)));
}

function conversationCard(conversation) {
  const source = conversation.type === 'direct' ? conversation.person : conversation;
  const avatar = avatarMarkup(source);
  const localMessages = state.messages[conversation.id] || [];
  return `<button class="conversation-card ${conversation.id === state.selectedId ? 'selected' : ''}" aria-pressed="${conversation.id === state.selectedId}" data-conversation-id="${escapeHtml(conversation.id)}"><span>${avatar}</span><span class="conversation-text"><span class="conversation-title-line"><strong>${escapeHtml(conversation.title)}</strong><time>${escapeHtml(localMessages.at(-1)?.time || conversation.time)}</time></span><span class="conversation-preview-line"><p>${escapeHtml(conversationPreview(conversation))}</p>${conversation.unread ? `<b class="unread-badge">${conversation.unread}</b>` : conversation.pinned ? '<svg class="icon card-pin"><use href="#i-pin"/></svg>' : ''}</span></span></button>`;
}

function selectConversation(id) {
  drafts[state.selectedId] = elements.messageInput.value;
  clearAttachment();
  closeMessageSearch(); closeMenu(); elements.emojiPicker.hidden = true;
  state.selectedId = id; const conversation = currentConversation(); conversation.unread = 0; state.filter = 'all'; elements.listSearch.value = '';
  elements.messageInput.value = drafts[id] || '';
  persist(); renderConversationList(); renderChat(); renderNavigation(); elements.workspace.classList.add('mobile-chat'); updateComposer();
}

function conversationPreview(conversation) { const message = (state.messages[conversation.id] || []).at(-1); return message ? `我：${message.text || (message.image ? '[图片]' : `[文件] ${message.file?.name || ''}`)}` : conversation.cleared ? '本地记录已清空' : conversation.preview; }

function renderChat() {
  const conversation = currentConversation();
  elements.chatAvatar.innerHTML = avatarMarkup(conversation.type === 'direct' ? conversation.person : conversation);
  const onlineMembers = (conversation.members || []).filter((key) => people[key].online).length;
  const person = people[conversation.person];
  elements.chatTitle.textContent = conversation.title;
  elements.chatSubtitle.textContent = conversation.type === 'group' ? `${conversation.members.length} 位成员 · ${onlineMembers} 位在线（演示）` : `${person.online ? '在线' : '离线'} · ${person.status}（演示）`;
  const messages = conversationMessages(conversation.id);
  elements.messageStream.innerHTML = `${messages.length ? messages.map(messageMarkup).join('') : '<div class="empty-state">对话从一句问候开始。<br>说声你好吧。</div>'}`;
  elements.messageStream.scrollTop = elements.messageStream.scrollHeight;
  if (!elements.messageSearch.hidden) highlightMessages();
}

function messageMarkup(message) {
  const author = people[message.author] || people.me;
  const hasAttachment = message.image || message.file;
  let body = message.image ? `<div class="message-photo"><img src="${escapeHtml(message.image)}" alt="${escapeHtml(message.imageCaption || message.file?.name || '分享的图片')}" loading="lazy"><div class="photo-caption"><span>${escapeHtml(message.imageCaption || message.file?.name || '一张图片')}</span><svg class="icon"><use href="#i-image"/></svg></div></div>` : message.file ? `<div class="message-file"><svg class="icon"><use href="#i-file"/></svg><span><strong>${escapeHtml(message.file.name)}</strong><small>${escapeHtml(message.file.size)} · 仅文件信息</small></span></div>` : '';
  if (message.text) body = `<div class="message-bubble ${hasAttachment ? 'with-attachment' : ''}">${escapeHtml(message.text)}</div>${body}`;
  return `<article class="message-row ${message.outgoing ? 'outgoing' : ''}" data-message-id="${escapeHtml(message.id)}" data-message-text="${escapeHtml([message.text, message.imageCaption, message.file?.name].filter(Boolean).join(' '))}">${avatarMarkup(message.author)}<div class="message-content"><div class="message-author"><strong>${escapeHtml(author.name)}</strong><time>${escapeHtml(message.time)}</time></div>${body}${message.outgoing ? '<div class="message-meta">仅本地 <svg class="icon"><use href="#i-check"/></svg></div>' : ''}</div></article>`;
}

function updateComposer() { const value = elements.messageInput.value; elements.sendButton.disabled = attachmentReading || (!value.trim() && !pendingAttachment); elements.messageInput.style.height = 'auto'; elements.messageInput.style.height = `${Math.max(48, Math.min(elements.messageInput.scrollHeight, 130))}px`; }

function sendMessage() {
  const text = elements.messageInput.value.trim(); const file = pendingAttachment; if (attachmentReading || (!text && !file) || text.length > 4000) return;
  const message = { id: `local-${crypto.randomUUID()}`, author: 'me', time: new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }), text, outgoing: true };
  if (file) { message.file = { name: file.name, size: formatFileSize(file.size) }; if (file.type.startsWith('image/') && file.dataUrl) message.image = file.dataUrl; }
  state.messages[state.selectedId] = [...(state.messages[state.selectedId] || []), message]; const saved = persist(); elements.messageInput.value = ''; delete drafts[state.selectedId]; renderChat(); renderConversationList(); if (saved) showToast('消息已保存在当前浏览器');
  clearAttachment();
}

function chooseAttachment(accept) { elements.attachmentInput.accept = accept === 'image/*' ? 'image/png,image/jpeg,image/webp,image/gif' : ''; elements.attachmentInput.click(); }

function handleAttachment() {
  const file = elements.attachmentInput.files[0];
  if (!file) return;
  if (file.size > 5 * 1024 * 1024 || (file.type.startsWith('image/') && file.size > 2 * 1024 * 1024)) { clearAttachment(); showToast('演示模式：图片最多 2 MB，其他附件最多 5 MB'); return; }
  if (file.type.startsWith('image/') && !['image/png', 'image/jpeg', 'image/webp', 'image/gif'].includes(file.type)) { clearAttachment(); showToast('图片仅支持 PNG、JPEG、WebP 和 GIF'); return; }
  const version = ++attachmentVersion;
  const reader = new FileReader();
  attachmentReading = true;
  updateComposer();
  reader.onload = () => {
    if (version !== attachmentVersion) return;
    pendingAttachment = { name: file.name, size: file.size, type: file.type, dataUrl: file.type.startsWith('image/') && typeof reader.result === 'string' ? reader.result : null };
    attachmentReading = false;
    elements.attachmentPreview.innerHTML = `<svg class="icon"><use href="#i-file"/></svg><span>${escapeHtml(file.name)} · ${formatFileSize(file.size)}</span><button class="icon-button" type="button" aria-label="移除附件"><svg class="icon"><use href="#i-close"/></svg></button>`;
    elements.attachmentPreview.hidden = false;
    elements.attachmentPreview.querySelector('button').addEventListener('click', clearAttachment);
    updateComposer();
  };
  reader.onerror = () => { if (version !== attachmentVersion) return; attachmentReading = false; clearAttachment(); showToast('无法读取这个附件'); };
  reader.readAsDataURL(file);
}

function clearAttachment() { attachmentVersion += 1; pendingAttachment = null; attachmentReading = false; elements.attachmentInput.value = ''; elements.attachmentPreview.hidden = true; elements.attachmentPreview.innerHTML = ''; updateComposer(); }

function confirmClearConversation() {
  elements.dialogEyebrow.textContent = 'CLEAR LOCAL DEMO';
  elements.dialogTitle.textContent = '清空当前会话？';
  elements.dialogContent.innerHTML = `<p class="dialog-copy">这只会移除当前浏览器中的演示消息，不会影响其他会话，也不会连接服务器。</p><div class="dialog-actions"><button data-cancel-clear>先不清空</button><button class="confirm-danger" data-confirm-clear>清空记录</button></div>`;
  elements.dialogContent.querySelector('[data-cancel-clear]').addEventListener('click', closeDialog);
  elements.dialogContent.querySelector('[data-confirm-clear]').addEventListener('click', () => { currentConversation().cleared = true; delete state.messages[state.selectedId]; persist(); closeDialog(); renderConversationList(); renderChat(); showToast('当前会话已清空'); });
  elements.appDialog.showModal();
}

function toggleEmojiPicker() { if (!elements.emojiPicker.children.length) ['🙂', '😊', '☀️', '🌿', '🌷', '✨', '🤍', '👏', '🎉', '☕', '📷', '🌙'].forEach((emoji) => { const button = document.createElement('button'); button.type = 'button'; button.textContent = emoji; button.addEventListener('click', () => { const input = elements.messageInput; if (input.value.length - (input.selectionEnd - input.selectionStart) + emoji.length > 4000) return; input.setRangeText(emoji, input.selectionStart, input.selectionEnd, 'end'); input.focus(); updateComposer(); }); elements.emojiPicker.appendChild(button); }); elements.emojiPicker.hidden = !elements.emojiPicker.hidden; document.getElementById('emoji-button').setAttribute('aria-expanded', String(!elements.emojiPicker.hidden)); }

function openMessageSearch() { elements.messageSearch.hidden = false; elements.messageQuery.focus(); }
function closeMessageSearch() { elements.messageSearch.hidden = true; elements.messageQuery.value = ''; highlightMessages(); }
function highlightMessages() { const query = elements.messageQuery.value.trim().toLowerCase(); let hits = 0; elements.messageStream.querySelectorAll('.message-row').forEach((row) => { const match = query && row.dataset.messageText.toLowerCase().includes(query); row.classList.toggle('message-search-hit', Boolean(match)); if (match) hits += 1; }); elements.messageMatchCount.textContent = query ? `${hits} 条结果` : ''; }

function toggleMenu() { elements.conversationMenu.innerHTML = `<button data-menu="pin"><svg class="icon"><use href="#i-pin"/></svg>${currentConversation().pinned ? '取消置顶' : '置顶会话'}</button><button data-menu="mute"><svg class="icon"><use href="#i-bell"/></svg>${currentConversation().muted ? '开启提醒偏好' : '静音提醒偏好'}</button><button class="danger" data-menu="delete"><svg class="icon"><use href="#i-close"/></svg>清空本地演示</button>`; elements.conversationMenu.hidden = !elements.conversationMenu.hidden; document.getElementById('more-button').setAttribute('aria-expanded', String(!elements.conversationMenu.hidden)); elements.conversationMenu.querySelectorAll('[data-menu]').forEach((button) => button.addEventListener('click', () => handleMenu(button.dataset.menu))); }
function closeMenu() { elements.conversationMenu.hidden = true; document.getElementById('more-button').setAttribute('aria-expanded', 'false'); }
function handleMenu(action) { closeMenu(); if (action === 'pin') { currentConversation().pinned = !currentConversation().pinned; persist(); renderConversationList(); showToast(currentConversation().pinned ? '已置顶会话' : '已取消置顶'); } if (action === 'mute') { currentConversation().muted = !currentConversation().muted; persist(); showToast('本地提醒偏好已保存'); } if (action === 'delete') confirmClearConversation(); }

function openNewConversation() { elements.dialogEyebrow.textContent = 'MAKE A CONNECTION'; elements.dialogTitle.textContent = '开启新对话'; elements.dialogContent.innerHTML = `<p class="dialog-copy">选择一位朋友，开始一段温柔的对话。</p><div class="dialog-list">${Object.keys(people).filter((key) => key !== 'me').map((key) => `<button class="dialog-contact" data-person="${key}">${avatarMarkup(key, 'small')}<span><strong>${escapeHtml(people[key].name)}</strong><small>${escapeHtml(people[key].status)}</small></span><svg class="icon"><use href="#i-chevron"/></svg></button>`).join('')}</div><button class="dialog-secondary" data-group-create>＋ 创建一个新群组</button>`; elements.dialogContent.querySelectorAll('[data-person]').forEach((button) => button.addEventListener('click', () => { const id = button.dataset.person; if (!conversations.some((conversation) => conversation.id === id)) conversations.push({ id, title: people[id].name, type: 'direct', person: id, preview: '开始一段新的对话吧。', time: '现在', unread: 0, pinned: false }); closeDialog(); switchView('chats'); selectConversation(id); })); elements.dialogContent.querySelector('[data-group-create]').addEventListener('click', () => { closeDialog(); openCreateGroup(); }); elements.appDialog.showModal(); }

function openCreateGroup() {
  elements.dialogEyebrow.textContent = 'BETTER TOGETHER';
  elements.dialogTitle.textContent = '创建一个小小群组';
  elements.dialogContent.innerHTML = `<p class="dialog-copy">邀请同频的人，共享一点日常。群组仅保存在本地演示中。</p><form id="create-group-form"><label class="form-label" for="group-name">群组名称</label><input id="group-name" class="form-input" maxlength="30" placeholder="例如：周末散步小分队" required><p class="form-label">邀请朋友（至少选择一位）</p><div class="dialog-list">${Object.keys(people).filter((key) => key !== 'me').map((key) => `<label class="dialog-contact">${avatarMarkup(key, 'small')}<span><strong>${escapeHtml(people[key].name)}</strong><small>${escapeHtml(people[key].status)}</small></span><input type="checkbox" name="member" value="${key}" aria-label="邀请${escapeHtml(people[key].name)}"></label>`).join('')}</div><p id="group-error" class="settings-note danger" role="status"></p><button class="dialog-submit" type="submit">创建本地群组</button></form>`;
  elements.dialogContent.querySelector('form').addEventListener('submit', (event) => {
    event.preventDefault();
    const title = document.getElementById('group-name').value.trim();
    const members = [...elements.dialogContent.querySelectorAll('input[name="member"]:checked')].map((input) => input.value);
    if (!title || !members.length) { document.getElementById('group-error').textContent = '请填写群名，并至少选择一位朋友。'; return; }
    const id = `group-${crypto.randomUUID()}`;
    conversations.push({ id, title, type: 'group', avatar: '☀', tone: 'group', members: [...members, 'me'], preview: '群组已创建，打个招呼吧。', time: '现在', unread: 0, pinned: false });
    closeDialog(); switchView('chats'); selectConversation(id); showToast('群组已创建（仅本地演示）');
  });
  elements.appDialog.showModal();
}

function openProfile() { elements.dialogEyebrow.textContent = 'YOUR WARM PROFILE'; elements.dialogTitle.textContent = '个人资料'; elements.dialogContent.innerHTML = `<div style="text-align:center">${avatarMarkup('me', 'large')}</div><label class="form-label" for="profile-name">昵称</label><input class="form-input" id="profile-name" value="${escapeHtml(state.profile.name)}" maxlength="30"><label class="form-label" for="profile-status">签名</label><input class="form-input" id="profile-status" value="${escapeHtml(state.profile.status)}" maxlength="60"><button class="dialog-submit" data-save-profile>保存资料</button><button class="dialog-secondary" data-logout>退出当前账号</button><p class="settings-note">这些资料只会保存在当前浏览器，后端接入后再同步到 PostgreSQL。</p>`; elements.dialogContent.querySelector('[data-save-profile]').addEventListener('click', () => { const name = elements.dialogContent.querySelector('#profile-name').value.trim() || '余温'; state.profile.name = name.slice(0, 30); state.profile.status = elements.dialogContent.querySelector('#profile-status').value.trim().slice(0, 60); people.me.name = state.profile.name; people.me.avatar = Array.from(state.profile.name)[0]; persist(); closeDialog(); renderAll(); showToast('个人资料已更新'); }); elements.dialogContent.querySelector('[data-logout]').addEventListener('click', logout); elements.appDialog.showModal(); }

function openSettings() {
  elements.dialogEyebrow.textContent = 'MAKE IT YOURS';
  elements.dialogTitle.textContent = '偏好设置';
  elements.dialogContent.innerHTML = `<label class="toggle-row">Enter 直接发送 <span class="toggle"><input type="checkbox" data-preference="enter" ${state.preferences.enterToSend ? 'checked' : ''}><span></span></span></label><div class="setting-divider"><label class="form-label" for="accent-select">界面强调色</label><select class="form-input" id="accent-select"><option value="clay">陶土红</option><option value="apricot">杏子橙</option><option value="olive">橄榄绿</option></select><p class="settings-note">偏好保存在当前浏览器中。当前没有真实通知、账号登录或服务器连接；后端由你接入 Go 与 PostgreSQL。</p></div><div class="dialog-actions"><button data-export>导出本地演示数据</button><button class="confirm-danger" data-reset>重置演示数据</button></div>`;
  const select = elements.dialogContent.querySelector('#accent-select');
  select.value = state.preferences.accent;
  select.addEventListener('change', (event) => applyAccent(event.target.value));
  elements.dialogContent.querySelector('[data-export]').addEventListener('click', exportState);
  elements.dialogContent.querySelector('[data-reset]').addEventListener('click', () => {
    elements.dialogTitle.textContent = '重置所有本地数据？';
    elements.dialogContent.innerHTML = '<p class="dialog-copy">将移除本地发送的消息、新建会话、群组、昵称及偏好。此操作无法撤销，建议先导出。</p><div class="dialog-actions"><button data-cancel-reset>取消</button><button class="confirm-danger" data-confirm-reset>确认重置</button></div>';
    elements.dialogContent.querySelector('[data-cancel-reset]').addEventListener('click', closeDialog);
    elements.dialogContent.querySelector('[data-confirm-reset]').addEventListener('click', resetDemo);
  });
  elements.dialogContent.querySelector('[data-preference="enter"]').addEventListener('change', (event) => { state.preferences.enterToSend = event.target.checked; persist(); document.getElementById('send-hint').textContent = state.preferences.enterToSend ? 'Enter 发送 · Shift + Enter 换行' : 'Ctrl / ⌘ + Enter 发送'; });
  elements.appDialog.showModal();
}
function exportState() { const blob = new Blob([JSON.stringify({ messages: state.messages, conversations, profile: state.profile, preferences: state.preferences }, null, 2)], { type: 'application/json' }); const link = document.createElement('a'); link.href = URL.createObjectURL(blob); link.download = 'websocket_chat_go-demo.json'; link.click(); URL.revokeObjectURL(link.href); showToast('本地演示数据已导出'); }
function resetDemo() { state.messages = {}; state.selectedId = 'sunset'; state.profile = { name: '余温', status: '把生活过成喜欢的样子。' }; state.preferences = { accent: 'clay', enterToSend: true }; conversations.splice(0, conversations.length, ...structuredClone(initialConversations)); Object.keys(drafts).forEach((id) => delete drafts[id]); elements.messageInput.value = ''; clearAttachment(); const account = authState.accounts.find((item) => item.email === authState.sessionEmail); if (account) applyAccountProfile(account); applyAccent('clay', false); state.view = 'chats'; state.filter = 'all'; elements.listSearch.value = ''; elements.paneTitle.textContent = '消息'; persist(); closeDialog(); renderAll(); showToast('演示数据已恢复'); }
function applyAccent(value, save = true) { const accents = { clay: ['#b96246', '#9d4e36', '#f4e4d8'], apricot: ['#c67b48', '#a66036', '#f7e6d1'], olive: ['#858b5d', '#697043', '#e7ead5'] }; const [accent, dark, soft] = accents[value] || accents.clay; document.documentElement.style.setProperty('--accent', accent); document.documentElement.style.setProperty('--accent-dark', dark); document.documentElement.style.setProperty('--accent-soft', soft); if (save && state.preferences) { state.preferences.accent = value; persist(); } }
function closeDialog() { elements.appDialog.close(); }
function unreadTotal() { return conversations.reduce((sum, conversation) => sum + conversation.unread, 0); }
function formatFileSize(bytes) { if (bytes < 1024) return `${bytes} B`; if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`; return `${(bytes / 1024 / 1024).toFixed(1)} MB`; }
function escapeHtml(value) { return String(value).replace(/[&<>'"]/g, (character) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[character])); }
function showToast(message) { elements.toast.textContent = message; elements.toast.hidden = false; clearTimeout(toastTimer); toastTimer = setTimeout(() => { elements.toast.hidden = true; }, 2600); }
