<template>
  <Teleport to="body">
    <Transition name="account-modal">
      <div v-if="modelValue" class="account-modal-overlay">
        <section class="account-modal" role="dialog" aria-modal="true" aria-labelledby="account-title">
          <header class="account-modal-head">
            <div class="account-title-block">
              <span class="account-avatar">{{ userInitial }}</span>
              <div>
                <h2 id="account-title">账户设置</h2>
                <p>管理登录凭据与找回邮箱</p>
              </div>
            </div>
            <button class="icon-btn" type="button" aria-label="关闭" @click="requestClose">×</button>
          </header>

          <div class="account-modal-body">
            <aside class="account-summary">
              <div class="summary-name">{{ currentUsername || '管理员' }}</div>
              <div class="summary-row">
                <span>登录用户名</span>
                <b>{{ currentUsername || '-' }}</b>
              </div>
              <div class="summary-row">
                <span>找回邮箱</span>
                <b>{{ recoveryEmail || '未设置' }}</b>
              </div>
              <p class="summary-note">修改用户名或密码时需要验证当前密码；仅修改用户名时，新密码可留空。</p>
            </aside>

            <main class="account-settings">
              <section class="settings-section">
                <div class="section-heading">
                  <h3>登录凭据</h3>
                  <span>Username & Password</span>
                </div>

                <div class="account-form">
                  <label class="account-field">
                    <span>新用户名</span>
                    <input
                      v-model.trim="creds.username"
                      type="text"
                      autocomplete="username"
                      placeholder="留空保持不变"
                      :class="{ 'input-error': usernameError }"
                    />
                    <small v-if="usernameError" class="field-error">{{ usernameError }}</small>
                  </label>

                  <label class="account-field">
                    <span>当前密码 <em>*</em></span>
                    <input v-model="creds.currentPassword" type="password" autocomplete="current-password" placeholder="用于确认身份" />
                  </label>

                  <div class="account-field account-field-wide">
                    <label class="field-label">
                      <span>新密码</span>
                      <input
                        v-model="creds.newPassword"
                        type="password"
                        autocomplete="new-password"
                        placeholder="留空则不修改密码"
                        :class="{ 'input-error': creds.newPassword && pwErrors.length }"
                        @focus="showRules = true"
                      />
                    </label>

                    <div v-if="showRules || creds.newPassword" class="pw-rules">
                      <div
                        v-for="rule in passwordRules"
                        :key="rule.key"
                        class="pw-rule"
                        :class="rule.ok(creds.newPassword) ? 'rule-ok' : (creds.newPassword ? 'rule-fail' : 'rule-idle')"
                      >
                        <span class="rule-icon">{{ rule.ok(creds.newPassword) ? '✓' : (creds.newPassword ? '×' : '○') }}</span>
                        <span>{{ rule.label }}</span>
                      </div>
                    </div>
                  </div>

                  <label class="account-field account-field-wide">
                    <span>确认新密码</span>
                    <input
                      v-model="creds.confirmPassword"
                      type="password"
                      autocomplete="new-password"
                      placeholder="再次输入新密码"
                      :class="{ 'input-error': creds.confirmPassword && creds.newPassword !== creds.confirmPassword }"
                    />
                    <small v-if="creds.confirmPassword && creds.newPassword !== creds.confirmPassword" class="field-error">两次输入的密码不一致</small>
                  </label>
                </div>

                <div class="username-rules">
                  用户名支持英文字母、数字及 <code>*@#!()-_</code>，长度 3-32 位
                </div>

                <div class="account-actions">
                  <button class="btn btn-primary btn-sm" :disabled="savingCreds || !canSave" @click="saveCreds">
                    {{ savingCreds ? '保存中...' : '保存凭据' }}
                  </button>
                  <div v-if="credsMsg" class="alert" :class="credsMsg.ok ? 'alert-success' : 'alert-error'">{{ credsMsg.text }}</div>
                </div>
              </section>

              <section class="settings-section">
                <div class="section-heading">
                  <h3>找回邮箱</h3>
                  <span>Recovery Email</span>
                </div>
                <div class="account-form single">
                  <label class="account-field account-field-wide">
                    <span>邮箱地址</span>
                    <input v-model.trim="recoveryEmail" type="email" autocomplete="email" placeholder="your@email.com" />
                  </label>
                </div>

                <div class="account-actions">
                  <button class="btn btn-outline btn-sm" :disabled="savingEmail" @click="saveEmail">
                    {{ savingEmail ? '保存中...' : '保存邮箱' }}
                  </button>
                  <div v-if="emailMsg" class="alert" :class="emailMsg.ok ? 'alert-success' : 'alert-error'">{{ emailMsg.text }}</div>
                </div>
              </section>
            </main>
          </div>
        </section>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup>
