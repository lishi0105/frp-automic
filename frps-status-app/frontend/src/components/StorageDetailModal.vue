<template>
  <Transition name="modal-pop">
    <div v-if="open" class="sd-mask">
      <section class="sd-modal" role="dialog" aria-modal="true" aria-labelledby="sd-title">
      <header class="sd-head">
        <div>
          <h3 id="sd-title">存储详情</h3>
        </div>
        <button class="sd-close" type="button" aria-label="关闭" @click="$emit('close')">×</button>
      </header>

      <div class="sd-body">
        <div v-if="loading" class="sd-loading">加载中…</div>
        <template v-else>
          <div v-if="loadError" class="sd-alert sd-alert-err">{{ loadError }}</div>
          <div v-if="storage?.below_disk_alert_threshold" class="sd-alert sd-alert-warn">
            当前分区剩余空间低于告警阈值，请关注磁盘或调整阈值。
          </div>

          <h4 class="sd-sec">当前存储空间详情</h4>
          <div class="sd-panel sd-panel-muted">
            <div v-if="part" class="sd-disk">
              <span class="sd-label">剩余 / 总量 / 已用</span>
              <div class="sd-disk-row">
                <span class="sd-metric sd-accent">{{ humanBytes(part.free_bytes) }}</span>
                <span class="sd-slash">/</span>
                <span class="sd-metric">{{ humanBytes(part.total_bytes) }}</span>
                <span class="sd-pct">约 {{ pctText }}% 已用</span>
              </div>
              <div class="sd-bar-wrap">
                <div class="sd-bar-fill" :style="{ width: barPct + '%' }"></div>
              </div>
              <div class="sd-foot-hint">
                日志文件合计 {{ humanBytes(storage?.log_files_bytes) }} · 库文件估算 {{ humanBytes(storage?.database_bytes) }}
              </div>
            </div>
          </div>

          <h4 class="sd-sec">当前进程占用（MB）</h4>
          <div class="sd-panel">
            <div class="sd-metrics-row">
              <div>
                <span class="sd-label">日志</span>
                <span class="sd-metric">{{ fmtMb(usage?.log_mb) }}</span>
              </div>
              <div>
                <span class="sd-label">数据</span>
                <span class="sd-metric">{{ fmtMb(usage?.data_mb) }}</span>
              </div>
              <div>
                <span class="sd-label">总计</span>
                <span class="sd-metric">{{ fmtMb(usage?.total_mb) }}</span>
              </div>
            </div>
          </div>

          <h4 class="sd-sec">存储清理</h4>
          <p class="sd-desc">
            删除非当日日志、按保留天数清理流量历史并执行 VACUUM。
          </p>
        </template>
      </div>

      <footer class="sd-foot">
        <button class="btn btn-outline btn-sm" type="button" @click="$emit('close')">关闭</button>
        <button
          class="btn btn-warn btn-sm"
          type="button"
          :disabled="loading || cleaning || !!loadError"
          @click="runCleanup"
        >
          {{ cleaning ? '执行中…' : '存储清理' }}
        </button>
      </footer>
      </section>
    </div>
  </Transition>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { api } from '../api/index.js'
import { humanBytes } from '../utils/format.js'

const props = defineProps({ open: Boolean })
const emit = defineEmits(['close', 'toast', 'refresh'])

const loading = ref(false)
const cleaning = ref(false)
const loadError = ref('')
const storage = ref(null)
const usage = ref(null)

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

async function runCleanup() {
  cleaning.value = true
  try {
    const r = await api.storageCleanup()
    if (r?.ok) {
      const bits = []
      if (r.log_files_removed != null) bits.push(`删除日志文件 ${r.log_files_removed} 个`)
      if (r.traffic_rows_deleted != null) bits.push(`清理流量行 ${r.traffic_rows_deleted}`)
      if (r.vacuum_ok) bits.push('已 VACUUM')
      emit('toast', { type: 'success', message: bits.length ? bits.join('；') : '存储清理已完成' })
      emit('refresh')
      await loadAll()
    } else {
      const err = r?.log_error || r?.purge_error || r?.vacuum_error || '清理未完全成功'
      emit('toast', { type: 'error', message: String(err) })
    }
  } catch (e) {
    emit('toast', { type: 'error', message: e.message || String(e) })
  } finally {
    cleaning.value = false
  }
}

