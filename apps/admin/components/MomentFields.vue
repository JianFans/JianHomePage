<script setup lang="ts">
import { computed, useId } from 'vue'
import type { Moment, PlatformLink, YujianContentSnapshot } from '@yujian/schema'
import type { AdminLocale } from '../utils/admin-locale'
import type { MomentDraft } from '../utils/moment-workbench'
import { momentCopy } from '../utils/moment-copy'
import { musicProviders } from '../utils/music-copy'
import type { SnapshotEditorIssue } from '../utils/snapshot-workbench'
import { issueFieldAttributes } from '../utils/issue-accessibility'
import LocalizedTextFields from './LocalizedTextFields.vue'

const model = defineModel<Moment>({ required: true })
const props = defineProps<{ snapshot: YujianContentSnapshot; draft: MomentDraft; locale: AdminLocale; path?: string; issues?: readonly SnapshotEditorIssue[] }>()
const issueId = useId()
const copy = computed(() => momentCopy(props.locale))
const imageAssets = computed(() => props.snapshot.assets.filter(asset => asset.kind === 'image' || asset.kind === 'gif'))
const contentOptions = computed(() => [
  ...props.snapshot.heroSlides.map(item => ({ id: item.id, title: item.headline })),
  ...props.snapshot.releases.map(item => ({ id: item.id, title: item.title })),
  ...props.snapshot.tracks.map(item => ({ id: item.id, title: item.title })),
  ...props.snapshot.videos.map(item => ({ id: item.id, title: item.title })),
  ...props.snapshot.events.map(item => ({ id: item.id, title: item.title })),
  ...props.draft.moments.map(item => ({ id: item.id, title: item.caption ?? { 'zh-CN': item.id } })),
  { id: props.snapshot.artist.id, title: props.snapshot.artist.name },
])

/** 将选中片段的可编辑字段映射到稳定诊断路径和描述节点。 */
function field(name: string) {
  const path = props.path ? `${props.path}/${name}` : ''
  return issueFieldAttributes(props.issues?.find(issue => issue.path === path), path, `${issueId}-${name.replaceAll('/', '-')}`, props.locale)
}

/** 显式开启说明后中文必填；关闭删除整个选填字段。 */
function toggleCaption(event: Event) {
  if ((event.target as HTMLInputElement).checked) model.value.caption = { 'zh-CN': '' }
  else delete model.value.caption
}

/** 重新建立唯一分支，避免残留上一类跳转字段。 */
function changeTarget(event: Event) {
  const kind = (event.target as HTMLSelectElement).value
  if (kind === 'internal') model.value.target = { kind, contentId: '' }
  else if (kind === 'external') model.value.target = { kind, link: { provider: 'website', url: '' } }
  else delete model.value.target
}

/** 单链接直接修改稳定对象，不借助临时数组承载输入。 */
function toggleLabel(link: PlatformLink, event: Event) {
  if ((event.target as HTMLInputElement).checked) link.label = { 'zh-CN': '' }
  else delete link.label
}
</script>

<template>
  <div class="music-fields">
    <label><span>{{ copy.asset }}</span><select
      v-model="model.assetId"
      data-testid="moment-asset"
      :data-snapshot-path="field('assetId')['data-snapshot-path']"
      :aria-invalid="field('assetId')['aria-invalid']"
      :aria-describedby="field('assetId')['aria-describedby']"
    ><option value="">{{ copy.choose }}</option><option
      v-for="asset in imageAssets"
      :key="asset.id"
      :value="asset.id"
    >{{ asset.alt[locale] || asset.alt['zh-CN'] }} · {{ asset.id }}</option></select><small
      v-if="field('assetId').description"
      :id="field('assetId')['aria-describedby']"
      class="sr-only"
    >{{ field('assetId').description }}</small></label>
    <label class="music-checkbox"><input
      type="checkbox"
      :checked="Boolean(model.caption)"
      data-testid="moment-custom-caption"
      @change="toggleCaption"
    ><span>{{ copy.customCaption }}</span></label>
    <LocalizedTextFields
      v-if="model.caption"
      v-model="model.caption"
      :locale="locale"
      :label="copy.caption"
      test-id="moment-caption"
      :path="path ? `${path}/caption` : undefined"
      :issues="issues"
    />
    <label><span>{{ copy.target }}</span><select
      :value="model.target?.kind || 'none'"
      data-testid="moment-target-kind"
      @change="changeTarget"
    ><option value="none">{{ copy.none }}</option><option value="internal">{{ copy.internal }}</option><option value="external">{{ copy.external }}</option></select></label>
    <label v-if="model.target?.kind === 'internal'"><span>{{ copy.content }}</span><select
      v-model="model.target.contentId"
      data-testid="moment-target-content"
      :data-snapshot-path="field('target/contentId')['data-snapshot-path']"
      :aria-invalid="field('target/contentId')['aria-invalid']"
      :aria-describedby="field('target/contentId')['aria-describedby']"
    ><option value="">{{ copy.chooseContent }}</option><option
      v-for="content in contentOptions"
      :key="content.id"
      :value="content.id"
    >{{ content.title[locale] || content.title['zh-CN'] || content.id }} · {{ content.id }}</option></select><small
      v-if="field('target/contentId').description"
      :id="field('target/contentId')['aria-describedby']"
      class="sr-only"
    >{{ field('target/contentId').description }}</small></label>
    <div
      v-if="model.target?.kind === 'external'"
      class="music-fields"
    >
      <div class="music-field-grid">
        <label><span>{{ copy.provider }}</span><select
          v-model="model.target.link.provider"
          data-testid="moment-target-provider"
          :data-snapshot-path="field('target/link/provider')['data-snapshot-path']"
          :aria-invalid="field('target/link/provider')['aria-invalid']"
          :aria-describedby="field('target/link/provider')['aria-describedby']"
        ><option
          v-for="provider in musicProviders"
          :key="provider"
          :value="provider"
        >{{ provider }}</option></select><small
          v-if="field('target/link/provider').description"
          :id="field('target/link/provider')['aria-describedby']"
          class="sr-only"
        >{{ field('target/link/provider').description }}</small></label>
        <label><span>{{ copy.url }}</span><input
          v-model="model.target.link.url"
          type="url"
          spellcheck="false"
          data-testid="moment-target-url"
          :data-snapshot-path="field('target/link/url')['data-snapshot-path']"
          :aria-invalid="field('target/link/url')['aria-invalid']"
          :aria-describedby="field('target/link/url')['aria-describedby']"
        ><small
          v-if="field('target/link/url').description"
          :id="field('target/link/url')['aria-describedby']"
          class="sr-only"
        >{{ field('target/link/url').description }}</small></label>
      </div>
      <label class="music-checkbox"><input
        type="checkbox"
        :checked="Boolean(model.target.link.label)"
        data-testid="moment-target-custom-label"
        @change="toggleLabel(model.target.link, $event)"
      ><span>{{ copy.customLabel }}</span></label>
      <LocalizedTextFields
        v-if="model.target.link.label"
        v-model="model.target.link.label"
        :locale="locale"
        :label="copy.label"
        test-id="moment-target-label"
        :path="path ? `${path}/target/link/label` : undefined"
        :issues="issues"
      />
    </div>
  </div>
</template>
