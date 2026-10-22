<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'

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
const loaded = ref(false)
const image = ref<HTMLImageElement | null>(null)

watch(() => props.src, (src) => {
  activeSrc.value = src
  failed.value = false
  loaded.value = false
}, { flush: 'sync' })

/** key 换源与节点身份共同隔离旧图片迟到的 load/error 事件。 */
function current(event: Event) {
  return event.target === image.value && image.value?.getAttribute('src') === activeSrc.value
}

/** 缓存命中可能早于组件挂载；保留浏览器的 picture 候选源选择。 */
onMounted(() => { if (image.value?.complete && image.value.naturalWidth > 0) loaded.value = true })

function loadedImage(event: Event) { if (current(event)) loaded.value = true }

function useFallback(event: Event) {
  if (failed.value || !current(event)) {
    return
  }
  failed.value = true
  activeSrc.value = fallbackSrc
}
</script>

<template>
  <img
    :key="src"
    ref="image"
    v-bind="$attrs"
    class="media-image"
    :src="activeSrc"
    :alt="alt"
    :width="width"
    :height="height"
    :data-fallback="failed ? 'true' : undefined"
    :data-media-state="failed ? 'error' : loaded ? 'ready' : 'loading'"
    @load="loadedImage"
    @error="useFallback"
  >
</template>

<style scoped>
.media-image { background-color: #20262f; }
@media (prefers-reduced-motion: no-preference) {
  .media-image[data-media-state="loading"] { animation: image-wait var(--motion-breathe, 1600ms) ease-in-out infinite alternate; }
  .media-image[data-media-state="ready"] { animation: image-ready var(--motion-normal, 220ms) ease-out; }
}
@keyframes image-wait { from { background-color: #151a20; } to { background-color: #29313c; } }
@keyframes image-ready { from { opacity: .7; } to { opacity: 1; } }
</style>
