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
            <div class="feature-meta">
              <p>统一配置流量阈值、总量限额与事件告警策略</p>
              <div class="feature-tags">
                <span>流量阈值</span>
                <span>总量限额</span>
                <span>事件告警</span>
              </div>
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

      <section class="storage-card card-surface">
        <div class="storage-top">
          <div class="storage-intro">
            <div class="storage-intro-icon">◉</div>
            <div class="storage-intro-text">
              <h3>存储空间维护</h3>
              <p>优化数据库、管理磁盘告警阈值与历史数据，释放存储空间</p>
            </div>
          </div>
          <div class="storage-col">
            <h4>数据清理</h4>
            <button type="button" class="btn btn-dark btn-sm storage-col-btn" @click="openStorageDetailModal">存储详情</button>
          </div>
          <div class="storage-col">
            <h4>存储空间设置</h4>
            <button type="button" class="btn btn-dark btn-sm storage-col-btn" @click="openStorageSettingsModal">打开设置</button>
          </div>
          <div class="storage-col storage-col-retain">
            <h4>历史数据保留天数</h4>
            <div class="purge-row">
              <span>保留</span>
              <input v-model.number="purgeDays" type="number" min="1" max="365" />
              <span>天</span>
              <button class="btn btn-dark btn-sm" :disabled="purging" @click="saveRetentionDays">{{ purging ? '保存中…' : '保存' }}</button>
            </div>
          </div>
        </div>
        <div v-if="retentionMsg" class="storage-banner" :class="retentionMsg.ok ? 'ok' : 'err'">{{ retentionMsg.text }}</div>
      </section>
    </div>

    <TrafficPolicyModal
      :open="policyModalOpen"
      :form="form"
      :saving="savingPolicy"
      @close="closePolicyModal"
      @save="savePolicy"
    />

    <SMTPConfigModal
      :open="smtpModalOpen"
      :form="form"
      :saving-smtp="savingSMTP"
      :testing-email="testingEmail"
      @close="closeSMTPModal"
      @save="saveSMTP"
      @test="testEmail"
    />

    <StorageDetailModal
      :open="storageDetailModalOpen"
      @close="closeStorageDetailModal"
      @toast="forwardToast"
      @refresh="$emit('refresh')"
    />

    <StorageSettingsModal
      :open="storageSettingsModalOpen"
      :threshold-mb="diskThresholdMb"
      @close="closeStorageSettingsModal"
      @toast="forwardToast"
      @saved="onStorageSettingsSaved"
    />
  </div>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/index.js'
import SMTPConfigModal from '../components/SMTPConfigModal.vue'
import StorageDetailModal from '../components/StorageDetailModal.vue'
import StorageSettingsModal from '../components/StorageSettingsModal.vue'
import TrafficPolicyModal from '../components/TrafficPolicyModal.vue'

const props = defineProps({ status: Object })
const emit = defineEmits(['refresh', 'toast'])
const route = useRoute()
const router = useRouter()

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
  initial_in_gb: 0,
  initial_out_gb: 0,
  deploy_date: '',
  alert_proxy_offline: 'false',
  alert_cert_expiry: 'false',
  alert_cert_days: 15,
  disk_free_space_alert_threshold_mb: 0
})

const smtpModalOpen = ref(false)
const policyModalOpen = ref(false)
const storageDetailModalOpen = ref(false)
const storageSettingsModalOpen = ref(false)
const savingSMTP = ref(false)
const savingPolicy = ref(false)
const testingEmail = ref(false)
const purging = ref(false)
const purgeDays = ref(60)

const retentionMsg = ref(null)
const savedSettings = ref(null)
const formInitialized = ref(false)

const smtpReady = computed(() =>
  Boolean(form.smtp_host && form.smtp_from && form.smtp_to && form.smtp_auth_code)
)

