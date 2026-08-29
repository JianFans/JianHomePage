<script setup lang="ts">
import type { PlatformLink } from '@yujian/schema'
import { ArrowUp } from '@lucide/vue'
import { computed } from 'vue'
import type { SupportedLocale } from '../../utils/localized'
import PlatformLinks from '../ui/PlatformLinks.vue'

const props = defineProps<{
  brand: string
  canonicalUrl: string
  socialLinks: PlatformLink[]
  locale: SupportedLocale
}>()

const hostname = computed(() => {
  try {
    return new URL(props.canonicalUrl).hostname
  } catch {
    return props.canonicalUrl
  }
})
const backLabel = computed(() => props.locale === 'en' ? 'Back to top' : '返回顶部')
</script>

<template>
  <footer class="site-footer">
    <div class="site-footer__inner">
      <a
        class="site-footer__identity"
        :href="canonicalUrl"
        :aria-label="brand"
      >
        <strong>{{ brand }}</strong>
        <small>{{ hostname }}</small>
      </a>

      <PlatformLinks
        :links="socialLinks"
        :locale="locale"
        menu-align="start"
      />

      <a
        class="site-footer__top"
        href="#top"
        :aria-label="backLabel"
        :title="backLabel"
      >
        <ArrowUp aria-hidden="true" />
      </a>
    </div>
  </footer>
</template>

<style scoped>
.site-footer {
  border-top: 1px solid var(--color-border);
  background: var(--color-bg);
}

.site-footer__inner {
  display: grid;
  min-height: 8rem;
  max-width: var(--content-max);
  grid-template-columns: 1fr auto 2.75rem;
  align-items: center;
  gap: 1rem;
  padding: 1.5rem 1.25rem;
  margin-inline: auto;
}

.site-footer__identity {
  display: inline-flex;
  width: fit-content;
  align-items: baseline;
  gap: 0.65rem;
  text-decoration: none;
}

.site-footer__identity strong,
.site-footer__identity small {
  letter-spacing: 0;
}

.site-footer__identity strong {
  font-size: 0.9rem;
  font-weight: 620;
}

.site-footer__identity small {
  color: var(--color-muted);
  font-size: 0.68rem;
}

.site-footer__top {
  display: grid;
  width: 2.75rem;
  height: 2.75rem;
  place-items: center;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-tool);
  color: var(--color-muted);
}

.site-footer__top:hover,
.site-footer__top:focus-visible {
  border-color: var(--color-accent);
  color: var(--color-text);
}

.site-footer__top svg {
  width: 1.1rem;
  height: 1.1rem;
  stroke-width: 1.6;
}

@media (max-width: 42rem) {
  .site-footer__inner {
    min-height: 11rem;
    grid-template-columns: 1fr 2.75rem;
  }

  .site-footer__inner :deep(.platform-links) {
    grid-column: 1 / -1;
    grid-row: 2;
  }

  .site-footer__top {
    grid-column: 2;
    grid-row: 1;
  }
}
</style>