import { reactive, ref, computed, watch } from 'vue'
import { api } from '../api/index.js'

const props = defineProps({ modelValue: Boolean })
const emit = defineEmits(['update:modelValue', 'saved'])

const ALLOWED_SPECIAL = new Set([...'*@#!()-_'])

function isAlpha(ch) { return /[a-zA-Z]/.test(ch) }
function isDigit(ch) { return /[0-9]/.test(ch) }
function isAllowed(ch) { return isAlpha(ch) || isDigit(ch) || ALLOWED_SPECIAL.has(ch) }
function hasInvalidChar(str) { return str.split('').some(ch => !isAllowed(ch)) }

function hasTripleRepeat(str) {
  for (let i = 2; i < str.length; i++) {
    if (str[i] === str[i - 1] && str[i - 1] === str[i - 2]) return true
  }
  return false
}

function hasTripleSequential(str) {
  for (let i = 2; i < str.length; i++) {
    const a = str[i - 2], b = str[i - 1], c = str[i]
    const bothLetters = isAlpha(a) && isAlpha(b) && isAlpha(c)
    const bothDigits = isDigit(a) && isDigit(b) && isDigit(c)
    if (bothLetters || bothDigits) {
      const ac = a.toLowerCase().charCodeAt(0)
      const bc = b.toLowerCase().charCodeAt(0)
      const cc = c.toLowerCase().charCodeAt(0)
      if ((bc === ac + 1 && cc === bc + 1) || (bc === ac - 1 && cc === bc - 1)) return true
    }
  }
  return false
}

const passwordRules = [
  { key: 'length', label: '长度大于 8 位且小于 32 位', ok: p => p.length > 8 && p.length < 32 },
  { key: 'charset', label: '仅含英文字母、数字及 *@#!()-_', ok: p => p.length > 0 && !hasInvalidChar(p) },
  { key: 'letter', label: '包含英文字母', ok: p => p.split('').some(isAlpha) },
  { key: 'digit', label: '包含数字', ok: p => p.split('').some(isDigit) },
  { key: 'repeat', label: '无 3 个及以上相同连续字符', ok: p => p.length > 0 && !hasTripleRepeat(p) },
  { key: 'seq', label: '无 3 个及以上连续字符', ok: p => p.length > 0 && !hasTripleSequential(p) },
]

const currentUsername = ref('')
const recoveryEmail = ref('')
const showRules = ref(false)
const savingCreds = ref(false)
const savingEmail = ref(false)
const credsMsg = ref(null)
const emailMsg = ref(null)
const savedUser = ref(null)

const creds = reactive({
  username: '',
  currentPassword: '',
  newPassword: '',
  confirmPassword: ''
})

const userInitial = computed(() => (currentUsername.value || 'U').slice(0, 1).toUpperCase())
const pwErrors = computed(() => creds.newPassword ? passwordRules.filter(r => !r.ok(creds.newPassword)) : [])
const usernameError = computed(() => validateUsernameStr(creds.username))
const canSave = computed(() => {
  if (!creds.currentPassword) return false
  if (usernameError.value) return false
  if (creds.newPassword) {
    if (pwErrors.value.length) return false
    if (creds.newPassword !== creds.confirmPassword) return false
  }
  return true
})

