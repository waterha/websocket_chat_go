// A runtime template fragment; whitespace and bindings belong to the root view.
export default `    <Teleport to="body">
      <div v-if="me && messageActions.menu && messageActions.menuMessage" :ref="element => messageActions.menuElement = element" class="message-context-menu" role="menu" aria-label="消息操作" :style="{ left: messageActions.menu.x + 'px', top: messageActions.menu.y + 'px' }" @contextmenu.prevent>
        <div class="reaction-picker" role="group" aria-label="添加表情回应"><button v-for="emoji in REACTION_CHOICES" :key="emoji" type="button" :disabled="reactionBusy" :aria-label="'回应 ' + emoji" @click="toggleReaction(messageActions.menuMessage, emoji); messageActions.closeMenu()">{{ emoji }}</button></div>
        <button type="button" role="menuitem" @click="messageActions.runMenu('copy')">复制</button>
        <button type="button" role="menuitem" @click="messageActions.runMenu('reply')">回复</button>
        <button type="button" role="menuitem" @click="messageActions.runMenu('forward')">转发</button>
        <button v-if="messageActions.menuMessage?.sender_id !== me.id" type="button" role="menuitem" @click="pokeMember(messageActions.menuMessage.sender_id); messageActions.closeMenu()">拍一拍</button>
        <button type="button" role="menuitem" @click="messageActions.runMenu('select')">多选</button>
        <button v-if="messageActions.recallAllowed" type="button" role="menuitem" class="danger" :disabled="sending" @click="messageActions.runMenu('recall')">撤回</button>
      </div>
      <div v-if="conversationMenu" :ref="element => conversationMenuElement = element" class="message-context-menu conversation-context-menu" role="menu" aria-label="会话操作" :style="{ left: conversationMenu.x + 'px', top: conversationMenu.y + 'px' }" @contextmenu.prevent>
        <button type="button" role="menuitem" @click="runConversationMenu('pin')">{{ conversationMenu.pinned ? '取消置顶' : '置顶会话' }}</button>
        <button type="button" role="menuitem" @click="runConversationMenu('mute')">{{ conversationMenu.muted ? '关闭免打扰' : '消息免打扰' }}</button>
        <button type="button" role="menuitem" class="danger" @click="runConversationMenu('delete')">删除会话</button>
      </div>
      <div v-if="me && messageActions.forwardQueue.length" class="modal-backdrop forward-backdrop" @click.self="messageActions.closeForward">
        <form class="modal forward-modal" role="dialog" aria-modal="true" aria-label="转发消息" @submit.prevent="messageActions.confirmForward">
          <div class="modal-head"><h2>转发消息</h2><button type="button" class="icon-button" aria-label="关闭转发" :disabled="messageActions.forwardBusy" @click="messageActions.closeForward">×</button></div>
          <p class="subtle">将 {{ messageActions.forwardQueue.length }} 条消息逐条转发到所选会话，图片和附件一同转发。</p>
          <label>选择会话<select v-model="messageActions.forwardTarget" :disabled="messageActions.forwardBusy || messageActions.forwardLocked" required aria-label="转发到会话"><option disabled value="">请选择会话</option><option v-for="conversation in messageActions.forwardTargets" :key="conversation.id" :value="conversation.id">{{ title(conversation) }}</option></select></label>
          <p v-if="!messageActions.forwardTargets.length" class="subtle">暂无可转发的会话，请先打开好友聊天或创建群聊。</p>
          <p v-if="messageActions.forwardError" class="forward-error" role="alert">{{ messageActions.forwardError }}</p>
          <div class="profile-actions"><button type="button" class="outline-button" :disabled="messageActions.forwardBusy" @click="messageActions.closeForward">取消</button><button type="submit" class="primary" :disabled="!messageActions.forwardTarget || messageActions.forwardBusy">{{ messageActions.forwardBusy ? '转发中…' : messageActions.forwardError ? '重试剩余消息' : '确认转发' }}</button></div>
        </form>
      </div>
    </Teleport>
`
