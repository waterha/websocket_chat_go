// A runtime template fragment; whitespace and bindings belong to the root view.
export default `          <div v-if="messageActions.selectionMode" class="message-selection-bar" role="toolbar" aria-label="批量消息操作">
            <span>已选 {{ messageActions.selected.length }} 条</span>
            <button type="button" class="outline-button" :disabled="!messageActions.selected.length" @click="messageActions.copyMessages(messageActions.selected)">复制</button>
            <button type="button" class="primary" :disabled="!messageActions.selected.length" @click="messageActions.openForward(messageActions.selected)">转发</button>
            <button type="button" class="text-button" @click="messageActions.cancelSelection">取消多选</button>
          </div>
`
