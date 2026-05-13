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
        <h1>内网穿透运维工具</h1>
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
          <span class="password-field">
            <input
              v-model="form.password"
              :type="showPassword ? 'text' : 'password'"
              autocomplete="current-password"
              placeholder="请输入密码"
            />
            <button
              type="button"
              class="password-toggle"
              :aria-label="showPassword ? '隐藏密码' : '显示密码'"
              :title="showPassword ? '隐藏密码' : '显示密码'"
              @click="showPassword = !showPassword"
            >
              <svg v-if="!showPassword" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                <path d="M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 0 1 0 .696 10.75 10.75 0 0 1-19.876 0" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>
                <circle cx="12" cy="12" r="3" stroke="currentColor" stroke-width="1.8"/>
              </svg>
              <svg v-else viewBox="0 0 24 24" fill="none" aria-hidden="true">
                <path d="m10.477 5.08-.12.02a10.75 10.75 0 0 0-8.295 6.553 1 1 0 0 0 0 .694 10.75 10.75 0 0 0 14.708 5.79" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>
                <path d="m14.084 14.158.01-.01a3 3 0 0 0-4.242-4.243" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>
                <path d="M2 2l20 20" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/>
                <path d="M20.94 12.35a10.75 10.75 0 0 0-5.44-5.1" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
            </button>
          </span>
        </label>

        <p v-if="error" class="login-error">{{ error }}</p>

        <button class="login-submit" type="submit" :disabled="loading">
          {{ loading ? '登录中…' : '进入控制台' }}
        </button>

        <p class="login-forgot-link">
          <button type="button" class="link-btn" @click="showForgot = true">忘记密码？</button>
        </p>

        <p class="login-version">内网穿透运维工具 v1.0.0</p>
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
const showPassword = ref(false)

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
    sessionStorage.setItem('frps_status_logged_in', '1')
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
.password-field {
  position: relative;
  display: block;
}
.password-field input {
  padding-right: 42px;
  width: 100%;
}
.password-toggle {
  position: absolute;
  top: 50%;
  right: 8px;
  transform: translateY(-50%);
  width: 30px;
  height: 30px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--text-2, #6b7280);
  padding: 0;
  line-height: 1;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: color .15s ease, background-color .15s ease;
}
.password-toggle svg {
  width: 17px;
  height: 17px;
}
.password-toggle:hover {
  color: var(--text, #0f172a);
  background: rgba(148, 163, 184, .16);
}
.password-toggle:focus-visible {
  outline: 2px solid rgba(37, 99, 235, .45);
  outline-offset: 1px;
}
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
