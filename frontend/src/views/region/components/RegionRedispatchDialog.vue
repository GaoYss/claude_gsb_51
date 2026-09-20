<template>
  <el-dialog
    :model-value="modelValue"
    :title="`再次派工 · ${item?.lamp_code || ''}`"
    width="560px"
    @update:model-value="$emit('update:modelValue', $event)"
    @open="syncForm"
  >
    <el-alert
      type="warning"
      :closable="false"
      show-icon
      :title="`该盏路灯上一次处置结果为「${lastResultText}」尚未恢复, 请再次派工返修。`"
      class="redispatch-hint"
    />
    <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
      <el-form-item label="维修人员" prop="repairman">
        <el-input v-model="form.repairman" />
      </el-form-item>
      <el-form-item label="维修班组" prop="repair_team">
        <el-input v-model="form.repair_team" />
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
        <el-input v-model="form.content" type="textarea" :rows="3" maxlength="512" show-word-limit />
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="$emit('update:modelValue', false)">取消</el-button>
      <el-button type="warning" :loading="submitting" @click="handleSubmit">确认再次派工</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { regionApi } from '@/api/region'
import { REPAIR_RESULT, dictLabel } from '@/constants/dict'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  regionId: { type: [Number, String], required: true },
  item: { type: Object, default: null },
})

const emit = defineEmits(['update:modelValue', 'saved'])

const formRef = ref(null)
const submitting = ref(false)

const lastResultText = computed(() => dictLabel(REPAIR_RESULT, props.item?.repair_result))

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
  form.repairman = props.item?.repairman || ''
}

async function handleSubmit() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    const payload = { ...form }
    if (!payload.started_at) delete payload.started_at
    await regionApi.redispatchLamp(props.regionId, props.item.fault_id, payload)
    ElMessage.success('已对该盏路灯再次派工')
    emit('update:modelValue', false)
    emit('saved')
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.redispatch-hint {
  margin-bottom: 16px;
}
</style>