watch(() => props.modelValue, (open) => {
  if (open) loadUser()
})

function validateUsernameStr(name) {
  if (!name) return ''
  if (name.length < 3 || name.length > 32) return '用户名长度必须在 3 到 32 位之间'
  if (hasInvalidChar(name)) return '用户名只能包含英文字母、数字及 *@#!()-_ 特殊字符'
  return ''
}

function requestClose() {
  resetDraft()
  emit('update:modelValue', false)
}

function applyUser(u) {
  if (!u) return
  currentUsername.value = u.username || ''
  recoveryEmail.value = u.recovery_email || ''
  creds.username = currentUsername.value
}

function resetDraft() {
  applyUser(savedUser.value)
  creds.currentPassword = ''
  creds.newPassword = ''
  creds.confirmPassword = ''
  showRules.value = false
  credsMsg.value = null
  emailMsg.value = null
}

function flash(msgRef, ok, text, ms = 4000) {
  msgRef.value = { ok, text }
  setTimeout(() => { msgRef.value = null }, ms)
}

async function loadUser() {
  try {
    const u = await api.getUser()
    savedUser.value = u
    applyUser(u)
    creds.currentPassword = ''
    creds.newPassword = ''
    creds.confirmPassword = ''
    showRules.value = false
  } catch {
    // session guard handles redirect
  }
}

async function saveCreds() {
  if (!canSave.value) return
  savingCreds.value = true
  try {
    await api.changeCredentials({
      username: creds.username || currentUsername.value,
      current_password: creds.currentPassword,
      new_password: creds.newPassword
    })
    currentUsername.value = creds.username || currentUsername.value
    savedUser.value = {
      ...(savedUser.value || {}),
      username: currentUsername.value,
      recovery_email: recoveryEmail.value,
      is_initial_password: false
    }
    creds.currentPassword = ''
    creds.newPassword = ''
    creds.confirmPassword = ''
    showRules.value = false
    flash(credsMsg, true, '凭据已更新')
    emit('saved', { username: currentUsername.value, is_initial_password: false })
  } catch (e) {
    flash(credsMsg, false, e.message || '保存失败')
  } finally {
    savingCreds.value = false
  }
}

async function saveEmail() {
  savingEmail.value = true
  try {
    await api.changeRecoveryEmail({ recovery_email: recoveryEmail.value })
    savedUser.value = {
      ...(savedUser.value || {}),
      username: currentUsername.value,
      recovery_email: recoveryEmail.value
    }
    flash(emailMsg, true, '找回邮箱已保存')
    emit('saved', { username: currentUsername.value, recovery_email: recoveryEmail.value })
  } catch (e) {
    flash(emailMsg, false, e.message || '保存失败')
  } finally {
    savingEmail.value = false
  }
}
</script>

<style scoped>
.account-modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 500;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  background: rgba(15, 23, 42, .56);
  color: #0f172a;
  --surface: #ffffff;
  --surface-2: #f8fafc;
  --border: #e2e8f0;
  --text: #0f172a;
  --text-2: #475569;
  --text-3: #94a3b8;
  --primary: #2563eb;
  --danger: #ef4444;
  --r: 8px;
  --r-sm: 4px;
}

.account-modal {
  width: min(860px, 100%);
  overflow: visible;
  display: flex;
  flex-direction: column;
  background: #fff;
  border: 1px solid #cbd5e1;
  border-radius: 10px;
  box-shadow: 0 24px 80px rgba(15, 23, 42, .34);
}

