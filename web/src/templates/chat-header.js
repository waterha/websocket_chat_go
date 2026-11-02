// A runtime template fragment; whitespace and bindings belong to the root view.
export default `           <header class="chat-head" aria-label="会话信息">
             <UserAvatar v-if="current.kind === 'direct'" :user="headMember" :name="title(current)" />
             <span v-else class="avatar">{{ title(current).slice(0, 1) }}</span>
             <div class="chat-head-text"><strong>{{ title(current) }}<span v-if="current.dismissed" class="mute-mark" aria-label="已解散">已解散</span><span v-if="current.pinned" class="pin-mark" aria-label="已置顶">置顶</span><span v-if="current.muted" class="mute-mark" aria-label="免打扰">免打扰</span></strong><p>{{ chatHeadMeta }}</p></div>
             <div class="chat-head-actions">
               <GroupFiles v-if="current.kind === 'group'" :key="current.id + ':' + me.id" :conversation="current" />
               <div class="chat-more">
                 <button type="button" class="head-icon" title="更多" aria-label="更多操作" :aria-expanded="chatMoreOpen" @click.stop="toggleChatMore"><MoreHorizontal :size="18" /></button>
                 <div v-if="chatMoreOpen" class="top-menu" role="menu" @click.stop>
                   <button type="button" role="menuitem" :disabled="preferenceBusy" @click="togglePreference(current.id, 'pinned'); chatMoreOpen = false"><Pin :size="15" />{{ current.pinned ? '取消置顶' : '置顶会话' }}</button>
                   <button type="button" role="menuitem" :disabled="preferenceBusy" @click="togglePreference(current.id, 'muted'); chatMoreOpen = false"><BellOff :size="15" />{{ current.muted ? '关闭免打扰' : '消息免打扰' }}</button>
                   <button type="button" role="menuitem" @click="openSearch(); chatMoreOpen = false"><Search :size="15" />搜索聊天记录</button>
                   <button v-if="current.kind === 'group'" type="button" role="menuitem" @click="openMembers(); chatMoreOpen = false"><Users :size="15" />成员管理<span v-if="current.pending_joins" class="request-count">{{ current.pending_joins > 99 ? '99+' : current.pending_joins }}</span></button>
                   <button v-if="current.kind === 'direct'" type="button" role="menuitem" @click="chatInfoOpen = !chatInfoOpen; chatMoreOpen = false"><Info :size="15" />会话信息</button>
                   <button type="button" role="menuitem" class="danger" @click="deleteConversation(current.id); chatMoreOpen = false">删除会话</button>
                 </div>
               </div>
             </div>
           </header>
`
