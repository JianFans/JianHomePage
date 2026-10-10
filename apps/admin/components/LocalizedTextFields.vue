<script setup lang="ts">
import { computed, useId } from 'vue'
import type { LocalizedText } from '@yujian/schema'
import type { AdminLocale } from '../utils/admin-locale'
import { musicCopy } from '../utils/music-copy'

const model = defineModel<LocalizedText>({ required: true })
const props = defineProps<{ label: string; locale: AdminLocale; testId?: string }>()
const id = useId()
const copy = computed(() => musicCopy(props.locale))

/** 清空选填英文时移除键，避免留下不符合 canonical 契约的空字符串。 */
function setEnglish(event: Event) {
  const value = (event.target as HTMLInputElement).value
  if (value) model.value.en = value
  else delete model.value.en
}
</script>

<template>
  <div class="music-field-grid">
    <label :for="`${id}-zh`">
      <span>{{ label }} · {{ copy.chinese }}</span>
      <input
        :id="`${id}-zh`"
        v-model="model['zh-CN']"
        :data-testid="testId ? `${testId}-zh` : undefined"
        type="text"
      >
    </label>
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
  </div>
</template>
