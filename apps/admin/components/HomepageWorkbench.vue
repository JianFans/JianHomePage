<script setup lang="ts">
import { ArrowDown, ArrowUp, LayoutList, Plus, X } from '@lucide/vue'
import { computed, ref, shallowRef, watch } from 'vue'
import type { YujianContentSnapshot } from '@yujian/schema'
import type { AdminLocale } from '../utils/admin-locale'
import type { SnapshotEditorIssue } from '../utils/snapshot-workbench'
import { createHeroSlide, removeHomepageHero, type HomepageDraft, type HomepageErrorCode } from '../utils/homepage-workbench'
import { homepageCopy } from '../utils/homepage-copy'
import HeroSlideFields from './HeroSlideFields.vue'
import HomepageSectionFields from './HomepageSectionFields.vue'

const draft = defineModel<HomepageDraft | null>('draft', { required: true })
const props = defineProps<{
  snapshot: YujianContentSnapshot | null
  locale: AdminLocale
  busy: boolean
  dirty: boolean
  stale: boolean
  error: HomepageErrorCode | null
  issues: readonly SnapshotEditorIssue[]
  pending: boolean
}>()
const emit = defineEmits<{ open: []; apply: []; close: []; keep: []; discard: [] }>()
const copy = computed(() => homepageCopy(props.locale))
const selectedId = ref<string | null>(null)
const selectedIndex = computed(() => draft.value?.heroSlides.findIndex(hero => hero.id === selectedId.value) ?? -1)
const editSnapshot = shallowRef(props.snapshot)

watch(() => props.snapshot, /** JSON 暂时无效仍显示草稿，仅选项沿用最近有效快照；应用始终由状态层校验最新 JSON。 */ (snapshot) => {
  if (snapshot) editSnapshot.value = snapshot
})

watch(() => draft.value, /** 重载时选择首条，避免旧选择指向已移除的记录。 */ () => {
  selectedId.value = draft.value?.heroSlides[0]?.id ?? null
}, { immediate: true })

/** 新记录只进入临时草稿，用户明确加入宣传位板块后才参与首页展示。 */
function addHero() {
  if (!draft.value || props.busy) return
  const hero = createHeroSlide()
  draft.value.heroSlides.push(hero)
  selectedId.value = hero.id
}

/** 删除后优先选择同一位置的下一条，末尾删除则选择上一条。 */
function removeHero() {
  if (!draft.value || props.busy || selectedIndex.value < 0) return
  const index = selectedIndex.value
  removeHomepageHero(draft.value, selectedId.value!)
  selectedId.value = draft.value.heroSlides[Math.min(index, draft.value.heroSlides.length - 1)]?.id ?? null
}

/** 板块按数组位置移动，重复 ID 不会导致排序或身份误判。 */
function moveSection(index: number, direction: -1 | 1) {
  if (!draft.value || props.busy) return
  const next = index + direction
  if (next < 0 || next >= draft.value.sections.length) return
  ;[draft.value.sections[index], draft.value.sections[next]] = [draft.value.sections[next]!, draft.value.sections[index]!]
}
</script>

