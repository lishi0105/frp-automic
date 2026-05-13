<template>
  <div class="speedtest-shell">
    <div class="page-header">
      <div>
        <div class="page-title">链路测速</div>
        <div class="page-sub">VPS 通过 frp 隧道连接内网 iperf3 服务端</div>
      </div>
      <button class="btn btn-outline btn-sm icon-btn" title="刷新" aria-label="刷新" :disabled="loading" @click="load">
        <span class="refresh-glyph" :class="{ 'is-spinning': loading }" aria-hidden="true">↻</span>
      </button>
    </div>

    <div class="page-body speedtest-page">
      <section class="speed-panel">
        <div class="section-head">
          <div class="section-title">创建测速任务</div>
          <span class="text-muted text-sm">{{ targets.length }} 个目标</span>
        </div>
        <div v-if="!targets.length" class="empty-box">未配置测速目标。请在服务配置中启用 <code>iperf_test</code> 后重新生成部署文件。</div>
        <div v-else class="speed-form">
          <label>
            <span>目标</span>
            <select class="form-select" v-model="form.target">
              <option v-for="target in targets" :key="target.name" :value="target.name">
                {{ target.name }} · {{ target.host }}:{{ target.port }}
              </option>
            </select>
          </label>
          <label>
            <span>方向</span>
            <select class="form-select" v-model="form.direction">
              <option value="forward">正向：VPS 到内网</option>
              <option value="reverse">反向：内网到 VPS</option>
            </select>
          </label>
          <label>
            <span>时长</span>
            <input class="form-input" type="number" min="3" max="60" v-model.number="form.duration_seconds" />
          </label>
          <button class="btn btn-dark" :disabled="running || !form.target" @click="startTest">
            {{ running ? '测速中' : '开始测速' }}
          </button>
        </div>
      </section>

      <section class="speed-panel">
        <div class="section-head">
          <div class="section-title">最近任务</div>
          <span class="text-muted text-sm">{{ tasks.length }} 条</span>
        </div>
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>目标</th>
                <th>方向</th>
                <th>状态</th>
                <th>时长</th>
                <th>发送</th>
                <th>接收</th>
                <th>重传</th>
                <th>完成时间</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="!tasks.length"><td colspan="8" class="empty">暂无测速任务</td></tr>
              <tr v-for="task in tasks" :key="task.id">
                <td><code>{{ task.target?.name }}</code></td>
                <td>{{ task.direction === 'reverse' ? '反向' : '正向' }}</td>
                <td><span class="status-chip" :class="task.status">{{ statusText(task) }}</span></td>
                <td>{{ task.duration_seconds }} 秒</td>
                <td>{{ mbps(task.result?.sent_mbps) }}</td>
                <td>{{ mbps(task.result?.received_mbps) }}</td>
                <td>{{ task.result?.retransmits ?? '-' }}</td>
                <td>{{ fmtTime(task.finished_at || task.started_at || task.created_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { api } from '../api/index.js'

const emit = defineEmits(['toast'])

const loading = ref(false)
const targets = ref([])
const tasks = ref([])
const running = ref(false)
const activeTaskId = ref('')
const form = reactive({
  target: '',
  direction: 'forward',
  duration_seconds: 10
})
let pollTimer = null
let refreshTimer = null

const sortedTasks = computed(() => [...tasks.value].sort((a, b) => new Date(b.created_at) - new Date(a.created_at)))

async function load() {
  loading.value = true
  try {
    const data = await api.getSpeedtests()
    targets.value = data.targets || []
    tasks.value = data.tasks || []
    running.value = !!data.running
    if (!form.target && targets.value.length) form.target = targets.value[0].name
    tasks.value = sortedTasks.value
  } catch (err) {
    emit('toast', { type: 'error', message: err.message || '读取测速信息失败' })
  } finally {
    loading.value = false
  }
}

async function startTest() {
  try {
    const task = await api.createSpeedtest({ ...form })
    activeTaskId.value = task.id
    running.value = true
    await load()
    startPolling()
  } catch (err) {
    emit('toast', { type: 'error', message: err.message || '创建测速任务失败' })
  }
}

function startPolling() {
  stopPolling()
  pollTimer = setInterval(pollActiveTask, 1500)
}

function stopPolling() {
  if (pollTimer) clearInterval(pollTimer)
  pollTimer = null
}

async function pollActiveTask() {
  if (!activeTaskId.value) {
    await load()
    return
  }
  try {
    const task = await api.getSpeedtest(activeTaskId.value)
    const index = tasks.value.findIndex(t => t.id === task.id)
    if (index >= 0) tasks.value[index] = task
    else tasks.value.unshift(task)
    running.value = task.status === 'queued' || task.status === 'running'
    if (!running.value) {
      activeTaskId.value = ''
      stopPolling()
      await load()
    }
  } catch {
    stopPolling()
    await load()
  }
}

function statusText(task) {
  if (task.status === 'queued') return '排队中'
  if (task.status === 'running') return '运行中'
  if (task.status === 'completed') return '完成'
  if (task.error) return '失败'
  return task.status
}

function mbps(value) {
  const n = Number(value || 0)
  return n > 0 ? `${n.toFixed(2)} Mbps` : '-'
}

function fmtTime(iso) {
  if (!iso) return '-'
  return new Date(iso).toLocaleString('zh-CN', { dateStyle: 'short', timeStyle: 'medium' })
}

onMounted(() => {
  load()
  refreshTimer = setInterval(load, 8000)
})

onUnmounted(() => {
  stopPolling()
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<style scoped>
.speedtest-shell {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.speedtest-page {
  display: grid;
  grid-template-columns: minmax(280px, 380px) 1fr;
  gap: 18px;
  align-items: start;
}

.speed-panel {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 16px;
}

.speed-form {
  display: grid;
  gap: 14px;
}

.speed-form label {
  display: grid;
  gap: 6px;
  color: var(--text-2);
  font-size: 13px;
}

.empty-box {
  color: var(--text-2);
  font-size: 14px;
  line-height: 1.7;
}

.status-chip {
  display: inline-flex;
  align-items: center;
  min-width: 56px;
  justify-content: center;
  padding: 3px 8px;
  border-radius: 999px;
  font-size: 12px;
  background: var(--surface-2);
  color: var(--text-2);
}

.status-chip.running,
.status-chip.queued {
  background: rgba(59, 130, 246, .12);
  color: #2563eb;
}

.status-chip.completed {
  background: rgba(16, 185, 129, .14);
  color: var(--success);
}

.status-chip.failed {
  background: rgba(239, 68, 68, .12);
  color: var(--danger);
}

@media (max-width: 980px) {
  .speedtest-page {
    grid-template-columns: 1fr;
  }
}
</style>
