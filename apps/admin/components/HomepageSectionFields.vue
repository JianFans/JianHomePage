<script setup lang="ts">
import { ArrowDown, ArrowUp, Plus, X } from '@lucide/vue'
import { computed, ref, useId, watch } from 'vue'
import type { PlatformLink, YujianContentSnapshot } from '@yujian/schema'
import type { AdminLocale } from '../utils/admin-locale'
import { homepageCopy } from '../utils/homepage-copy'
import { musicProviders } from '../utils/music-copy'
import type { SnapshotEditorIssue } from '../utils/snapshot-workbench'
import { issueFieldAttributes } from '../utils/issue-accessibility'
import { homepageItemOptions, homepageItemVisibility, type HomepageDraft, type HomepageSection } from '../utils/homepage-workbench'
import LocalizedTextFields from './LocalizedTextFields.vue'

const model = defineModel<HomepageSection>({ required: true })
const props = defineProps<{ snapshot: YujianContentSnapshot; draft: HomepageDraft; locale: AdminLocale; path?: string; issues?: readonly SnapshotEditorIssue[] }>()
const issueId = useId()

/** 编排重排时由父级更新路径，更多链接与上限使用同一诊断契约。 */
function field(name: string) {
  const path = props.path ? `${props.path}/${name}` : ''
  return issueFieldAttributes(props.issues?.find(issue => issue.path === path), path, `${issueId}-${name.replaceAll('/', '-')}`, props.locale)
}
const copy = computed(() => homepageCopy(props.locale))
const selectedId = ref('')
const options = computed(() => homepageItemOptions(props.snapshot, props.draft, model.value))
const optionIndex = computed(() => new Map(options.value.map(option => [option.id, option])))
const visibility = computed(() => homepageItemVisibility(props.snapshot, props.draft, model.value))
const maxItems = computed(() => model.value.type === 'hero' ? 5 : model.value.type === 'artist' ? 1 : Infinity)
const minItems = computed(() => model.value.type === 'hero' ? 1 : 0)
const limitMaximum = computed(() => ({ music: 8, video: 5, event: 6, moment: 8 })[model.value.type as 'music' | 'video' | 'event' | 'moment'])

watch(() => model.value, /** 按位置复用组件时清除上一板块的选择；嵌套字段编辑不影响待添加内容。 */ () => {
  selectedId.value = ''
})

watch(optionIndex, /** 候选记录删除后清除失效选择，其他候选变化保留有效选择。 */ (index) => {
  if (selectedId.value && !index.has(selectedId.value)) selectedId.value = ''
})

/** 每次新增一个引用位置，允许重复内容但不超出宣传位和艺人的结构上限。 */
function addItem() {
  if (!selectedId.value || !optionIndex.value.has(selectedId.value) || model.value.itemIds.length >= maxItems.value) return
  ;(model.value.itemIds as string[]).push(selectedId.value)
}

/** 交换指定位置，绝不通过 ID 查找而误移动重复引用的首项。 */
function moveItem(index: number, direction: -1 | 1) {
  const next = index + direction
  const items = model.value.itemIds as string[]
  if (next < 0 || next >= items.length) return
  ;[items[index], items[next]] = [items[next]!, items[index]!]
}

/** 保留正常编辑所需的最少宣传位引用；记录删除后的无效草稿仍由应用校验。 */
function removeItem(index: number) {
  if (model.value.itemIds.length > minItems.value) model.value.itemIds.splice(index, 1)
}

/** 更多链接使用稳定单对象，关闭时移除整个可选字段。 */
function toggleMore(event: Event) {
  if (model.value.type !== 'music') return
  if ((event.target as HTMLInputElement).checked) model.value.moreLink = { provider: 'website', url: '' }
  else delete model.value.moreLink
}

/** 自定义标签关闭时移除键，不保存空标签对象。 */
function toggleLabel(link: PlatformLink, event: Event) {
  if ((event.target as HTMLInputElement).checked) link.label = { 'zh-CN': '' }
  else delete link.label
}
</script>

