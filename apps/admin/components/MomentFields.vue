<script setup lang="ts">
import { computed } from 'vue'
import type { Moment, PlatformLink, YujianContentSnapshot } from '@yujian/schema'
import type { AdminLocale } from '../utils/admin-locale'
import type { MomentDraft } from '../utils/moment-workbench'
import { momentCopy } from '../utils/moment-copy'
import { musicProviders } from '../utils/music-copy'
import LocalizedTextFields from './LocalizedTextFields.vue'

const model = defineModel<Moment>({ required: true })
const props = defineProps<{ snapshot: YujianContentSnapshot; draft: MomentDraft; locale: AdminLocale }>()
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
    ><option value="">{{ copy.choose }}</option><option
      v-for="asset in imageAssets"
      :key="asset.id"
      :value="asset.id"
    >{{ asset.alt[locale] || asset.alt['zh-CN'] }} · {{ asset.id }}</option></select></label>
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
    />
    <label><span>{{ copy.target }}</span><select
      :value="model.target?.kind || 'none'"
      data-testid="moment-target-kind"
      @change="changeTarget"
    ><option value="none">{{ copy.none }}</option><option value="internal">{{ copy.internal }}</option><option value="external">{{ copy.external }}</option></select></label>
    <label v-if="model.target?.kind === 'internal'"><span>{{ copy.content }}</span><select
      v-model="model.target.contentId"
      data-testid="moment-target-content"
    ><option value="">{{ copy.choose }}</option><option
      v-for="content in contentOptions"
      :key="content.id"
      :value="content.id"
    >{{ content.title[locale] || content.title['zh-CN'] || content.id }} · {{ content.id }}</option></select></label>
    <div
      v-if="model.target?.kind === 'external'"
      class="music-fields"
    >
      <div class="music-field-grid">
        <label><span>{{ copy.provider }}</span><select
          v-model="model.target.link.provider"
          data-testid="moment-target-provider"
        ><option
          v-for="provider in musicProviders"
          :key="provider"
          :value="provider"
        >{{ provider }}</option></select></label>
        <label><span>{{ copy.url }}</span><input
          v-model="model.target.link.url"
          type="url"
          spellcheck="false"
          data-testid="moment-target-url"
        ></label>
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
      />
    </div>
  </div>
</template>
