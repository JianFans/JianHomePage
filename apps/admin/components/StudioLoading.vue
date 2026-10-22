<script setup lang="ts">
import ActivityIndicator from '../../web/components/ui/ActivityIndicator.vue'
import LoadingSkeleton from './LoadingSkeleton.vue'
defineProps<{ label: string }>()
</script>

<template>
  <div
    class="studio-loading-layout"
    data-testid="studio-loading"
    aria-busy="true"
  >
    <aside
      class="studio-loading-layout__rail"
      aria-hidden="true"
    >
      <span class="studio-brand-mark">遇</span><LoadingSkeleton :count="6" />
    </aside>
    <div class="studio-loading-layout__main">
      <div class="studio-loading-layout__toolbar">
        <ActivityIndicator /><span role="status">{{ label }}</span>
      </div>
      <div class="studio-loading-layout__body">
        <LoadingSkeleton :count="2" /><LoadingSkeleton
          kind="cards"
          :count="3"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.studio-loading-layout { display: grid; grid-template-columns: var(--rail-width) minmax(0, 1fr); min-height: 100dvh; }
.studio-loading-layout__rail { padding: 2rem 1rem; background: #151a20; border-right: 1px solid var(--border); }
.studio-loading-layout__rail .studio-brand-mark { margin-bottom: 2rem; }
.studio-loading-layout__main { grid-column: 2; min-width: 0; }
.studio-loading-layout__toolbar { min-height: 5rem; display: flex; align-items: center; gap: .75rem; padding: 1rem 2rem; color: var(--muted); border-bottom: 1px solid var(--border); font-size: .85rem; }
.studio-loading-layout__body { display: grid; gap: 2rem; padding: 2rem; }
@media (max-width: 1000px) { .studio-loading-layout { grid-template-columns: 0 minmax(0, 1fr); } .studio-loading-layout__rail { display: none; } }
@media (max-width: 700px) { .studio-loading-layout__body, .studio-loading-layout__toolbar { padding: 1rem; } }
</style>
