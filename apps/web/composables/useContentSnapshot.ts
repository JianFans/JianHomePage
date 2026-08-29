import type { YujianContentSnapshot } from '@yujian/schema'
import fixture from '../../../content/fixtures/homepage.json'

const fallbackSnapshot = fixture as unknown as YujianContentSnapshot

export function useContentSnapshot(): Readonly<YujianContentSnapshot> {
  const runtimeConfig = useRuntimeConfig()
  const configured = runtimeConfig.public.contentSnapshot
  if (configured && typeof configured === 'object' && !Array.isArray(configured)) {
    return configured as Readonly<YujianContentSnapshot>
  }
  return fallbackSnapshot
}
