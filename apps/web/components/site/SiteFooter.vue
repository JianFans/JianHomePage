<script setup lang="ts">
import type { PlatformLink } from '@yujian/schema'
import { computed } from 'vue'
import type { SupportedLocale } from '../../utils/localized'
import PlatformLinks from '../ui/PlatformLinks.vue'

const props = defineProps<{
  brand: string
  canonicalUrl: string
  socialLinks: PlatformLink[]
  locale: SupportedLocale
  copyrightYear: number
  showSocialLinks?: boolean
}>()

const hostname = computed(() => {
  try {
    return new URL(props.canonicalUrl).hostname
  } catch {
    return props.canonicalUrl
  }
})
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
        <small>{{ hostname }} · © {{ copyrightYear }}</small>
      </a>

      <PlatformLinks
        v-if="showSocialLinks !== false"
        :links="socialLinks"
        :locale="locale"
        menu-align="start"
      />
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
  grid-template-columns: 1fr auto;
  align-items: center;
  gap: 1rem;
  padding: 1.5rem 1.25rem;
  margin-inline: auto;
}

.site-footer__identity {
  display: inline-flex;
  width: fit-content;
  min-height: 2.75rem;
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

@media (max-width: 42rem) {
  .site-footer__inner {
    grid-template-columns: 1fr;
  }

  .site-footer__inner :deep(.platform-links) {
    grid-column: 1 / -1;
    grid-row: 2;
  }

}
</style>
