<template>
  <div>
    <div class="page-header">
      <div>
        <div class="page-title">系统配置</div>
        <div class="page-sub">SMTP 告警 · 数据管理</div>
      </div>
    </div>

    <div class="page-body">
      <!-- SMTP 配置 -->
      <div class="card section">
        <div class="form-section-title">邮件告警（SMTP）</div>
        <div class="form-grid">
          <div class="form-item">
            <label class="form-label">SMTP 服务器</label>
            <input class="form-input" v-model="form.smtp_host" placeholder="smtp.example.com" />
          </div>
          <div class="form-item">
            <label class="form-label">端口</label>
            <input class="form-input" type="number" v-model.number="form.smtp_port" placeholder="587" />
          </div>
          <div class="form-item">
            <label class="form-label">用户名</label>
            <input class="form-input" v-model="form.smtp_user" placeholder="user@example.com" />
          </div>
          <div class="form-item">
            <label class="form-label">授权码 / 密码</label>
            <input class="form-input" type="password" v-model="form.smtp_password" :placeholder="hasSMTPPass ? '已设置（留空不修改）' : '未设置'" />
          </div>
          <div class="form-item">
            <label class="form-label">发件人邮箱</label>
            <input class="form-input" v-model="form.smtp_from" placeholder="noreply@example.com" />
          </div>
          <div class="form-item">
            <label class="form-label">收件人（多个用逗号分隔）</label>
            <input class="form-input" v-model="form.smtp_to" placeholder="admin@example.com" />
          </div>
          <div class="form-item">
            <label class="form-label">启用邮件告警</label>
            <select class="form-select" v-model="form.smtp_enabled">
              <option value="true">启用</option>
              <option value="false">停用</option>
            </select>
          </div>
        </div>

        <div class="form-actions">
          <button class="btn btn-primary" :disabled="saving" @click="saveSettings">
            {{ saving ? '保存中…' : '保存配置' }}
          </button>
          <button class="btn btn-outline" :disabled="testingEmail" @click="testEmail">
            {{ testingEmail ? '发送中…' : '发送测试邮件' }}
          </button>
        </div>
        <div v-if="smtpMsg" class="alert mt-3" :class="smtpMsg.ok ? 'alert-success' : 'alert-error'">
          {{ smtpMsg.text }}
        </div>
      </div>

      <!-- 告警阈值 -->
      <div class="card section">
        <div class="form-section-title">流量告警阈值</div>
        <div class="form-grid">
          <div class="form-item">
            <label class="form-label">月上行阈值（GB）</label>
            <input class="form-input" type="number" min="0" step="0.1" v-model.number="form.alert_in_gb" placeholder="0 表示不限" />
          </div>
          <div class="form-item">
            <label class="form-label">月下行阈值（GB）</label>
            <input class="form-input" type="number" min="0" step="0.1" v-model.number="form.alert_out_gb" placeholder="0 表示不限" />
          </div>
        </div>
        <div class="form-actions">
          <button class="btn btn-primary" :disabled="saving" @click="saveSettings">保存阈值</button>
        </div>
        <div v-if="saveMsg" class="alert mt-3" :class="saveMsg.ok ? 'alert-success' : 'alert-error'">
          {{ saveMsg.text }}
        </div>
      </div>

      <!-- 数据管理 -->
      <div class="card section">
        <div class="form-section-title">数据管理</div>

        <div style="display:flex;flex-direction:column;gap:20px">
          <!-- Vacuum -->
          <div>
            <div style="font-size:13px;font-weight:600;margin-bottom:6px">数据库压缩（VACUUM）</div>
            <div class="text-muted text-sm" style="margin-bottom:10px">整理 SQLite 数据库碎片，释放磁盘空间。</div>
            <button class="btn btn-outline" :disabled="vacuuming" @click="doVacuum">
              {{ vacuuming ? '执行中…' : '立即压缩' }}
            </button>
            <div v-if="vacuumMsg" class="alert mt-3" :class="vacuumMsg.ok ? 'alert-success' : 'alert-error'">
              {{ vacuumMsg.text }}
            </div>
          </div>

          <hr class="divider" style="margin:0" />

          <!-- Purge -->
          <div>
            <div style="font-size:13px;font-weight:600;margin-bottom:6px">清理旧数据</div>
            <div class="text-muted text-sm" style="margin-bottom:10px">删除超过指定天数的每日流量记录。</div>
            <div style="display:flex;gap:10px;align-items:center;flex-wrap:wrap">
              <div class="flex-center" style="gap:8px">
                <span class="text-muted text-sm">保留近</span>
                <input class="form-input" type="number" min="1" max="365" style="width:80px" v-model.number="purgeDays" />
                <span class="text-muted text-sm">天</span>
              </div>
              <button class="btn btn-danger" :disabled="purging" @click="doPurge">
                {{ purging ? '执行中…' : '清理数据' }}
              </button>
            </div>
            <div v-if="purgeMsg" class="alert mt-3" :class="purgeMsg.ok ? 'alert-success' : 'alert-error'">
              {{ purgeMsg.text }}
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, watch } from 'vue'
import { api } from '../api/index.js'

const props = defineProps({ status: Object, daily: Array, loading: Boolean })

const form = reactive({
  smtp_host: '',
  smtp_port: 587,
  smtp_user: '',
  smtp_password: '',
  smtp_from: '',
  smtp_to: '',
  smtp_enabled: 'false',
  alert_in_gb: 0,
  alert_out_gb: 0
})

const hasSMTPPass = ref(false)
const saving = ref(false)
const testingEmail = ref(false)
const vacuuming = ref(false)
const purging = ref(false)
const purgeDays = ref(30)

const saveMsg = ref(null)
const smtpMsg = ref(null)
const vacuumMsg = ref(null)
const purgeMsg = ref(null)

function fillForm(s) {
  if (!s) return
  form.smtp_host = s.smtp_host || ''
  form.smtp_port = s.smtp_port || 587
  form.smtp_user = s.smtp_user || ''
  form.smtp_from = s.smtp_from || ''
  form.smtp_to = s.smtp_to || ''
  form.smtp_enabled = s.smtp_enabled ? 'true' : 'false'
  form.alert_in_gb = s.alert_in_gb || 0
  form.alert_out_gb = s.alert_out_gb || 0
  hasSMTPPass.value = s.smtp_password_set || false
}

watch(() => props.status?.settings, fillForm, { immediate: true })

function flash(msgRef, ok, text, ms = 4000) {
  msgRef.value = { ok, text }
  setTimeout(() => { msgRef.value = null }, ms)
}

async function saveSettings() {
  saving.value = true
  try {
    const payload = { ...form }
    if (!payload.smtp_password) delete payload.smtp_password
    await api.saveSettings(payload)
    flash(saveMsg, true, '配置已保存')
  } catch (e) {
    flash(saveMsg, false, '保存失败：' + e.message)
  } finally {
    saving.value = false
  }
}

async function testEmail() {
  testingEmail.value = true
  smtpMsg.value = null
  try {
    const r = await api.testEmail()
    flash(smtpMsg, r.ok, r.ok ? '测试邮件发送成功' : '发送失败：' + r.error)
  } catch (e) {
    flash(smtpMsg, false, '发送失败：' + e.message)
  } finally {
    testingEmail.value = false
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
