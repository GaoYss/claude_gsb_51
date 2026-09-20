<template>
  <el-dialog
    :model-value="modelValue"
    :title="`登记处置结果 · ${item?.lamp_code || ''}`"
    width="600px"
    @update:model-value="$emit('update:modelValue', $event)"
    @open="syncForm"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
      <el-form-item label="处置结果" prop="result">
        <el-select v-model="form.result" style="width: 100%">
          <el-option v-for="(entry, key) in REPAIR_RESULT" :key="key" :label="entry.label" :value="key" />
        </el-select>
        <div class="form-hint text-muted">选择"已修复"后, 该盏路灯计入已恢复; 其余结果视为未恢复, 可对该盏再次派工。</div>
      </el-form-item>
      <el-form-item label="完工时间" prop="finished_at">
        <el-date-picker
          v-model="form.finished_at"
          type="datetime"
          value-format="YYYY-MM-DD HH:mm:ss"
          placeholder="默认取当前时间"
          style="width: 100%"
        />
      </el-form-item>
      <el-form-item label="处置内容" prop="content">
        <el-input v-model="form.content" type="textarea" :rows="2" maxlength="512" show-word-limit />
      </el-form-item>
      <el-form-item label="使用耗材" prop="materials">
        <el-input v-model="form.materials" placeholder="例如: 电缆 20 米 / 交流接触器 1 只" />
      </el-form-item>
      <el-form-item label="费用(元)" prop="cost">
        <el-input-number v-model="form.cost" :min="0" :precision="2" :step="10" style="width: 100%" />
      </el-form-item>
      <el-form-item label="备注" prop="remark">
        <el-input v-model="form.remark" type="textarea" :rows="2" maxlength="255" show-word-limit />
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="$emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">提交本盏结果</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { regionApi } from '@/api/region'
import { REPAIR_RESULT } from '@/constants/dict'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  regionId: { type: [Number, String], required: true },
  item: { type: Object, default: null },
})

const emit = defineEmits(['update:modelValue', 'saved'])

const formRef = ref(null)
const submitting = ref(false)

const createForm = () => ({
  result: 'fixed',
  finished_at: '',
  content: '',
  materials: '',
  cost: 0,
  remark: '',
})
const form = reactive(createForm())

const rules = {
  result: [{ required: true, message: '请选择处置结果', trigger: 'change' }],
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
    if (!payload.finished_at) delete payload.finished_at
    await regionApi.resolveLamp(props.regionId, props.item.fault_id, payload)
    ElMessage.success(form.result === 'fixed' ? '该盏路灯已恢复' : '处置结果已登记, 该盏仍未恢复')
    emit('update:modelValue', false)
    emit('saved')
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.form-hint {
  font-size: 12px;
  line-height: 1.6;
}
</style>
