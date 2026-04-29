<template>
  <div>
    <div class="page-header">
      <div>
        <div class="page-title">系统配置</div>
        <div class="page-sub">管理 FRPS 监控平台的全局参数与数据存储</div>
      </div>
    </div>

    <div class="page-body settings-page">
      <section class="settings-card threshold-card">
        <h3 class="settings-card-title">流量阈值设置</h3>
        <div class="threshold-inline">
          <label class="threshold-inline-item">
            <span>月上行 (GB)</span>
            <input v-model.number="form.alert_in_gb" type="number" min="0" step="0.1" />
          </label>
          <label class="threshold-inline-item">
            <span>月下行 (GB)</span>
            <input v-model.number="form.alert_out_gb" type="number" min="0" step="0.1" />
          </label>
          <label class="threshold-inline-item threshold-total">
            <span>上下行总量 (GB)</span>
            <input v-model.number="form.alert_total_gb" type="number" min="0" step="0.1" />
          </label>
          <button class="btn-solid-blue btn-threshold-save" :disabled="savingThreshold" @click="saveThreshold">
            {{ savingThreshold ? '保存中…' : '保存' }}
          </button>
        </div>
        <div v-if="saveMsg" class="alert mt-3" :class="saveMsg.ok ? 'alert-success' : 'alert-error'">{{ saveMsg.text }}</div>
      </section>

      <section class="settings-card event-alert-card">
        <h3 class="settings-card-title">事件告警</h3>
        <div class="event-rows">
          <div class="event-row">
            <span class="event-label">代理离线告警</span>
            <button class="switch-btn" :class="{ on: form.alert_proxy_offline === 'true' }" type="button" @click="toggleField('alert_proxy_offline')"><span></span></button>
          </div>
          <div class="event-row">
            <span class="event-label">SSL证书到期告警</span>
            <div class="event-row-right">
              <button class="switch-btn" :class="{ on: form.alert_cert_expiry === 'true' }" type="button" @click="toggleField('alert_cert_expiry')"><span></span></button>
              <template v-if="form.alert_cert_expiry === 'true'">
                <span class="event-sub">提前</span>
                <input v-model.number="form.alert_cert_days" type="number" min="1" max="90" class="event-days-input" />
                <span class="event-sub">天提醒</span>
              </template>
            </div>
          </div>
        </div>
        <div class="event-footer">
          <button class="btn-solid-blue btn-event-save" :disabled="savingEvents" @click="saveEvents">
            {{ savingEvents ? '保存中…' : '保存' }}
          </button>
          <div v-if="eventMsg" class="alert" :class="eventMsg.ok ? 'alert-success' : 'alert-error'">{{ eventMsg.text }}</div>
        </div>
      </section>

      <section class="settings-card notify-card">
        <div class="notify-left">
          <h3 class="settings-card-title notify-title">告警通知</h3>
          <div class="smtp-ready-box" :class="{ ok: smtpReady }">
            <span class="smtp-ready-dot"></span>
            <span>{{ smtpReady ? 'SMTP 已就绪' : 'SMTP 未就绪' }}</span>
          </div>
          <span class="smtp-current">{{ smtpRecipientsPreview }}</span>
        </div>
        <button class="btn-outline-blue notify-btn" @click="smtpModalOpen = true">配置邮件</button>
      </section>

      <section class="settings-card db-card">
        <h3 class="settings-card-title">数据库维护 (SQLite)</h3>
        <div class="db-line db-line-vacuum">
          <span>空间碎片整理 (VACUUM)：释放物理磁盘空间</span>
          <button class="btn-dark db-line-btn" :disabled="vacuuming" @click="doVacuum">{{ vacuuming ? '执行中…' : '执行' }}</button>
        </div>
        <div class="db-line db-line-purge">
          <div class="purge-inline">
            <span>自动清理策略：保留近</span>
            <input v-model.number="purgeDays" type="number" min="1" max="365" />
            <span>天历史记录</span>
          </div>
          <button class="btn-warn db-line-btn" :disabled="purging" @click="doPurge">{{ purging ? '执行中…' : '清理' }}</button>
        </div>
        <div v-if="vacuumMsg" class="alert mt-3" :class="vacuumMsg.ok ? 'alert-success' : 'alert-error'">{{ vacuumMsg.text }}</div>
        <div v-if="purgeMsg" class="alert mt-3" :class="purgeMsg.ok ? 'alert-success' : 'alert-error'">{{ purgeMsg.text }}</div>
      </section>
    </div>

    <div v-if="smtpModalOpen" class="smtp-mask" @click.self="smtpModalOpen = false">
      <div class="smtp-modal">
        <div class="smtp-modal-head">
          <div>
            <h3>配置邮件服务</h3>
          </div>
          <button class="smtp-close" type="button" @click="smtpModalOpen = false">×</button>
        </div>

        <div class="smtp-modal-body">
          <label class="smtp-line">
            <span class="smtp-label"><i>*</i> 名称</span>
            <div class="smtp-input-wrap">
              <input v-model.trim="form.smtp_user" type="text" placeholder="请输入邮箱名称" />
            </div>
          </label>

          <label class="smtp-line">
            <span class="smtp-label"><i>*</i> 发送人邮箱</span>
            <div class="smtp-input-wrap">
              <input v-model.trim="form.smtp_from" type="email" placeholder="请输入发送人邮箱" />
            </div>
          </label>

          <label class="smtp-line">
            <span class="smtp-label"><i>*</i> SMTP授权码</span>
            <div class="smtp-input-wrap password-row">
              <input
                v-model="form.smtp_auth_code"
                :type="showSMTPPass ? 'text' : 'password'"
                placeholder="请输入SMTP授权码"
              />
              <button class="pass-toggle" type="button" @click="showSMTPPass = !showSMTPPass">
                {{ showSMTPPass ? '隐藏' : '显示' }}
              </button>
            </div>
          </label>

          <label class="smtp-line">
            <span class="smtp-label"><i>*</i> SMTP服务器</span>
            <div class="smtp-input-wrap">
              <input v-model.trim="form.smtp_host" type="text" placeholder="请输入SMTP服务器" />
            </div>
          </label>

          <label class="smtp-line">
            <span class="smtp-label"><i>*</i> 端口</span>
            <div class="smtp-input-wrap">
              <input v-model.number="form.smtp_port" type="number" min="1" max="65535" placeholder="465" />
            </div>
          </label>

          <label class="smtp-line smtp-line-top">
            <span class="smtp-label"><i>*</i> 接收邮箱</span>
            <div class="smtp-input-wrap">
              <textarea v-model.trim="form.smtp_to" rows="3" placeholder="接收者邮箱，每行1个"></textarea>
            </div>
          </label>

          <div class="smtp-line">
            <span class="smtp-label">启用</span>
            <div class="smtp-input-wrap">
              <button class="switch-btn" :class="{ on: form.smtp_enabled === 'true' }" type="button" @click="toggleSMTP">
                <span></span>
              </button>
            </div>
          </div>

          <ul class="smtp-tips">
            <li>推荐使用465端口，协议为SSL/TLS</li>
            <li>25端口为SMTP协议，587端口为STARTTLS协议</li>
          </ul>

          <div v-if="smtpMsg" class="alert mt-3" :class="smtpMsg.ok ? 'alert-success' : 'alert-error'">
            {{ smtpMsg.text }}
          </div>
        </div>

        <div class="smtp-modal-foot">
          <button class="btn-test" :class="{ 'is-busy': testingEmail }" :disabled="testingEmail" @click="testEmail">
            <span v-if="testingEmail" class="btn-spinner"></span>
            {{ testingEmail ? '发送中…' : '发送测试邮件' }}
          </button>
          <div class="foot-right">
            <button class="btn-plain" @click="smtpModalOpen = false">取消</button>
            <button class="btn-dark" :class="{ 'is-busy': savingSMTP }" :disabled="savingSMTP" @click="saveSMTP">
              <span v-if="savingSMTP" class="btn-spinner"></span>
              {{ savingSMTP ? '确定中…' : '确定' }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { api } from '../api/index.js'

const props = defineProps({ status: Object })

const form = reactive({
  smtp_host: '',
  smtp_port: 465,
  smtp_user: '',
  smtp_auth_code: '',
  smtp_from: '',
  smtp_to: '',
  smtp_enabled: 'false',
  alert_in_gb: 0,
  alert_out_gb: 0,
  alert_total_gb: 0,
  alert_proxy_offline: 'false',
  alert_cert_expiry: 'false',
  alert_cert_days: 15
})

const showSMTPPass = ref(false)
const smtpModalOpen = ref(false)
const savingThreshold = ref(false)
const savingSMTP = ref(false)
const savingEvents = ref(false)
const testingEmail = ref(false)
const vacuuming = ref(false)
const purging = ref(false)
const purgeDays = ref(30)

const saveMsg = ref(null)
const smtpMsg = ref(null)
const eventMsg = ref(null)
const vacuumMsg = ref(null)
const purgeMsg = ref(null)

const smtpReady = computed(() =>
  Boolean(form.smtp_host && form.smtp_from && form.smtp_to && form.smtp_auth_code)
)

const smtpRecipientsPreview = computed(() => {
  if (!form.smtp_to) return '未设置'
  const list = form.smtp_to.split(',').map((s) => s.trim()).filter(Boolean)
  if (!list.length) return '未设置'
  return list.length > 1 ? `${list[0]} ...` : list[0]
})

function fillForm(s) {
  if (!s) return
  form.smtp_host = s.smtp_host || ''
  form.smtp_port = s.smtp_port || 465
  form.smtp_user = s.smtp_user || ''
  form.smtp_from = s.smtp_from || ''
  form.smtp_to = s.smtp_to || ''
  form.smtp_enabled = s.smtp_enabled ? 'true' : 'false'
  form.alert_in_gb = s.alert_in_gb || 0
  form.alert_out_gb = s.alert_out_gb || 0
  form.alert_total_gb = s.alert_total_gb || 0
  form.smtp_auth_code = s.smtp_auth_code || ''
  form.alert_proxy_offline = s.alert_proxy_offline ? 'true' : 'false'
  form.alert_cert_expiry = s.alert_cert_expiry ? 'true' : 'false'
  form.alert_cert_days = s.alert_cert_days || 15
}

watch(() => props.status?.settings, fillForm, { immediate: true })

function flash(msgRef, ok, text, ms = 4000) {
  msgRef.value = { ok, text }
  setTimeout(() => {
    msgRef.value = null
  }, ms)
}

function makePayload() {
  return { ...form }
}

function makeSMTPPayload() {
  return {
    smtp_host: form.smtp_host,
    smtp_port: form.smtp_port,
    smtp_user: form.smtp_user,
    smtp_auth_code: form.smtp_auth_code,
    smtp_from: form.smtp_from,
    smtp_to: form.smtp_to,
    smtp_enabled: form.smtp_enabled
  }
}

async function saveThreshold() {
  savingThreshold.value = true
  try {
    await api.saveSettings(makePayload())
    flash(saveMsg, true, '阈值已保存')
  } catch (e) {
    flash(saveMsg, false, '保存失败：' + e.message)
  } finally {
    savingThreshold.value = false
  }
}

async function saveSMTP() {
  savingSMTP.value = true
  try {
    const saved = await api.saveSettings(makeSMTPPayload())
    fillForm(saved)
    flash(smtpMsg, true, 'SMTP 配置已保存')
  } catch (e) {
    flash(smtpMsg, false, '保存失败：' + e.message)
  } finally {
    savingSMTP.value = false
  }
}

async function testEmail() {
  testingEmail.value = true
  smtpMsg.value = null
  try {
    const r = await api.testEmail()
    if (r.ok) {
      flash(smtpMsg, true, '测试邮件发送成功')
    } else {
      smtpMsg.value = { ok: false, text: '发送失败：' + r.error }
    }
  } catch (e) {
    smtpMsg.value = { ok: false, text: '发送失败：' + e.message }
  } finally {
    testingEmail.value = false
  }
}

function toggleSMTP() {
  form.smtp_enabled = form.smtp_enabled === 'true' ? 'false' : 'true'
}

function toggleField(key) {
  form[key] = form[key] === 'true' ? 'false' : 'true'
}

async function saveEvents() {
  savingEvents.value = true
  try {
    await api.saveSettings({
      alert_proxy_offline: form.alert_proxy_offline,
      alert_cert_expiry: form.alert_cert_expiry,
      alert_cert_days: form.alert_cert_days
    })
    flash(eventMsg, true, '事件告警配置已保存')
  } catch (e) {
    flash(eventMsg, false, '保存失败：' + e.message)
  } finally {
    savingEvents.value = false
  }
}

async function doVacuum() {
  vacuuming.value = true
  vacuumMsg.value = null
  try {
    await api.vacuum()
    flash(vacuumMsg, true, '数据库压缩完成')
  } catch (e) {
    flash(vacuumMsg, false, '压缩失败：' + e.message)
  } finally {
    vacuuming.value = false
  }
}

async function doPurge() {
  if (!confirm(`确定删除 ${purgeDays.value} 天前的流量记录？此操作不可恢复。`)) return
  purging.value = true
  purgeMsg.value = null
  try {
    const r = await api.purge(purgeDays.value)
    flash(purgeMsg, true, `已删除 ${r.deleted} 条记录`)
  } catch (e) {
    flash(purgeMsg, false, '清理失败：' + e.message)
  } finally {
    purging.value = false
  }
}
</script>

<style scoped>
.settings-page {
  background: #f8fafc;
  min-height: calc(100vh - 72px);
  display: grid;
  gap: 10px;
}

.settings-card {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  padding: 14px 16px;
}

.settings-card-title {
  font-size: 13px;
  font-weight: 600;
  color: #1e293b;
  margin-bottom: 8px;
}

.threshold-inline {
  display: flex;
  align-items: center;
  gap: 20px;
  flex-wrap: wrap;
}

.threshold-inline-item {
  display: flex;
  align-items: center;
  gap: 8px;
}

.threshold-inline-item span {
  font-size: 13px;
  color: #64748b;
  white-space: nowrap;
}

.threshold-inline-item input {
  height: 28px;
  width: 100px;
  border-radius: 4px;
  border: 1px solid #d1d5db;
  padding: 0 12px;
}

.threshold-total input {
  width: 110px;
}

.btn-threshold-save {
  margin-left: auto;
  min-width: 60px;
  height: 28px;
  padding: 0 12px;
  border-radius: 4px;
  font-size: 12px;
}

.btn-solid-blue,
.btn-outline-blue,
.btn-dark,
.btn-warn,
.btn-plain,
.btn-test {
  height: 32px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 600;
  padding: 0 16px;
  cursor: pointer;
}

.btn-solid-blue {
  border: 0;
  background: #2563eb;
  color: #fff;
}

.btn-outline-blue {
  border: 1px solid #2563eb;
  background: #fff;
  color: #2563eb;
}

.smtp-ready-box {
  height: 26px;
  border-radius: 8px;
  border: 1px solid #fee2e2;
  background: #fef2f2;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  color: #991b1b;
  font-size: 12px;
  font-weight: 500;
}

.smtp-ready-box.ok {
  background: #f0fdf4;
  border-color: #bbf7d0;
  color: #166534;
}

.smtp-ready-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}

