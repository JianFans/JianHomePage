import { afterEach, beforeEach, vi } from 'vitest'

// happy-dom 不读取图片文件；默认保持 pending，由媒体用例显式模拟成功或失败。
beforeEach(() => { vi.spyOn(HTMLImageElement.prototype, 'complete', 'get').mockReturnValue(false) })

afterEach(() => {
  window.localStorage.clear()
  vi.restoreAllMocks()
})
