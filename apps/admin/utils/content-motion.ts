import type { ObjectDirective } from 'vue'

type MotionState = { query: MediaQueryList; animation?: Animation; changed: (event: MediaQueryListEvent) => void }
const states = new WeakMap<HTMLElement, MotionState>()

/** 挂载与记录更新共用同一动效，重入及系统偏好变化均可立即取消。 */
function play(element: HTMLElement, state: MotionState) {
  state.animation?.cancel()
  state.animation = undefined
  if (state.query.matches) return
  const duration = Number.parseFloat(getComputedStyle(element).getPropertyValue('--motion-normal')) || 220
  const animation = element.animate([{ opacity: .65 }, { opacity: 1 }], { duration, easing: 'ease-out' })
  animation.id = 'studio-record-change'
  state.animation = animation
  // 主动 cancel 会拒绝 finished；消费该终态，避免正常切换产生未处理异常。
  void animation.finished.then(
    () => { if (state.animation === animation) state.animation = undefined },
    () => { if (state.animation === animation) state.animation = undefined },
  )
}

/** 动画只装饰现有编辑区；切换记录不重建表单、移动焦点或等待完成。 */
export const vContentMotion: ObjectDirective<HTMLElement, string | null | undefined> = {
  mounted(element) {
    if (typeof element.animate !== 'function' || typeof window.matchMedia !== 'function') return
    const query = window.matchMedia('(prefers-reduced-motion: reduce)')
    const state: MotionState = { query, changed: (event) => {
      if (event.matches) { state.animation?.cancel(); state.animation = undefined }
    } }
    states.set(element, state)
    query.addEventListener('change', state.changed)
    play(element, state)
  },
  updated(element, binding) {
    const state = states.get(element)
    if (!state || binding.value === binding.oldValue) return
    play(element, state)
  },
  beforeUnmount(element) {
    const state = states.get(element)
    state?.animation?.cancel()
    if (state) state.query.removeEventListener('change', state.changed)
    states.delete(element)
  },
}
