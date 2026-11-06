import { Paperclip, Download } from 'lucide-vue-next'
import { hasBlocks } from './composer-blocks.js'
import { formatBytes } from './media.js'
import { replyExcerpt } from './reply.js'

export const favoriteMediaURL = (favoriteID, mediaID) => `/api/v1/favorites/${encodeURIComponent(favoriteID)}/media/${encodeURIComponent(mediaID)}`

// Mirror chat bubble/block/media markup. Only Vue text interpolation renders bodies.
export const SavedMessageContent = {
  components: { Paperclip, Download },
  props: { favorite: { type: Object, required: true } },
  computed: {
    item() { return this.favorite.message || {} },
    usesBlocks() { return hasBlocks(this.item) },
    segments() { return (this.item.blocks || []).map(block => ({ type: block.type, text: block.text || '', mediaId: block.media_id, fileName: block.file_name || '', byteSize: block.byte_size || 0 })) },
    quoteName() { return this.item.reply?.sender_name || (this.item.reply?.sender_id === this.item.sender_id ? this.favorite.sender_name : '成员') },
    quoteText() { return replyExcerpt(this.item.reply) },
  },
  methods: { url(id) { return favoriteMediaURL(this.favorite.id, id) }, formatBytes },
  template: `<div class="saved-message-content" :class="{ mine: favorite.is_own }"><div v-if="item.reply" class="reply-quote"><strong>{{ quoteName }}</strong><span>{{ quoteText }}</span></div><div class="bubble" :class="{ media: item.kind === 'image' || item.kind === 'video' }"><template v-if="usesBlocks"><div class="block-stack"><template v-for="(segment, index) in segments" :key="index"><p v-if="segment.type === 'text'" class="block-text">{{ segment.text }}</p><a v-else-if="segment.type === 'image'" class="block-image" :href="url(segment.mediaId)" target="_blank" rel="noopener" :aria-label="'查看图片 ' + segment.fileName"><img :src="url(segment.mediaId)" :alt="segment.fileName" loading="lazy" /></a><video v-else-if="segment.type === 'video'" class="media-video" :src="url(segment.mediaId)" controls preload="metadata" :aria-label="'播放视频 ' + segment.fileName"></video><audio v-else-if="segment.type === 'audio'" class="media-audio" :src="url(segment.mediaId)" controls preload="metadata" aria-label="播放语音"></audio><a v-else class="block-file" :href="url(segment.mediaId)" :download="segment.fileName"><Paperclip :size="16" aria-hidden="true" /><span>{{ segment.fileName }}</span></a></template></div></template><template v-else><template v-if="item.kind === 'image'"><a class="media-item" :href="url(item.media_id)" target="_blank" rel="noopener" :aria-label="'查看图片 ' + (item.media?.file_name || '')"><img class="media-image" :src="url(item.media_id)" :alt="item.media?.file_name || ''" loading="lazy" /></a><p v-if="item.content" class="media-caption">{{ item.content }}</p></template><template v-else-if="item.kind === 'video'"><video class="media-video" :src="url(item.media_id)" controls preload="metadata" :aria-label="'播放视频 ' + (item.media?.file_name || '')"></video><p v-if="item.content" class="media-caption">{{ item.content }}</p></template><template v-else-if="item.kind === 'audio'"><audio class="media-audio" :src="url(item.media_id)" controls preload="metadata" aria-label="播放语音"></audio><p v-if="item.content" class="media-caption">{{ item.content }}</p></template><template v-else-if="item.kind === 'file'"><a class="media-file" :href="url(item.media_id)" :download="item.media?.file_name"><Paperclip :size="20" aria-hidden="true" /><span class="media-file-text"><strong>{{ item.media?.file_name }}</strong><small>{{ formatBytes(item.media?.byte_size) }}</small></span><Download :size="16" aria-hidden="true" /></a><p v-if="item.content" class="media-caption">{{ item.content }}</p></template><template v-else>{{ item.content }}</template></template></div></div>`,
}
