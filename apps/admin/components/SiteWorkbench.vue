<script setup lang="ts">
import { computed, useId } from 'vue'
import type { YujianContentSnapshot } from '@yujian/schema'
import type { AdminLocale } from '../utils/admin-locale'
import type { SiteDraft, SiteError } from '../utils/site-workbench'
import type { SnapshotEditorIssue } from '../utils/snapshot-workbench'
import { issueFieldAttributes } from '../utils/issue-accessibility'
import LocalizedTextFields from './LocalizedTextFields.vue'
import AssetPickerButton from './AssetPickerButton.vue'
import PlatformLinkFields from './PlatformLinkFields.vue'

const draft = defineModel<SiteDraft | null>('draft', { required: true })
const props = defineProps<{ snapshot: YujianContentSnapshot | null; locale: AdminLocale; locked: boolean; stale: boolean; issues: readonly SnapshotEditorIssue[]; error?: SiteError | null }>()
const id = useId()
const en = computed(() => props.locale === 'en')
const images = computed(() => props.snapshot?.assets.filter(asset => asset.kind === 'image' || asset.kind === 'gif') || [])
const copy = computed(() => en.value
  ? { title: 'Site profile', brand: 'Brand', artistName: 'Artist name in site header', canonical: 'Canonical URL', seo: 'Search & sharing', seoTitle: 'Page title', description: 'Description', image: 'Sharing image', artist: 'Artist profile', name: 'Artist name', bio: 'Short biography', portrait: 'Portrait', siteLinks: 'Site social links', artistLinks: 'Artist platforms', none: 'None', stale: 'Snapshot changed. Reopen this module before saving.' }
  : { title: '站点资料', brand: '站点名称', artistName: '站点艺人名称', canonical: '规范网址', seo: '搜索与分享', seoTitle: '页面标题', description: '页面描述', image: '分享图片', artist: '音乐人资料', name: '音乐人名称', bio: '简介', portrait: '肖像', siteLinks: '站点社交入口', artistLinks: '艺人平台入口', none: '不设置', stale: '快照已变化，请重新进入此模块后保存。' })
/** 精确关联完整快照诊断，不将错误提示混入内容字段。 */
function field(path: string) { return issueFieldAttributes(props.issues.find(issue => issue.path === path), path, `${id}-${path.replaceAll('/', '-')}`, props.locale) }
/** 可选图片清空时移除键，保持 canonical 契约。 */
function setImage(kind: 'ogAssetId' | 'portraitAssetId', event: Event) {
  if (!draft.value || props.locked) return
  const value = (event.target as HTMLSelectElement).value
  if (kind === 'ogAssetId') { if (value) draft.value.site.seo.ogAssetId = value; else delete draft.value.site.seo.ogAssetId }
  else if (value) draft.value.artist.portraitAssetId = value
  else delete draft.value.artist.portraitAssetId
}
</script>

