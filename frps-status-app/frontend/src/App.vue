<template>
  <RouterView v-if="isLogin" />
  <div v-else class="layout">
    <aside class="sidebar">
      <div class="sidebar-logo">
        <img :src="logoUrl" class="logo-icon" alt="FRPS状态监控" />
        FRPS状态监控
        <span class="logo-dot" :class="{ offline: !frpsOnline }"></span>
      </div>
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
        <RouterLink class="nav-item" to="/settings"><span class="nav-icon">⚙️</span> 系统配置</RouterLink>
      </nav>
      <div class="sidebar-footer">
        <span>{{ updatedAt || '加载中…' }}</span>
        <button class="logout-btn" type="button" @click="logout">退出</button>
      </div>
    </aside>
    <div class="main-wrap">
      <RouterView :status="status" :daily="daily" :loading="loading" @refresh="load" />
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { api } from './api/index.js'
import logoUrl from './assets/logo.svg'

const route = useRoute()
const router = useRouter()
const status = ref(null)
const daily = ref([])
const loading = ref(false)
const updatedAt = ref('')
const statsOpen = ref(false)

const isLogin = computed(() => route.path === '/login')
const frpsOnline = computed(() => status.value?.frps?.bind?.ok ?? false)
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

async function load() {
  loading.value = true
  try {
    const [s, d] = await Promise.all([api.getStatus(), api.getDaily()])
    status.value = s
    daily.value = d
    updatedAt.value = new Date(s.generated_at).toLocaleString('zh-CN', { timeStyle: 'short', dateStyle: 'short' })
  } catch (e) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

async function logout() {
  await api.logout()
  router.replace('/login')
}

let timer
onMounted(() => { if (!isLogin.value) load(); timer = setInterval(() => { if (!isLogin.value) load() }, 30000) })
onUnmounted(() => clearInterval(timer))
</script>
