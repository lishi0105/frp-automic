<template>
  <div v-if="open" class="ss-mask" @click.self="$emit('close')">
    <section class="ss-modal" role="dialog" aria-modal="true" aria-labelledby="ss-title">
      <header class="ss-head">
        <div>
          <h3 id="ss-title">存储设置</h3>
          <p class="ss-sub">GET /api/storage · 阈值单位 MB</p>
        </div>
        <button class="ss-close" type="button" aria-label="关闭" @click="$emit('close')">×</button>
      </header>

      <div class="ss-body">
        <div v-if="loading" class="ss-loading">加载中…</div>
        <template v-else>
          <div v-if="loadError" class="ss-alert ss-alert-err">{{ loadError }}</div>

          <h4 class="ss-sec">当前存储</h4>
          <div class="ss-panel ss-panel-muted">
            <div class="ss-grid-paths">
              <div>
                <span class="ss-label">数据目录</span>
                <span class="ss-mono">{{ storage?.data_dir || '—' }}</span>
              </div>
              <div>
                <span class="ss-label">数据库文件</span>
                <span class="ss-mono">{{ storage?.db_path || '—' }}</span>
              </div>
              <div>
                <span class="ss-label">日志目录</span>
                <span class="ss-mono">{{ storage?.log_dir || '—' }}</span>
              </div>
            </div>
            <div v-if="part" class="ss-disk">
              <div class="ss-disk-cols">
                <div>
                  <span class="ss-label">可用空间</span>
                  <span class="ss-metric">{{ humanBytes(part.free_bytes) }}</span>
                </div>
                <div>
                  <span class="ss-label">已用空间</span>
                  <span class="ss-metric ss-accent">{{ humanBytes(part.used_bytes) }}</span>
                </div>
                <div>
                  <span class="ss-label">总容量</span>
                  <span class="ss-metric">{{ humanBytes(part.total_bytes) }}</span>
                </div>
              </div>
              <div class="ss-bar-wrap">
                <div class="ss-bar-fill" :style="{ width: barPct + '%' }"></div>
              </div>
              <div class="ss-bar-cap">约 {{ pctText }}% 已用</div>
            </div>
          </div>

          <div class="ss-panel ss-row-metrics">
            <div>
              <span class="ss-label">SQLite 库</span>
              <span class="ss-metric-sm">{{ fmtMb(usage?.data_mb) }} MB</span>
            </div>
            <div>
              <span class="ss-label">日志合计</span>
              <span class="ss-metric-sm">{{ fmtMb(usage?.log_mb) }} MB</span>
            </div>
            <div class="ss-status">
              <span class="ss-label">监控状态</span>
              <span class="ss-ok"><i></i>{{ storage?.ok ? '正常' : '异常' }}</span>
            </div>
          </div>

          <h4 class="ss-sec">磁盘空闲告警阈值</h4>
          <p class="ss-desc">
            当分区可用空间低于该阈值（MB，1 MB = 1024×1024 字节）时触发应急清理；写入值与系统配置 disk_free_space_alert_threshold_mb 一致。填 0 表示由服务端按分区剩余约 20% 自动折算并持久化。
          </p>
          <div class="ss-input-row">
            <input
              v-model.number="localMb"
              type="number"
              min="0"
              step="1"
              class="ss-input"
            />
            <span class="ss-unit">MB</span>
          </div>
          <p class="ss-hint">* 与后端 PollLoop 内磁盘检测使用同一配置项。</p>
        </template>
      </div>

      <footer class="ss-actions">
        <button class="btn btn-outline btn-sm" type="button" @click="$emit('close')">取消</button>
        <button class="btn btn-dark btn-sm" type="button" :disabled="saving || loading" @click="submit">
          {{ saving ? '保存中…' : '保存' }}
        </button>
      </footer>
    </section>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { api } from '../api/index.js'
import { humanBytes } from '../utils/format.js'

const props = defineProps({
  open: Boolean,
  thresholdMb: { type: Number, default: 0 }
})
const emit = defineEmits(['close', 'saved', 'toast'])

const loading = ref(false)
const saving = ref(false)
const loadError = ref('')
const storage = ref(null)
const usage = ref(null)
const localMb = ref(0)

const part = computed(() => storage.value?.partition || null)
const pctText = computed(() => {
  const p = part.value?.usage_percent
  if (p == null || Number.isNaN(p)) return '—'
  return Math.round(p * 10) / 10
})
const barPct = computed(() => {
  const p = part.value?.usage_percent
  if (p == null || Number.isNaN(p)) return 0
  return Math.min(100, Math.max(0, p))
})

function fmtMb(v) {
  if (v == null || Number.isNaN(Number(v))) return '—'
  return Number(v).toFixed(2)
}

