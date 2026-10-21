<script setup lang="ts">
import { ArrowRight } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import AssetWorkbench from './AssetWorkbench.vue'
import MusicWorkbench from './MusicWorkbench.vue'
import HomepageWorkbench from './HomepageWorkbench.vue'
import VideoWorkbench from './VideoWorkbench.vue'
import EventWorkbench from './EventWorkbench.vue'
import MomentWorkbench from './MomentWorkbench.vue'
import SnapshotInsights from './SnapshotInsights.vue'
import PublishStatusPanel from './PublishStatusPanel.vue'
import type { StudioWorkspace } from '../composables/useStudioWorkspace'
import type { AdminLocale } from '../utils/admin-locale'
import { studioVersionStatus, type StudioModule } from '../utils/studio-copy'

const props = defineProps<{ studio: StudioWorkspace; locale: AdminLocale }>()
const emit = defineEmits<{ navigate: [module: StudioModule]; assetBusy: [value: boolean]; assetDirty: [value: boolean] }>()
const { workspace, session, toasts, music, homepage, video, event, moment } = props.studio
const copy = computed(() => props.studio.copy)
const assetOpened = ref(false)
watch(() => props.studio.module, (module) => { if (module === 'assets') assetOpened.value = true }, { immediate: true })
/** 模块导航复用全局未保存保护，不在展示组件中自行重建编辑上下文。 */
function navigate(module: StudioModule) { emit('navigate', module) }
</script>

<template>
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
        <div class="studio-section-heading">
          <h2>{{ copy.versions }}</h2><button
            type="button"
            class="button"
            :disabled="workspace.listing"
            @click="workspace.listVersions()"
          >
            {{ copy.refresh }}
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
        <div class="studio-history-list">
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
        <p
          v-if="!workspace.versions.length"
          class="studio-muted"
        >
          {{ copy.noVersions }}
        </p>
        <button
          v-if="workspace.nextCursor"
          type="button"
          class="button"
          :disabled="workspace.listing"
          @click="workspace.listVersions(true)"
        >
          {{ copy.more }}
        </button>
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
    @busy="value => emit('assetBusy', value)"
    @dirty="value => emit('assetDirty', value)"
    @unauthorized="session.expire"
    @notify="(message, tone) => toasts.notify(message, tone)"
  />
</template>
