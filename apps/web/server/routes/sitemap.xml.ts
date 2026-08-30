import { renderSitemap } from '../../utils/crawler-files'

export default defineEventHandler((event) => {
  const snapshot = useRuntimeConfig(event).public.contentSnapshot
  setHeader(event, 'content-type', 'application/xml; charset=utf-8')

  return renderSitemap(snapshot.site.canonicalUrl)
})
