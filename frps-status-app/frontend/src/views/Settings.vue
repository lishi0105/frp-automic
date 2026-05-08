<template>
  <div class="sysset-page">
    <div class="page-header sysset-header">
      <div>
        <div class="page-title">系统配置</div>
        <div class="page-sub">管理 FRPS 监控平台的全局参数与通知策略</div>
      </div>
    </div>

    <div class="sysset-main">
      <section class="summary-card card-surface">
        <div class="summary-item">
          <div class="summary-icon in">↓</div>
          <div>
            <h3>入站策略</h3>
            <div class="summary-metrics">
              <span><small>阈值</small><b>{{ form.threshold_in_gb || 0 }} GB</b></span>
              <span><small>限额</small><b>{{ form.limit_in_gb || 0 }} GB</b></span>
            </div>
          </div>
        </div>
        <div class="summary-item">
          <div class="summary-icon out">↑</div>
          <div>
            <h3>出站策略</h3>
            <div class="summary-metrics">
              <span><small>阈值</small><b>{{ form.threshold_out_gb || 0 }} GB</b></span>
              <span><small>限额</small><b>{{ form.limit_out_gb || 0 }} GB</b></span>
            </div>
          </div>
        </div>
        <div class="summary-item">
          <div class="summary-icon total">◔</div>
          <div>
            <h3>总量策略</h3>
            <div class="summary-metrics">
              <span><small>阈值</small><b>{{ form.threshold_total_gb || 0 }} GB</b></span>
              <span><small>限额</small><b>{{ form.limit_total_gb || 0 }} GB</b></span>
            </div>
          </div>
        </div>
        <div class="summary-item">
          <div class="summary-icon smtp">✉</div>
          <div>
            <h3>SMTP 状态</h3>
            <b>{{ smtpReady ? '已配置' : '未配置' }}</b>
          </div>
        </div>
      </section>

      <section class="feature-card card-surface">
        <div class="feature-left">
          <div class="feature-logo">∿</div>
          <div>
            <h3>流量与告警策略</h3>
            <p>统一配置流量阈值、总量限额与事件告警策略</p>
            <div class="feature-tags">
              <span>流量阈值</span>
              <span>总量限额</span>
              <span>事件告警</span>
              <span>通知策略</span>
            </div>
          </div>
        </div>
        <button class="btn btn-dark btn-lg" @click="openPolicyModal">前往配置</button>
      </section>

      <section class="feature-card card-surface">
        <div class="feature-left">
          <div class="feature-logo">✉</div>
          <div>
            <h3>邮件通知配置</h3>
            <p>配置 SMTP 服务，用于告警邮件发送</p>
          </div>
        </div>
        <div class="mail-right">
          <span class="mail-status" :class="{ off: !smtpReady }">
            <i></i>{{ smtpReady ? '已配置' : '未配置' }}
          </span>
          <button class="btn btn-dark btn-lg" @click="openSMTPModal">配置邮件</button>
        </div>
      </section>

      <section class="db-card card-surface">
        <div class="db-main">
          <div class="feature-left">
            <div class="feature-logo">◍</div>
            <div>
              <h3>数据库维护</h3>
              <p>优化数据库性能，清理冗余数据，释放存储空间</p>
            </div>
          </div>
        </div>
        <div class="db-maintain">
          <div class="db-item">
            <h4>空间碎片整理（VACUUM）</h4>
            <p>释放数据库文件空间，减少碎片</p>
            <button class="btn btn-outline btn-sm" :disabled="vacuuming" @click="doVacuum">{{ vacuuming ? '执行中…' : '立即执行' }}</button>
          </div>
          <div class="db-item">
            <h4>历史数据保留天数</h4>
            <p>设置历史数据保留天数，超期数据由后端自动清理</p>
            <div class="purge-row">
              <span>保留近</span>
              <input v-model.number="purgeDays" type="number" min="1" max="365" />
              <span>天</span>
              <span>历史记录</span>
              <button class="btn btn-dark btn-sm" :disabled="purging" @click="saveRetentionDays">{{ purging ? '保存中…' : '保存' }}</button>
            </div>
          </div>
        </div>
        <div class="db-messages">
          <div v-if="vacuumMsg" class="alert" :class="vacuumMsg.ok ? 'alert-success' : 'alert-error'">{{ vacuumMsg.text }}</div>
          <div v-if="retentionMsg" class="alert" :class="retentionMsg.ok ? 'alert-success' : 'alert-error'">{{ retentionMsg.text }}</div>
        </div>
      </section>
    </div>

    <TrafficPolicyModal
      :open="policyModalOpen"
      :form="form"
      :msg="policyMsg"
      :saving="savingPolicy"
      @close="closePolicyModal"
      @save="savePolicy"
    />

    <SMTPConfigModal
      :open="smtpModalOpen"
      :form="form"
      :smtp-msg="smtpMsg"
      :saving-smtp="savingSMTP"
      :testing-email="testingEmail"
      @close="closeSMTPModal"
      @save="saveSMTP"
      @test="testEmail"
    />
  </div>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { api } from '../api/index.js'
