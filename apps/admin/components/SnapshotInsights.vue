<script setup lang="ts">
import { CircleCheck, CircleX } from '@lucide/vue'
import { computed } from 'vue'
import type { SnapshotAnalysis } from '../utils/snapshot-workbench'

const props = defineProps<{
  analysis: SnapshotAnalysis
  locale: 'zh-CN' | 'en'
}>()

const copy = computed(() => props.locale === 'en'
  ? {
      labels: {
        sections: 'Sections',
        releases: 'Releases',
        tracks: 'Tracks',
        videos: 'Videos',
        events: 'Events',
        moments: 'Moments',
        assets: 'Assets',
        previews: 'Previews',
      },
      valid: 'Snapshot valid',
      issues: 'Contract issues',
      more: (count: number) => `${count} more`,
    }
  : {
      labels: {
        sections: '板块',
        releases: '作品',
        tracks: '曲目',
        videos: '影像',
        events: '现场',
        moments: '片段',
        assets: '素材',
        previews: '试听',
      },
      valid: '快照有效',
      issues: '契约问题',
      more: (count: number) => `另有 ${count} 项`,
    })

const summaryEntries = computed(() => {
  const summary = props.analysis.summary
  if (!summary) return []
  return (Object.keys(summary) as Array<keyof typeof summary>).map(key => ({
    key,
    label: copy.value.labels[key],
    value: summary[key],
  }))
})
const visibleIssues = computed(() => props.analysis.issues.slice(0, 8))
const remainingIssues = computed(() => Math.max(0, props.analysis.issues.length - visibleIssues.value.length))

function issueLabel(code: string): string {
  const labels = props.locale === 'en'
    ? {
        'invalid-json': 'Invalid JSON',
        'object-root': 'Root must be an object',
        required: 'Required field missing',
        additionalProperties: 'Field is not allowed',
        type: 'Incorrect field type',
        format: 'Incorrect value format',
        pattern: 'Incorrect value format',
        'duplicate-id': 'Duplicate identifier',
        'missing-reference': 'Reference not found',
        'asset-kind': 'Asset type mismatch',
        'hidden-target': 'Target is not visible',
      }
    : {
        'invalid-json': 'JSON 格式无效',
        'object-root': '根节点必须为对象',
        required: '缺少必填字段',
        additionalProperties: '存在不允许的字段',
        type: '字段类型错误',
        format: '字段格式错误',
        pattern: '字段格式错误',
        'duplicate-id': '标识重复',
        'missing-reference': '引用不存在',
        'asset-kind': '素材类型不匹配',
        'hidden-target': '目标未在首页展示',
      }
  return labels[code as keyof typeof labels]
    ?? (props.locale === 'en' ? 'Content contract mismatch' : '内容契约不匹配')
}
</script>

<template>
  <section class="snapshot-insights">
    <template v-if="analysis.summary">
      <div class="insight-heading insight-heading--valid">
        <CircleCheck
          :size="16"
          aria-hidden="true"
        />
        <span>{{ copy.valid }}</span>
      </div>
      <dl
        class="summary-grid"
        data-testid="snapshot-summary"
      >
        <div
          v-for="entry in summaryEntries"
          :key="entry.key"
        >
          <dt>{{ entry.label }}</dt>
          <dd>{{ entry.value }}</dd>
        </div>
      </dl>
    </template>

    <template v-else>
      <div class="insight-heading insight-heading--invalid">
        <CircleX
          :size="16"
          aria-hidden="true"
        />
        <span>{{ copy.issues }}</span>
      </div>
      <ol
        class="issue-list"
        data-testid="snapshot-issues"
      >
        <li
          v-for="issue in visibleIssues"
          :key="`${issue.path}:${issue.code}`"
          data-testid="snapshot-issue"
        >
          <code>{{ issue.path }}</code>
          <span>{{ issueLabel(issue.code) }}</span>
        </li>
      </ol>
      <p
        v-if="remainingIssues"
        class="issue-overflow"
      >
        {{ copy.more(remainingIssues) }}
      </p>
    </template>
  </section>
</template>

<style scoped>
.snapshot-insights {
  margin-bottom: 1rem;
  border: 1px solid var(--border);
  background: var(--surface-raised);
}

.insight-heading {
  display: flex;
  align-items: center;
  gap: .5rem;
  min-height: 2.75rem;
  padding: .65rem .8rem;
  border-bottom: 1px solid var(--border);
  color: var(--muted);
  font-size: .72rem;
  letter-spacing: .08em;
  text-transform: uppercase;
}

.insight-heading--valid { color: #aec3b5; }
.insight-heading--invalid { color: var(--danger); }

.summary-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  margin: 0;
}

.summary-grid div {
  min-width: 0;
  padding: .75rem .8rem;
  border-right: 1px solid var(--border);
  border-bottom: 1px solid var(--border);
}

.summary-grid div:nth-child(4n) { border-right: 0; }
.summary-grid div:nth-last-child(-n + 4) { border-bottom: 0; }
.summary-grid dt { color: var(--muted); font-size: .68rem; }
.summary-grid dd { margin: .15rem 0 0; color: var(--text); font-size: 1.2rem; font-variant-numeric: tabular-nums; }

.issue-list {
  display: grid;
  gap: 0;
  margin: 0;
  padding: 0;
  list-style: none;
}

.issue-list li {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: .75rem;
  padding: .6rem .8rem;
  border-bottom: 1px solid var(--border);
  font-size: .72rem;
}

.issue-list code {
  overflow-wrap: anywhere;
  color: #b6c0bd;
}

.issue-list span { color: var(--danger); text-align: right; }
.issue-overflow { margin: 0; padding: .65rem .8rem; color: var(--muted); font-size: .72rem; }

@media (max-width: 520px) {
  .summary-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .summary-grid div:nth-child(2n) { border-right: 0; }
  .summary-grid div:nth-last-child(-n + 4) { border-bottom: 1px solid var(--border); }
  .summary-grid div:nth-last-child(-n + 2) { border-bottom: 0; }
  .issue-list li { grid-template-columns: 1fr; gap: .2rem; }
  .issue-list span { text-align: left; }
}
</style>
