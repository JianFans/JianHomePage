import type { AdminLocale } from './admin-locale'

/** 现场表单使用会话语言，不转换时间原文或内容语言。 */
export function eventCopy(locale: AdminLocale) {
  return locale === 'en'
    ? {
        title: 'Events', open: 'Open event editor', reload: 'Reload events', add: 'Add event', close: 'Close event editor',
        eventTitle: 'Title', city: 'City', venue: 'Venue', dateTime: 'Date and time', status: 'Status',
        timezone: 'ISO 8601 with explicit timezone Z or ±HH:MM, e.g. 2030-01-02T20:00:00+08:00.',
        scheduled: 'Scheduled', soldOut: 'Sold out', cancelled: 'Cancelled', detailUrl: 'Details URL (HTTPS)', poster: 'Poster (optional)',
        none: 'None', apply: 'Apply to snapshot', keep: 'Keep editing', discard: 'Discard and continue',
        discardPrompt: 'Discard unapplied event changes?', empty: 'Load or import a valid snapshot to edit events.',
        placement: 'Apply a new event first, then add its reference in the homepage editor.',
        stale: 'Snapshot changed. Reload events before applying.',
        /** 仅播报截断诊断的剩余数量。 */
        moreIssues: (count: number) => `${count} more issues`,
        errors: { 'stale-snapshot': 'Snapshot changed. Reload events before applying.', 'invalid-snapshot': 'The current snapshot is invalid.', 'immutable-event': 'Existing event IDs must be preserved.', 'invalid-draft': 'Check fields and references before applying.' },
      }
    : {
        title: '现场', open: '打开现场表单', reload: '重载现场', add: '新增现场', close: '关闭现场表单',
        eventTitle: '标题', city: '城市', venue: '场馆', dateTime: '日期与时间', status: '状态',
        timezone: 'ISO 8601 原文须含明确时区 Z 或 ±HH:MM，例如 2030-01-02T20:00:00+08:00。',
        scheduled: '已排期', soldOut: '已售罄', cancelled: '已取消', detailUrl: '详情链接（HTTPS）', poster: '海报（可选）',
        none: '无', apply: '应用到快照', keep: '继续编辑', discard: '放弃并继续',
        discardPrompt: '放弃未应用的现场修改？', empty: '载入或导入有效快照后编辑现场。',
        placement: '新增现场先应用，再去首页编排添加引用。', stale: '快照已变化，请重载现场后再应用。',
        /** 仅播报截断诊断的剩余数量。 */
        moreIssues: (count: number) => `另有 ${count} 项错误`,
        errors: { 'stale-snapshot': '快照已变化，请重载现场后再应用。', 'invalid-snapshot': '当前快照无效。', 'immutable-event': '必须保留已有现场 ID。', 'invalid-draft': '请检查字段与引用后再应用。' },
      }
}