<template>
  <div class="music-fields">
    <div class="music-field-grid">
      <label class="music-checkbox"><input
        v-model="model.enabled"
        type="checkbox"
        data-testid="homepage-section-enabled"
      ><span>{{ copy.enabled }}</span></label>
      <label v-if="'limit' in model"><span>{{ copy.limit }}</span><input
        v-model.number="model.limit"
        type="number"
        min="1"
        :max="limitMaximum"
        data-testid="homepage-section-limit"
        :data-snapshot-path="field('limit')['data-snapshot-path']"
        :aria-invalid="field('limit')['aria-invalid']"
        :aria-describedby="field('limit')['aria-describedby']"
      ></label>
    </div>
    <ol class="homepage-items">
      <li
        v-for="(itemId, index) in model.itemIds"
        :key="index"
        class="homepage-item"
      >
        <div class="homepage-item-caption">
          <span>{{ optionIndex.get(itemId)?.title[locale] || optionIndex.get(itemId)?.title['zh-CN'] || itemId }}</span><small class="music-muted">{{ index + 1 }} · {{ itemId }} · {{ copy.status[visibility[index]!] }}</small>
        </div>
        <div class="music-tools">
          <button
            type="button"
            class="icon-tool"
            data-testid="homepage-item-up"
            :disabled="index === 0"
            :aria-label="`${copy.up} ${itemId} ${index + 1}`"
            @click="moveItem(index, -1)"
          >
            <ArrowUp
              :size="16"
              aria-hidden="true"
            />
          </button>
          <button
            type="button"
            class="icon-tool"
            data-testid="homepage-item-down"
            :disabled="index === model.itemIds.length - 1"
            :aria-label="`${copy.down} ${itemId} ${index + 1}`"
            @click="moveItem(index, 1)"
          >
            <ArrowDown
              :size="16"
              aria-hidden="true"
            />
          </button>
          <button
            type="button"
            class="icon-tool"
            data-testid="homepage-item-remove"
            :disabled="model.itemIds.length <= minItems"
            :aria-label="`${copy.remove} ${itemId} ${index + 1}`"
            @click="removeItem(index)"
          >
            <X
              :size="16"
              aria-hidden="true"
            />
          </button>
        </div>
      </li>
    </ol>
    <div class="music-link-row">
      <label><span>{{ copy.addItem }}</span><select
        v-model="selectedId"
        data-testid="homepage-item-add-select"
        :data-snapshot-path="field('itemIds')['data-snapshot-path']"
        :aria-invalid="field('itemIds')['aria-invalid']"
        :aria-describedby="field('itemIds')['aria-describedby']"
        :disabled="model.itemIds.length >= maxItems"
      ><option value="">{{ copy.choose }}</option><option
        v-for="option in options"
        :key="option.id"
        :value="option.id"
      >{{ option.title[locale] || option.title['zh-CN'] || option.id }} · {{ option.id }}</option></select></label>
      <button
        type="button"
        class="icon-tool"
        data-testid="homepage-item-add"
        :disabled="!optionIndex.has(selectedId) || model.itemIds.length >= maxItems"
        :aria-label="copy.addItem"
        @click="addItem"
      >
        <Plus
          :size="16"
          aria-hidden="true"
        />
      </button>
    </div>
    <template v-if="model.type === 'music'">
      <label class="music-checkbox"><input
        type="checkbox"
        :checked="Boolean(model.moreLink)"
        data-testid="homepage-more-enabled"
        @change="toggleMore"
      ><span>{{ copy.more }}</span></label>
      <template v-if="model.moreLink">
        <div class="music-field-grid">
          <label><span>{{ copy.provider }}</span><select
            v-model="model.moreLink.provider"
            data-testid="homepage-more-provider"
            :data-snapshot-path="field('moreLink/provider')['data-snapshot-path']"
            :aria-invalid="field('moreLink/provider')['aria-invalid']"
            :aria-describedby="field('moreLink/provider')['aria-describedby']"
          ><option
            v-for="provider in musicProviders"
            :key="provider"
            :value="provider"
          >{{ provider }}</option></select></label>
          <label><span>{{ copy.url }}</span><input
            v-model="model.moreLink.url"
            type="url"
            spellcheck="false"
            data-testid="homepage-more-url"
            :data-snapshot-path="field('moreLink/url')['data-snapshot-path']"
            :aria-invalid="field('moreLink/url')['aria-invalid']"
            :aria-describedby="field('moreLink/url')['aria-describedby']"
          ></label>
        </div>
        <label class="music-checkbox"><input
          type="checkbox"
          :checked="Boolean(model.moreLink.label)"
          data-testid="homepage-more-custom-label"
          @change="toggleLabel(model.moreLink, $event)"
        ><span>{{ copy.customLabel }}</span></label>
        <LocalizedTextFields
          v-if="model.moreLink.label"
          v-model="model.moreLink.label"
          :locale="locale"
          :label="copy.label"
          test-id="homepage-more-label"
          :path="path ? `${path}/moreLink/label` : undefined"
          :issues="issues"
        />
      </template>
    </template>
    <template
      v-for="name in ['itemIds', 'limit', 'moreLink/provider', 'moreLink/url']"
      :key="name"
    >
      <small
        v-if="field(name).description"
        :id="field(name)['aria-describedby']"
        class="field-issue-hint"
      >{{ field(name).description }}</small>
    </template>
  </div>
</template>

<style scoped>
.homepage-items { list-style: none; margin: 0; padding: 0; }
.homepage-item { display: flex; align-items: center; justify-content: space-between; gap: .75rem; padding: .5rem 0; border-bottom: 1px solid var(--border); }
.homepage-item-caption { display: grid; gap: .25rem; min-width: 0; overflow-wrap: anywhere; font-size: .8rem; }
.homepage-item > .music-tools { flex-shrink: 0; }
@media (max-width: 600px) { .homepage-item { align-items: start; flex-direction: column; } }
</style>
