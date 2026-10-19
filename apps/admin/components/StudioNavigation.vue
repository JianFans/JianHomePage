<script setup lang="ts">
import { CalendarDays, ChevronLeft, ChevronRight, Film, History, Images, LayoutDashboard, LayoutList, LogOut, Music2, FolderOpen, UserRound } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import type { AdminLocale } from '../utils/admin-locale'
import { studioCopy, type StudioModule } from '../utils/studio-copy'

const props = defineProps<{ active: StudioModule; locale: AdminLocale; name: string; picture?: string; collapsed?: boolean; drawer?: boolean; busy?: boolean; logoutBusy?: boolean }>()
const emit = defineEmits<{ navigate: [module: StudioModule]; logout: []; collapse: [] }>()
const copy = computed(() => studioCopy(props.locale))
const failedPicture = ref('')
const pictureURL = computed(() => {
  try {
    const url = new URL(props.picture || '')
    return ['https:', 'http:'].includes(url.protocol) && !url.username && !url.password ? url.href : ''
  } catch { return '' }
})
watch(() => props.picture, () => { failedPicture.value = '' })
const modules = [
  { id: 'overview', icon: LayoutDashboard, tone: 'sage' }, { id: 'homepage', icon: LayoutList, tone: 'sage' },
  { id: 'music', icon: Music2, tone: 'sage' }, { id: 'video', icon: Film, tone: 'blue' },
  { id: 'event', icon: CalendarDays, tone: 'amber' }, { id: 'moment', icon: Images, tone: 'purple' },
  { id: 'assets', icon: FolderOpen, tone: 'blue' }, { id: 'history', icon: History, tone: 'purple' },
] as const
</script>

<template>
  <aside
    class="studio-navigation"
    :class="{ 'studio-navigation--collapsed': collapsed && !drawer, 'studio-navigation--drawer': drawer }"
  >
    <div class="studio-brand">
      <span
        class="studio-brand-mark"
        aria-hidden="true"
      >遇</span><div v-if="!collapsed || drawer">
        <strong>{{ copy.brand }}</strong><small>STUDIO</small>
      </div>
    </div>
    <nav :aria-label="copy.studio">
      <button
        v-for="item in modules"
        :key="item.id"
        type="button"
        :data-testid="`nav-${item.id}`"
        :data-tone="item.tone"
        :class="{ 'is-active': active === item.id }"
        :aria-current="active === item.id ? 'page' : undefined"
        :aria-label="copy.modules[item.id]"
        :title="copy.modules[item.id]"
        :disabled="busy"
        @click="emit('navigate', item.id)"
      >
        <component
          :is="item.icon"
          :size="19"
          aria-hidden="true"
        /><span v-if="!collapsed || drawer">{{ copy.modules[item.id] }}</span>
      </button>
    </nav>
    <footer>
      <div
        class="studio-account"
        :title="name"
      >
        <span
          class="studio-avatar"
          aria-hidden="true"
        >
          <img
            v-if="pictureURL && failedPicture !== pictureURL"
            :src="pictureURL"
            alt=""
            width="32"
            height="32"
            referrerpolicy="no-referrer"
            decoding="async"
            data-testid="studio-avatar"
            @error="failedPicture = pictureURL"
          >
          <UserRound
            v-else
            :size="18"
            data-testid="studio-avatar-fallback"
          />
        </span>
        <div
          v-if="!collapsed || drawer"
          class="studio-account-info"
        >
          <span data-testid="studio-account-name">{{ name }}</span><small>{{ copy.studio }}</small>
        </div>
      </div>
      <button
        type="button"
        class="icon-tool"
        :aria-label="copy.logout"
        :title="copy.logout"
        :disabled="busy || logoutBusy"
        data-testid="studio-logout"
        @click="emit('logout')"
      >
        <LogOut
          :size="18"
          aria-hidden="true"
        />
      </button>
      <button
        v-if="!drawer"
        type="button"
        class="icon-tool studio-collapse"
        :aria-label="collapsed ? copy.expand : copy.collapse"
        :title="collapsed ? copy.expand : copy.collapse"
        @click="emit('collapse')"
      >
        <component
          :is="collapsed ? ChevronRight : ChevronLeft"
          :size="18"
          aria-hidden="true"
        />
      </button>
    </footer>
  </aside>
</template>