<template>
  <section
    class="panel music-panel"
    aria-labelledby="homepage-title"
    data-testid="homepage-workbench"
  >
    <header class="panel-heading">
      <div>
        <p class="eyebrow">
          <LayoutList
            :size="16"
            aria-hidden="true"
          />
        </p><h2 id="homepage-title">
          {{ copy.title }}
        </h2>
      </div>
      <button
        type="button"
        class="button"
        data-testid="homepage-open"
        :disabled="busy || !snapshot"
        @click="emit('open')"
      >
        {{ draft ? copy.reload : copy.open }}
      </button>
    </header>
    <div
      v-if="pending"
      class="music-notice"
      data-testid="homepage-discard-prompt"
      role="alert"
    >
      <p>{{ copy.discardPrompt }}</p>
      <div class="music-tools">
        <button
          type="button"
          class="button"
          data-testid="homepage-keep"
          :disabled="busy"
          @click="emit('keep')"
        >
          {{ copy.keep }}
        </button>
        <button
          type="button"
          class="button button--danger"
          data-testid="homepage-discard"
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
      data-testid="homepage-error"
      role="alert"
    >
      {{ copy.errors[error] }}
      <ul v-if="issues.length">
        <li
          v-for="(issue, index) in issues"
          :key="index"
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
    <template v-if="draft && editSnapshot">
      <div class="music-editor-heading">
        <span class="music-muted">{{ dirty ? copy.unapplied : copy.title }}</span>
        <button
          type="button"
          class="icon-tool"
          data-testid="homepage-close"
          :disabled="busy"
          :aria-label="copy.close"
          :title="copy.close"
          @click="emit('close')"
        >
          <X
            :size="18"
            aria-hidden="true"
          />
        </button>
      </div>
      <fieldset
        class="music-fields"
        :disabled="busy"
        data-testid="homepage-fields"
      >
        <legend class="music-sr-only">
          {{ copy.title }}
        </legend>
        <div class="music-track-heading">
          <h3>{{ copy.heroes }}</h3>
          <button
            type="button"
            class="icon-tool"
            data-testid="homepage-hero-add"
            :disabled="busy"
            :aria-label="copy.addHero"
            :title="copy.addHero"
            @click="addHero"
          >
            <Plus
              :size="18"
              aria-hidden="true"
            />
          </button>
        </div>
        <div class="music-layout">
          <nav
            class="music-release-list"
            :aria-label="copy.heroes"
          >
            <button
              v-for="(hero, index) in draft.heroSlides"
              :key="index"
              type="button"
              class="music-release"
              :class="{ 'music-release--active': hero.id === selectedId }"
              :aria-pressed="hero.id === selectedId"
              :disabled="busy"
              :data-testid="`homepage-hero-${hero.id}`"
              @click="selectedId = hero.id"
            >
              <span><strong>{{ hero.headline[locale] || hero.headline['zh-CN'] || hero.id }}</strong><small>{{ hero.id }}</small></span>
            </button>
          </nav>
          <div
            v-if="selectedIndex >= 0"
            class="music-editor"
          >
            <div class="music-editor-heading">
              <code data-testid="homepage-hero-id">{{ selectedId }}</code>
              <button
                type="button"
                class="icon-tool"
                data-testid="homepage-hero-remove"
                :disabled="busy"
                :aria-label="copy.removeHero"
                :title="copy.removeHero"
                @click="removeHero"
              >
                <X
                  :size="16"
                  aria-hidden="true"
                />
              </button>
            </div>
            <HeroSlideFields
              v-model="draft.heroSlides[selectedIndex]!"
              :snapshot="editSnapshot"
              :draft="draft"
              :locale="locale"
            />
          </div>
          <p
            v-else
            class="music-empty"
          >
            {{ copy.noHeroes }}
          </p>
        </div>
        <h3>{{ copy.sections }}</h3>
        <p class="music-muted">
          {{ copy.referenceTime }} · {{ editSnapshot.generatedAt }}
        </p>
        <div
          v-for="(section, index) in draft.sections"
          :key="index"
          class="music-placement"
          :data-testid="`homepage-section-${section.id}`"
        >
          <div class="music-track-heading">
            <span>{{ copy.types[section.type] }} · <code>{{ section.id }}</code></span>
            <div class="music-tools">
              <button
                type="button"
                class="icon-tool"
                data-testid="homepage-section-up"
                :disabled="busy || index === 0"
                :aria-label="`${copy.up} ${copy.types[section.type]}`"
                @click="moveSection(index, -1)"
              >
                <ArrowUp
                  :size="16"
                  aria-hidden="true"
                />
              </button>
              <button
                type="button"
                class="icon-tool"
                data-testid="homepage-section-down"
                :disabled="busy || index === draft.sections.length - 1"
                :aria-label="`${copy.down} ${copy.types[section.type]}`"
                @click="moveSection(index, 1)"
              >
                <ArrowDown
                  :size="16"
                  aria-hidden="true"
                />
              </button>
            </div>
          </div>
          <HomepageSectionFields
            v-model="draft.sections[index]!"
            :snapshot="editSnapshot"
            :draft="draft"
            :locale="locale"
          />
        </div>
      </fieldset>
      <footer class="music-apply-row">
        <button
          type="button"
          class="button button--primary"
          data-testid="homepage-apply"
          :disabled="busy || !dirty || stale"
          @click="emit('apply')"
        >
          {{ copy.apply }}
        </button>
      </footer>
    </template>
  </section>
</template>