<template>
  <section
    class="panel music-panel"
    data-testid="site-workbench"
    :aria-label="copy.title"
  >
    <p
      v-if="!draft && error"
      class="studio-muted"
      role="status"
    >
      {{ en ? 'Could not load site profile. Check the snapshot and reopen this module.' : '无法读取站点资料，请检查快照并重新进入此模块。' }}
    </p>
    <p
      v-if="stale"
      class="studio-muted"
    >
      {{ copy.stale }}
    </p>
    <fieldset
      v-if="draft"
      class="site-fields"
      :disabled="locked"
    >
      <legend>{{ copy.title }}</legend>
      <LocalizedTextFields
        v-model="draft.site.brand"
        :label="copy.brand"
        :locale="locale"
        test-id="site-brand"
        path="/site/brand"
        :issues="issues"
      />
      <LocalizedTextFields
        v-model="draft.site.artistName"
        :label="copy.artistName"
        :locale="locale"
        test-id="site-artist-name"
        path="/site/artistName"
        :issues="issues"
      />
      <label><span>{{ copy.canonical }}</span><input
        v-model="draft.site.canonicalUrl"
        type="url"
        required
        data-testid="site-canonical"
        :data-snapshot-path="field('/site/canonicalUrl')['data-snapshot-path']"
        :aria-invalid="field('/site/canonicalUrl')['aria-invalid']"
        :aria-describedby="field('/site/canonicalUrl')['aria-describedby']"
      ><small
        v-if="field('/site/canonicalUrl').description"
        :id="field('/site/canonicalUrl')['aria-describedby']"
      >{{ field('/site/canonicalUrl').description }}</small></label>
      <fieldset>
        <legend>{{ copy.seo }}</legend>
        <LocalizedTextFields
          v-model="draft.site.seo.title"
          :label="copy.seoTitle"
          :locale="locale"
          test-id="site-seo-title"
          path="/site/seo/title"
          :issues="issues"
        />
        <LocalizedTextFields
          v-model="draft.site.seo.description"
          :label="copy.description"
          :locale="locale"
          test-id="site-seo-description"
          path="/site/seo/description"
          :issues="issues"
        />
        <div class="asset-field">
          <label :for="`${id}-og`">{{ copy.image }}</label><AssetPickerButton
            :kinds="['image', 'gif']"
            :label="copy.image"
            @select="id => draft!.site.seo.ogAssetId = id"
          /><select
            :id="`${id}-og`"
            :value="draft.site.seo.ogAssetId || ''"
            data-testid="site-og-image"
            :data-snapshot-path="field('/site/seo/ogAssetId')['data-snapshot-path']"
            :aria-invalid="field('/site/seo/ogAssetId')['aria-invalid']"
            :aria-describedby="field('/site/seo/ogAssetId')['aria-describedby']"
            @change="setImage('ogAssetId', $event)"
          >
            <option value="">
              {{ copy.none }}
            </option><option
              v-for="asset in images"
              :key="asset.id"
              :value="asset.id"
            >
              {{ asset.alt[locale] || asset.alt['zh-CN'] }}
            </option>
          </select>
          <small
            v-if="field('/site/seo/ogAssetId').description"
            :id="field('/site/seo/ogAssetId')['aria-describedby']"
          >{{ field('/site/seo/ogAssetId').description }}</small>
        </div>
      </fieldset>
      <fieldset>
        <legend>{{ copy.artist }}</legend>
        <LocalizedTextFields
          v-model="draft.artist.name"
          :label="copy.name"
          :locale="locale"
          test-id="site-profile-name"
          path="/artist/name"
          :issues="issues"
        />
        <LocalizedTextFields
          v-model="draft.artist.shortBio"
          :label="copy.bio"
          :locale="locale"
          test-id="site-bio"
          path="/artist/shortBio"
          :issues="issues"
        />
        <div class="asset-field">
          <label :for="`${id}-portrait`">{{ copy.portrait }}</label><AssetPickerButton
            :kinds="['image', 'gif']"
            :label="copy.portrait"
            @select="id => draft!.artist.portraitAssetId = id"
          /><select
            :id="`${id}-portrait`"
            :value="draft.artist.portraitAssetId || ''"
            data-testid="site-portrait"
            :data-snapshot-path="field('/artist/portraitAssetId')['data-snapshot-path']"
            :aria-invalid="field('/artist/portraitAssetId')['aria-invalid']"
            :aria-describedby="field('/artist/portraitAssetId')['aria-describedby']"
            @change="setImage('portraitAssetId', $event)"
          >
            <option value="">
              {{ copy.none }}
            </option><option
              v-for="asset in images"
              :key="asset.id"
              :value="asset.id"
            >
              {{ asset.alt[locale] || asset.alt['zh-CN'] }}
            </option>
          </select>
          <small
            v-if="field('/artist/portraitAssetId').description"
            :id="field('/artist/portraitAssetId')['aria-describedby']"
          >{{ field('/artist/portraitAssetId').description }}</small>
        </div>
      </fieldset>
      <fieldset>
        <legend>{{ copy.siteLinks }}</legend><PlatformLinkFields
          v-model="draft.site.socialLinks"
          :locale="locale"
          path="/site/socialLinks"
          :issues="issues"
        />
      </fieldset>
      <fieldset>
        <legend>{{ copy.artistLinks }}</legend><PlatformLinkFields
          v-model="draft.artist.platformLinks"
          :locale="locale"
          path="/artist/platformLinks"
          :issues="issues"
        />
      </fieldset>
    </fieldset>
  </section>
</template>

<style scoped>
.site-fields, .site-fields > fieldset { display: grid; min-width: 0; gap: 1.1rem; border: 0; padding: 0; margin: 0; }
.site-fields > fieldset { border-top: 1px solid var(--border); padding-top: 1rem; }
legend { padding-block: .5rem; font-weight: 600; }
label { display: grid; gap: .5rem; min-width: 0; }
</style>
