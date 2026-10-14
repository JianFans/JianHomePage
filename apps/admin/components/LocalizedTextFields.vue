<script setup lang="ts">
import { computed, useId } from 'vue'
import type { LocalizedText } from '@yujian/schema'
import type { AdminLocale } from '../utils/admin-locale'
import { musicCopy } from '../utils/music-copy'

const model = defineModel<LocalizedText>({ required: true })
const props = defineProps<{ label: string; locale: AdminLocale; testId?: string }>()
const id = useId()
const copy = computed(() => musicCopy(props.locale))
const invalid = computed(() => !model.value['zh-CN'].trim())
const requiredMessage = computed(() => props.locale === 'en' ? `${props.label}: Chinese text is required` : `${props.label}：请填写中文内容`)

/** 清空选填英文时移除键，避免留下不符合 canonical 契约的空字符串。 */
function setEnglish(event: Event) {
  const value = (event.target as HTMLInputElement).value
  if (value) model.value.en = value
  else delete model.value.en
}
</script>

<template>
  <div class="localized-fields">
    <label :for="`${id}-zh`">
      <span>{{ label }} · {{ copy.chinese }}</span>
      <input
        :id="`${id}-zh`"
        v-model="model['zh-CN']"
        :data-testid="testId ? `${testId}-zh` : undefined"
        type="text"
        required
        :aria-invalid="invalid ? true : undefined"
        :aria-describedby="invalid ? `${id}-error` : undefined"
      >
      <span
        v-if="invalid"
        :id="`${id}-error`"
        class="sr-only"
      >{{ requiredMessage }}</span>
    </label>
    <details
      class="localized-english"
      :open="locale === 'en'"
    >
      <summary>English <span>{{ model.en ? '·' : '' }}</span></summary>
      <label :for="`${id}-en`">
        <span>{{ label }} · {{ copy.english }}</span>
        <input
          :id="`${id}-en`"
          :value="model.en || ''"
          :data-testid="testId ? `${testId}-en` : undefined"
          type="text"
          @input="setEnglish"
        >
      </label>
    </details>
  </div>
</template>

<style scoped>
.localized-fields { display: grid; gap: .6rem; min-width: 0; }
.localized-english { min-width: 0; }
.localized-english summary { min-height: 2.75rem; padding: .65rem 0; cursor: pointer; font-size: .75rem; color: var(--muted); }
.localized-english summary span { color: var(--purple, var(--accent)); }
</style>
