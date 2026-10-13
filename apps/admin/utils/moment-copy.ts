import type { AdminLocale } from './admin-locale'

/** 片段字段和稳定错误使用当前会话语言。 */
export function momentCopy(locale: AdminLocale) {
  return locale === 'en'
    ? {
        title: 'Moments', open: 'Open moment editor', reload: 'Reload moments', add: 'Add moment', close: 'Close moment editor',
        asset: 'Image / GIF', choose: 'Choose an asset', caption: 'Caption', customCaption: 'Add caption',
        target: 'Target', none: 'None', internal: 'Internal', external: 'External', content: 'Content', chooseContent: 'Choose content',
        provider: 'Platform', url: 'HTTPS URL', customLabel: 'Custom label', label: 'Link label',
        apply: 'Apply to snapshot', keep: 'Keep editing', discard: 'Discard and continue',
        discardPrompt: 'Discard unapplied moment changes?', empty: 'Load or import a valid snapshot to edit moments.',
        placement: 'Apply a new moment first, then add its reference in the homepage editor.',
        stale: 'Snapshot changed. Reload moments before applying.',
        /** 仅播报截断诊断的剩余数量。 */
        moreIssues: (count: number) => `${count} more issues`,
        errors: { 'stale-snapshot': 'Snapshot changed. Reload moments before applying.', 'invalid-snapshot': 'The current snapshot is invalid.', 'immutable-moment': 'Existing moment IDs must be preserved.', 'invalid-draft': 'Check fields and references before applying.' },
      }
    : {
        title: '片段', open: '打开片段表单', reload: '重载片段', add: '新增片段', close: '关闭片段表单',
        asset: '图片 / GIF', choose: '选择素材', caption: '说明', customCaption: '添加说明',
        target: '跳转目标', none: '无', internal: '内部', external: '外部', content: '内容', chooseContent: '选择内容',
        provider: '平台', url: 'HTTPS 链接', customLabel: '自定义标签', label: '链接标签',
        apply: '应用到快照', keep: '继续编辑', discard: '放弃并继续',
        discardPrompt: '放弃未应用的片段修改？', empty: '载入或导入有效快照后编辑片段。',
        placement: '新增片段先应用，再去首页编排添加引用。', stale: '快照已变化，请重载片段后再应用。',
        /** 仅播报截断诊断的剩余数量。 */
        moreIssues: (count: number) => `另有 ${count} 项错误`,
        errors: { 'stale-snapshot': '快照已变化，请重载片段后再应用。', 'invalid-snapshot': '当前快照无效。', 'immutable-moment': '必须保留已有片段 ID。', 'invalid-draft': '请检查字段与引用后再应用。' },
      }
}
