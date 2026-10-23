<script setup lang="ts">
import { ArrowRight } from '@lucide/vue'
import { computed, ref, shallowRef, provide, onBeforeUnmount, watch } from 'vue'
import type { Asset } from '@yujian/schema'
import AssetWorkbench from './AssetWorkbench.vue'
import MusicWorkbench from './MusicWorkbench.vue'
import HomepageWorkbench from './HomepageWorkbench.vue'
import VideoWorkbench from './VideoWorkbench.vue'
import EventWorkbench from './EventWorkbench.vue'
import MomentWorkbench from './MomentWorkbench.vue'
import SiteWorkbench from './SiteWorkbench.vue'
import AssetPicker from './AssetPicker.vue'
import ContentDeletionPanel from './ContentDeletionPanel.vue'
import VersionComparison from './VersionComparison.vue'
import { assetPickerKey, type AssetPickerRequest } from '../utils/asset-selection'
import SnapshotInsights from './SnapshotInsights.vue'
import PublishStatusPanel from './PublishStatusPanel.vue'
import AsyncButton from './AsyncButton.vue'
import LoadingSkeleton from './LoadingSkeleton.vue'
import type { StudioWorkspace } from '../composables/useStudioWorkspace'
import type { AdminLocale } from '../utils/admin-locale'
import { studioVersionStatus, type StudioModule } from '../utils/studio-copy'

const props = defineProps<{ studio: StudioWorkspace; locale: AdminLocale }>()
const emit = defineEmits<{ navigate: [module: StudioModule]; assetBusy: [value: boolean]; assetDirty: [value: boolean] }>()
const { workspace, session, toasts, music, homepage, video, event, moment, site } = props.studio
const copy = computed(() => props.studio.copy)
const assetOpened = ref(false)
const libraryBusy = ref(false)
const libraryDirty = ref(false)
const pickerBusy = ref(false)
const pickerDirty = ref(false)
// 两个上传工作区各自保留状态，关闭选择器不能清掉主素材库的保护。
watch(() => libraryBusy.value || pickerBusy.value, value => emit('assetBusy', value), { immediate: true, flush: 'sync' })
watch(() => libraryDirty.value || pickerDirty.value, value => emit('assetDirty', value), { immediate: true, flush: 'sync' })
const historyAction = ref('')
const pickerRequest = shallowRef<(AssetPickerRequest & { before: string; module: StudioModule; finish: (asset: Asset | null) => void }) | null>(null)
const deleteMusicKind = ref<'release' | 'track'>('release')
const deletionKind = computed(() => ({ homepage: 'hero', music: deleteMusicKind.value, video: 'video', event: 'event', moment: 'moment' } as const)[props.studio.module as 'homepage' | 'music' | 'video' | 'event' | 'moment'])
/** 所有字段共享一个选择器，关闭和上下文变化立即取消待选择。 */
function closePicker() { const current = pickerRequest.value; pickerRequest.value = null; pickerBusy.value = false; pickerDirty.value = false; current?.finish(null) }
provide(assetPickerKey, request => {
  closePicker()
  if (props.studio.locked) return Promise.resolve(null)
  return new Promise(resolve => { pickerRequest.value = { ...request, before: workspace.editorText, module: props.studio.module, finish: resolve } })
})
/** 只在原文和模块均未变化时接受结果，不能让迟到回调绑定其他表单。 */
function chooseAsset(asset: Asset) {
  const request = pickerRequest.value
  if (!request || props.studio.locked || request.before !== workspace.editorText || request.module !== props.studio.module) { closePicker(); return }
  pickerRequest.value = null
  pickerBusy.value = false
  pickerDirty.value = false
  if (props.studio.acceptPickedAsset(asset, request.kinds)) request.finish(asset)
  else request.finish(null)
}
watch(() => [props.studio.module, workspace.editorText], () => { if (pickerRequest.value) closePicker() }, { flush: 'sync' })
onBeforeUnmount(closePicker)
watch(() => props.studio.module, (module) => { if (module === 'assets') assetOpened.value = true }, { immediate: true })
/** 模块导航复用全局未保存保护，不在展示组件中自行重建编辑上下文。 */
function navigate(module: StudioModule) { emit('navigate', module) }
/** 区分刷新与追加的发起按钮，不把整个列表显示成新的空状态。 */
async function listHistory(append = false) {
  if (workspace.listing) return
  historyAction.value = append ? 'more' : 'refresh'
  try { await workspace.listVersions(append) } finally { historyAction.value = '' }
}
</script>