import SMTPConfigModal from '../components/SMTPConfigModal.vue'
import TrafficPolicyModal from '../components/TrafficPolicyModal.vue'

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
  threshold_in_gb: 0,
  threshold_out_gb: 0,
  threshold_total_gb: 0,
  limit_in_gb: 0,
  limit_out_gb: 0,
  limit_total_gb: 0,
  alert_proxy_offline: 'false',
  alert_cert_expiry: 'false',
  alert_cert_days: 15
})

const smtpModalOpen = ref(false)
const policyModalOpen = ref(false)
const savingSMTP = ref(false)
const savingPolicy = ref(false)
const testingEmail = ref(false)
const vacuuming = ref(false)
const purging = ref(false)
const purgeDays = ref(60)

const smtpMsg = ref(null)
const policyMsg = ref(null)
const vacuumMsg = ref(null)
const retentionMsg = ref(null)
const savedSettings = ref(null)
const formInitialized = ref(false)

const smtpReady = computed(() =>
  Boolean(form.smtp_host && form.smtp_from && form.smtp_to && form.smtp_auth_code)
)

function fillForm(s) {
  if (!s) return
  form.smtp_host = s.smtp_host || ''
  form.smtp_port = s.smtp_port || 465
  form.smtp_user = s.smtp_user || ''
  form.smtp_from = s.smtp_from || ''
  form.smtp_to = s.smtp_to || ''
  form.smtp_enabled = s.smtp_enabled ? 'true' : 'false'
  form.threshold_in_gb = s.threshold_in_gb || 0
  form.threshold_out_gb = s.threshold_out_gb || 0
  form.threshold_total_gb = s.threshold_total_gb || 0
  form.limit_in_gb = s.limit_in_gb || 0
  form.limit_out_gb = s.limit_out_gb || 0
  form.limit_total_gb = s.limit_total_gb || 0
  form.alert_proxy_offline = s.alert_proxy_offline ? 'true' : 'false'
  form.alert_cert_expiry = s.alert_cert_expiry ? 'true' : 'false'
  form.alert_cert_days = s.alert_cert_days || 15
  form.smtp_auth_code = s.smtp_auth_code || ''
  purgeDays.value = Number(s.history_retention_days) > 0 ? Number(s.history_retention_days) : 60
}

watch(() => props.status?.settings, (settings) => {
  if (smtpModalOpen.value || policyModalOpen.value) return
  if (formInitialized.value) return
  savedSettings.value = settings || savedSettings.value
  fillForm(settings)
  if (settings) formInitialized.value = true
}, { immediate: true })

function flash(msgRef, ok, text, ms = 4000) {
  msgRef.value = { ok, text }
  setTimeout(() => { msgRef.value = null }, ms)
}

function openSMTPModal() {
  smtpMsg.value = null
  smtpModalOpen.value = true
}

function closeSMTPModal() {
  smtpModalOpen.value = false
  fillForm(savedSettings.value || props.status?.settings)
}

function openPolicyModal() {
  policyMsg.value = null
  policyModalOpen.value = true
}

