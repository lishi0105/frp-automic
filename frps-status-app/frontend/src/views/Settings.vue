<template>
  <div>
    <div class="page-header">
      <div>
        <div class="page-title">系统配置</div>
        <div class="page-sub">管理 FRPS 监控平台的全局参数与数据存储</div>
      </div>
      <button class="btn btn-outline btn-sm" @click="$emit('refresh')">↻ 刷新</button>
    </div>

    <div class="page-body settings-page">
      <section class="analytics-overview">
        <div>
          <div class="section-title">告警配置概览</div>
          <div class="text-muted text-sm">月流量阈值、事件告警、SMTP 与数据库维护策略</div>
          <div class="analytics-overview-bar"><div class="analytics-overview-bar-inner settings-overview-fill"></div></div>
        </div>
        <div class="analytics-overview-metrics">
          <div><b>{{ form.alert_in_gb || 0 }} GB</b><small>月上行阈值</small></div>
          <div><b>{{ form.alert_out_gb || 0 }} GB</b><small>月下行阈值</small></div>
          <div><b>SMTP</b><small>{{ form.smtp_enabled === 'true' ? '已启用' : '未启用' }}</small></div>
        </div>
      </section>

      <div class="settings-grid">
        <section class="settings-card threshold-card">
          <h3 class="settings-card-title">流量阈值设置</h3>
          <div class="threshold-grid">
            <label class="threshold-item">
              <span>月上行 (GB)</span>
              <input v-model.number="form.alert_in_gb" type="number" min="0" step="0.1" />
            </label>
            <label class="threshold-item">
              <span>月下行 (GB)</span>
              <input v-model.number="form.alert_out_gb" type="number" min="0" step="0.1" />
            </label>
            <label class="threshold-item threshold-span">
              <span>上下行总量 (GB)</span>
              <input v-model.number="form.alert_total_gb" type="number" min="0" step="0.1" />
            </label>
          </div>
          <div class="section-actions">
            <button class="btn btn-dark btn-sm" :disabled="savingThreshold" @click="saveThreshold">
              {{ savingThreshold ? '保存中…' : '保存阈值' }}
            </button>
            <div v-if="saveMsg" class="alert" :class="saveMsg.ok ? 'alert-success' : 'alert-error'">{{ saveMsg.text }}</div>
          </div>
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
                  <span class="event-sub">天</span>
                </template>
              </div>
            </div>
          </div>
          <div class="section-actions">
            <button class="btn btn-dark btn-sm" :disabled="savingEvents" @click="saveEvents">
              {{ savingEvents ? '保存中…' : '保存策略' }}
            </button>
            <div v-if="eventMsg" class="alert" :class="eventMsg.ok ? 'alert-success' : 'alert-error'">{{ eventMsg.text }}</div>
          </div>
        </section>

        <section class="settings-card notify-card">
          <h3 class="settings-card-title notify-title">告警通知 (SMTP)</h3>
          <div class="notify-line">
            <div class="smtp-ready-box" :class="{ ok: smtpReady }">
              <span class="smtp-ready-dot"></span>
              <span>{{ smtpReady ? 'SMTP 已就绪' : 'SMTP 未就绪' }}</span>
            </div>
            <span class="smtp-current">{{ smtpRecipientsPreview }}</span>
          </div>
          <div class="notify-foot">
            <button class="btn btn-outline btn-sm" :disabled="testingEmail" @click="testEmail">{{ testingEmail ? '发送中…' : '测试邮件' }}</button>
            <button class="btn btn-dark btn-sm" @click="smtpModalOpen = true">配置邮件</button>
          </div>
          <div v-if="smtpMsg" class="alert mt-3" :class="smtpMsg.ok ? 'alert-success' : 'alert-error'">{{ smtpMsg.text }}</div>
        </section>

        <section class="settings-card db-card">
          <h3 class="settings-card-title">数据库维护</h3>
          <div class="db-line db-line-vacuum">
            <div>
              <div class="db-title">空间碎片整理 (VACUUM)</div>
              <div class="text-muted text-sm">释放数据库文件空间，减少碎片</div>
            </div>
            <button class="btn btn-dark btn-sm" :disabled="vacuuming" @click="doVacuum">{{ vacuuming ? '执行中…' : '执行' }}</button>
          </div>
          <div class="db-line db-line-purge">
            <div class="purge-inline">
              <span>保留近</span>
              <input v-model.number="purgeDays" type="number" min="1" max="365" />
              <span>天历史记录</span>
            </div>
            <button class="btn btn-sm btn-warn" :disabled="purging" @click="doPurge">{{ purging ? '执行中…' : '清理' }}</button>
          </div>
          <div v-if="vacuumMsg" class="alert mt-3" :class="vacuumMsg.ok ? 'alert-success' : 'alert-error'">{{ vacuumMsg.text }}</div>
          <div v-if="purgeMsg" class="alert mt-3" :class="purgeMsg.ok ? 'alert-success' : 'alert-error'">{{ purgeMsg.text }}</div>
        </section>
      </div>
    </div>

    <SMTPConfigModal
      :open="smtpModalOpen"
      :form="form"
      :smtp-msg="smtpMsg"
      :saving-smtp="savingSMTP"
      :testing-email="testingEmail"
      @close="smtpModalOpen = false"
      @save="saveSMTP"
      @test="testEmail"
    />
  </div>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { api } from '../api/index.js'
