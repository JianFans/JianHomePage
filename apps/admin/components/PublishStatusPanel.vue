<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RefreshCw, Check, AlertCircle } from '@lucide/vue'
import type { AdminPublishJob } from '../utils/admin-api'
import type { AdminLocale } from '../utils/admin-locale'
import StudioDialog from './StudioDialog.vue'
import AsyncButton from './AsyncButton.vue'
import ActivityIndicator from '../../web/components/ui/ActivityIndicator.vue'

const props = defineProps<{ job: AdminPublishJob | null; locale: AdminLocale; busy: boolean; unsaved: boolean; canRecover: boolean; refreshing: boolean; readError: boolean }>()
const emit = defineEmits<{ refresh: []; retry: []; recover: [input: { note: string; deploymentId?: string; confirmedAbsent?: true }] }>()
const mode = ref<'retry' | 'recover' | null>(null)
const outcome = ref('absent')
const note = ref('')
const deploymentId = ref('')
const verified = ref(false)
const en = computed(() => props.locale === 'en')
const uncertain = computed(() => props.job?.status === 'building' && props.job.build?.phase === 'submitting' && Boolean(props.job.build.error || props.job.build.configurationMismatch))
const locked = computed(() => props.busy || props.unsaved || props.refreshing)
const phase = computed(() => {
  const job = props.job
  if (!job) return ''
  if (job.status === 'failed' || job.status === 'succeeded') return job.status
  if (uncertain.value) return 'uncertain'
  if (job.build?.phase === 'succeeded' || job.build?.phase === 'failed') return 'syncing'
  if (job.build?.configurationMismatch && job.build.phase !== 'deployed') return 'mismatch'
  if (!job.build && job.status === 'building') return 'processing'
  return job.build?.phase || job.status
})
const status = computed(() => {
  const labels: Record<string, string> = en.value
    ? { pending: 'Waiting', queued: 'Queued', processing: 'Processing publish task', building: 'Generating pages', submitting: 'Submitting deployment', deployed: 'Deployment accepted · checking result', succeeded: 'Published', failed: 'Failed', uncertain: 'Verification required', mismatch: 'Publish configuration changed', syncing: 'Synchronizing publish result' }
    : { pending: '等待处理', queued: '排队中', processing: '发布处理中', building: '生成静态页面', submitting: '提交云端部署', deployed: '部署已接受，等待结果', succeeded: '发布成功', failed: '发布失败', uncertain: '部署结果待核实', mismatch: '发布配置已变更', syncing: '同步发布结果中' }
  return labels[phase.value] || phase.value
})
const canConfirm = computed(() => !locked.value && (mode.value === 'retry'
  ? props.job?.status === 'failed'
  : props.canRecover && uncertain.value && verified.value && note.value.trim().length > 0 && note.value.trim().length <= 500 && !/\p{Cc}/u.test(note.value.trim())
    && (outcome.value === 'absent' || (!props.job?.build?.configurationMismatch && /^[A-Za-z0-9][A-Za-z0-9_-]{0,255}$/.test(deploymentId.value)))))

/** 任务已被替换或核实完成时关闭旧确认，失败响应保持原表单输入。 */
watch(() => [props.job?.id, props.job?.build?.phase, props.job?.status], () => {
  if (mode.value === 'recover' && !uncertain.value || mode.value === 'retry' && props.job?.status !== 'failed') mode.value = null
})
watch(() => props.job?.id, () => { mode.value = null; note.value = ''; deploymentId.value = ''; verified.value = false; outcome.value = 'absent' })

/** 只发送经过用户确认的领域操作；不自动重新创建结果不明的部署。 */
function confirm() {
  if (!canConfirm.value) return
  if (mode.value === 'retry') { mode.value = null; emit('retry') }
  else emit('recover', outcome.value === 'absent' ? { note: note.value.trim(), confirmedAbsent: true } : { note: note.value.trim(), deploymentId: deploymentId.value })
}
</script>