function closePolicyModal() {
  policyModalOpen.value = false
  fillForm(savedSettings.value || props.status?.settings)
}

async function saveSMTP() {
  savingSMTP.value = true
  try {
    const saved = await api.saveSettings({
      smtp_host: form.smtp_host,
      smtp_port: form.smtp_port,
      smtp_user: form.smtp_user,
      smtp_auth_code: form.smtp_auth_code,
      smtp_from: form.smtp_from,
      smtp_to: form.smtp_to,
      smtp_enabled: form.smtp_enabled
    })
    savedSettings.value = saved
    fillForm(saved)
    formInitialized.value = true
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
    if (r.ok) flash(smtpMsg, true, '测试邮件发送成功')
    else smtpMsg.value = { ok: false, text: '发送失败：' + r.error }
  } catch (e) {
    smtpMsg.value = { ok: false, text: '发送失败：' + e.message }
  } finally {
    testingEmail.value = false
  }
}

async function savePolicy() {
  savingPolicy.value = true
  try {
    const saved = await api.saveSettings({
      threshold_in_gb: form.threshold_in_gb,
      threshold_out_gb: form.threshold_out_gb,
      threshold_total_gb: form.threshold_total_gb,
      limit_in_gb: form.limit_in_gb,
      limit_out_gb: form.limit_out_gb,
      limit_total_gb: form.limit_total_gb,
      alert_proxy_offline: form.alert_proxy_offline,
      alert_cert_expiry: form.alert_cert_expiry,
      alert_cert_days: form.alert_cert_days,
      smtp_enabled: form.smtp_enabled
    })
    savedSettings.value = saved
    fillForm(saved)
    formInitialized.value = true
    flash(policyMsg, true, '策略已保存')
  } catch (e) {
    flash(policyMsg, false, '保存失败：' + e.message)
  } finally {
    savingPolicy.value = false
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

async function saveRetentionDays() {
  if (!Number.isFinite(purgeDays.value) || purgeDays.value < 1) {
    purgeDays.value = 60
  }
  purgeDays.value = Math.min(365, Math.max(1, Math.floor(purgeDays.value)))
  purging.value = true
  retentionMsg.value = null
  try {
    const saved = await api.saveSettings({
      history_retention_days: purgeDays.value
    })
    savedSettings.value = saved
    fillForm(saved)
    formInitialized.value = true
    flash(retentionMsg, true, `历史数据保留天数已更新为 ${purgeDays.value} 天`)
  } catch (e) {
    flash(retentionMsg, false, '保存失败：' + e.message)
  } finally {
    purging.value = false
  }
}
</script>

<style scoped>
.sysset-page { display: flex; flex-direction: column; gap: 0; }
.sysset-header { min-height: auto; }
.sysset-main { display: grid; gap: 12px; padding: 0 24px 24px; }
.card-surface {
  background: #fff;
  border: 1px solid #d9e4f3;
  border-radius: 8px;
  box-shadow: 0 8px 28px rgba(15, 23, 42, .05);
}
.summary-card {
  min-height: 118px;
  padding: 0;
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 0;
}
.summary-item {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 14px 18px;
  min-width: 0;
}
.summary-item + .summary-item {
  border-left: 1px solid #e2e8f0;
}
.summary-icon {
  width: 54px;
  height: 54px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  font-size: 18px;
  font-weight: var(--fw-strong);
}
.summary-icon.in { color: #2563eb; background: #dbeafe; }
.summary-icon.out { color: #10b981; background: #dcfce7; }
.summary-icon.total { color: #f59e0b; background: #fef3c7; }
.summary-icon.smtp { color: #7c3aed; background: #ede9fe; }
.summary-item h3 {
  margin: 0 0 8px;
  font-size: var(--fs-card-title);
  line-height: 1.2;
  font-weight: var(--fw-title);
  color: #0f172a;
}
.summary-metrics {
  display: flex;
  gap: 18px;
  min-width: 0;
}
.summary-metrics span {
  display: grid;
  gap: 4px;
  min-width: 54px;
}
.summary-item b { display: block; font-size: var(--fs-page-title-sm); color: #0f172a; line-height: 1.15; white-space: nowrap; }
.summary-item small { color: #64748b; font-size: var(--fs-caption); }

.feature-card {
  min-height: 116px;
  padding: 14px 18px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.feature-left { display: flex; align-items: center; gap: 14px; min-width: 0; }
.feature-logo {
  width: 54px;
  height: 54px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  color: #fff;
  font-size: var(--fs-icon-sm);
  background: linear-gradient(145deg, #3b82f6, #1d4ed8);
}
.feature-left h3 { margin: 0 0 4px; font-size: var(--fs-section-title); line-height: 1.25; font-weight: var(--fw-section); color: #0f172a; }
.feature-left p { margin: 0; color: #64748b; font-size: var(--fs-caption); }
.feature-tags { margin-top: 8px; display: flex; flex-wrap: wrap; gap: 6px; }
.feature-tags span { border: 1px solid #cfe0fb; color: #2563eb; background: #eff6ff; border-radius: 6px; font-size: var(--fs-caption); padding: 2px 7px; }
.btn-lg { min-width: 96px; height: 34px; font-size: var(--fs-caption); }

.mail-right { display: flex; align-items: center; gap: 12px; }
.mail-status { color: #16a34a; font-size: var(--fs-caption); display: inline-flex; align-items: center; gap: 6px; }
.mail-status i { width: 8px; height: 8px; border-radius: 50%; background: currentColor; }
.mail-status.off { color: #ef4444; }

.db-card {
  min-height: 132px;
  padding: 0;
  display: grid;
  grid-template-columns: 1.2fr .8fr 1fr;
  gap: 0;
  overflow: hidden;
}
.db-main {
  display: flex;
  align-items: center;
  padding: 14px 18px;
  border-right: 1px solid #e2e8f0;
}
.db-maintain {
  display: contents;
}
.db-item {
  padding: 14px 18px;
  border-right: 1px solid #e2e8f0;
}
.db-item:last-child {
  border-right: 0;
}
.db-item h4 { margin: 0; font-size: var(--fs-card-title); font-weight: var(--fw-section); color: #0f172a; }
.db-item p { margin: 6px 0 10px; color: #64748b; font-size: var(--fs-caption); }
.purge-row { display: flex; align-items: center; gap: 6px; color: #334155; font-size: var(--fs-caption); }
.purge-row input { width: 54px; height: 30px; border: 1px solid #d5e2f6; border-radius: 6px; text-align: center; font-size: var(--fs-caption); }
.db-messages { grid-column: 1 / -1; display: grid; gap: 8px; }

.btn-dark { border-color: #2563eb; background: #2563eb; color: #fff; }
.btn-dark:hover { background: #1d4ed8; border-color: #1d4ed8; }

@media (max-width: 1200px) {
  .summary-card { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .summary-item:nth-child(odd) { border-left: 0; }
  .summary-item:nth-child(n + 3) { border-top: 1px solid #e2e8f0; }
  .db-card { grid-template-columns: 1fr; }
  .db-main,
  .db-item {
    border-right: 0;
    border-bottom: 1px solid #ecf2fb;
  }
  .db-item:last-child { border-bottom: 0; }
}

@media (max-width: 860px) {
  .sysset-main { padding: 0 12px 12px; }
  .summary-card { grid-template-columns: 1fr; }
  .summary-item,
  .summary-item:nth-child(n) {
    border-left: 0;
    border-top: 1px solid #e2e8f0;
    padding: 18px;
  }
  .summary-item:first-child { border-top: 0; }
  .feature-card { flex-direction: column; align-items: flex-start; }
  .mail-right { width: 100%; justify-content: space-between; }
  .summary-item b { font-size: var(--fs-page-title-sm); }
  .feature-left h3 { font-size: var(--fs-section-title); }
  .feature-left p { font-size: var(--fs-body); }
  .db-item h4 { font-size: var(--fs-section-title); }
  .db-item p { font-size: var(--fs-body); }
}
</style>