import SMTPConfigModal from '../components/SMTPConfigModal.vue'

const props = defineProps({ status: Object })
defineEmits(['refresh'])

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

const smtpModalOpen = ref(false)
const savingThreshold = ref(false)
const savingSMTP = ref(false)
const savingEvents = ref(false)
const testingEmail = ref(false)
const vacuuming = ref(false)
const purging = ref(false)
const purgeDays = ref(60)

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
.settings-page { display: grid; gap: 12px; }
.settings-overview-fill { width: 78%; }
.settings-grid { display: grid; grid-template-columns: 2fr 1fr; gap: 12px; }
.settings-card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--r);
  padding: 14px;
  box-shadow: var(--shadow);
}
.settings-card-title { font-size: 15px; font-weight: 700; margin-bottom: 10px; }
.threshold-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; }
.threshold-item { display: grid; gap: 6px; }
.threshold-item span { color: var(--text-2); font-size: 12px; }
.threshold-item input {
  height: 36px;
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 0 10px;
}
.section-actions { margin-top: 12px; display: flex; align-items: center; gap: 10px; }
.event-rows { display: grid; gap: 10px; }
.event-row {
  display: flex; justify-content: space-between; align-items: center;
  border: 1px solid var(--border); border-radius: 8px; padding: 10px; min-height: 48px;
}
.event-label { font-size: 13px; }
.event-row-right { display: flex; align-items: center; gap: 8px; }
.event-sub { color: var(--text-2); font-size: 12px; }
.event-days-input {
  width: 58px; height: 30px; border: 1px solid var(--border); border-radius: 8px;
  text-align: center;
}
.notify-card { display: grid; align-content: start; gap: 10px; }
.notify-line { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.smtp-ready-box {
  height: 28px; border-radius: 8px; border: 1px solid #fee2e2; background: #fef2f2;
  display: flex; align-items: center; gap: 8px; padding: 0 12px; color: #991b1b; font-size: 12px;
}
.smtp-ready-box.ok { background: #f0fdf4; border-color: #bbf7d0; color: #166534; }
.smtp-ready-dot { width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
.smtp-current { color: var(--text-2); font-size: 12px; }
.notify-foot { display: flex; gap: 8px; justify-content: flex-end; }
.db-card { grid-column: 2; grid-row: 2; }
.db-line {
  border-radius: 8px; display: flex; align-items: center; justify-content: space-between;
  gap: 10px; padding: 10px; margin-top: 8px;
}
.db-line-vacuum { background: var(--surface-2); }
.db-line-purge { background: #fffbeb; color: #92400e; }
.db-title { font-size: 13px; font-weight: 600; }
.purge-inline { display: flex; align-items: center; gap: 8px; font-size: 13px; }
.purge-inline input {
  width: 52px; height: 30px; border-radius: 8px; border: 1px solid #d97706;
  text-align: center; background: #fff;
}
.btn-warn { background: #d97706; border-color: #d97706; color: #fff; }

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

.btn-dark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  transition: transform .2s ease, box-shadow .2s ease, opacity .2s ease;
}

.btn-dark.is-busy {
  transform: translateY(-1px);
  box-shadow: 0 4px 10px rgba(15, 23, 42, .16);
}

button:disabled {
  opacity: .65;
  cursor: not-allowed;
}
@media (max-width: 980px) {
  .settings-grid { grid-template-columns: 1fr; }
  .db-card { grid-column: auto; grid-row: auto; }
  .threshold-grid { grid-template-columns: 1fr; }
  .db-line { flex-direction: column; align-items: flex-start; }
}
@media (max-width: 640px) {
  .notify-foot { justify-content: flex-start; }
}
</style>
