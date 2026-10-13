<script setup lang="ts">
import { computed, useId } from 'vue'
import type { HeroSlide, PlatformLink, YujianContentSnapshot } from '@yujian/schema'
import type { AdminLocale } from '../utils/admin-locale'
import { homepageCopy } from '../utils/homepage-copy'
import { musicProviders } from '../utils/music-copy'
import { homepageItemVisibility, type HomepageDraft } from '../utils/homepage-workbench'
import LocalizedTextFields from './LocalizedTextFields.vue'

const model = defineModel<HeroSlide>({ required: true })
const props = defineProps<{ snapshot: YujianContentSnapshot; draft: HomepageDraft; locale: AdminLocale }>()
const copy = computed(() => homepageCopy(props.locale))
const id = useId()
const mediaAssets = computed(() => props.snapshot.assets.filter(asset => asset.kind === model.value.mediaKind))
const posterAssets = computed(() => props.snapshot.assets.filter(asset => asset.kind === 'image' || asset.kind === 'gif'))
const contentOptions = computed(() => [
  ...props.draft.heroSlides.map(hero => ({ id: hero.id, title: hero.headline })),
  ...props.snapshot.releases.map(item => ({ id: item.id, title: item.title })),
  ...props.snapshot.tracks.map(item => ({ id: item.id, title: item.title })),
  ...props.snapshot.videos.map(item => ({ id: item.id, title: item.title })),
  ...props.snapshot.events.map(item => ({ id: item.id, title: item.title })),
  ...props.snapshot.moments.map(item => ({ id: item.id, title: item.caption ?? { 'zh-CN': item.id } })),
  { id: props.snapshot.artist.id, title: props.snapshot.artist.name },
])
const targetStatus = computed(() => {
  if (model.value.target?.kind !== 'internal') return ''
  const contentId = model.value.target.contentId
  const statuses = props.draft.sections.flatMap(section => {
    const visibility = homepageItemVisibility(props.snapshot, props.draft, section)
    return section.itemIds.flatMap((itemId, index) => {
      const matches = itemId === contentId || (section.type === 'music'
        && props.snapshot.releases.some(release => release.id === itemId && release.trackIds.includes(contentId)))
      return matches ? [visibility[index]!] : []
    })
  })
  if (statuses.includes('visible')) return copy.value.status.visible
  return statuses.length ? [...new Set(statuses)].map(status => copy.value.status[status]).join(' · ') : copy.value.absent
})

/** 空白选填输入删除对应键，日期原文保持 UTC 契约而不做本机时区转换。 */
function optionalField(key: 'mobileAssetId' | 'posterAssetId' | 'releaseId' | 'startsAt' | 'endsAt', event: Event) {
  const value = (event.target as HTMLInputElement).value
  if (value) model.value[key] = value
  else delete model.value[key]
}

/** 目标类型变更显式建立该分支的字段；空输入留给 canonical 校验。 */
function changeTarget(event: Event) {
  const kind = (event.target as HTMLSelectElement).value
  if (kind === 'internal') model.value.target = { kind, contentId: '' }
  else if (kind === 'external') model.value.target = { kind, link: { provider: 'website', url: '' } }
  else delete model.value.target
}

/** 单链接直接修改稳定对象，不使用会丢失 push/splice 的临时数组。 */
function toggleLabel(link: PlatformLink, event: Event) {
  if ((event.target as HTMLInputElement).checked) link.label = { 'zh-CN': '' }
  else delete link.label
}
</script>

