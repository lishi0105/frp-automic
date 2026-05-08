<template>
  <div v-if="open" class="policy-mask" @click.self="$emit('close')">
    <section class="policy-modal" role="dialog" aria-modal="true" aria-labelledby="policy-title">
      <header class="policy-head">
        <div>
          <h3 id="policy-title">流量与告警策略</h3>
          <p>统一配置流量阈值、限额规则与事件告警策略</p>
        </div>
        <button class="close-btn" type="button" @click="$emit('close')">×</button>
      </header>

      <div class="policy-body">
        <div class="group">
          <h4>流量阈值设置</h4>
          <div class="grid">
            <label>
              <span>月入站阈值 (GB)</span>
              <input v-model.number="form.threshold_in_gb" type="number" min="0" step="0.1" />
              <small>达到阈值时触发告警</small>
            </label>
            <label>
              <span>月出站阈值 (GB)</span>
              <input v-model.number="form.threshold_out_gb" type="number" min="0" step="0.1" />
              <small>达到阈值时触发告警</small>
            </label>
            <label>
              <span>网卡总量阈值 (GB)</span>
              <input v-model.number="form.threshold_total_gb" type="number" min="0" step="0.1" />
              <small>达到阈值时触发告警</small>
            </label>
          </div>
        </div>

        <div class="group">
          <h4>流量限额设置</h4>
          <div class="grid">
            <label>
              <span>月入站限额 (GB)</span>
              <input v-model.number="form.limit_in_gb" type="number" min="0" step="0.1" />
              <small>达到限额时触发告警</small>
            </label>
            <label>
              <span>月出站限额 (GB)</span>
              <input v-model.number="form.limit_out_gb" type="number" min="0" step="0.1" />
              <small>达到限额时触发告警</small>
            </label>
            <label>
              <span>总流量限额 (GB)</span>
              <input v-model.number="form.limit_total_gb" type="number" min="0" step="0.1" />
              <small>达到限额时触发告警</small>
            </label>
          </div>
        </div>

        <div class="group">
          <h4>首月初始流量</h4>
          <div class="grid initial-grid">
            <label>
              <span>初始入站流量 (GB)</span>
              <input v-model.number="form.initial_in_gb" type="number" min="0" step="0.1" />
              <small>仅部署月份计入统计</small>
            </label>
            <label>
              <span>初始出站流量 (GB)</span>
              <input v-model.number="form.initial_out_gb" type="number" min="0" step="0.1" />
              <small>次月自动不再叠加</small>
            </label>
            <div class="deploy-note">
              <span>部署日期</span>
              <b>{{ form.deploy_date || '-' }}</b>
              <small>根据数据库初始化日期自动生成</small>
            </div>
          </div>
        </div>

        <div class="group">
          <h4>事件告警</h4>
          <div class="events">
            <div class="event-item">
              <div>
                <b>代理离线告警</b>
                <small>代理节点连续离线时触发告警</small>
              </div>
              <button class="switch-btn" :class="{ on: form.alert_proxy_offline === 'true' }" type="button" @click="toggle('alert_proxy_offline')"><span></span></button>
            </div>
            <div class="event-item">
              <div>
                <b>SSL 证书到期告警</b>
                <small>证书到期前触发告警</small>
              </div>
              <div class="event-right">
                <button class="switch-btn" :class="{ on: form.alert_cert_expiry === 'true' }" type="button" @click="toggle('alert_cert_expiry')"><span></span></button>
                <template v-if="form.alert_cert_expiry === 'true'">
                  <span>提前</span>
                  <input v-model.number="form.alert_cert_days" type="number" min="1" max="90" class="days-input" />
                  <span>天</span>
                </template>
              </div>
            </div>
          </div>
        </div>

        <div class="tip">SMTP 未配置时仅保留站内提醒，配置邮件后可发送告警通知。</div>
        <div v-if="msg" class="alert save-feedback" :class="msg.ok ? 'alert-success save-feedback-ok' : 'alert-error'">{{ msg.text }}</div>
      </div>

      <footer class="policy-foot">
        <button class="btn btn-outline btn-sm" type="button" @click="$emit('close')">取消</button>
        <button class="btn btn-dark btn-sm" :disabled="saving" type="button" @click="$emit('save')">
          {{ saving ? '保存中…' : '保存策略' }}
        </button>
      </footer>
    </section>
  </div>
