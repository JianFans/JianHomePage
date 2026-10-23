<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import type { Asset, YujianContentSnapshot } from '@yujian/schema'
import type { AdminLocale } from '../utils/admin-locale'
import AssetWorkbench from './AssetWorkbench.vue'
import StudioDialog from './StudioDialog.vue'

const props = defineProps<{ open: boolean; snapshot: YujianContentSnapshot; kinds: readonly Asset['kind'][]; locale: AdminLocale; label: string; apiBaseUrl: string; csrfToken: string; locked: boolean }>()
const emit = defineEmits<{ select: [asset: Asset]; close: []; unauthorized: []; notify: [message: string, tone: 'success' | 'error']; busy: [value: boolean]; dirty: [value: boolean] }>()
const tab = ref<'snapshot' | 'library'>('snapshot')
const query = ref('')
const uploadBusy = ref(false)
const uploadDirty = ref(false)
const libraryOpened = ref(false)
const confirming = ref(false)
const pendingSelection = shallowRef<Asset | null>(null)
const en = computed(() => props.locale === 'en')
const choices = computed(() => props.snapshot.assets.filter(asset => props.kinds.includes(asset.kind)
  && [asset.id, asset.alt['zh-CN'], asset.alt.en, asset.src].some(value => value?.toLowerCase().includes(query.value.toLowerCase()))))
// 会话过期仅隐藏视图，保留文件、表单与确认重试；恢复后重新确认选择。
watch(() => props.open, open => { if (!open) keepEditing() })
watch(tab, value => { if (value === 'library') libraryOpened.value = true }, { flush: 'sync' })
watch(uploadBusy, value => emit('busy', value), { immediate: true, flush: 'sync' })
watch(uploadDirty, value => emit('dirty', value), { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { emit('busy', false); emit('dirty', false) })
/** 关闭与选择都会离开上传工作区，先明确确认放弃未完成的输入。 */
function leave(asset: Asset | null = null) {
  if (!props.open || uploadBusy.value) return
  if (uploadDirty.value) { pendingSelection.value = asset; confirming.value = true; return }
  if (asset) emit('select', asset)
  else emit('close')
}
/** 保留同一个工作台实例，取消确认不会丢掉文件及完成重试。 */
function keepEditing() { confirming.value = false; pendingSelection.value = null }
/** 用户明确丢弃后才完成选择或关闭，由父级销毁上传工作区。 */
function discard() {
  if (!props.open || uploadBusy.value) return
  const asset = pendingSelection.value
  keepEditing()
  if (asset && !props.locked) emit('select', asset)
  else if (!asset) emit('close')
}
/** 两种来源都由父级再次校验并以同一事务绑定，不在弹窗内修改编辑器。 */
function select(asset: Asset) {
  const selected = props.snapshot.assets.find(existing => existing.id === asset.id) || asset
  if (!props.locked && props.kinds.includes(selected.kind)) leave(selected)
}
</script>

<template>
  <StudioDialog
    :open="open"
    keep-mounted
    :title="confirming ? (en ? 'Discard upload inputs?' : '放弃上传输入？') : label"
    :content-key="confirming ? 'confirm' : 'picker'"
    :close-label="en ? 'Close asset picker' : '关闭素材选择'"
    :busy="uploadBusy"
    wide
    @close="confirming ? keepEditing() : leave()"
  >
    <div v-if="confirming">
      <p>{{ en ? 'The selected file, upload inputs and pending confirmation will be discarded. Uploaded media will remain in the library.' : '将放弃所选文件、上传输入和待确认状态；已上传的媒体仍保留在素材库中。' }}</p>
      <button
        type="button"
        class="button"
        data-testid="picker-keep"
        @click="keepEditing"
      >
        {{ en ? 'Keep editing' : '继续编辑' }}
      </button>
      <button
        type="button"
        class="button"
        data-testid="picker-discard"
        @click="discard"
      >
        {{ en ? 'Discard and continue' : '放弃并继续' }}
      </button>
    </div>
    <div v-show="!confirming">
      <div
        class="picker-tabs"
        role="group"
        :aria-label="en ? 'Asset source' : '素材来源'"
      >
        <button
          type="button"
          class="button"
          :aria-pressed="tab === 'snapshot'"
          :disabled="uploadBusy"
          @click="tab = 'snapshot'"
        >
          {{ en ? 'Current snapshot' : '当前快照' }}
        </button>
        <button
          type="button"
          class="button"
          data-testid="picker-library"
          :aria-pressed="tab === 'library'"
          :disabled="uploadBusy"
          @click="tab = 'library'"
        >
          {{ en ? 'Library / Upload' : '素材库／上传' }}
        </button>
      </div>
      <div v-if="tab === 'snapshot'">
        <label>{{ en ? 'Search assets' : '搜索素材' }}<input
          v-model="query"
          type="search"
          data-testid="picker-search"
        ></label>
        <div class="picker-grid">
          <button
            v-for="asset in choices"
            :key="asset.id"
            type="button"
            class="picker-choice"
            :disabled="locked"
            data-testid="picker-snapshot-asset"
            @click="select(asset)"
          >
            <img
              v-if="asset.kind === 'image' || asset.kind === 'gif'"
              :src="asset.src"
              alt=""
              loading="lazy"
              referrerpolicy="no-referrer"
            >
            <strong>{{ asset.alt[locale] || asset.alt['zh-CN'] }}</strong><small>{{ asset.kind }} · {{ asset.id }}</small>
          </button>
        </div>
        <p
          v-if="!choices.length"
          class="studio-muted"
        >
          {{ en ? 'No matching assets' : '没有匹配素材' }}
        </p>
      </div>
      <AssetWorkbench
        v-if="libraryOpened"
        v-show="tab === 'library'"
        studio
        :locale="locale"
        :editor-text="JSON.stringify(snapshot)"
        :api-base-url="apiBaseUrl"
        token=""
        :csrf-token="csrfToken"
        :authenticated="open"
        :can-upload="!locked"
        :snapshot-locked="locked"
        :picker-kinds="kinds"
        @select="select"
        @busy="value => uploadBusy = value"
        @dirty="value => uploadDirty = value"
        @unauthorized="emit('unauthorized')"
        @notify="(message, tone) => emit('notify', message, tone)"
      />
    </div>
  </StudioDialog>
</template>

<style scoped>
.picker-tabs { display: flex; flex-wrap: wrap; gap: .6rem; margin-block-end: 1rem; }
.picker-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 11rem), 1fr)); gap: .75rem; margin-block: 1rem; }
.picker-choice { min-width: 0; display: grid; align-content: start; gap: .5rem; padding: .75rem; border: 1px solid var(--border); border-radius: .6rem; background: var(--surface); color: var(--text); text-align: start; }
.picker-choice img { width: 100%; aspect-ratio: 1; object-fit: cover; }
.picker-choice small { overflow-wrap: anywhere; color: var(--muted); }
</style>
