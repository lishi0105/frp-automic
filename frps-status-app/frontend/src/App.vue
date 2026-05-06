<template>
  <RouterView v-if="isLogin" />
  <div v-else class="layout">
    <Teleport to="body">
      <div v-if="warnings.length > 0" class="warn-anchor" :class="{ 'with-initial-banner': currentUser.is_initial_password }">
        <button
          class="warn-bell"
          :class="{ open: warnOpen }"
          :aria-label="`${warnings.length} 条告警`"
          @click.stop="warnOpen = !warnOpen"
        >
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/></svg>
          <span class="warn-badge">{{ warnings.length }}</span>
        </button>
        <Transition name="warn-drop">
          <div v-if="warnOpen" class="warn-panel" @click.stop>
            <div class="warn-panel-hd">
              <span class="warn-panel-title">系统告警</span>
              <span class="warn-count-chip">{{ warnings.length }}</span>
              <button class="warn-panel-close" @click="warnOpen = false">✕</button>
            </div>
            <ul class="warn-list">
              <li v-for="w in warnings" :key="w.key" class="warn-item">
                <span class="warn-item-dot" :class="warnCategory(w.key)"></span>
                <div class="warn-item-body">
                  <span class="warn-item-msg">{{ w.message }}</span>
                  <span class="warn-item-time">{{ fmtTime(w.created_at) }}</span>
                </div>
              </li>
            </ul>
          </div>
        </Transition>
      </div>
    </Teleport>

    <aside class="sidebar">
      <div class="sidebar-logo">
        <img :src="logoUrl" class="logo-icon" alt="FRPS状态监控" />
        FRPS状态监控
        <span class="logo-dot" :class="{ offline: !frpsOnline }"></span>
      </div>
      <button class="sidebar-user" type="button" @click="openAccount(false)">
        <span class="sidebar-user-avatar">{{ userInitial }}</span>
        <span class="sidebar-user-meta">
          <span class="sidebar-user-label">当前用户</span>
          <span class="sidebar-user-name">{{ currentUser.username || '加载中' }}</span>
        </span>
      </button>
      <nav>
        <RouterLink class="nav-item" to="/"><span class="nav-icon">📊</span> 数据看板</RouterLink>
        <RouterLink class="nav-item" to="/proxies"><span class="nav-icon">🔗</span> 代理列表</RouterLink>
        <RouterLink class="nav-item" to="/certificates"><span class="nav-icon">🔒</span> 证书列表</RouterLink>
        <div class="nav-group">
          <div class="nav-row">
            <RouterLink class="nav-item nav-parent" to="/statistics"><span class="nav-icon">📈</span> 历史统计</RouterLink>
            <button class="nav-toggle" type="button" :aria-expanded="statsOpen" @click="statsOpen = !statsOpen">{{ statsOpen ? '⌄' : '›' }}</button>
          </div>
          <div v-if="statsOpen" class="nav-children">
            <RouterLink
              v-for="p in proxyNavItems"
              :key="p.name + p.type"
              class="nav-subitem"
              :to="'/statistics/' + encodeURIComponent(p.name)"
            >
              <span class="nav-subdot"></span>
              <span class="nav-subtext">{{ p.name }}</span>
            </RouterLink>
          </div>
        </div>
        <RouterLink class="nav-item" to="/traffic-statistics"><span class="nav-icon">🛰️</span> 流量统计</RouterLink>
        <RouterLink class="nav-item" to="/settings"><span class="nav-icon">⚙️</span> 系统配置</RouterLink>
      </nav>
      <div class="sidebar-footer">
        <span>{{ updatedAt || '加载中…' }}</span>
        <button class="logout-btn" type="button" @click="logout">退出</button>
      </div>
    </aside>
    <div class="main-wrap">
      <button
        v-if="currentUser.is_initial_password"
        class="initial-password-banner"
        type="button"
        @click="openAccount"
      >
        <span class="initial-password-mark">!</span>
        <span>当前仍在使用初始密码，请先更新密码后继续使用控制台。</span>
        <span class="initial-password-action">立即修改</span>
      </button>
      <RouterView :status="status" :daily="daily" :loading="loading" @refresh="load" />
    </div>
    <AccountSettingsModal
      v-model="accountOpen"
      @saved="handleAccountSaved"
    />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { api } from './api/index.js'
import AccountSettingsModal from './views/Account.vue'
import logoUrl from './assets/logo.svg'

const route = useRoute()
const router = useRouter()
const status = ref(null)
const daily = ref([])
const loading = ref(false)
const updatedAt = ref('')
const statsOpen = ref(false)
const warnings = ref([])
const warnOpen = ref(false)
const accountOpen = ref(false)
const currentUser = ref({ username: '' })

