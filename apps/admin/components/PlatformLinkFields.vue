<script setup lang="ts">
import { Plus, X } from '@lucide/vue'
import { computed } from 'vue'
import type { PlatformLink } from '@yujian/schema'
import type { AdminLocale } from '../utils/admin-locale'
import { musicCopy, musicProviders } from '../utils/music-copy'
import LocalizedTextFields from './LocalizedTextFields.vue'

const model = defineModel<PlatformLink[]>({ required: true })
const props = defineProps<{ locale: AdminLocale }>()
const copy = computed(() => musicCopy(props.locale))

/** 新链接保留空地址，只有整个表单通过 HTTPS 校验后才会写入快照。 */
function addLink() {
  model.value.push({ provider: 'qq-music', url: '' })
}

/** 显式切换自定义标签，关闭时移除整个选填字段。 */
function toggleLabel(link: PlatformLink, event: Event) {
  if ((event.target as HTMLInputElement).checked) link.label = { 'zh-CN': '' }
  else delete link.label
}
</script>

<template>
  <details class="music-details">
    <summary>{{ copy.links }} <span>{{ model.length }}</span></summary>
    <div
      v-for="(link, index) in model"
      :key="index"
      class="music-link-fields"
      data-testid="platform-link"
    >
      <div class="music-link-row">
        <label>
          <span>{{ copy.provider }}</span>
          <select
            v-model="link.provider"
            data-testid="link-provider"
          >
            <option
              v-for="provider in musicProviders"
              :key="provider"
              :value="provider"
            >{{ provider }}</option>
          </select>
        </label>
        <label>
          <span>{{ copy.url }}</span>
          <input
            v-model="link.url"
            type="url"
            data-testid="link-url"
            spellcheck="false"
          >
        </label>
        <button
          type="button"
          class="icon-tool"
          :aria-label="`${copy.remove} ${copy.links} ${index + 1}`"
          @click="model.splice(index, 1)"
        >
          <X
            :size="16"
            aria-hidden="true"
          />
        </button>
      </div>
      <label class="music-checkbox">
        <input
          type="checkbox"
          :checked="Boolean(link.label)"
          @change="toggleLabel(link, $event)"
        >
        <span>{{ copy.customLabel }}</span>
      </label>
      <LocalizedTextFields
        v-if="link.label"
        v-model="link.label"
        :locale="locale"
        :label="copy.label"
      />
    </div>
    <button
      type="button"
      class="button"
      data-testid="link-add"
      @click="addLink"
    >
      <Plus
        :size="15"
        aria-hidden="true"
      /> {{ copy.addLink }}
    </button>
  </details>
</template>
