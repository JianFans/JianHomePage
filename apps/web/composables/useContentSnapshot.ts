import type { YujianContentSnapshot } from '@yujian/schema'
import fixture from '../../../content/fixtures/homepage.json'

const snapshot = fixture as unknown as YujianContentSnapshot

export function useContentSnapshot(): Readonly<YujianContentSnapshot> {
  return snapshot
}