const isLogin = computed(() => route.path === '/login')
const frpsOnline = computed(() => status.value?.frps?.bind?.ok ?? false)
const userInitial = computed(() => (currentUser.value.username || 'U').slice(0, 1).toUpperCase())
const proxyNavItems = computed(() => {
  const map = new Map()
  for (const p of (status.value?.proxies ?? [])) {
    if (!map.has(p.name)) map.set(p.name, p)
  }
  return [...map.values()].sort((a, b) => a.name.localeCompare(b.name))
})

watch(() => route.path, (path) => {
  if (path.startsWith('/statistics')) statsOpen.value = true
}, { immediate: true })

watch(() => route.query, (query) => {
  if (!isLogin.value && query.account === '1') {
    accountOpen.value = true
  }
}, { immediate: true })

watch(accountOpen, (open) => {
  if (!open && route.query.account) {
    router.replace({ path: route.path, query: {} })
  }
})

watch(isLogin, (loginPage, wasLoginPage) => {
  if (!loginPage && wasLoginPage) {
    load()
    loadUser()
  }
})

function warnCategory(key) {
  if (key.startsWith('proxy_offline')) return 'cat-proxy'
  if (key.startsWith('cert_expiry')) return 'cat-cert'
  if (key.startsWith('traffic')) return 'cat-traffic'
  return 'cat-config'
}

function fmtTime(iso) {
  try {
    return new Date(iso).toLocaleString('zh-CN', { dateStyle: 'short', timeStyle: 'short' })
  } catch {
    return ''
  }
}

function onDocClick() {
  if (warnOpen.value) warnOpen.value = false
}

