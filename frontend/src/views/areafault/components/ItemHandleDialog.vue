<template>
  <el-dialog
    :model-value="modelValue"
    :title="`逐盏登记处置结果 · ${item?.lamp_code || ''}`"
    width="480px"
    @update:model-value="$emit('update:modelValue', $event)"
    @open="syncForm"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="92px">
      <el-form-item label="所在道路">
        <span class="text-muted">{{ item?.road_name || '-' }}</span>
      </el-form-item>
      <el-form-item label="处置结果" prop="result">
        <el-select v-model="form.result" style="width: 100%">
          <el-option
            v-for="key in resultKeys"
            :key="key"
            :label="AREA_ITEM_RESULT[key].label"
            :value="key"
            :disabled="key === 'pending'"
          />
        </el-select>
      </el-form-item>
      <el-form-item label="处置人" prop="handler">
        <el-input v-model="form.handler" placeholder="默认取区域故障负责人" />
      </el-form-item>
      <el-form-item label="处置说明" prop="remark">
        <el-input
          v-model="form.remark"
          type="textarea"
          :rows="3"
          maxlength="255"
          show-word-limit
          placeholder="如: 更换中间接头复测绝缘正常 / 端子缺货待料"
        />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="$emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">确认登记</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { areaFaultApi } from '@/api/areaFault'
import { AREA_ITEM_RESULT } from '@/constants/dict'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  item: { type: Object, default: null },
})
const emit = defineEmits(['update:modelValue', 'saved'])

const resultKeys = Object.keys(AREA_ITEM_RESULT)
const formRef = ref(null)
const submitting = ref(false)
const form = reactive({ result: 'recovered', handler: '', remark: '' })

const rules = {
  result: [{ required: true, message: '请选择处置结果', trigger: 'change' }],
}

function syncForm() {
  form.result = props.item?.result && props.item.result !== 'pending' ? props.item.result : 'recovered'
  form.handler = props.item?.handler || ''
  form.remark = props.item?.handle_remark || ''
}

async function handleSubmit() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  submitting.value = true
  try {
    await areaFaultApi.handleItem(props.item.id, { ...form })
    ElMessage.success('处置结果已登记')
    emit('update:modelValue', false)
    emit('saved')
  } finally {
    submitting.value = false
  }
}
</script>
