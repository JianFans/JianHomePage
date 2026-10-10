import type { AdminLocale } from './admin-locale'

const english = {
  title: 'Music', newRelease: 'New release', selectRelease: 'Choose a release', empty: 'Import or load a valid snapshot first.',
  releaseTitle: 'Release title', kind: 'Release type', single: 'Single', ep: 'EP', album: 'Album',
  date: 'Release date', cover: 'Cover', choose: 'Choose an asset', featured: 'Featured',
  tracks: 'Tracks', addTrack: 'Add track', trackTitle: 'Track title', duration: 'Duration · seconds',
  preview: 'Preview audio', noPreview: 'No preview', previewDuration: 'Preview · seconds (1–90)',
  links: 'Platform links', addLink: 'Add link', provider: 'Platform', url: 'HTTPS URL', customLabel: 'Custom label',
  label: 'Link label', remove: 'Remove', up: 'Move up', down: 'Move down',
  credits: 'Credits', addCredit: 'Add credit', role: 'Role', name: 'Name',
  sections: 'Homepage placement', include: 'Include in section', visible: 'Within display limit',
  hidden: 'Outside display limit or section disabled', noSection: 'No music section; this release can still be edited.',
  apply: 'Apply to snapshot', close: 'Close editor', unapplied: 'Unapplied changes',
  discardPrompt: 'Discard unapplied changes?', keep: 'Keep editing', discard: 'Discard and continue',
  stale: 'Snapshot changed. Reload the release before applying.', chinese: 'Chinese', english: 'English · optional',
  errors: {
    'stale-snapshot': 'Snapshot changed. Reload the release before applying.',
    'invalid-snapshot': 'Fix or import a valid snapshot first.',
    'missing-release': 'This release no longer exists.',
    'immutable-id': 'Existing release IDs cannot be changed.',
    'duplicate-id': 'A release or track ID is duplicated.',
    'track-ownership': 'Tracks must belong to this release and cannot reuse another record.',
    'invalid-draft': 'Check the fields and references before applying.',
  },
}

const chinese: typeof english = {
  title: '音乐', newRelease: '新增作品', selectRelease: '选择作品开始编辑', empty: '请先导入或载入有效快照',
  releaseTitle: '作品标题', kind: '作品类型', single: '单曲', ep: 'EP', album: '专辑',
  date: '发行日期', cover: '封面', choose: '选择素材', featured: '精选',
  tracks: '曲目', addTrack: '添加曲目', trackTitle: '曲目标题', duration: '时长 · 秒',
  preview: '试听音频', noPreview: '无试听', previewDuration: '试听时长 · 秒（1–90）',
  links: '平台链接', addLink: '添加链接', provider: '平台', url: 'HTTPS 地址', customLabel: '自定义标签',
  label: '链接标签', remove: '移除', up: '上移', down: '下移',
  credits: '署名', addCredit: '添加署名', role: '角色', name: '姓名',
  sections: '首页展示', include: '加入板块', visible: '位于展示范围内',
  hidden: '超过展示数量或板块未启用', noSection: '尚无音乐板块，仍可编辑作品',
  apply: '应用到快照', close: '关闭编辑', unapplied: '表单未应用',
  discardPrompt: '放弃尚未应用的修改？', keep: '继续编辑', discard: '放弃并继续',
  stale: '快照已变化，请重新载入作品后应用', chinese: '中文', english: '英文 · 选填',
  errors: {
    'stale-snapshot': '快照已变化，请重新载入作品后应用',
    'invalid-snapshot': '请先修复或导入有效快照',
    'missing-release': '作品已不存在',
    'immutable-id': '已有作品 ID 不可修改',
    'duplicate-id': '作品或曲目 ID 重复',
    'track-ownership': '曲目必须属于当前作品，且不能复用其他记录',
    'invalid-draft': '请检查字段与引用后再应用',
  },
}

export const musicProviders = ['qq-music', 'netease-music', 'weibo', 'bilibili', 'douyin', 'website', 'other'] as const

/** 按当前界面语言读取文案，稳定错误代码不会因语言切换而丢失。 */
export function musicCopy(locale: AdminLocale) {
  return locale === 'en' ? english : chinese
}