<template>
  <div class="music-fields">
    <LocalizedTextFields
      v-model="model.headline"
      :locale="locale"
      :label="copy.headline"
      test-id="homepage-headline"
    />
    <div class="music-field-grid">
      <label><span>{{ copy.mediaKind }}</span><select
        v-model="model.mediaKind"
        data-testid="homepage-media-kind"
      ><option value="image">{{ copy.image }}</option><option value="gif">{{ copy.gif }}</option><option value="video">{{ copy.video }}</option></select></label>
      <label><span>{{ copy.asset }}</span><select
        v-model="model.assetId"
        data-testid="homepage-hero-asset"
      ><option value="">{{ copy.choose }}</option><option
        v-for="asset in mediaAssets"
        :key="asset.id"
        :value="asset.id"
      >{{ asset.alt[locale] || asset.alt['zh-CN'] }} · {{ asset.id }}</option></select></label>
      <label><span>{{ copy.mobileAsset }}</span><select
        :value="model.mobileAssetId || ''"
        data-testid="homepage-mobile-asset"
        @change="optionalField('mobileAssetId', $event)"
      ><option value="">{{ copy.none }}</option><option
        v-for="asset in mediaAssets"
        :key="asset.id"
        :value="asset.id"
      >{{ asset.alt[locale] || asset.alt['zh-CN'] }} · {{ asset.id }}</option></select></label>
      <label><span>{{ copy.poster }}</span><select
        :value="model.posterAssetId || ''"
        data-testid="homepage-poster-asset"
        @change="optionalField('posterAssetId', $event)"
      ><option value="">{{ copy.none }}</option><option
        v-for="asset in posterAssets"
        :key="asset.id"
        :value="asset.id"
      >{{ asset.alt[locale] || asset.alt['zh-CN'] }} · {{ asset.id }}</option></select></label>
      <label><span>{{ copy.focalX }}</span><input
        v-model.number="model.focalPoint.x"
        type="number"
        min="0"
        max="100"
        data-testid="homepage-focal-x"
      ></label>
      <label><span>{{ copy.focalY }}</span><input
        v-model.number="model.focalPoint.y"
        type="number"
        min="0"
        max="100"
        data-testid="homepage-focal-y"
      ></label>
    </div>
    <label class="music-checkbox"><input
      v-model="model.autoplay"
      type="checkbox"
      data-testid="homepage-autoplay"
    ><span>{{ copy.autoplay }}</span></label>
    <label><span>{{ copy.release }}</span><select
      :value="model.releaseId || ''"
      data-testid="homepage-release"
      @change="optionalField('releaseId', $event)"
    ><option value="">{{ copy.none }}</option><option
      v-for="release in snapshot.releases"
      :key="release.id"
      :value="release.id"
    >{{ release.title[locale] || release.title['zh-CN'] }} · {{ release.id }}</option></select></label>
    <label><span>{{ copy.target }}</span><select
      :value="model.target?.kind || 'none'"
      data-testid="homepage-target-kind"
      @change="changeTarget"
    ><option value="none">{{ copy.none }}</option><option value="internal">{{ copy.internal }}</option><option value="external">{{ copy.external }}</option></select></label>
    <template v-if="model.target?.kind === 'internal'">
      <label><span>{{ copy.content }}</span><select
        v-model="model.target.contentId"
        data-testid="homepage-target-content"
      ><option value="">{{ copy.choose }}</option><option
        v-for="content in contentOptions"
        :key="content.id"
        :value="content.id"
      >{{ content.title[locale] || content.title['zh-CN'] || content.id }} · {{ content.id }}</option></select></label>
      <p
        class="music-muted"
        data-testid="homepage-target-visibility"
      >
        {{ targetStatus }}
      </p>
    </template>
    <div
      v-if="model.target?.kind === 'external'"
      class="music-fields"
    >
      <div class="music-field-grid">
        <label><span>{{ copy.provider }}</span><select
          v-model="model.target.link.provider"
          data-testid="homepage-target-provider"
        ><option
          v-for="provider in musicProviders"
          :key="provider"
          :value="provider"
        >{{ provider }}</option></select></label>
        <label><span>{{ copy.url }}</span><input
          v-model="model.target.link.url"
          type="url"
          spellcheck="false"
          data-testid="homepage-target-url"
        ></label>
      </div>
      <label class="music-checkbox"><input
        type="checkbox"
        :checked="Boolean(model.target.link.label)"
        data-testid="homepage-target-custom-label"
        @change="toggleLabel(model.target.link, $event)"
      ><span>{{ copy.customLabel }}</span></label>
      <LocalizedTextFields
        v-if="model.target.link.label"
        v-model="model.target.link.label"
        :locale="locale"
        :label="copy.label"
        test-id="homepage-target-label"
      />
    </div>
    <div class="music-field-grid">
      <label><span>{{ copy.startsAt }}</span><input
        :value="model.startsAt || ''"
        type="text"
        spellcheck="false"
        placeholder="2026-08-29T00:00:00Z"
        :aria-describedby="`${id}-utc`"
        data-testid="homepage-starts-at"
        @input="optionalField('startsAt', $event)"
      ></label>
      <label><span>{{ copy.endsAt }}</span><input
        :value="model.endsAt || ''"
        type="text"
        spellcheck="false"
        placeholder="2026-09-01T00:00:00Z"
        :aria-describedby="`${id}-utc`"
        data-testid="homepage-ends-at"
        @input="optionalField('endsAt', $event)"
      ></label>
    </div>
    <p
      :id="`${id}-utc`"
      class="music-muted"
    >
      {{ copy.utc }}
    </p>
  </div>
</template>
