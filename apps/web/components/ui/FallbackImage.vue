<script setup lang="ts">
import { ref, watch } from 'vue'

defineOptions({ inheritAttrs: false })

const props = defineProps<{
  src: string
  alt: string
  width?: string | number
  height?: string | number
}>()

const fallbackSvg = encodeURIComponent(`
  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1200 800">
    <rect width="1200" height="800" fill="#15191b"/>
    <path d="M0 610 360 330l210 170 250-260 380 310v250H0Z" fill="#303638"/>
    <circle cx="900" cy="210" r="74" fill="#9aadaf" opacity=".52"/>
    <path d="M0 730 1200 420v380H0Z" fill="#a48a79" opacity=".22"/>
  </svg>
`)
const fallbackSrc = `data:image/svg+xml;charset=UTF-8,${fallbackSvg}`
const activeSrc = ref(props.src)
const failed = ref(false)

watch(() => props.src, (src) => {
  activeSrc.value = src
  failed.value = false
})

function useFallback() {
  if (failed.value) {
    return
  }
  failed.value = true
  activeSrc.value = fallbackSrc
}
</script>

<template>
  <img
    v-bind="$attrs"
    :src="activeSrc"
    :alt="alt"
    :width="width"
    :height="height"
    :data-fallback="failed ? 'true' : undefined"
    @error="useFallback"
  >
</template>
