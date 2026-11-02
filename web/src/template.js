import settings from './templates/settings.js'
import authentication from './templates/authentication.js'
import conversationSidebar from './templates/conversation-sidebar.js'
import chatHeader from './templates/chat-header.js'
import historySearch from './templates/history-search.js'
import messageStream from './templates/message-stream.js'
import messageSelection from './templates/message-selection.js'
import messageComposer from './templates/message-composer.js'
import conversationDetails from './templates/conversation-details.js'
import threadWindow from './templates/thread-window.js'
import announcementDialog from './templates/announcement-dialog.js'
import createGroupDialog from './templates/create-group-dialog.js'
import friendDialogs from './templates/friend-dialogs.js'
import memberDialog from './templates/member-dialog.js'
import globalSearchDialog from './templates/global-search-dialog.js'
import inviteDialog from './templates/invite-dialog.js'
import contactsDialog from './templates/contacts-dialog.js'
import profileDialog from './templates/profile-dialog.js'
import actionOverlays from './templates/action-overlays.js'
import errorToast from './templates/error-toast.js'

// Keep one runtime-compiled view: fragments share the root setup bindings.
// Join without separators so whitespace and v-if/v-else adjacency stay intact.
export const template = [
  settings,
  authentication,
  `    <div v-else class="layout">
`,
  conversationSidebar,
  `      <main class="chat">
        <template v-if="current">
`,
  chatHeader,
  historySearch,
  messageStream,
  messageSelection,
  messageComposer,
  `         <div v-else class="welcome"><span class="brand-mark">✦</span><h2>开启一段新对话</h2><p>从左侧联系人选择用户即可开始私聊，也可以创建群聊。</p><div class="welcome-actions"><button class="outline-button" @click="openCreate()">创建群聊</button></div></div>
      </main>
`,
  conversationDetails,
  threadWindow,
  announcementDialog,
  createGroupDialog,
  friendDialogs,
  memberDialog,
  globalSearchDialog,
  inviteDialog,
  contactsDialog,
  profileDialog,
  `    </div>
`,
  actionOverlays,
  errorToast
].join('')
