<script setup lang="ts">
import { CheckCheck, Send, Undo2, Upload } from '@lucide/vue'
import { computed } from 'vue'
import type { StudioWorkspace } from '../composables/useStudioWorkspace'
import { studioVersionStatus } from '../utils/studio-copy'
import AsyncButton from './AsyncButton.vue'

const props = defineProps<{ studio: StudioWorkspace }>()
const emit = defineEmits<{ workflow: [mode: 'review' | 'publish' | 'rollback'] }>()
const status = computed(() => props.studio.workspace.version && studioVersionStatus(props.studio.workspace.version))
</script>

<template>
  <div class="studio-workflow-actions">
    <AsyncButton
      v-if="studio.canEdit && status === 'draft'"
      class="button"
      type="button"
      :disabled="!studio.workspace.canSubmitReview || studio.unsaved"
      :loading="studio.workspace.workflow.status === 'reviewing'"
      @click="studio.workspace.submitReview"
    >
      <Send
        :size="16"
        aria-hidden="true"
      />{{ studio.copy.submit }}
    </AsyncButton>
    <button
      v-if="studio.canReview && status === 'in_review'"
      class="button"
      type="button"
      :disabled="!studio.workspace.canApprove || studio.unsaved"
      @click="emit('workflow', 'review')"
    >
      <CheckCheck
        :size="16"
        aria-hidden="true"
      />{{ studio.copy.approve }} / {{ studio.copy.reject }}
    </button>
    <button
      v-if="studio.canPublish && status === 'approved'"
      class="button button--primary"
      type="button"
      :disabled="!studio.workspace.canPublish || studio.unsaved"
      @click="emit('workflow', 'publish')"
    >
      <Upload
        :size="16"
        aria-hidden="true"
      />{{ studio.copy.publish }}
    </button>
    <button
      v-if="studio.canPublish && (status === 'published' || status === 'archived')"
      class="button"
      type="button"
      :disabled="!studio.workspace.canRollback || studio.unsaved"
      @click="emit('workflow', 'rollback')"
    >
      <Undo2
        :size="16"
        aria-hidden="true"
      />{{ studio.copy.rollback }}
    </button>
  </div>
</template>
