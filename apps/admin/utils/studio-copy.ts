import type { AdminLocale } from './admin-locale'

export type StudioModule = 'overview' | 'homepage' | 'music' | 'video' | 'event' | 'moment' | 'assets' | 'history'

/** CMS 壳层共享双语文案，不把界面标签或语言写入内容快照。 */
export function studioCopy(locale: AdminLocale) {
  return locale === 'en' ? {
    brand: 'Meet Jian', studio: 'Content studio', modules: { overview: 'Overview', homepage: 'Homepage', music: 'Music', video: 'Videos', event: 'Live', moment: 'Moments', assets: 'Assets', history: 'Versions' },
    login: 'Sign in', loginHint: 'Continue to the configured identity provider.', unavailable: 'Sign-in is not configured. Contact the site administrator.', retry: 'Try again', loading: 'Opening your workspace…',
    logout: 'Sign out', language: '切换中文', menu: 'Open navigation', close: 'Close', collapse: 'Collapse navigation', expand: 'Expand navigation',
    save: 'Save draft', saved: 'Saved', unsaved: 'Unsaved changes', readonly: 'Read only', preview: 'Preview', tools: 'Snapshot tools', clone: 'Create from version', cloneName: 'Version name', create: 'Create draft',
    noVersion: 'No content version', firstVersion: 'Import a snapshot to create your first draft.', latest: 'Recent versions', more: 'Load more', refresh: 'Refresh', source: 'Based on', revision: 'Revision',
    statuses: { draft: 'Draft', in_review: 'In review', publishing: 'Publishing', published: 'Published', archived: 'Archived' },
    leaveTitle: 'Keep your changes?', leaveHint: 'Save before continuing, or discard the changes in the current context.', keep: 'Keep editing', discard: 'Discard and continue', saveContinue: 'Save and continue',
    import: 'Import JSON', export: 'Export snapshot', editor: 'JSON snapshot editor', valid: 'Valid', invalid: 'Invalid snapshot', formInvalid: 'Check the highlighted fields. Your input has been kept.',
    submit: 'Submit review', approve: 'Approve', reject: 'Return to draft', reason: 'Return reason', publish: 'Publish', rollback: 'Rollback', confirm: 'Confirm', publishingHint: 'This changes the public site. Continue?',
    expired: 'Your session expired. Export any unsaved content before signing in again.', sessionError: 'Could not restore the session.', logoutError: 'Sign-out failed. Please try again.',
    noVersions: 'No versions yet', current: 'Current version', overviewHint: 'Choose a content module to start editing.', snapshot: 'Snapshot', versions: 'Version history', newName: 'New draft',
    listError: 'Could not load versions. Try refreshing.', uploadHint: 'Finish the asset upload before signing out.', assetLeaveHint: 'The selected asset and its upload details have not been uploaded. Return to Assets to upload, or discard them and sign out.',
  } : {
    brand: '遇健我', studio: '内容工作室', modules: { overview: '概览', homepage: '首页编排', music: '音乐', video: '影像', event: '现场', moment: '片段', assets: '素材库', history: '版本历史' },
    login: '登录', loginHint: '前往已配置的身份服务登录。', unavailable: '登录尚未配置，请联系站点管理员。', retry: '重试', loading: '正在打开工作区…',
    logout: '退出登录', language: 'Switch to English', menu: '打开导航', close: '关闭', collapse: '收起导航', expand: '展开导航',
    save: '保存草稿', saved: '已保存', unsaved: '未保存更改', readonly: '只读', preview: '预览', tools: '快照工具', clone: '基于此版本新建', cloneName: '版本名称', create: '新建草稿',
    noVersion: '尚无内容版本', firstVersion: '导入快照后创建第一份草稿。', latest: '最近版本', more: '加载更多', refresh: '刷新', source: '来源', revision: '修订',
    statuses: { draft: '草稿', in_review: '审核中', publishing: '发布中', published: '已发布', archived: '已归档' },
    leaveTitle: '保留当前更改？', leaveHint: '保存后继续，或放弃当前上下文中的更改。', keep: '继续编辑', discard: '放弃并继续', saveContinue: '保存并继续',
    import: '导入 JSON', export: '导出快照', editor: 'JSON 快照编辑器', valid: '有效', invalid: '快照无效', formInvalid: '请检查表单字段，当前输入已保留。',
    submit: '提交审核', approve: '通过审核', reject: '退回草稿', reason: '退回原因', publish: '发布', rollback: '回滚', confirm: '确认', publishingHint: '此操作会改变公开站点，是否继续？',
    expired: '会话已过期。重新登录前可导出尚未保存的内容。', sessionError: '无法恢复登录状态。', logoutError: '退出失败，请重试。',
    noVersions: '暂无版本', current: '当前版本', overviewHint: '选择内容模块开始编辑。', snapshot: '内容快照', versions: '版本历史', newName: '新草稿',
    listError: '无法加载版本，请重试刷新。', uploadHint: '请等待素材上传完成后再退出登录。', assetLeaveHint: '所选素材及上传信息尚未上传。请返回素材库上传，或放弃后退出登录。',
  }
}
