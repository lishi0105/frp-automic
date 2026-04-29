<template>
  <main class="login-page">
    <section class="login-hero">
      <div class="login-pattern" aria-hidden="true">
        <div class="trace trace-a"></div>
        <div class="trace trace-b"></div>
        <div class="hex"></div>
      </div>
      <div class="login-hero-content">
        <div class="login-hero-brand">
          <img :src="logoUrl" alt="FRPS状态监控" />
          <span>FRPS状态监控</span>
        </div>
        <h1>FRPS 综合管理平台</h1>
        <p>SSL证书 · 流量告警 · 历史统计</p>
        <ul class="login-feature-list">
          <li><span class="feature-dot blue"></span>自动化证书签发与续期</li>
          <li><span class="feature-dot green"></span>自定义流量阈值监控</li>
          <li><span class="feature-dot amber"></span>SMTP 邮件事件告警</li>
        </ul>
      </div>
    </section>

    <section class="login-panel-wrap">
      <form class="login-panel" @submit.prevent="submit">
        <img :src="logoUrl" class="login-panel-logo" alt="FRPS状态监控" />
        <h2>管理端登录</h2>

        <label class="login-field">
          <span>管理员账号</span>
          <input v-model.trim="form.username" type="text" autocomplete="username" placeholder="admin" />
        </label>

        <label class="login-field">
          <span>密码</span>
          <input v-model="form.password" type="password" autocomplete="current-password" placeholder="••••••••••••" />
        </label>

        <p v-if="error" class="login-error">{{ error }}</p>

        <button class="login-submit" type="submit" :disabled="loading">
          {{ loading ? '登录中…' : '进入控制台' }}
        </button>

        <p class="login-version">安全管理后台 v0.1</p>
      </form>
    </section>
  </main>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/index.js'
import logoUrl from '../assets/logo.svg'

const router = useRouter()
const loading = ref(false)
const error = ref('')
const form = reactive({
  username: '',
  password: ''
})

async function submit() {
  error.value = ''
  loading.value = true
  try {
    await api.login(form)
    router.replace('/')
  } catch (e) {
    error.value = '用户名或密码不正确'
  } finally {
    loading.value = false
  }
}
</script>
