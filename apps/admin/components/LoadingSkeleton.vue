<script setup lang="ts">
withDefaults(defineProps<{ kind?: 'cards' | 'rows' | 'preview'; count?: number }>(), { kind: 'rows', count: 3 })
</script>

<template>
  <div
    class="loading-skeleton"
    :class="`loading-skeleton--${kind}`"
    aria-hidden="true"
  >
    <div
      v-for="item in count"
      :key="item"
      class="loading-skeleton__item"
    >
      <span class="loading-skeleton__media" /><span class="loading-skeleton__line" /><span class="loading-skeleton__line loading-skeleton__line--short" />
    </div>
  </div>
</template>

<style scoped>
.loading-skeleton { display: grid; gap: .8rem; min-width: 0; }
.loading-skeleton__item { display: grid; gap: .75rem; padding: 1rem; border: 1px solid var(--border, #3b4553); border-radius: .6rem; background: var(--surface, #20262f); }
.loading-skeleton__line, .loading-skeleton__media { display: block; background: var(--surface-soft, #303a46); border-radius: .3rem; }
.loading-skeleton__line { height: .65rem; width: 85%; }
.loading-skeleton__line--short { width: 55%; }
.loading-skeleton__media { display: none; }
.loading-skeleton--cards { grid-template-columns: repeat(auto-fit, minmax(min(10rem, 100%), 1fr)); }
.loading-skeleton--cards .loading-skeleton__media { display: block; aspect-ratio: 4 / 3; }
.loading-skeleton--preview { height: 100%; }
.loading-skeleton--preview .loading-skeleton__item { align-content: start; padding: 8%; }
.loading-skeleton--preview .loading-skeleton__media { display: block; height: 45%; min-height: 5rem; margin-bottom: 2rem; }
@media (prefers-reduced-motion: no-preference) {
  .loading-skeleton__line, .loading-skeleton__media { animation: skeleton-breathe var(--motion-breathe, 1600ms) ease-in-out infinite alternate; }
}
@keyframes skeleton-breathe { from { opacity: .45; } to { opacity: .9; } }
</style>