<template>
  <section
    class="publish-status"
    data-testid="publish-status"
    :aria-label="en ? 'Publish task' : '发布任务'"
  >
    <header>
      <strong
        role="status"
        class="publish-phase"
        :data-phase="phase"
      >
        <ActivityIndicator v-if="['pending', 'queued', 'processing', 'building', 'submitting', 'deployed', 'syncing'].includes(phase)" />
        <Check
          v-else-if="phase === 'succeeded'"
          :size="17"
          aria-hidden="true"
        />
        <AlertCircle
          v-else-if="['failed', 'uncertain', 'mismatch'].includes(phase)"
          :size="17"
          aria-hidden="true"
        />
        <span
          :key="phase"
          class="publish-phase__label"
        >{{ status || (en ? 'No publish task' : '暂无发布任务') }}</span>
      </strong>
      <AsyncButton
        type="button"
        class="button"
        :disabled="busy || refreshing"
        :loading="refreshing"
        data-testid="publish-refresh"
        @click="emit('refresh')"
      >
        <RefreshCw
          :size="15"
          aria-hidden="true"
        />{{ en ? 'Refresh status' : '刷新状态' }}
      </AsyncButton>
    </header>
    <template v-if="job">
      <dl>
        <div><dt>{{ en ? 'Version' : '版本' }}</dt><dd>{{ job.versionId }}</dd></div>
        <div><dt>{{ en ? 'Task' : '任务' }}</dt><dd>{{ job.id }}</dd></div>
        <div v-if="job.build?.deploymentId">
          <dt>{{ en ? 'Deployment' : '部署' }}</dt><dd>{{ job.build.deploymentId }}</dd>
        </div>
      </dl>
      <p
        v-if="job.build?.error"
        class="studio-muted"
      >
        {{ job.build.error }}
      </p>
      <p
        v-if="job.build?.configurationMismatch && !['deployed', 'syncing', 'succeeded', 'failed'].includes(phase)"
        class="studio-muted"
      >
        {{ en ? 'The current target differs from the frozen task. Restore its original configuration to continue; confirm absence only after checking the original project.' : '当前配置与任务冻结目标不一致。继续部署需恢复原配置；确认未创建前，必须先检查原项目。' }}
      </p>
      <p
        v-if="uncertain"
        class="studio-muted"
      >
        {{ en ? 'Check deployment records in the original EdgeOne project. Do not retry an unknown creation.' : '请在原 EdgeOne 项目核对部署记录，避免重复创建结果不明的部署。' }}
      </p>
      <div class="publish-actions">
        <button
          v-if="job.status === 'failed'"
          type="button"
          class="button"
          :disabled="locked"
          data-testid="publish-retry"
          @click="mode = 'retry'"
        >
          {{ en ? 'Retry task' : '重试任务' }}
        </button>
        <button
          v-if="uncertain && canRecover"
          type="button"
          class="button"
          :disabled="locked"
          data-testid="publish-recover"
          @click="mode = 'recover'"
        >
          {{ en ? 'Record verification' : '核实部署结果' }}
        </button>
        <p
          v-if="uncertain && !canRecover"
          class="studio-muted"
        >
          {{ en ? 'Ask an administrator to record the verified result.' : '请管理员核实并登记部署结果。' }}
        </p>
      </div>
    </template>
    <p
      v-if="readError"
      role="status"
    >
      {{ en ? 'Status could not be read. Refresh to try again.' : '暂时无法读取状态，可点击刷新重试。' }}
    </p>
    <StudioDialog
      :open="Boolean(mode)"
      :title="mode === 'retry' ? (en ? 'Retry failed task' : '重试失败任务') : (en ? 'Verify deployment' : '核实部署')"
      :close-label="en ? 'Close' : '关闭'"
      :busy="busy"
      @close="mode = null"
    >
      <p v-if="mode === 'retry'">
        {{ en ? 'Create a new task for this version using its original operation. The failed record is kept.' : '将为该任务版本重新创建同类型任务，保留原失败记录。' }}
      </p>
      <div
        v-else
        class="publish-recovery-form"
      >
        <label>{{ en ? 'Verified outcome' : '核实结果' }}
          <select
            v-model="outcome"
            :disabled="busy"
          >
            <option value="absent">{{ en ? 'No deployment was created' : '未创建部署' }}</option>
            <option
              value="existing"
              :disabled="job?.build?.configurationMismatch"
            >{{ en ? 'Deployment exists' : '部署已创建' }}</option>
          </select>
        </label>
        <label v-if="outcome === 'existing'">{{ en ? 'Deployment ID' : '部署 ID' }}<input
          v-model="deploymentId"
          :disabled="busy"
          maxlength="256"
        ></label>
        <label>{{ en ? 'Verification note (one line)' : '核实说明（单行）' }}<input
          v-model="note"
          :disabled="busy"
          maxlength="500"
        ></label>
        <label class="publish-confirm-check"><input
          v-model="verified"
          type="checkbox"
          :disabled="busy"
        >{{ en ? 'I checked the deployment records in the original project.' : '我已检查原项目中的部署记录。' }}</label>
      </div>
      <AsyncButton
        type="button"
        class="button button--primary"
        :disabled="!canConfirm"
        data-testid="publish-confirm"
        :loading="busy"
        @click="confirm"
      >
        {{ en ? 'Confirm' : '确认' }}
      </AsyncButton>
    </StudioDialog>
  </section>
</template>

<style scoped>
.publish-status { margin-block: 1.5rem; padding: 1.25rem; border: 1px solid var(--border); background: var(--surface); border-radius: .8rem; }
.publish-status header, .publish-actions { display: flex; align-items: center; justify-content: space-between; gap: 1rem; flex-wrap: wrap; }
.publish-phase { display: inline-flex; align-items: center; gap: .6rem; }
.publish-phase > svg { flex: 0 0 auto; }
@media (prefers-reduced-motion: no-preference) { .publish-phase__label { animation: publish-phase-in var(--motion-normal, 220ms) ease-out; } }
@keyframes publish-phase-in { from { opacity: .5; } to { opacity: 1; } }
.publish-status dl { display: grid; gap: .5rem; font-size: .8rem; }
.publish-status dl > div { display: flex; gap: 1rem; }
.publish-status dt { flex: 0 0 4.5rem; color: var(--muted); }
.publish-status dd { margin: 0; overflow-wrap: anywhere; min-width: 0; }
.publish-recovery-form { display: grid; gap: 1rem; margin-block-end: 1.5rem; }
.publish-recovery-form label { display: grid; gap: .5rem; }
.publish-recovery-form .publish-confirm-check { display: flex; align-items: flex-start; }
.publish-confirm-check input { width: auto; margin-block-start: .25rem; }
</style>
