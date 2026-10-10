<script setup lang="ts">
import { ArrowDown, ArrowUp, Plus, X } from '@lucide/vue'
import { computed } from 'vue'
import type { Asset, Track } from '@yujian/schema'
import type { AdminLocale } from '../utils/admin-locale'
import { musicCopy } from '../utils/music-copy'
import LocalizedTextFields from './LocalizedTextFields.vue'
import PlatformLinkFields from './PlatformLinkFields.vue'

const track = defineModel<Track>({ required: true })
const props = defineProps<{ locale: AdminLocale; audioAssets: Asset[]; index: number; count: number }>()
const emit = defineEmits<{ up: []; down: []; remove: [] }>()
const copy = computed(() => musicCopy(props.locale))

/** 移除试听时同时清理仅属于试听的时长，素材本身仍留在快照中。 */
function setPreview(event: Event) {
  const id = (event.target as HTMLSelectElement).value
  if (id) track.value.previewAssetId = id
  else {
    delete track.value.previewAssetId
    delete track.value.previewDurationSeconds
  }
}

/** 保留可选时长的缺省语义，数值范围交给共享 Schema 最终校验。 */
function setPreviewDuration(event: Event) {
  const value = (event.target as HTMLInputElement).value
  if (value === '') delete track.value.previewDurationSeconds
  else track.value.previewDurationSeconds = Number(value)
}

/** 以空必填字段创建署名草稿，不给实际创作人员填入默认身份。 */
function addCredit() {
  track.value.credits.push({ role: { 'zh-CN': '' }, name: '' })
}
</script>

<template>
  <section
    class="music-track"
    data-testid="track-editor"
    :aria-label="`${copy.tracks} ${index + 1}`"
  >
    <header class="music-track-heading">
      <span class="eyebrow">{{ String(index + 1).padStart(2, '0') }}</span>
      <div class="music-tools">
        <button
          type="button"
          class="icon-tool"
          data-testid="track-up"
          :disabled="index === 0"
          :aria-label="`${copy.up} ${copy.tracks} ${index + 1}`"
          @click="emit('up')"
        >
          <ArrowUp
            :size="16"
            aria-hidden="true"
          />
        </button>
        <button
          type="button"
          class="icon-tool"
          data-testid="track-down"
          :disabled="index === count - 1"
          :aria-label="`${copy.down} ${copy.tracks} ${index + 1}`"
          @click="emit('down')"
        >
          <ArrowDown
            :size="16"
            aria-hidden="true"
          />
        </button>
        <button
          type="button"
          class="icon-tool"
          data-testid="track-remove"
          :disabled="count === 1"
          :aria-label="`${copy.remove} ${copy.tracks} ${index + 1}`"
          @click="emit('remove')"
        >
          <X
            :size="16"
            aria-hidden="true"
          />
        </button>
      </div>
    </header>
    <LocalizedTextFields
      v-model="track.title"
      :locale="locale"
      :label="copy.trackTitle"
      test-id="track-title"
    />
    <div class="music-field-grid music-field-grid--three">
      <label><span>{{ copy.duration }}</span><input
        v-model.number="track.durationSeconds"
        type="number"
        min="1"
        step="1"
        data-testid="track-duration"
      ></label>
      <label>
        <span>{{ copy.preview }}</span>
        <select
          :value="track.previewAssetId || ''"
          data-testid="track-preview"
          @change="setPreview"
        >
          <option value="">{{ copy.noPreview }}</option>
          <option
            v-for="asset in audioAssets"
            :key="asset.id"
            :value="asset.id"
          >{{ asset.alt[locale] || asset.alt['zh-CN'] }} · {{ asset.id }}</option>
        </select>
      </label>
      <label v-if="track.previewAssetId">
        <span>{{ copy.previewDuration }}</span>
        <input
          :value="track.previewDurationSeconds ?? ''"
          type="number"
          min="1"
          max="90"
          step="1"
          data-testid="track-preview-duration"
          @input="setPreviewDuration"
        >
      </label>
    </div>
    <PlatformLinkFields
      v-model="track.platformLinks"
      :locale="locale"
    />
    <details class="music-details">
      <summary>{{ copy.credits }} <span>{{ track.credits.length }}</span></summary>
      <div
        v-for="(credit, creditIndex) in track.credits"
        :key="creditIndex"
        class="music-credit"
      >
        <LocalizedTextFields
          v-model="credit.role"
          :locale="locale"
          :label="copy.role"
        />
        <div class="music-link-row">
          <label><span>{{ copy.name }}</span><input
            v-model="credit.name"
            type="text"
            data-testid="credit-name"
          ></label>
          <button
            type="button"
            class="icon-tool"
            :aria-label="`${copy.remove} ${copy.credits} ${creditIndex + 1}`"
            @click="track.credits.splice(creditIndex, 1)"
          >
            <X
              :size="16"
              aria-hidden="true"
            />
          </button>
        </div>
      </div>
      <button
        type="button"
        class="button"
        data-testid="credit-add"
        @click="addCredit"
      >
        <Plus
          :size="15"
          aria-hidden="true"
        /> {{ copy.addCredit }}
      </button>
    </details>
  </section>
</template>
