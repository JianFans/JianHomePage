<script setup lang="ts">
import { Languages, X } from '@lucide/vue'
import { useI18n } from 'vue-i18n'
import { useLocale } from '../../composables/useLocale'
import IconButton from '../ui/IconButton.vue'

const { t } = useI18n({ useScope: 'global' })
const { noticeVisible } = useLocale()

defineProps<{
  raised?: boolean
}>()
</script>

<template>
  <Transition name="locale-notice">
    <div
      v-if="noticeVisible"
      class="locale-notice"
      :class="{ 'locale-notice--raised': raised }"
      role="status"
      aria-live="polite"
    >
      <Languages aria-hidden="true" />
      <span>{{ t('locale.automatic') }}</span>
      <IconButton
        :label="t('actions.close')"
        @click="noticeVisible = false"
      >
        <X aria-hidden="true" />
      </IconButton>
    </div>
  </Transition>
</template>

<style scoped>
.locale-notice {
  position: fixed;
  z-index: 50;
  right: 1rem;
  bottom: 1rem;
  display: flex;
  max-width: min(25rem, calc(100vw - 2rem));
  min-height: 3.5rem;
  align-items: center;
  gap: 0.75rem;
  padding: 0.35rem 0.35rem 0.35rem 1rem;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-tool);
  background: var(--color-surface-raised);
  box-shadow: 0 1rem 2.5rem rgba(0, 0, 0, 0.3);
  color: var(--color-text);
  font-size: 0.8rem;
  pointer-events: none;
}

.locale-notice--raised {
  bottom: 6.5rem;
}

.locale-notice :deep(.icon-button-shell) {
  pointer-events: auto;
}

.locale-notice > svg {
  width: 1rem;
  height: 1rem;
  flex: 0 0 1rem;
  color: var(--color-accent);
}

.locale-notice-enter-active,
.locale-notice-leave-active {
  transition: opacity 160ms var(--ease-standard), transform 160ms var(--ease-standard);
}

.locale-notice-enter-from,
.locale-notice-leave-to {
  transform: translateY(0.5rem);
  opacity: 0;
}
</style>
