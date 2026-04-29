<template>
  <RouterView v-if="isLogin" />
  <div v-else class="layout">
    <aside class="sidebar">
      <div class="sidebar-logo">
        <span class="logo-dot" :class="{ offline: !frpsOnline }"></span>
        FRPS Status
      </div>
      <nav>
        <RouterLink class="nav-item" to="/"><span class="nav-icon">📊</span> 数据看板</RouterLink>
        <RouterLink class="nav-item" to="/proxies"><span class="nav-icon">🔗</span> 代理列表</RouterLink>
        <RouterLink class="nav-item" to="/statistics"><span class="nav-icon">📈</span> 历史统计</RouterLink>
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
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { api } from './api/index.js'

const route = useRoute()
const router = useRouter()
const status = ref(null)
const daily = ref([])
const loading = ref(false)
const updatedAt = ref('')

const isLogin = computed(() => route.path === '/login')
const frpsOnline = computed(() => status.value?.frps?.bind?.ok ?? false)

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
