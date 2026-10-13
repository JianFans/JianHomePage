import type { AdminLocale } from './admin-locale'

/** 影像表单使用会话语言，不改变内容或路由语言。 */
export function videoCopy(locale: AdminLocale) {
  return locale === 'en'
    ? {
        title: 'Videos', open: 'Open video editor', reload: 'Reload videos', add: 'Add video', close: 'Close video editor',
        videoTitle: 'Title', poster: 'Poster', asset: 'Video asset (optional)', duration: 'Duration (seconds)', featured: 'Featured',
        none: 'None', select: 'Select an asset', apply: 'Apply to snapshot', keep: 'Keep editing', discard: 'Discard and continue',
        discardPrompt: 'Discard unapplied video changes?', empty: 'Load or import a valid snapshot to edit videos.',
        placement: 'Apply a new video first, then add its reference in the homepage editor.',
        stale: 'Snapshot changed. Reload videos before applying.',
        errors: { 'stale-snapshot': 'Snapshot changed. Reload videos before applying.', 'invalid-snapshot': 'The current snapshot is invalid.', 'immutable-video': 'Existing video IDs must be preserved.', 'invalid-draft': 'Check fields and references before applying.' },
      }
    : {
        title: '影像', open: '打开影像表单', reload: '重载影像', add: '新增影像', close: '关闭影像表单',
        videoTitle: '标题', poster: '海报', asset: '视频素材（可选）', duration: '时长（秒）', featured: '精选',
        none: '无', select: '选择素材', apply: '应用到快照', keep: '继续编辑', discard: '放弃并继续',
        discardPrompt: '放弃未应用的影像修改？', empty: '载入或导入有效快照后编辑影像。',
        placement: '新增影像先应用，再去首页编排添加引用。', stale: '快照已变化，请重载影像后再应用。',
        errors: { 'stale-snapshot': '快照已变化，请重载影像后再应用。', 'invalid-snapshot': '当前快照无效。', 'immutable-video': '必须保留已有影像 ID。', 'invalid-draft': '请检查字段与引用后再应用。' },
      }
}
