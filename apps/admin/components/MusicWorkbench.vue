<script setup lang="ts">
import { ArrowDown, ArrowUp, Music2, Plus, X } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import type { YujianContentSnapshot } from '@yujian/schema'
import type { AdminLocale } from '../utils/admin-locale'
import type { SnapshotEditorIssue } from '../utils/snapshot-workbench'
import { createMusicTrack, musicAssets, type MusicDraft, type MusicErrorCode } from '../utils/music-workbench'
import { musicCopy } from '../utils/music-copy'
import LocalizedTextFields from './LocalizedTextFields.vue'
import PlatformLinkFields from './PlatformLinkFields.vue'
import MusicTrackFields from './MusicTrackFields.vue'

const draft = defineModel<MusicDraft | null>('draft', { required: true })
const props = defineProps<{
  snapshot: YujianContentSnapshot | null
  apiBaseUrl?: string
  locale: AdminLocale
  busy: boolean
  dirty: boolean
  stale: boolean
  error: MusicErrorCode | null
  issues: readonly SnapshotEditorIssue[]
  pending: boolean
}>()
const emit = defineEmits<{ select: [id: string | null]; apply: []; close: []; keep: []; discard: [] }>()
const copy = computed(() => musicCopy(props.locale))
const covers = computed(() => props.snapshot ? musicAssets(props.snapshot, 'cover') : [])
const audioAssets = computed(() => props.snapshot ? musicAssets(props.snapshot, 'preview') : [])
const coverIndex = computed(() => new Map(covers.value.map(asset => [asset.id, asset])))
const brokenCovers = ref<Record<string, boolean>>({})
const sections = computed(() => props.snapshot?.homepage.sections.filter(section => section.type === 'music') ?? [])

watch(() => props.apiBaseUrl, /** 新连接允许重新加载同 ID 封面，不沿用旧连接的失败状态。 */ () => {
  brokenCovers.value = {}
})

/** 仅为本地素材预览解析 API Origin，快照中始终保留原始稳定地址。 */
function coverSource(src: string): string {
  if (!src.startsWith('/media/') || !props.apiBaseUrl) return src
  try {
    return new URL(src, props.apiBaseUrl).href
  } catch {
    return src
  }
}

/** 新曲目只进入临时表单，作品和归属引用在应用事务中一次性写入。 */
function addTrack() {
  if (!draft.value || props.busy) return
  draft.value.tracks.push(createMusicTrack(draft.value.release.id))
}

/** 只移动相邻曲目，不改变 ID 或归属；边界操作保持不变。 */
function moveTrack(index: number, direction: -1 | 1) {
  if (!draft.value || props.busy) return
  const tracks = draft.value.tracks
  const next = index + direction
  if (index < 0 || next < 0 || index >= tracks.length || next >= tracks.length) return
  ;[tracks[index], tracks[next]] = [tracks[next]!, tracks[index]!]
}

/** 明确维护选中作品的音乐板块成员关系，不改变板块启用状态和限制。 */
function toggleSection(section: MusicDraft['sections'][number], event: Event) {
  if (!draft.value || props.busy) return
  const id = draft.value.release.id
  if ((event.target as HTMLInputElement).checked) {
    if (!section.itemIds.includes(id)) section.itemIds.push(id)
  } else section.itemIds = section.itemIds.filter(item => item !== id)
}

/** 在已有板块中移动当前作品，展示限制和其他条目均保持原值。 */
function moveInSection(section: MusicDraft['sections'][number], direction: -1 | 1) {
  if (!draft.value || props.busy) return
  const index = section.itemIds.indexOf(draft.value.release.id)
  const next = index + direction
  if (index < 0 || next < 0 || next >= section.itemIds.length) return
  ;[section.itemIds[index], section.itemIds[next]] = [section.itemIds[next]!, section.itemIds[index]!]
}

