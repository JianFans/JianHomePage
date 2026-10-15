<script setup lang="ts">
import { Plus, X } from '@lucide/vue'
import { computed, useId } from 'vue'
import type { PlatformLink } from '@yujian/schema'
import type { AdminLocale } from '../utils/admin-locale'
import { musicCopy, musicProviders } from '../utils/music-copy'
import type { SnapshotEditorIssue } from '../utils/snapshot-workbench'
import { issueFieldAttributes } from '../utils/issue-accessibility'
import LocalizedTextFields from './LocalizedTextFields.vue'

const model = defineModel<PlatformLink[]>({ required: true })
const props = defineProps<{ locale: AdminLocale; path?: string; issues?: readonly SnapshotEditorIssue[] }>()
const copy = computed(() => musicCopy(props.locale))
const issueId = useId()

/** 链接增删后使用当前位置，与 canonical 数组诊断保持一致。 */
function field(name = '') {
  const path = props.path ? `${props.path}${name ? `/${name}` : ''}` : ''
  return issueFieldAttributes(props.issues?.find(issue => issue.path === path), path, `${issueId}-${name.replaceAll('/', '-')}`, props.locale)
}

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
            :data-snapshot-path="field(`${index}/provider`)['data-snapshot-path']"
            :aria-invalid="field(`${index}/provider`)['aria-invalid']"
            :aria-describedby="field(`${index}/provider`)['aria-describedby']"
          >
            <option
              v-for="provider in musicProviders"
              :key="provider"
              :value="provider"
            >{{ provider }}</option>
          </select>
          <small
            v-if="field(`${index}/provider`).description"
            :id="field(`${index}/provider`)['aria-describedby']"
            class="field-issue-hint"
          >{{ field(`${index}/provider`).description }}</small>
        </label>
        <label>
          <span>{{ copy.url }}</span>
          <input
            v-model="link.url"
            type="url"
            data-testid="link-url"
            spellcheck="false"
            :data-snapshot-path="field(`${index}/url`)['data-snapshot-path']"
            :aria-invalid="field(`${index}/url`)['aria-invalid']"
            :aria-describedby="field(`${index}/url`)['aria-describedby']"
          >
          <small
            v-if="field(`${index}/url`).description"
            :id="field(`${index}/url`)['aria-describedby']"
            class="field-issue-hint"
          >{{ field(`${index}/url`).description }}</small>
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
        :path="path ? `${path}/${index}/label` : undefined"
        :issues="issues"
      />
    </div>
    <button
      type="button"
      class="button"
      data-testid="link-add"
      :data-snapshot-path="field()['data-snapshot-path']"
      :aria-invalid="field()['aria-invalid']"
      :aria-describedby="field()['aria-describedby']"
      @click="addLink"
    >
      <Plus
        :size="15"
        aria-hidden="true"
      /> {{ copy.addLink }}
    </button>
    <small
      v-if="field().description"
      :id="field()['aria-describedby']"
      class="field-issue-hint"
    >{{ field().description }}</small>
  </details>
</template>
