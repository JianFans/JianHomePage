<script setup lang="ts">
import type { PlatformLink } from '@yujian/schema'
import {
  Clapperboard,
  Disc3,
  Ellipsis,
  ExternalLink,
  Music2,
  Radio,
  Video,
} from '@lucide/vue'
import { computed } from 'vue'
import type { SupportedLocale } from '../../utils/localized'

const props = defineProps<{
  links: PlatformLink[]
  locale: SupportedLocale
  menuAlign?: 'start' | 'end'
}>()

const providerMeta = {
  'qq-music': {
    icon: Music2,
    label: { 'zh-CN': 'QQ 音乐', en: 'QQ Music' },
    mark: 'Q',
  },
  'netease-music': {
    icon: Disc3,
    label: { 'zh-CN': '网易云音乐', en: 'NetEase Cloud Music' },
    mark: '163',
  },
  'weibo': {
    icon: Radio,
    label: { 'zh-CN': '微博', en: 'Weibo' },
    mark: 'W',
  },
  'bilibili': {
    icon: Video,
    label: { 'zh-CN': '哔哩哔哩', en: 'Bilibili' },
    mark: 'B',
  },
  'douyin': {
    icon: Clapperboard,
    label: { 'zh-CN': '抖音', en: 'Douyin' },
    mark: 'D',
  },
  'website': {
    icon: ExternalLink,
    label: { 'zh-CN': '官方网站', en: 'Official website' },
    mark: '',
  },
  'other': {
    icon: ExternalLink,
    label: { 'zh-CN': '外部平台', en: 'External platform' },
    mark: '',
  },
} as const

const safeLinks = computed(() => props.links.filter((link) => {
  try {
    return new URL(link.url).protocol === 'https:'
  } catch {
    return false
  }
}))
const labels = computed(() => props.locale === 'en'
  ? { more: 'More platforms' }
  : { more: '更多平台' })

function metaFor(link: PlatformLink) {
  const provider = link.provider as keyof typeof providerMeta
  return providerMeta[provider] ?? providerMeta.other
}

function labelFor(link: PlatformLink) {
  const custom = link.label?.[props.locale] || link.label?.['zh-CN']
  return custom || metaFor(link).label[props.locale]
}
</script>

<template>
  <div
    v-if="safeLinks.length"
    class="platform-links"
    data-testid="platform-links"
  >
    <div class="platform-links__desktop">
      <span
        v-for="link in safeLinks"
        :key="`${link.provider}:${link.url}`"
        class="platform-link-shell"
      >
        <a
          class="platform-link"
          :href="link.url"
          target="_blank"
          rel="noopener noreferrer"
          :aria-label="labelFor(link)"
        >
          <component
            :is="metaFor(link).icon"
            aria-hidden="true"
          />
          <small v-if="metaFor(link).mark">{{ metaFor(link).mark }}</small>
        </a>
        <span role="tooltip">{{ labelFor(link) }}</span>
      </span>
    </div>

    <div class="platform-links__mobile">
      <span class="platform-link-shell">
        <a
          class="platform-link"
          data-testid="platform-primary"
          :href="safeLinks[0]!.url"
          target="_blank"
          rel="noopener noreferrer"
          :aria-label="labelFor(safeLinks[0]!)"
        >
          <component
            :is="metaFor(safeLinks[0]!).icon"
            aria-hidden="true"
          />
          <small v-if="metaFor(safeLinks[0]!).mark">{{ metaFor(safeLinks[0]!).mark }}</small>
        </a>
        <span role="tooltip">{{ labelFor(safeLinks[0]!) }}</span>
      </span>

      <details
        v-if="safeLinks.length > 1"
        class="platform-more"
        :class="{ 'platform-more--start': menuAlign === 'start' }"
        data-testid="platform-more"
      >
        <summary :aria-label="labels.more">
          <Ellipsis aria-hidden="true" />
        </summary>
        <div class="platform-more__menu">
          <a
            v-for="link in safeLinks.slice(1)"
            :key="`${link.provider}:${link.url}`"
            :href="link.url"
            target="_blank"
            rel="noopener noreferrer"
            :aria-label="labelFor(link)"
          >
            <component
              :is="metaFor(link).icon"
              aria-hidden="true"
            />
            <span>{{ labelFor(link) }}</span>
          </a>
        </div>
      </details>
    </div>
  </div>
</template>

<style scoped>
.platform-links,
.platform-links__desktop,
.platform-links__mobile {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.platform-links__mobile {
  display: none;
}

.platform-link-shell {
  position: relative;
  display: inline-grid;
  width: 2.75rem;
  height: 2.75rem;
}

.platform-link {
  position: relative;
  display: grid;
  width: 2.75rem;
  height: 2.75rem;
  place-items: center;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-tool);
  background: var(--color-bg);
  color: var(--color-muted);
}

.platform-link:hover,
.platform-link:focus-visible {
  border-color: var(--color-accent);
  color: var(--color-text);
}

.platform-link svg,
.platform-more summary svg,
.platform-more__menu svg {
  width: 1.1rem;
  height: 1.1rem;
  stroke-width: 1.7;
}

.platform-link small {
  position: absolute;
  right: 0.2rem;
  bottom: 0.08rem;
  color: var(--color-muted);
  font-size: 0.5rem;
  letter-spacing: 0;
}

.platform-link-shell > [role="tooltip"] {
  position: absolute;
  z-index: 20;
  right: 0;
  bottom: calc(100% + 0.45rem);
  width: max-content;
  max-width: 11rem;
  padding: 0.3rem 0.45rem;
  border: 1px solid var(--color-border);
  border-radius: 3px;
  background: var(--color-surface-raised);
  color: var(--color-text);
  font-size: 0.7rem;
  opacity: 0;
  pointer-events: none;
}

.platform-link:hover + [role="tooltip"],
.platform-link:focus-visible + [role="tooltip"] {
  opacity: 1;
}

.platform-more {
  position: relative;
}

.platform-more summary {
  display: grid;
  width: 2.75rem;
  height: 2.75rem;
  place-items: center;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-tool);
  background: var(--color-bg);
  color: var(--color-muted);
  cursor: pointer;
  list-style: none;
}

.platform-more summary::-webkit-details-marker {
  display: none;
}

.platform-more__menu {
  position: absolute;
  z-index: 18;
  right: 0;
  bottom: calc(100% + 0.45rem);
  display: grid;
  min-width: 11rem;
  padding: 0.35rem;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-tool);
  background: var(--color-surface-raised);
  box-shadow: 0 1rem 2.5rem rgba(0, 0, 0, 0.32);
}

.platform-more--start .platform-more__menu {
  right: auto;
  left: 0;
}

.platform-more__menu a {
  display: grid;
  min-height: 2.75rem;
  grid-template-columns: 1.5rem 1fr;
  align-items: center;
  gap: 0.55rem;
  padding-inline: 0.65rem;
  color: var(--color-text);
  font-size: 0.75rem;
  text-decoration: none;
}

@media (max-width: 42rem) {
  .platform-links__desktop {
    display: none;
  }

  .platform-links__mobile {
    display: flex;
  }
}
</style>
