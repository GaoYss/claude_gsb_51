<template>
  <el-dialog
    :model-value="modelValue"
    title="区域故障统一派工"
    width="480px"
    @update:model-value="$emit('update:modelValue', $event)"
    @open="syncForm"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="92px">
      <el-form-item label="区域单号">
        <span class="text-muted">{{ area?.area_no }}</span>
      </el-form-item>
      <el-form-item label="负责人" prop="assignee">
        <el-input v-model="form.assignee" placeholder="本次处置负责人" />
      </el-form-item>
      <el-form-item label="维修班组" prop="repair_team">
        <el-input v-model="form.repair_team" placeholder="如: 市政照明二班" />
      </el-form-item>
      <el-form-item label="联系电话" prop="contact_phone">
        <el-input v-model="form.contact_phone" placeholder="选填" />
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
import { areaFaultApi } from '@/api/areaFault'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  area: { type: Object, default: null },
})
const emit = defineEmits(['update:modelValue', 'saved'])

const formRef = ref(null)
const submitting = ref(false)
const form = reactive({ assignee: '', repair_team: '', contact_phone: '' })

const rules = {
  assignee: [{ required: true, message: '请填写派工负责人', trigger: 'blur' }],
}

function syncForm() {
  form.assignee = props.area?.assignee || ''
  form.repair_team = props.area?.repair_team || ''
  form.contact_phone = props.area?.contact_phone || ''
}

async function handleSubmit() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  submitting.value = true
  try {
    await areaFaultApi.dispatch(props.area.id, { ...form })
    ElMessage.success('派工成功, 区域故障进入处置中')
    emit('update:modelValue', false)
    emit('saved')
  } finally {
    submitting.value = false
  }
}
</script>
