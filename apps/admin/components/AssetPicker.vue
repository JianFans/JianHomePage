<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Asset, YujianContentSnapshot } from '@yujian/schema'
import type { AdminLocale } from '../utils/admin-locale'
import AssetWorkbench from './AssetWorkbench.vue'
import StudioDialog from './StudioDialog.vue'

const props = defineProps<{ open: boolean; snapshot: YujianContentSnapshot; kinds: readonly Asset['kind'][]; locale: AdminLocale; label: string; apiBaseUrl: string; csrfToken: string; locked: boolean }>()
const emit = defineEmits<{ select: [asset: Asset]; close: []; unauthorized: []; notify: [message: string, tone: 'success' | 'error'] }>()
const tab = ref<'snapshot' | 'library'>('snapshot')
const query = ref('')
const uploadBusy = ref(false)
const en = computed(() => props.locale === 'en')
const choices = computed(() => props.snapshot.assets.filter(asset => props.kinds.includes(asset.kind)
  && [asset.id, asset.alt['zh-CN'], asset.alt.en, asset.src].some(value => value?.toLowerCase().includes(query.value.toLowerCase()))))
watch(() => props.open, () => { tab.value = 'snapshot'; query.value = '' })
/** 两种来源都由父级再次校验并以同一事务绑定，不在弹窗内修改编辑器。 */
function select(asset: Asset) { if (!props.locked && !uploadBusy.value && props.kinds.includes(asset.kind)) emit('select', asset) }
</script>

<template>
  <StudioDialog
    :open="open"
    :title="label"
    :close-label="en ? 'Close asset picker' : '关闭素材选择'"
    :busy="uploadBusy"
    wide
    @close="emit('close')"
  >
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
      v-else-if="open"
      studio
      :locale="locale"
      :editor-text="JSON.stringify(snapshot)"
      :api-base-url="apiBaseUrl"
      token=""
      :csrf-token="csrfToken"
      authenticated
      :can-upload="!locked"
      :snapshot-locked="locked"
      :picker-kinds="kinds"
      @select="select"
      @busy="value => uploadBusy = value"
      @unauthorized="emit('unauthorized')"
      @notify="(message, tone) => emit('notify', message, tone)"
    />
  </StudioDialog>
</template>

<style scoped>
.picker-tabs { display: flex; flex-wrap: wrap; gap: .6rem; margin-block-end: 1rem; }
.picker-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 11rem), 1fr)); gap: .75rem; margin-block: 1rem; }
.picker-choice { min-width: 0; display: grid; align-content: start; gap: .5rem; padding: .75rem; border: 1px solid var(--border); border-radius: .6rem; background: var(--surface); color: var(--text); text-align: start; }
.picker-choice img { width: 100%; aspect-ratio: 1; object-fit: cover; }
.picker-choice small { overflow-wrap: anywhere; color: var(--muted); }
</style>
