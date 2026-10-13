import type { AdminLocale } from './admin-locale'

const english = {
  title: 'Homepage', open: 'Open homepage editor', reload: 'Reload homepage', heroes: 'Heroes', addHero: 'Add hero', removeHero: 'Delete hero',
  headline: 'Headline', mediaKind: 'Media type', image: 'Image', gif: 'GIF', video: 'Video', asset: 'Main asset', mobileAsset: 'Mobile asset · optional',
  poster: 'Poster · image / GIF', choose: 'Choose', none: 'None', focalX: 'Focus X · 0–100', focalY: 'Focus Y · 0–100', autoplay: 'Autoplay',
  release: 'Related release · optional', target: 'Target', internal: 'Internal content', external: 'External link', content: 'Content',
  startsAt: 'Starts at · optional', endsAt: 'Ends at · optional', utc: 'UTC ISO 8601 · YYYY-MM-DDTHH:mm:ssZ. Blank means no boundary.',
  sections: 'Homepage arrangement', enabled: 'Enabled', limit: 'Display limit', addItem: 'Add reference', more: 'More link',
  provider: 'Platform', url: 'HTTPS URL', customLabel: 'Custom label', label: 'Link label', remove: 'Remove reference', up: 'Move up', down: 'Move down',
  apply: 'Apply to snapshot', close: 'Close editor', unapplied: 'Unapplied changes', empty: 'Import or load a valid snapshot first.',
  noHeroes: 'No heroes. Add a record to start editing.', discardPrompt: 'Discard unapplied changes?', keep: 'Keep editing', discard: 'Discard and continue',
  stale: 'Snapshot changed. Reload the homepage before applying.', referenceTime: 'Visibility at snapshot time', absent: 'Not referenced by a section',
  status: { visible: 'Displayed', disabled: 'Section disabled', limit: 'Outside display limit', 'time-window': 'Outside time window' },
  types: { hero: 'Heroes', music: 'Music', video: 'Videos', event: 'Events', moment: 'Moments', artist: 'Artist' },
  errors: {
    'stale-snapshot': 'Snapshot changed. Reload the homepage before applying.',
    'invalid-snapshot': 'Fix or import a valid snapshot first.',
    'immutable-section': 'Keep the existing section IDs, types and layouts.',
    'invalid-draft': 'Check the fields and references before applying.',
  },
}

const chinese: typeof english = {
  title: '首页', open: '打开首页编辑', reload: '重新载入首页', heroes: '宣传位', addHero: '新增宣传位', removeHero: '删除宣传位',
  headline: '标题', mediaKind: '媒体类型', image: '图片', gif: 'GIF', video: '视频', asset: '主素材', mobileAsset: '移动端素材 · 选填',
  poster: '海报 · 图片／GIF', choose: '请选择', none: '无', focalX: '焦点 X · 0–100', focalY: '焦点 Y · 0–100', autoplay: '自动播放',
  release: '关联作品 · 选填', target: '跳转目标', internal: '内部内容', external: '外部链接', content: '内容',
  startsAt: '开始时间 · 选填', endsAt: '结束时间 · 选填', utc: 'UTC ISO 8601 · YYYY-MM-DDTHH:mm:ssZ，留空表示不限制。',
  sections: '首页编排', enabled: '启用', limit: '展示上限', addItem: '添加引用', more: '更多链接',
  provider: '平台', url: 'HTTPS 地址', customLabel: '自定义标签', label: '链接标签', remove: '移除引用', up: '上移', down: '下移',
  apply: '应用到快照', close: '关闭编辑', unapplied: '表单未应用', empty: '请先导入或载入有效快照',
  noHeroes: '暂无宣传位，请新增记录开始编辑', discardPrompt: '放弃尚未应用的修改？', keep: '继续编辑', discard: '放弃并继续',
  stale: '快照已变化，请重新载入首页后应用', referenceTime: '展示状态参考快照时间', absent: '未加入板块',
  status: { visible: '展示', disabled: '板块未启用', limit: '超过展示上限', 'time-window': '时间窗口外' },
  types: { hero: '宣传位', music: '音乐', video: '影像', event: '现场', moment: '片段', artist: '艺人' },
  errors: {
    'stale-snapshot': '快照已变化，请重新载入首页后应用',
    'invalid-snapshot': '请先修复或导入有效快照',
    'immutable-section': '请保留已有板块 ID、类型和布局',
    'invalid-draft': '请检查字段与引用后再应用',
  },
}

/** 界面语言只影响文案，稳定的错误代码与临时草稿保持原值。 */
export function homepageCopy(locale: AdminLocale) {
  return locale === 'en' ? english : chinese
}