.notify-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.notify-left {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.notify-title {
  margin: 0;
}

.smtp-current {
  margin-top: 0;
  color: #64748b;
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.notify-btn {
  height: 28px;
  border-radius: 6px;
  padding: 0 12px;
}

.db-card {
  margin-bottom: 0;
}

.db-line {
  min-height: 30px;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 0 10px;
  margin-top: 5px;
}

.db-line-vacuum {
  background: #f8fafc;
  color: #334155;
  font-size: 13px;
}

.db-line-purge {
  background: #fffbeb;
  color: #92400e;
}

.btn-dark {
  border: 0;
  background: #334155;
  color: #fff;
}

.purge-inline {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}

.purge-inline input {
  width: 40px;
  height: 24px;
  border-radius: 4px;
  border: 1px solid #d97706;
  text-align: center;
  background: #fff;
}

.btn-warn {
  border: 0;
  background: #d97706;
  color: #fff;
}

.db-line-btn {
  height: 24px;
  border-radius: 4px;
  padding: 0 10px;
  font-size: 11px;
}

.smtp-mask {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, .4);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  z-index: 60;
}

.smtp-modal {
  width: min(640px, 100%);
  max-height: 90vh;
  border-radius: 16px;
  background: #fff;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.smtp-modal-head {
  height: 70px;
  border-bottom: 1px solid #f1f5f9;
  padding: 0 24px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.smtp-modal-head h3 {
  font-size: 18px;
  font-weight: 700;
  color: #1e293b;
}

.smtp-close {
  border: 0;
  background: transparent;
  color: #94a3b8;
  font-size: 24px;
  line-height: 1;
  cursor: pointer;
}

.smtp-modal-body {
  padding: 20px 24px 12px;
  overflow-y: auto;
  flex: 1;
}

.smtp-line {
  display: grid;
  grid-template-columns: 112px minmax(0, 1fr);
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
}

.smtp-line-top {
  align-items: flex-start;
}

.smtp-label {
  text-align: right;
  color: #475569;
  font-size: 14px;
}

.smtp-label i {
  color: #ef4444;
  font-style: normal;
  margin-right: 4px;
}

.smtp-input-wrap {
  width: 100%;
}

.smtp-input-wrap input,
.smtp-input-wrap textarea {
  width: 100%;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  padding: 0 12px;
  font: inherit;
}

.smtp-input-wrap input {
  height: 36px;
}

.smtp-input-wrap textarea {
  resize: vertical;
  min-height: 80px;
  padding-top: 9px;
  padding-bottom: 9px;
}

.password-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 62px;
  gap: 10px;
}

.pass-toggle {
  height: 36px;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  background: #fff;
  color: #64748b;
  font-size: 12px;
  cursor: pointer;
}

.switch-btn {
  width: 50px;
  height: 24px;
  border: 0;
  border-radius: 999px;
  background: #cbd5e1;
  padding: 0 3px;
  display: flex;
  align-items: center;
  cursor: pointer;
  transition: background .15s;
}

.switch-btn span {
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: #fff;
  transform: translateX(0);
  transition: transform .15s;
}

.switch-btn.on {
  background: #10b981;
}

.switch-btn.on span {
  transform: translateX(26px);
}

.smtp-tips {
  margin: 4px 0 10px 124px;
  color: #64748b;
  font-size: 13px;
  line-height: 1.7;
}

.smtp-modal-foot {
  background: #f8fafc;
  height: 70px;
  padding: 0 24px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.btn-test,
.btn-plain {
  border: 1px solid #cbd5e1;
  background: #fff;
  color: #475569;
}

.foot-right {
  display: flex;
  gap: 10px;
}


.btn-test,
.btn-dark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  transition: transform .2s ease, box-shadow .2s ease, opacity .2s ease;
}

.btn-test.is-busy,
.btn-dark.is-busy {
  transform: translateY(-1px);
  box-shadow: 0 4px 10px rgba(15, 23, 42, .16);
}

.btn-spinner {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  border: 2px solid currentColor;
  border-right-color: transparent;
  animation: btnspin .7s linear infinite;
}

@keyframes btnspin {
  to { transform: rotate(360deg); }
}

button:disabled {
  opacity: .65;
  cursor: not-allowed;
}

.event-rows {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.event-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 30px;
  gap: 12px;
}

.event-label {
  font-size: 13px;
  color: #334155;
}

.event-row-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.event-sub {
  font-size: 12px;
  color: #64748b;
  white-space: nowrap;
}

.event-days-input {
  width: 52px;
  height: 26px;
  border: 1px solid #cbd5e1;
  border-radius: 4px;
  padding: 0 6px;
  text-align: center;
  font: inherit;
  font-size: 13px;
}

.event-footer {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 10px;
}

.btn-event-save {
  height: 26px;
  padding: 0 14px;
  border-radius: 4px;
  font-size: 12px;
  flex-shrink: 0;
}

.event-footer .alert {
  margin: 0;
  padding: 4px 10px;
  font-size: 12px;
}

@media (max-width: 980px) {
  .notify-card,
  .notify-left {
    align-items: flex-start;
    flex-direction: column;
  }

  .db-line {
    flex-direction: column;
    align-items: flex-start;
    padding: 8px 12px;
  }
}

@media (max-width: 640px) {
  .threshold-inline,
  .smtp-line {
    gap: 10px;
  }

  .threshold-inline-item {
    width: 100%;
    justify-content: space-between;
  }

  .btn-threshold-save {
    width: 100%;
    margin-left: 0;
  }

  .smtp-label {
    text-align: left;
  }

  .smtp-tips {
    margin-left: 18px;
  }
}
</style>
