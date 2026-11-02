// A runtime template fragment; whitespace and bindings belong to the root view.
export default `      <div v-if="showCreate" class="modal-backdrop" @click.self="showCreate = false">
        <div class="modal">
           <div class="modal-head"><h2>创建群聊</h2><button @click="showCreate = false">×</button></div>
           <p class="subtle">创建后群内只有你自己，之后可在群内按需邀请其他用户。含自己最多 1000 人。</p>
           <input v-model="groupName" maxlength="100" placeholder="群聊名称" />
           <button class="primary" :disabled="conversationBusy || !groupName.trim()" @click="createGroup">{{ conversationBusy ? '处理中…' : '创建群聊' }}</button>
         </div>
       </div>
`