<template>
  <ContentDeletionPanel
    v-if="deletionKind"
    :key="studio.module"
    :text="workspace.editorText"
    :analyzed-text="workspace.analyzedText"
    :snapshot="workspace.editorAnalysis.snapshot"
    :kind="deletionKind"
    :locale="locale"
    :locked="studio.locked || studio.formDirty"
    @update:text="studio.applyContentText"
    @notify="(message, tone) => toasts.notify(message, tone)"
  />
  <label
    v-if="studio.module === 'music'"
    class="studio-muted"
  >{{ locale === 'en' ? 'Delete from' : '删除对象' }}<select
    v-model="deleteMusicKind"
    :disabled="studio.locked || studio.formDirty"
  ><option value="release">{{ locale === 'en' ? 'Releases' : '作品' }}</option><option value="track">{{ locale === 'en' ? 'Tracks' : '曲目' }}</option></select></label>
  <Transition
    name="studio-page"
    mode="out-in"
  >
    <div
      :key="studio.module"
      class="studio-module"
      :data-module="studio.module"
    >
      <section
        v-if="studio.module === 'overview'"
        class="studio-overview"
      >
        <p class="studio-muted studio-overview-hint">
          {{ workspace.version ? copy.overviewHint : copy.firstVersion }}
        </p>
        <SnapshotInsights
          v-if="workspace.editorAnalysis.snapshot"
          :analysis="workspace.editorAnalysis"
          :locale="locale"
        />
        <div class="studio-quick-links">
          <button
            v-for="name in (['homepage', 'music', 'video', 'event', 'moment', 'assets'] as const)"
            :key="name"
            type="button"
            @click="navigate(name)"
          >
            <span>{{ copy.modules[name] }}</span><ArrowRight
              :size="17"
              aria-hidden="true"
            />
          </button>
        </div>
        <PublishStatusPanel
          v-if="studio.canPublish"
          :job="workspace.publishJob"
          :locale="locale"
          :busy="workspace.busy"
          :unsaved="studio.unsaved"
          :can-recover="Boolean(session.principal?.roles.includes('admin'))"
          :refreshing="workspace.publishRefreshing"
          :read-error="workspace.publishReadError"
          @refresh="workspace.publishJob ? workspace.refreshPublishStatus() : workspace.restorePublish()"
          @retry="workspace.retryPublish"
          @recover="workspace.recoverPublish"
        />
        <div class="studio-section-heading">
          <h2>{{ copy.latest }}</h2><button
            type="button"
            class="button"
            @click="navigate('history')"
          >
            {{ copy.versions }}<ArrowRight
              :size="16"
              aria-hidden="true"
            />
          </button>
        </div>
        <div class="studio-history-list">
          <button
            v-for="item in workspace.versions.slice(0, 4)"
            :key="item.id"
            type="button"
            :disabled="workspace.busy"
            @click="studio.request({ kind: 'version', id: item.id })"
          >
            <span
              class="studio-status-dot"
              :data-status="studioVersionStatus(item)"
              aria-hidden="true"
            /><span>{{ item.name || item.id }}</span><small>{{ copy.statuses[studioVersionStatus(item)] }}</small><ArrowRight
              :size="16"
              aria-hidden="true"
            />
          </button>
        </div>
      </section>
      <SiteWorkbench
        v-else-if="studio.module === 'site'"
        v-model:draft="site.draft"
        :snapshot="workspace.editorAnalysis.snapshot"
        :locale="locale"
        :locked="studio.locked"
        :stale="site.stale"
        :issues="site.issues"
      />
      <HomepageWorkbench
        v-else-if="studio.module === 'homepage'"
        v-model:draft="homepage.draft"
        studio
        :snapshot="workspace.editorAnalysis.snapshot"
        :locale="locale"
        :busy="workspace.busy"
        :read-only="!workspace.editable || !studio.canEdit"
        :dirty="homepage.dirty"
        :stale="homepage.stale"
        :error="homepage.error"
        :issues="homepage.issues"
        :pending="Boolean(homepage.pending)"
        @open="homepage.open"
        @apply="!studio.locked && homepage.apply()"
        @close="homepage.close"
        @keep="homepage.keepEditing"
        @discard="homepage.discardPending"
      />
      <MusicWorkbench
        v-else-if="studio.module === 'music'"
        v-model:draft="music.draft"
        studio
        :snapshot="workspace.editorAnalysis.snapshot"
        :api-base-url="workspace.apiBaseUrl"
        :locale="locale"
        :busy="workspace.busy"
        :read-only="!workspace.editable || !studio.canEdit"
        :dirty="music.dirty"
        :stale="music.stale"
        :error="music.error"
        :issues="music.issues"
        :pending="Boolean(music.pending)"
        @select="id => studio.request({ kind: 'release', id })"
        @apply="!studio.locked && music.apply()"
        @close="studio.request({ kind: 'close-release' })"
        @keep="music.keepEditing"
        @discard="music.discardPending"
      />
      <VideoWorkbench
        v-else-if="studio.module === 'video'"
        v-model:draft="video.draft"
        studio
        :snapshot="workspace.editorAnalysis.snapshot"
        :locale="locale"
        :busy="workspace.busy"
        :read-only="!workspace.editable || !studio.canEdit"
        :dirty="video.dirty"
        :stale="video.stale"
        :error="video.error"
        :issues="video.issues"
        :pending="Boolean(video.pending)"
        @open="video.open"
        @apply="!studio.locked && video.apply()"
        @close="video.close"
        @keep="video.keepEditing"
        @discard="video.discardPending"
      />
      <EventWorkbench
        v-else-if="studio.module === 'event'"
        v-model:draft="event.draft"
        studio
        :snapshot="workspace.editorAnalysis.snapshot"
        :locale="locale"
        :busy="workspace.busy"
        :read-only="!workspace.editable || !studio.canEdit"
        :dirty="event.dirty"
        :stale="event.stale"
        :error="event.error"
        :issues="event.issues"
        :pending="Boolean(event.pending)"
        @open="event.open"
        @apply="!studio.locked && event.apply()"
        @close="event.close"
        @keep="event.keepEditing"
        @discard="event.discardPending"
      />
      <MomentWorkbench
        v-else-if="studio.module === 'moment'"
        v-model:draft="moment.draft"
        studio
        :snapshot="workspace.editorAnalysis.snapshot"
        :locale="locale"
        :busy="workspace.busy"
        :read-only="!workspace.editable || !studio.canEdit"
        :dirty="moment.dirty"
        :stale="moment.stale"
        :error="moment.error"
        :issues="moment.issues"
        :pending="Boolean(moment.pending)"
        @open="moment.open"
        @apply="!studio.locked && moment.apply()"
        @close="moment.close"
        @keep="moment.keepEditing"
        @discard="moment.discardPending"
      />
      <section
        v-else-if="studio.module === 'history'"
        class="studio-history"
      >
        <VersionComparison
          :versions="workspace.versions"
          :current-version-id="workspace.version?.id"
          :api-base-url="workspace.apiBaseUrl"
          :csrf-token="session.csrfToken"
          :locale="locale"
          :active="session.authenticated"
          :next-cursor="workspace.nextCursor"
          :listing="workspace.listing"
          @unauthorized="session.expire"
          @more="listHistory(true)"
        />
        <div class="studio-section-heading">
          <h2>{{ copy.versions }}</h2><AsyncButton
            type="button"
            class="button"
            :disabled="workspace.listing"
            :loading="workspace.listing && historyAction === 'refresh'"
            @click="listHistory()"
          >
            {{ copy.refresh }}
          </AsyncButton>
        </div>
        <PublishStatusPanel
          v-if="studio.canPublish"
          :job="workspace.publishJob"
          :locale="locale"
          :busy="workspace.busy"
          :unsaved="studio.unsaved"
          :can-recover="Boolean(session.principal?.roles.includes('admin'))"
          :refreshing="workspace.publishRefreshing"
          :read-error="workspace.publishReadError"
          @refresh="workspace.publishJob ? workspace.refreshPublishStatus() : workspace.restorePublish()"
          @retry="workspace.retryPublish"
          @recover="workspace.recoverPublish"
        />
        <div
          class="studio-history-list"
          :aria-busy="workspace.listing"
        >
          <button
            v-for="item in workspace.versions"
            :key="item.id"
            type="button"
            :aria-pressed="workspace.version?.id === item.id"
            :disabled="workspace.busy"
            @click="studio.request({ kind: 'version', id: item.id })"
          >
            <span
              class="studio-status-dot"
              :data-status="studioVersionStatus(item)"
              aria-hidden="true"
            /><span>{{ item.name || item.id }}<small v-if="item.sourceVersionId">{{ copy.source }} {{ item.sourceVersionId }}</small></span><small>{{ copy.statuses[studioVersionStatus(item)] }} · {{ copy.revision }} {{ item.revision }}</small><ArrowRight
              :size="16"
              aria-hidden="true"
            />
          </button>
        </div>
        <div
          v-if="workspace.listing && !workspace.versions.length"
          role="status"
          :aria-label="copy.loadingList"
        >
          <LoadingSkeleton />
        </div>
        <p
          v-else-if="!workspace.versions.length"
          class="studio-muted"
        >
          {{ copy.noVersions }}
        </p>
        <AsyncButton
          v-if="workspace.nextCursor"
          type="button"
          class="button"
          :disabled="workspace.listing"
          :loading="workspace.listing && historyAction === 'more'"
          @click="listHistory(true)"
        >
          {{ copy.more }}
        </AsyncButton>
      </section>
    </div>
  </Transition>
  <AssetWorkbench
    v-if="assetOpened"
    v-show="studio.module === 'assets'"
    v-model:editor-text="workspace.editorText"
    :locale="locale"
    :api-base-url="workspace.apiBaseUrl"
    :token="workspace.token"
    :csrf-token="session.csrfToken"
    :authenticated="session.authenticated"
    :can-upload="studio.canEdit && !session.busy"
    studio
    :snapshot-locked="studio.locked"
    @busy="value => libraryBusy = value"
    @dirty="value => libraryDirty = value"
    @unauthorized="session.expire"
    @notify="(message, tone) => toasts.notify(message, tone)"
  />
  <AssetPicker
    v-if="pickerRequest && workspace.editorAnalysis.snapshot"
    :open="session.authenticated && !studio.initializing && !session.busy"
    :snapshot="workspace.editorAnalysis.snapshot"
    :kinds="pickerRequest.kinds"
    :label="pickerRequest.label"
    :locale="locale"
    :api-base-url="workspace.apiBaseUrl"
    :csrf-token="session.csrfToken"
    :locked="studio.locked"
    @select="chooseAsset"
    @close="closePicker"
    @busy="value => pickerBusy = value"
    @dirty="value => pickerDirty = value"
    @unauthorized="session.expire"
    @notify="(message, tone) => toasts.notify(message, tone)"
  />
</template>
