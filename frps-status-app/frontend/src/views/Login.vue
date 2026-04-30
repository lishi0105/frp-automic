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
      <form v-if="!showForgot" class="login-panel" @submit.prevent="submit">
        <img :src="logoUrl" class="login-panel-logo" alt="FRPS状态监控" />
        <h2>管理端登录</h2>

        <label class="login-field">
          <span>管理员账号</span>
          <input v-model.trim="form.username" type="text" autocomplete="username" placeholder="请输入用户名" />
        </label>

        <label class="login-field">
          <span>密码</span>
          <input v-model="form.password" type="password" autocomplete="current-password" placeholder="请输入密码" />
        </label>

        <p v-if="error" class="login-error">{{ error }}</p>

        <button class="login-submit" type="submit" :disabled="loading">
          {{ loading ? '登录中…' : '进入控制台' }}
        </button>

        <p class="login-forgot-link">
          <button type="button" class="link-btn" @click="showForgot = true">忘记密码？</button>
        </p>

        <p class="login-version">安全管理后台 v0.1</p>
      </form>

      <form v-else class="login-panel" @submit.prevent="submitForgot">
        <img :src="logoUrl" class="login-panel-logo" alt="FRPS状态监控" />
        <h2>找回密码</h2>
        <p class="forgot-hint">输入您预设的找回邮箱，系统将发送重置后的账户凭据。</p>

        <label class="login-field">
          <span>找回邮箱</span>
          <input v-model.trim="forgotEmail" type="email" autocomplete="email" placeholder="your@email.com" />
        </label>

        <p v-if="forgotError" class="login-error">{{ forgotError }}</p>
        <p v-if="forgotSuccess" class="login-success">{{ forgotSuccess }}</p>

        <button class="login-submit" type="submit" :disabled="forgotLoading">
          {{ forgotLoading ? '发送中…' : '发送重置邮件' }}
        </button>

        <p class="login-forgot-link">
          <button type="button" class="link-btn" @click="showForgot = false">返回登录</button>
        </p>
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
const form = reactive({ username: '', password: '' })

const showForgot = ref(false)
const forgotEmail = ref('')
const forgotLoading = ref(false)
const forgotError = ref('')
const forgotSuccess = ref('')

async function submit() {
  error.value = ''
  loading.value = true
  try {
    const result = await api.login(form)
    if (result.force_change) {
      router.replace('/?force_change=1')
    } else {
      router.replace('/')
    }
  } catch {
    error.value = '用户名或密码不正确'
  } finally {
    loading.value = false
  }
}

async function submitForgot() {
  forgotError.value = ''
  forgotSuccess.value = ''
  if (!forgotEmail.value) {
    forgotError.value = '请输入找回邮箱'
    return
  }
  forgotLoading.value = true
  try {
    await api.forgotPassword({ email: forgotEmail.value })
    forgotSuccess.value = '重置成功，请查收邮件获取新凭据'
    forgotEmail.value = ''
  } catch (e) {
    forgotError.value = e.message || '发送失败，请稍后重试'
  } finally {
    forgotLoading.value = false
  }
}
</script>

<style scoped>
.login-forgot-link {
  text-align: center;
  margin-top: 4px;
}
.link-btn {
  background: none;
  border: none;
  color: var(--primary, #6366f1);
  font-size: 13px;
  cursor: pointer;
  padding: 0;
  text-decoration: underline;
}
.forgot-hint {
  color: var(--text-2, #94a3b8);
  font-size: 13px;
  margin-bottom: 16px;
  line-height: 1.5;
}
.login-success {
  color: #4ade80;
  font-size: 13px;
  margin: 0;
}
</style>
