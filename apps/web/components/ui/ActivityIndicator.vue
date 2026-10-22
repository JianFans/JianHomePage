<template>
  <svg
    class="activity-indicator"
    viewBox="0 0 24 24"
    fill="none"
    aria-hidden="true"
    focusable="false"
  >
    <circle
      cx="12"
      cy="12"
      r="9"
      stroke="currentColor"
      stroke-width="2"
      opacity=".2"
    />
    <path
      d="M12 3a9 9 0 0 1 9 9"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
    />
  </svg>
</template>

<style scoped>
.activity-indicator { display: inline-block; width: 1em; height: 1em; flex: 0 0 auto; vertical-align: middle; }
@media (prefers-reduced-motion: no-preference) {
  .activity-indicator { animation: activity-turn var(--motion-spin, 900ms) linear infinite; }
}
@keyframes activity-turn { to { transform: rotate(360deg); } }
</style>