watch(
  () => props.open,
  (v) => {
    if (v) loadAll()
  }
)
</script>

<style scoped>
.sd-mask {
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
.sd-modal {
  width: min(680px, 100%);
  max-height: 92vh;
  overflow: hidden;
  border-radius: 12px;
  border: 1px solid #d9e4f3;
  background: #fff;
  box-shadow: 0 20px 70px rgba(15, 23, 42, 0.28);
  display: flex;
  flex-direction: column;
}
.sd-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 20px 22px 12px;
  border-bottom: 1px solid #e2e8f0;
}
.sd-head h3 {
  margin: 0;
  font-size: var(--fs-modal-title, 1.15rem);
  font-weight: 700;
  color: #0f172a;
}
.sd-sub {
  margin: 6px 0 0;
  font-size: 12px;
  color: #64748b;
}
.sd-close {
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
.sd-body {
  padding: 16px 22px 12px;
  overflow-y: auto;
  display: grid;
  gap: 12px;
}
.sd-loading {
  padding: 24px;
  text-align: center;
  color: #64748b;
}
.sd-sec {
  margin: 4px 0 0;
  font-size: 14px;
  font-weight: 700;
  color: #0f172a;
}
.sd-panel {
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  padding: 14px 16px;
}
.sd-panel-muted {
  background: #f8fafc;
}
.sd-grid-paths {
  display: grid;
  gap: 12px;
  margin-bottom: 14px;
}
.sd-label {
  display: block;
  font-size: 11px;
  color: #64748b;
  margin-bottom: 4px;
}
.sd-mono {
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 13px;
  color: #0f172a;
  word-break: break-all;
}
.sd-disk-row {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 6px 10px;
  margin-top: 4px;
}
.sd-metric {
  font-size: 17px;
  font-weight: 700;
  color: #0f172a;
}
.sd-accent {
  color: #2563eb;
}
.sd-slash {
  color: #94a3b8;
  font-weight: 600;
}
.sd-pct {
  font-size: 13px;
  color: #334155;
}
.sd-bar-wrap {
  margin-top: 10px;
  height: 8px;
  border-radius: 4px;
  background: #e2e8f0;
  overflow: hidden;
}
.sd-bar-fill {
  height: 100%;
  border-radius: 4px;
  background: linear-gradient(90deg, #2f80ed, #1d5fea);
  max-width: 100%;
}
.sd-foot-hint {
  margin-top: 10px;
  font-size: 12px;
  color: #64748b;
}
.sd-metrics-row {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}
.sd-desc {
  margin: 0;
  font-size: 13px;
  color: #334155;
  line-height: 1.55;
}
.sd-tip {
  font-size: 12px;
  color: #64748b;
  background: #fffbeb;
  border: 1px solid #fde68a;
  border-radius: 8px;
  padding: 10px 12px;
}
.sd-alert {
  border-radius: 8px;
  padding: 10px 12px;
  font-size: 13px;
}
.sd-alert-err {
  background: #fef2f2;
  border: 1px solid #fecaca;
  color: #b91c1c;
}
.sd-alert-warn {
  background: #fffbeb;
  border: 1px solid #fde68a;
  color: #92400e;
}
.sd-foot {
  padding: 12px 22px 16px;
  border-top: 1px solid #e2e8f0;
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}
.btn-warn {
  border-color: #ea580c;
  background: #ea580c;
  color: #fff;
}
.btn-warn:hover:not(:disabled) {
  background: #c2410c;
  border-color: #c2410c;
}
.btn-warn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
@media (max-width: 640px) {
  .sd-metrics-row {
    grid-template-columns: 1fr;
  }
}
</style>