const diskThresholdMb = computed(() => Math.max(0, Math.floor(Number(form.disk_free_space_alert_threshold_mb) || 0)))

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
  form.initial_in_gb = s.initial_in_gb || 0
  form.initial_out_gb = s.initial_out_gb || 0
  form.deploy_date = s.deploy_date || ''
  form.alert_proxy_offline = s.alert_proxy_offline ? 'true' : 'false'
  form.alert_cert_expiry = s.alert_cert_expiry ? 'true' : 'false'
  form.alert_cert_days = s.alert_cert_days || 15
  form.smtp_auth_code = s.smtp_auth_code || ''
  form.disk_free_space_alert_threshold_mb = Number(s.disk_free_space_alert_threshold_mb) || 0
  purgeDays.value = Number(s.history_retention_days) > 0 ? Number(s.history_retention_days) : 60
}

watch(() => props.status?.settings, (settings) => {
  savedSettings.value = settings || savedSettings.value
  if (formInitialized.value && (smtpModalOpen.value || policyModalOpen.value || storageDetailModalOpen.value || storageSettingsModalOpen.value)) return
  if (formInitialized.value) return
  fillForm(settings)
  if (settings) formInitialized.value = true
}, { immediate: true })

watch(() => route.query.modal, (modal) => {
  policyModalOpen.value = modal === 'policy'
  smtpModalOpen.value = modal === 'smtp'
  storageDetailModalOpen.value = modal === 'storage'
  storageSettingsModalOpen.value = modal === 'storage-settings'
}, { immediate: true })

function setModalQuery(modal) {
  const query = { ...route.query }
  if (modal) query.modal = modal
  else delete query.modal
  router.replace({ path: route.path, query })
}

function flash(msgRef, ok, text, ms = 4000) {
  msgRef.value = { ok, text }
  setTimeout(() => { msgRef.value = null }, ms)
}

function toast(ok, message) {
  emit('toast', { type: ok ? 'success' : 'error', message })
}

function forwardToast(payload) {
  if (payload && typeof payload === 'object' && payload.message) {
    emit('toast', payload)
  }
}

function openSMTPModal() {
  fillForm(savedSettings.value || props.status?.settings)
  smtpModalOpen.value = true
  setModalQuery('smtp')
}

function closeSMTPModal() {
  smtpModalOpen.value = false
  fillForm(savedSettings.value || props.status?.settings)
  setModalQuery(null)
}

function openPolicyModal() {
  fillForm(savedSettings.value || props.status?.settings)
  policyModalOpen.value = true
  setModalQuery('policy')
}

function closePolicyModal() {
  policyModalOpen.value = false
  fillForm(savedSettings.value || props.status?.settings)
  setModalQuery(null)
}

function openStorageDetailModal() {
  storageDetailModalOpen.value = true
  setModalQuery('storage')
}

function closeStorageDetailModal() {
  storageDetailModalOpen.value = false
  setModalQuery(null)
}

function openStorageSettingsModal() {
  fillForm(savedSettings.value || props.status?.settings)
  storageSettingsModalOpen.value = true
  setModalQuery('storage-settings')
}

function closeStorageSettingsModal() {
  storageSettingsModalOpen.value = false
  fillForm(savedSettings.value || props.status?.settings)
  setModalQuery(null)
}

function onStorageSettingsSaved(mb) {
  form.disk_free_space_alert_threshold_mb = mb
  const base = savedSettings.value || props.status?.settings || {}
  savedSettings.value = { ...base, disk_free_space_alert_threshold_mb: mb }
  emit('refresh')
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
    toast(true, 'SMTP 配置已保存')
  } catch (e) {
    toast(false, '保存失败：' + e.message)
  } finally {
    savingSMTP.value = false
  }
}

