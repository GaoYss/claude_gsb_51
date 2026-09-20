<template>
  <el-dialog
    :model-value="modelValue"
    title="统一派工"
    width="600px"
    @update:model-value="$emit('update:modelValue', $event)"
    @open="syncForm"
  >
    <el-alert
      type="info"
      :closable="false"
      show-icon
      title="一次派工对区域单内全部受影响路灯同时开工, 逐盏生成维修记录。"
      class="dispatch-hint"
    />
    <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
      <el-form-item label="维修人员" prop="repairman">
        <el-input v-model="form.repairman" placeholder="例如 陈师傅" />
      </el-form-item>
      <el-form-item label="维修班组" prop="repair_team">
        <el-input v-model="form.repair_team" placeholder="例如 市政照明二班" />
      </el-form-item>
      <el-form-item label="联系电话" prop="contact_phone">
        <el-input v-model="form.contact_phone" placeholder="选填" />
      </el-form-item>
      <el-form-item label="派工时间" prop="started_at">
        <el-date-picker
          v-model="form.started_at"
          type="datetime"
          value-format="YYYY-MM-DD HH:mm:ss"
          placeholder="默认取当前时间"
          style="width: 100%"
        />
      </el-form-item>
      <el-form-item label="处置内容" prop="content">
        <el-input v-model="form.content" type="textarea" :rows="3" maxlength="512" show-word-limit
          placeholder="统一处置安排, 例如: 先断开回路, 排查电缆/控制箱故障点后逐段恢复" />
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="$emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">确认派工</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { regionApi } from '@/api/region'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  regionId: { type: [Number, String], required: true },
})

const emit = defineEmits(['update:modelValue', 'saved'])

const formRef = ref(null)
const submitting = ref(false)

const createForm = () => ({
  repairman: '',
  repair_team: '',
  contact_phone: '',
  started_at: '',
  content: '',
})
const form = reactive(createForm())

const rules = {
  repairman: [{ required: true, message: '请填写维修人员', trigger: 'blur' }],
}

function syncForm() {
  Object.assign(form, createForm())
}

async function handleSubmit() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    const payload = { ...form }
    if (!payload.started_at) delete payload.started_at
    await regionApi.dispatch(props.regionId, payload)
    ElMessage.success('已统一派工, 全部受影响路灯进入处置中')
    emit('update:modelValue', false)
    emit('saved')
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.dispatch-hint {
  margin-bottom: 16px;
}
</style>