async function loadAll() {
  loadError.value = ''
  storage.value = null
  usage.value = null
  loading.value = true
  try {
    const [s, u] = await Promise.all([api.getStorage(), api.getStorageAppUsage()])
    storage.value = s
    usage.value = u
    if (!s?.ok && s?.error) loadError.value = s.error
  } catch (e) {
    loadError.value = e.message || String(e)
  } finally {
    loading.value = false
  }
}

watch(
  () => props.open,
  (v) => {
    if (v) {
      localMb.value = Math.max(0, Math.floor(Number(props.thresholdMb) || 0))
      loadAll()
    }
  }
)

watch(
  () => props.thresholdMb,
  (n) => {
    if (props.open) localMb.value = Math.max(0, Math.floor(Number(n) || 0))
  }
)

async function submit() {
  const mb = Math.max(0, Math.floor(Number(localMb.value) || 0))
  saving.value = true
  try {
    await api.saveSettings({ disk_free_space_alert_threshold_mb: mb })
    emit('toast', { type: 'success', message: '存储阈值已保存' })
    emit('saved', mb)
    emit('close')
  } catch (e) {
    emit('toast', { type: 'error', message: e.message || String(e) })
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.ss-mask {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.45);
  backdrop-filter: blur(2px);
  z-index: 72;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 18px;
}
.ss-modal {
  width: min(600px, 100%);
  max-height: 92vh;
  overflow: hidden;
  border-radius: 12px;
  border: 1px solid #d9e4f3;
  background: #fff;
  box-shadow: 0 20px 70px rgba(15, 23, 42, 0.28);
  display: flex;
  flex-direction: column;
}
.ss-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 20px 22px 12px;
  border-bottom: 1px solid #e2e8f0;
}
.ss-head h3 {
  margin: 0;
  font-size: var(--fs-modal-title, 1.15rem);
  font-weight: 700;
  color: #0f172a;
}
.ss-sub {
  margin: 6px 0 0;
  font-size: 13px;
  color: #64748b;
}
.ss-close {
  width: 36px;
  height: 36px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #f8fafc;
  color: #64748b;
  font-size: 22px;
  line-height: 1;
  cursor: pointer;
}
.ss-body {
  padding: 16px 22px 8px;
  overflow-y: auto;
  display: grid;
  gap: 12px;
}
.ss-loading {
  padding: 20px;
  text-align: center;
  color: #64748b;
}
.ss-sec {
  margin: 4px 0 0;
  font-size: 14px;
  font-weight: 700;
  color: #0f172a;
}
.ss-panel {
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  padding: 14px 16px;
}
.ss-panel-muted {
  background: #f8fafc;
}
.ss-grid-paths {
  display: grid;
  gap: 12px;
  margin-bottom: 14px;
}
.ss-label {
  display: block;
  font-size: 12px;
  color: #64748b;
  margin-bottom: 4px;
}
.ss-mono {
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 13px;
  color: #0f172a;
  word-break: break-all;
}
.ss-disk-cols {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}
.ss-metric {
  font-size: 18px;
  font-weight: 700;
  color: #0f172a;
}
.ss-accent {
  color: #2563eb;
}
.ss-bar-wrap {
  margin-top: 12px;
  height: 10px;
  border-radius: 5px;
  background: #e2e8f0;
  overflow: hidden;
}
.ss-bar-fill {
  height: 100%;
  border-radius: 5px;
  background: linear-gradient(90deg, #2f80ed, #1d5fea);
  max-width: 100%;
}
.ss-bar-cap {
  margin-top: 6px;
  text-align: right;
  font-size: 12px;
  color: #64748b;
}
.ss-row-metrics {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  background: #f8fafc;
}
.ss-metric-sm {
  font-size: 14px;
  font-weight: 600;
  color: #334155;
}
.ss-status .ss-ok {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: #16a34a;
  font-weight: 500;
}
.ss-status .ss-ok i {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: #22c55e;
}
.ss-desc {
  margin: 0;
  font-size: 13px;
  color: #334155;
  line-height: 1.55;
}
.ss-input-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 4px;
}
.ss-input {
  width: 120px;
  height: 44px;
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  padding: 0 12px;
  font-size: 17px;
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  text-align: center;
}
.ss-unit {
  font-size: 13px;
  color: #64748b;
}
.ss-hint {
  margin: 0;
  font-size: 12px;
  color: #64748b;
}
.ss-alert {
  border-radius: 8px;
  padding: 10px 12px;
  font-size: 13px;
}
.ss-alert-err {
  background: #fef2f2;
  border: 1px solid #fecaca;
  color: #b91c1c;
}
.ss-actions {
  padding: 12px 22px 16px;
  border-top: 1px solid #e2e8f0;
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}
@media (max-width: 560px) {
  .ss-disk-cols,
  .ss-row-metrics {
    grid-template-columns: 1fr;
  }
}
</style>
