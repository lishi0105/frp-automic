<template>
  <div v-if="open" class="smtp-mask" @click.self="$emit('close')">
    <div class="smtp-modal">
      <div class="smtp-modal-head">
        <h3>配置邮件服务</h3>
        <button class="smtp-close" type="button" @click="$emit('close')">×</button>
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
            <input v-model="form.smtp_auth_code" :type="showPass ? 'text' : 'password'" placeholder="请输入SMTP授权码" />
            <button class="pass-toggle" type="button" @click="showPass = !showPass">{{ showPass ? '隐藏' : '显示' }}</button>
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

        <div v-if="smtpMsg" class="alert mt-3 save-feedback" :class="smtpMsg.ok ? 'alert-success save-feedback-ok' : 'alert-error'">{{ smtpMsg.text }}</div>
      </div>

      <div class="smtp-modal-foot">
        <button class="btn btn-outline btn-sm smtp-action" :class="{ 'is-busy': testingEmail }" :disabled="testingEmail" @click="$emit('test')">
          <span v-if="testingEmail" class="btn-spinner"></span>
          {{ testingEmail ? '发送中…' : '发送测试邮件' }}
        </button>
        <div class="foot-right">
          <button class="btn btn-outline btn-sm smtp-action" @click="$emit('close')">取消</button>
          <button class="btn btn-dark btn-sm smtp-action" :class="{ 'is-busy': savingSMTP }" :disabled="savingSMTP" @click="$emit('save')">
            <span v-if="savingSMTP" class="btn-spinner"></span>
            {{ savingSMTP ? '确定中…' : '确定' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'

const props = defineProps({
  open: Boolean,
  form: Object,
  smtpMsg: Object,
  savingSMTP: Boolean,
  testingEmail: Boolean
})
defineEmits(['close', 'save', 'test'])

const showPass = ref(false)
function toggleSMTP() {
  props.form.smtp_enabled = props.form.smtp_enabled === 'true' ? 'false' : 'true'
}
</script>

<style scoped>
.smtp-mask { position: fixed; inset: 0; background: rgba(15, 23, 42, .56); display: flex; align-items: center; justify-content: center; padding: 20px; z-index: 60; }
.smtp-modal { width: min(640px, 100%); max-height: 90vh; border-radius: 10px; background: var(--surface); border: 1px solid var(--border); overflow: hidden; display: flex; flex-direction: column; box-shadow: 0 24px 80px rgba(15, 23, 42, .34); }
.smtp-modal-head { height: 62px; border-bottom: 1px solid var(--border); padding: 0 24px; display: flex; justify-content: space-between; align-items: center; }
.smtp-modal-head h3 { font-size: var(--fs-modal-title); font-weight: var(--fw-title); color: var(--text); }
.smtp-close { width: 32px; height: 32px; border: 1px solid var(--border); border-radius: 6px; background: var(--surface); color: var(--text-2); font-size: 22px; line-height: 1; cursor: pointer; }
.smtp-close:hover { background: var(--surface-2); color: var(--text); }
.smtp-modal-body { padding: 20px 24px 12px; overflow-y: auto; flex: 1; }
.smtp-line { display: grid; grid-template-columns: 112px minmax(0, 1fr); align-items: center; gap: 12px; margin-bottom: 10px; }
.smtp-line-top { align-items: flex-start; }
.smtp-label { text-align: right; color: var(--text-2); font-size: var(--fs-base); }
.smtp-label i { color: #ef4444; font-style: normal; margin-right: 4px; }
.smtp-input-wrap { width: 100%; }
.smtp-input-wrap input, .smtp-input-wrap textarea { width: 100%; border: 1px solid var(--border); border-radius: 6px; padding: 0 12px; font: inherit; background: var(--surface-2); color: var(--text); }
.smtp-input-wrap input { height: 36px; }
.smtp-input-wrap textarea { resize: vertical; min-height: 80px; padding-top: 9px; padding-bottom: 9px; }
.password-row { display: grid; grid-template-columns: minmax(0, 1fr) 62px; gap: 10px; }
.pass-toggle { height: 36px; border: 1px solid var(--border); border-radius: 6px; background: var(--surface); color: var(--text-2); font-size: var(--fs-caption); cursor: pointer; }
.switch-btn { width: 50px; height: 24px; border: 0; border-radius: 999px; background: #475569; padding: 0 3px; display: flex; align-items: center; cursor: pointer; transition: background .15s; }
.switch-btn span { width: 18px; height: 18px; border-radius: 50%; background: #f8fafc; transform: translateX(0); transition: transform .15s; }
.switch-btn.on { background: #10b981; }
.switch-btn.on span { transform: translateX(26px); }
.smtp-tips { margin: 4px 0 10px 124px; color: var(--text-2); font-size: var(--fs-body); line-height: 1.7; }
.smtp-modal-foot { background: var(--surface-2); min-height: 62px; padding: 12px 24px; display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.foot-right { display: flex; gap: 10px; }
.smtp-action { min-width: 72px; height: 32px; }
.btn-dark { border-color: var(--primary); background: var(--primary); color: #fff; }
.smtp-action.is-busy { transform: translateY(-1px); box-shadow: 0 4px 10px rgba(15, 23, 42, .16); }
.btn-spinner { width: 12px; height: 12px; border-radius: 50%; border: 2px solid currentColor; border-right-color: transparent; animation: btnspin .7s linear infinite; }
@keyframes btnspin { to { transform: rotate(360deg); } }
@media (max-width: 640px) {
  .smtp-line { grid-template-columns: 1fr; gap: 6px; }
  .smtp-label { text-align: left; }
  .smtp-tips { margin-left: 18px; }
}
</style>