async function testEmail() {
  testingEmail.value = true
  try {
    const r = await api.testEmail()
    if (r.ok) toast(true, '测试邮件发送成功')
    else toast(false, '发送失败：' + r.error)
  } catch (e) {
    toast(false, '发送失败：' + e.message)
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
      initial_in_gb: form.initial_in_gb,
      initial_out_gb: form.initial_out_gb,
      alert_proxy_offline: form.alert_proxy_offline,
      alert_cert_expiry: form.alert_cert_expiry,
      alert_cert_days: form.alert_cert_days
    })
    savedSettings.value = saved
    fillForm(saved)
    formInitialized.value = true
    toast(true, '策略已保存')
  } catch (e) {
    toast(false, '保存失败：' + e.message)
  } finally {
    savingPolicy.value = false
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
.sysset-main { display: grid; gap: 10px; padding: 0 24px 24px; }
.card-surface {
  background: #fff;
  border: 1px solid #d9e4f3;
  border-radius: 8px;
  box-shadow: 0 8px 28px rgba(15, 23, 42, .05);
}
.summary-card {
  min-height: 150px;
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
  width: 56px;
  height: 56px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  font-size: 18px;
  font-weight: var(--fw-strong);
  flex-shrink: 0;
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
  gap: 14px;
  min-width: 0;
}
.summary-metrics span {
  display: grid;
  gap: 2px;
  min-width: 50px;
}
.summary-item b { display: block; font-size: var(--fs-section-title); color: #0f172a; line-height: 1.15; white-space: nowrap; }
.summary-item small { color: #64748b; font-size: var(--fs-caption); }

.feature-card {
  min-height: 88px;
  padding: 12px 18px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
}
.feature-left { display: flex; align-items: center; gap: 14px; min-width: 0; }
.feature-logo {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  color: #fff;
  font-size: var(--fs-icon-sm);
  background: linear-gradient(145deg, #3b82f6, #1d4ed8);
  flex-shrink: 0;
}
.feature-left h3 { margin: 0 0 4px; font-size: var(--fs-section-title); line-height: 1.25; font-weight: var(--fw-section); color: #0f172a; }
.feature-left p { margin: 0; color: #64748b; font-size: var(--fs-caption); }
.feature-meta { display: flex; align-items: center; flex-wrap: wrap; gap: 6px 10px; }
.feature-tags { display: flex; flex-wrap: wrap; gap: 5px; }
.feature-tags span { border: 1px solid #cfe0fb; color: #2563eb; background: #eff6ff; border-radius: 6px; font-size: var(--fs-caption); padding: 2px 7px; }
.btn-lg { min-width: 96px; height: 34px; font-size: var(--fs-caption); }

.mail-right { display: flex; align-items: center; gap: 12px; }
.mail-status { color: #16a34a; font-size: var(--fs-caption); display: inline-flex; align-items: center; gap: 6px; }
.mail-status i { width: 8px; height: 8px; border-radius: 50%; background: currentColor; }
.mail-status.off { color: #ef4444; }

/* 存储空间维护：左介绍 + 三列 */
.storage-card {
  display: flex;
  flex-direction: column;
  min-height: 178px;
  padding: 0;
  overflow: hidden;
}
.storage-top {
  display: grid;
  grid-template-columns: minmax(200px, 1.15fr) repeat(3, minmax(0, 1fr));
  gap: 0;
  flex: 1;
}
.storage-intro {
  display: flex;
  align-items: flex-start;
  gap: 14px;
  padding: 22px 20px 22px 22px;
  border-right: 1px solid #e2e8f0;
  background: linear-gradient(180deg, #fbfdff 0%, #fff 100%);
}
.storage-intro-icon {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  color: #fff;
  font-size: 20px;
  background: linear-gradient(145deg, #3b82f6, #1d4ed8);
  flex-shrink: 0;
}
.storage-intro-text h3 {
  margin: 0 0 6px;
  font-size: 20px;
  font-weight: 700;
  color: #0f172a;
}
.storage-intro-text p {
  margin: 0;
  font-size: 13px;
  color: #64748b;
  line-height: 1.5;
  max-width: 36ch;
}
.storage-col {
  padding: 18px 18px 20px;
  border-right: 1px solid #e2e8f0;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  min-width: 0;
}
.storage-top .storage-col:last-of-type {
  border-right: 0;
}
.storage-col-icon {
  width: 40px;
  height: 40px;
  margin-bottom: 6px;
  border-radius: 10px;
  background: #eff6ff;
  border: 1px solid #bfdbfe;
  position: relative;
}
.storage-col-icon-spark {
  background: radial-gradient(circle at 30% 30%, #dbeafe, #eff6ff);
}
.storage-col-icon-spark::before,
.storage-col-icon-spark::after {
  content: '';
  position: absolute;
  background: #2563eb;
  border-radius: 1px;
}
.storage-col-icon-spark::before {
  width: 2px;
  height: 18px;
  left: 50%;
  top: 8px;
  transform: translateX(-50%) rotate(35deg);
}
.storage-col-icon-spark::after {
  width: 18px;
  height: 2px;
  left: 11px;
  top: 50%;
  transform: translateY(-50%) rotate(35deg);
}
.storage-col-icon-sliders::before {
  content: '';
  position: absolute;
  left: 10px;
  right: 10px;
  top: 12px;
  height: 6px;
  border: 2px solid #2563eb;
  border-radius: 4px;
  border-bottom: 0;
}
.storage-col-icon-sliders::after {
  content: '';
  position: absolute;
  left: 14px;
  right: 14px;
  bottom: 10px;
  height: 10px;
  border-left: 2px solid #2563eb;
  border-right: 2px solid #2563eb;
  border-bottom: 2px solid #2563eb;
  border-radius: 0 0 4px 4px;
}
.storage-col-icon-cal {
  border-radius: 8px;
}
.storage-col-icon-cal::before {
  content: '';
  position: absolute;
  left: 8px;
  right: 8px;
  top: 14px;
  height: 2px;
  background: #2563eb;
  border-radius: 1px;
}
.storage-col-icon-cal::after {
  content: '';
  position: absolute;
  left: 10px;
  right: 10px;
  top: 8px;
  height: 6px;
  border: 2px solid #2563eb;
  border-bottom: 0;
  border-radius: 4px 4px 0 0;
}
.storage-col h4 {
  margin: 0 0 6px;
  font-size: 15px;
  font-weight: 700;
  color: #0f172a;
}
.storage-col-desc {
  margin: 0 0 12px;
  font-size: 13px;
  color: #64748b;
  line-height: 1.45;
  flex: 1;
}
.storage-col-btn {
  min-width: 96px;
  height: 34px;
  margin-top: auto;
}
.storage-col-retain .purge-row {
  margin-top: auto;
}
.purge-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  color: #334155;
  font-size: 13px;
}
.purge-row input {
  width: 64px;
  height: 34px;
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  text-align: center;
  font-size: 15px;
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
}
.storage-banner {
  padding: 10px 16px;
  font-size: 13px;
  border-top: 1px solid #e2e8f0;
}
.storage-banner.ok {
  background: #ecfdf5;
  color: #047857;
}
.storage-banner.err {
  background: #fef2f2;
  color: #b91c1c;
}

.btn-dark { border-color: #2563eb; background: #2563eb; color: #fff; }
.btn-dark:hover { background: #1d4ed8; border-color: #1d4ed8; }

@media (max-width: 1200px) {
  .summary-card { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .summary-item:nth-child(odd) { border-left: 0; }
  .summary-item:nth-child(n + 3) { border-top: 1px solid #e2e8f0; }
  .storage-top {
    grid-template-columns: 1fr;
  }
  .storage-intro {
    border-right: 0;
    border-bottom: 1px solid #e2e8f0;
  }
  .storage-top .storage-col {
    border-right: 0;
    border-bottom: 1px solid #e2e8f0;
  }
  .storage-top .storage-col:last-of-type {
    border-bottom: 0;
  }
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
}
</style>