/** 按当前板块启用状态和实际条目位置显示可见性，不假设加入后立即可见。 */
function visible(section: MusicDraft['sections'][number]): boolean {
  const original = sections.value.find(item => item.id === section.id)
  const index = section.itemIds.indexOf(draft.value?.release.id ?? '')
  return Boolean(original?.enabled && index >= 0 && index < original.limit)
}
</script>

<template>
  <section
    class="panel music-panel"
    aria-labelledby="music-title"
    data-testid="music-workbench"
  >
    <header class="panel-heading">
      <div>
        <p class="eyebrow">
          ♫
        </p><h2 id="music-title">
          {{ copy.title }}
        </h2>
      </div>
      <button
        type="button"
        class="icon-tool"
        data-testid="music-new"
        :aria-label="copy.newRelease"
        :title="copy.newRelease"
        :disabled="busy || !snapshot"
        @click="emit('select', null)"
      >
        <Plus
          :size="18"
          aria-hidden="true"
        />
      </button>
    </header>
    <div
      v-if="pending"
      class="music-notice"
      data-testid="music-discard-prompt"
      role="alert"
    >
      <p>{{ copy.discardPrompt }}</p>
      <div class="music-tools">
        <button
          type="button"
          class="button"
          data-testid="music-keep"
          :disabled="busy"
          @click="emit('keep')"
        >
          {{ copy.keep }}
        </button>
        <button
          type="button"
          class="button button--danger"
          data-testid="music-discard"
          :disabled="busy"
          @click="emit('discard')"
        >
          {{ copy.discard }}
        </button>
      </div>
    </div>
    <p
      v-if="stale"
      class="music-notice"
      role="status"
    >
      {{ copy.stale }}
    </p>
    <div
      v-if="error"
      class="music-error"
      role="alert"
      data-testid="music-error"
    >
      {{ copy.errors[error] }}
      <ul v-if="issues.length">
        <li
          v-for="(issue, index) in issues.slice(0, 8)"
          :key="`${issue.path}-${index}`"
        >
          <code>{{ issue.path }}</code> · {{ issue.code }}
        </li>
      </ul>
    </div>
    <p
      v-if="!snapshot && !draft"
      class="music-empty"
    >
      {{ copy.empty }}
    </p>
    <div
      v-else
      class="music-layout"
    >
      <nav
        class="music-release-list"
        :aria-label="copy.title"
      >
        <button
          v-for="release in snapshot?.releases || []"
          :key="release.id"
          type="button"
          class="music-release"
          :class="{ 'music-release--active': draft?.originalId === release.id }"
          :aria-pressed="draft?.originalId === release.id"
          :disabled="busy"
          :data-testid="`music-release-${release.id}`"
          @click="emit('select', release.id)"
        >
          <img
            v-if="coverIndex.get(release.coverAssetId) && !brokenCovers[release.coverAssetId]"
            :src="coverSource(coverIndex.get(release.coverAssetId)!.src)"
            alt=""
            width="56"
            height="56"
            loading="lazy"
            @error="brokenCovers[release.coverAssetId] = true"
          >
          <Music2
            v-else
            :size="24"
            aria-hidden="true"
          />
          <span><strong>{{ release.title[locale] || release.title['zh-CN'] }}</strong><small>{{ release.releaseDate }} · {{ release.trackIds.length }}</small></span>
        </button>
      </nav>
      <div
        v-if="draft"
        class="music-editor"
      >
        <div class="music-editor-heading">
          <code>{{ draft.release.id }}</code>
          <span
            v-if="dirty"
            class="music-muted"
            role="status"
          >{{ copy.unapplied }}</span>
          <button
            type="button"
            class="icon-tool"
            data-testid="music-close"
            :aria-label="copy.close"
            :disabled="busy"
            @click="emit('close')"
          >
            <X
              :size="17"
              aria-hidden="true"
            />
          </button>
        </div>
        <fieldset
          :disabled="busy"
          class="music-fields"
          data-testid="music-fields"
        >
          <legend class="music-sr-only">
            {{ copy.releaseTitle }}
          </legend>
          <LocalizedTextFields
            v-model="draft.release.title"
            :locale="locale"
            :label="copy.releaseTitle"
            test-id="release-title"
          />
          <div class="music-field-grid music-field-grid--three">
            <label><span>{{ copy.kind }}</span><select
              v-model="draft.release.kind"
              data-testid="music-kind"
            ><option value="single">{{ copy.single }}</option><option value="ep">{{ copy.ep }}</option><option value="album">{{ copy.album }}</option></select></label>
            <label><span>{{ copy.date }}</span><input
              v-model="draft.release.releaseDate"
              type="date"
              data-testid="music-date"
            ></label>
            <label class="music-checkbox"><input
              v-model="draft.release.featured"
              type="checkbox"
              data-testid="music-featured"
            ><span>{{ copy.featured }}</span></label>
          </div>
          <label>
            <span>{{ copy.cover }}</span>
            <select
              v-model="draft.release.coverAssetId"
              data-testid="music-cover"
            >
              <option value="">{{ copy.choose }}</option>
              <option
                v-for="asset in covers"
                :key="asset.id"
                :value="asset.id"
              >{{ asset.alt[locale] || asset.alt['zh-CN'] }} · {{ asset.id }}</option>
            </select>
          </label>
          <PlatformLinkFields
            v-model="draft.release.platformLinks"
            :locale="locale"
          />
          <h3>{{ copy.tracks }}</h3>
          <MusicTrackFields
            v-for="(track, index) in draft.tracks"
            :key="track.id"
            v-model="draft.tracks[index]!"
            :locale="locale"
            :audio-assets="audioAssets"
            :index="index"
            :count="draft.tracks.length"
            @up="moveTrack(index, -1)"
            @down="moveTrack(index, 1)"
            @remove="draft.tracks.splice(index, 1)"
          />
          <button
            type="button"
            class="button"
            data-testid="track-add"
            @click="addTrack"
          >
            <Plus
              :size="15"
              aria-hidden="true"
            /> {{ copy.addTrack }}
          </button>
          <h3>{{ copy.sections }}</h3>
          <p
            v-if="!draft.sections.length"
            class="music-muted"
          >
            {{ copy.noSection }}
          </p>
          <div
            v-for="section in draft.sections"
            :key="section.id"
            class="music-placement"
            :data-testid="`section-${section.id}`"
          >
            <label class="music-checkbox"><input
              type="checkbox"
              :checked="section.itemIds.includes(draft.release.id)"
              data-testid="section-include"
              @change="toggleSection(section, $event)"
            ><span>{{ copy.include }} · {{ section.id }}</span></label>
            <template v-if="section.itemIds.includes(draft.release.id)">
              <span class="music-muted">{{ section.itemIds.indexOf(draft.release.id) + 1 }} / {{ section.itemIds.length }} · {{ visible(section) ? copy.visible : copy.hidden }}</span>
              <div class="music-tools">
                <button
                  type="button"
                  class="icon-tool"
                  data-testid="section-up"
                  :aria-label="`${copy.up} ${section.id}`"
                  :disabled="section.itemIds.indexOf(draft.release.id) === 0"
                  @click="moveInSection(section, -1)"
                >
                  <ArrowUp
                    :size="16"
                    aria-hidden="true"
                  />
                </button>
                <button
                  type="button"
                  class="icon-tool"
                  data-testid="section-down"
                  :aria-label="`${copy.down} ${section.id}`"
                  :disabled="section.itemIds.indexOf(draft.release.id) === section.itemIds.length - 1"
                  @click="moveInSection(section, 1)"
                >
                  <ArrowDown
                    :size="16"
                    aria-hidden="true"
                  />
                </button>
              </div>
            </template>
          </div>
        </fieldset>
        <footer class="music-apply-row">
          <button
            type="button"
            class="button button--primary"
            data-testid="music-apply"
            :disabled="busy || !dirty || stale"
            @click="emit('apply')"
          >
            {{ copy.apply }}
          </button>
        </footer>
      </div>
      <p
        v-else
        class="music-empty"
      >
        {{ copy.selectRelease }}
      </p>
    </div>
  </section>
</template>