.account-modal-head {
  padding: 14px 18px;
  border-bottom: 1px solid var(--border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.account-title-block { display: flex; align-items: center; gap: 12px; min-width: 0; }
.account-title-block h2 { font-size: 18px; line-height: 1.2; font-weight: 700; }
.account-title-block p { margin-top: 3px; color: var(--text-2); font-size: 12px; }
.account-avatar {
  width: 40px;
  height: 40px;
  border-radius: 8px;
  background: #0f172a;
  color: #fff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
}
.icon-btn {
  width: 32px;
  height: 32px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: #fff;
  color: var(--text-2);
  font-size: 20px;
  line-height: 1;
  cursor: pointer;
}
.icon-btn:hover { background: var(--surface-2); color: var(--text); }

.account-modal-body {
  display: grid;
  grid-template-columns: 220px 1fr;
  overflow: visible;
}
.account-summary {
  padding: 16px 18px;
  border-right: 1px solid var(--border);
  background: #f8fafc;
}
.summary-name {
  font-size: 18px;
  font-weight: 700;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.summary-row {
  margin-top: 12px;
  display: grid;
  gap: 4px;
  padding-top: 10px;
  border-top: 1px solid var(--border);
}
.summary-row span { color: var(--text-2); font-size: 12px; }
.summary-row b {
  color: var(--text);
  font-size: 13px;
  font-weight: 600;
  overflow-wrap: anywhere;
}
.summary-note {
  margin-top: 14px;
  color: var(--text-2);
  font-size: 12px;
  line-height: 1.6;
}

.account-settings {
  display: grid;
  gap: 12px;
  padding: 16px 18px;
}
.settings-section {
  border: 1px solid var(--border);
  border-radius: var(--r);
  padding: 14px;
  background: #fff;
}
.section-heading {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}
.section-heading h3 { font-size: 15px; font-weight: 700; }
.section-heading span { color: var(--text-3); font-size: 11px; text-transform: uppercase; }

.account-form { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
.account-form.single { grid-template-columns: 1fr; }
.account-field,
.field-label { display: grid; gap: 6px; }
.account-field-wide { grid-column: 1 / -1; }
.account-field span,
.field-label span { color: var(--text-2); font-size: 12px; font-weight: 600; }
.account-field em { color: var(--danger); font-style: normal; }

.account-field input,
.field-label input {
  width: 100%;
  height: 36px;
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 0 11px;
  background: #fff;
  color: var(--text);
  font: inherit;
  font-size: 13px;
  outline: none;
  transition: border-color .15s, box-shadow .15s;
}
.account-field input:focus,
.field-label input:focus {
  border-color: var(--primary);
  box-shadow: 0 0 0 3px rgba(37, 99, 235, .12);
}
.input-error { border-color: var(--danger) !important; }
.field-error { color: #dc2626; font-size: 11px; }

.pw-rules {
  margin-top: 8px;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 4px 12px;
  padding: 8px 10px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: 6px;
}
.pw-rule {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11.5px;
  line-height: 1.4;
}
.rule-icon { width: 14px; flex-shrink: 0; font-weight: 700; }
.rule-ok { color: #059669; }
.rule-fail { color: #dc2626; }
.rule-idle { color: var(--text-3); }

.username-rules {
  margin-top: 8px;
  font-size: 11.5px;
  color: var(--text-2);
}
.username-rules code {
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: 4px;
  padding: 0 4px;
}

.account-actions {
  margin-top: 10px;
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.alert { padding: 6px 10px; border-radius: 6px; font-size: 12px; }
.account-modal-enter-active,
.account-modal-leave-active {
  transition: opacity .15s ease;
}
.account-modal-enter-from,
.account-modal-leave-to {
  opacity: 0;
}

@media (max-width: 760px) {
  .account-modal-overlay { padding: 12px; }
  .account-modal { max-height: calc(100vh - 24px); }
  .account-modal-body { grid-template-columns: 1fr; }
  .account-summary { border-right: 0; border-bottom: 1px solid var(--border); }
  .account-form,
  .pw-rules { grid-template-columns: 1fr; }
}
</style>
