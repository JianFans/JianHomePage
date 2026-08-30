import { renderRobots } from '../../utils/crawler-files'

export default defineEventHandler((event) => {
  const snapshot = useRuntimeConfig(event).public.contentSnapshot
  setHeader(event, 'content-type', 'text/plain; charset=utf-8')

  return renderRobots(snapshot.site.canonicalUrl)
})