</template>

<script setup>
const props = defineProps({
  open: Boolean,
  form: Object,
  msg: Object,
  saving: Boolean
})
defineEmits(['close', 'save'])

function toggle(key) {
  props.form[key] = props.form[key] === 'true' ? 'false' : 'true'
}
</script>

<style scoped>
.policy-mask { position: fixed; inset: 0; background: rgba(15, 23, 42, .45); backdrop-filter: blur(2px); z-index: 70; display: flex; align-items: center; justify-content: center; padding: 18px; }
.policy-modal { width: min(860px, 100%); max-height: 92vh; overflow: hidden; border-radius: 10px; border: 1px solid #dbe5f4; background: #fff; box-shadow: 0 20px 70px rgba(15, 23, 42, .28); display: flex; flex-direction: column; }
.policy-head { display: flex; align-items: flex-start; justify-content: space-between; padding: 18px 22px 12px; border-bottom: 1px solid #edf2fb; }
.policy-head h3 { margin: 0; font-size: var(--fs-modal-title); line-height: var(--lh-title); font-weight: var(--fw-title); color: #0f172a; }
.policy-head p { margin: 6px 0 0; color: #64748b; font-size: var(--fs-body); }
.close-btn { width: 34px; height: 34px; border: 1px solid #d3deef; border-radius: 8px; background: #fff; color: #334155; font-size: 24px; line-height: 1; cursor: pointer; }
.policy-body { padding: 16px 22px 14px; overflow-y: auto; display: grid; gap: 14px; }
.group { border-top: 1px solid #edf2fb; padding-top: 14px; }
.group:first-child { border-top: 0; padding-top: 0; }
.group h4 { margin: 0 0 10px; color: #0f172a; font-size: var(--fs-section-title); font-weight: var(--fw-section); }
.grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.grid label { display: grid; gap: 8px; }
.grid span { color: #334155; font-size: var(--fs-body); font-weight: var(--fw-medium); }
.grid input { height: 36px; border: 1px solid #d7e1ef; border-radius: 8px; padding: 0 12px; font-size: var(--fs-base); color: #0f172a; font-weight: var(--fw-medium); }
.grid small { color: #64748b; font-size: var(--fs-caption); }
.deploy-note { min-height: 36px; border: 1px solid #e5ecf8; border-radius: 8px; padding: 8px 10px; display: grid; gap: 4px; background: #f8fbff; }
.deploy-note span { color: #334155; font-size: var(--fs-body); font-weight: var(--fw-medium); }
.deploy-note b { color: #0f172a; font-size: var(--fs-base); }
.deploy-note small { color: #64748b; font-size: var(--fs-caption); }
.events { display: grid; gap: 10px; }
.event-item { border: 1px solid #e5ecf8; border-radius: 12px; padding: 12px 14px; display: flex; justify-content: space-between; align-items: center; gap: 10px; }
.event-item b { display: block; font-size: var(--fs-base); color: #0f172a; }
.event-item small { color: #64748b; font-size: var(--fs-caption); }
.event-right { display: flex; align-items: center; gap: 8px; color: #475569; }
.days-input { width: 64px; height: 30px; border: 1px solid #d3deef; border-radius: 8px; text-align: center; }
.switch-btn { width: 52px; height: 26px; border: 0; border-radius: 999px; background: #64748b; padding: 0 3px; display: flex; align-items: center; cursor: pointer; }
.switch-btn span { width: 20px; height: 20px; border-radius: 50%; background: #fff; transform: translateX(0); transition: transform .15s; }
.switch-btn.on { background: #2563eb; }
.switch-btn.on span { transform: translateX(26px); }
.tip { background: #ecf4ff; border: 1px solid #cddff9; border-radius: 8px; color: #334155; font-size: var(--fs-caption); padding: 10px 12px; }
.policy-foot { padding: 12px 22px 16px; border-top: 1px solid #edf2fb; display: flex; justify-content: flex-end; gap: 10px; }
@media (max-width: 980px) {
  .policy-head h3 { font-size: var(--fs-modal-title); }
  .grid { grid-template-columns: 1fr; }
  .grid input { font-size: var(--fs-base); }
}
</style>