async function load() {
  loading.value = true
  try {
    const [s, d, w] = await Promise.all([api.getStatus(), api.getDaily(), api.getWarnings()])
    status.value = s
    daily.value = d
    warnings.value = w
    updatedAt.value = new Date(s.generated_at).toLocaleString('zh-CN', { timeStyle: 'short', dateStyle: 'short' })
  } catch (e) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

async function loadUser() {
  try {
    currentUser.value = await api.getUser()
  } catch (e) {
    console.error(e)
  }
}

function openAccount() {
  accountOpen.value = true
}

function handleAccountSaved(user) {
  if (user?.username) currentUser.value = { ...currentUser.value, ...user }
  if (user?.is_initial_password === false) {
    currentUser.value = { ...currentUser.value, is_initial_password: false }
  }
  if (route.query.account) {
    router.replace({ path: route.path, query: {} })
  }
}

async function logout() {
  await api.logout()
  router.replace('/login')
}

let timer
onMounted(() => {
  if (!isLogin.value) {
    load()
    loadUser()
  }
  timer = setInterval(() => { if (!isLogin.value) load() }, 30000)
  document.addEventListener('click', onDocClick)
})
onUnmounted(() => {
  clearInterval(timer)
  document.removeEventListener('click', onDocClick)
})
</script>

<style scoped>
/* ── warning anchor ── */
.warn-anchor {
  position: fixed;
  top: 86px;
  right: 22px;
  z-index: 200;
}
.warn-anchor.with-initial-banner { top: 138px; }

.warn-bell {
  position: relative;
  width: 38px;
  height: 38px;
  border-radius: 50%;
  border: none;
  background: #7f1d1d;
  color: #fca5a5;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 2px 8px rgba(0,0,0,.35);
  transition: background .15s, transform .15s;
  animation: warn-pulse 2.4s ease-in-out infinite;
}
.warn-bell:hover,
.warn-bell.open {
  background: #991b1b;
  transform: scale(1.08);
  animation: none;
}

@keyframes warn-pulse {
  0%, 100% { box-shadow: 0 2px 8px rgba(0,0,0,.35); }
  50%       { box-shadow: 0 2px 16px rgba(239,68,68,.6); }
}

.warn-badge {
  position: absolute;
  top: -4px;
  right: -4px;
  min-width: 18px;
  height: 18px;
  border-radius: 999px;
  background: #ef4444;
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 4px;
  border: 2px solid #0f172a;
  line-height: 1;
}

/* ── dropdown panel ── */
.warn-panel {
  position: absolute;
  top: calc(100% + 10px);
  right: 0;
  width: 340px;
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 10px;
  box-shadow: 0 8px 30px rgba(0,0,0,.45);
  overflow: hidden;
}

.warn-panel-hd {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 14px;
  border-bottom: 1px solid #334155;
  background: #162032;
}
.warn-panel-title {
  font-size: 13px;
  font-weight: 700;
  color: #f1f5f9;
  flex: 1;
}
.warn-count-chip {
  background: #ef4444;
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  border-radius: 999px;
  padding: 1px 7px;
  line-height: 1.6;
}
.warn-panel-close {
  background: none;
  border: none;
  color: #64748b;
  cursor: pointer;
  font-size: 13px;
  line-height: 1;
  padding: 2px 4px;
  border-radius: 4px;
}
.warn-panel-close:hover { color: #f1f5f9; background: #334155; }

.warn-list {
  list-style: none;
  max-height: 380px;
  overflow-y: auto;
}

.warn-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 11px 14px;
  border-bottom: 1px solid #1e293b;
  background: #0f172a;
}
.warn-item:last-child { border-bottom: none; }
.warn-item:hover { background: #162032; }

.warn-item-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
  margin-top: 4px;
}
.cat-config  { background: #f59e0b; }
.cat-proxy   { background: #ef4444; }
.cat-cert    { background: #f97316; }
.cat-traffic { background: #a78bfa; }

.warn-item-body {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}
.warn-item-msg {
  font-size: 12.5px;
  color: #e2e8f0;
  line-height: 1.45;
  word-break: break-all;
}
.warn-item-time {
  font-size: 11px;
  color: #475569;
}

/* transition */
.warn-drop-enter-active,
.warn-drop-leave-active {
  transition: opacity .15s ease, transform .15s ease;
}
.warn-drop-enter-from,
.warn-drop-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}

.initial-password-banner {
  margin: 12px 24px 0;
  min-height: 40px;
  padding: 9px 12px;
  border: 1px solid #fde68a;
  border-radius: var(--r);
  background: #fffbeb;
  color: #92400e;
  display: flex;
  align-items: center;
  gap: 10px;
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  text-align: left;
  cursor: pointer;
  box-shadow: var(--shadow);
}
.initial-password-banner:hover { background: #fef3c7; }
.initial-password-mark {
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: #f59e0b;
  color: #fff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  flex-shrink: 0;
}
.initial-password-action {
  margin-left: auto;
  color: #2563eb;
  white-space: nowrap;
}

.sidebar-user {
  margin: 10px 8px 6px;
  padding: 10px;
  border: 1px solid rgba(148, 163, 184, .16);
  border-radius: var(--r);
  background: rgba(15, 23, 42, .72);
  color: var(--sb-active-text);
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  text-align: left;
  transition: background .15s, border-color .15s;
}
.sidebar-user:hover {
  background: var(--sb-active);
  border-color: rgba(148, 163, 184, .28);
}
.sidebar-user-avatar {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: #2563eb;
  color: #fff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 700;
  flex-shrink: 0;
}
.sidebar-user-meta {
  display: grid;
  gap: 1px;
  min-width: 0;
}
.sidebar-user-label {
  color: var(--sb-text);
  font-size: 11px;
}
.sidebar-user-name {
  max-width: 130px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  font-weight: 600;
}

@media (max-width: 900px) {
  .layout {
    flex-direction: column;
    height: auto;
    min-height: 100vh;
    overflow: visible;
  }
  .sidebar {
    width: 100%;
    max-height: none;
    overflow: visible;
  }
  .sidebar-logo {
    padding: 12px 14px;
    font-size: 15px;
  }
  .sidebar-user {
    margin: 8px 10px 4px;
    padding: 8px 10px;
  }
  .sidebar nav {
    display: flex;
    gap: 6px;
    overflow-x: auto;
    padding: 8px 10px 10px;
    scrollbar-width: thin;
  }
  .nav-group {
    display: flex;
    margin: 0;
  }
  .nav-row {
    gap: 0;
  }
  .nav-item {
    margin-bottom: 0;
    min-height: 34px;
    white-space: nowrap;
  }
  .nav-parent {
    min-width: max-content;
  }
  .nav-toggle {
    width: 26px;
    height: 34px;
  }
  .nav-children {
    position: absolute;
    top: 100%;
    left: 10px;
    right: 10px;
    margin: 4px 0 0;
    padding: 6px;
    border: 1px solid rgba(148, 163, 184, .24);
    border-radius: 8px;
    background: #111827;
    z-index: 3;
    max-height: 42vh;
    overflow: auto;
  }
  .sidebar-footer {
    padding: 8px 12px 10px;
  }
  .main-wrap {
    min-height: 0;
  }
  .initial-password-banner {
    margin: 10px 12px 0;
    font-size: 12px;
    gap: 8px;
  }
  .warn-anchor {
    top: 66px;
    right: 12px;
  }
  .warn-anchor.with-initial-banner {
    top: 116px;
  }
  .warn-panel {
    width: min(92vw, 360px);
  }
}
</style>
