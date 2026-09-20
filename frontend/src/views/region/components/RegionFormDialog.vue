<template>
  <el-dialog
    :model-value="modelValue"
    title="建立区域故障"
    width="760px"
    @update:model-value="$emit('update:modelValue', $event)"
    @open="syncForm"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
      <el-form-item label="关联方式">
        <el-radio-group v-model="mode">
          <el-radio-button label="circuit">按整条回路</el-radio-button>
          <el-radio-button label="lamps">勾选受影响路灯</el-radio-button>
        </el-radio-group>
      </el-form-item>

      <el-form-item v-if="mode === 'circuit'" label="受影响回路" prop="circuit_code">
        <el-select v-model="form.circuit_code" filterable placeholder="选择照明回路(回路下在运路灯全部关联)" style="width: 100%">
          <el-option v-for="item in circuitOptions" :key="item" :label="item" :value="item" />
        </el-select>
      </el-form-item>

      <template v-else>
        <el-form-item label="受影响路灯" required>
          <el-select
            v-model="form.lamp_ids"
            multiple
            filterable
            collapse-tags
            collapse-tags-tooltip
            :reserve-keyword="false"
            placeholder="搜索并勾选同回路受影响路灯"
            style="width: 100%"
          >
            <el-option v-for="item in lampCandidates" :key="item.id" :label="lampLabel(item)" :value="item.id" />
          </el-select>
          <div v-if="selectedCircuit" class="form-hint text-muted">
            所选路灯回路: {{ selectedCircuit }} · 已选 {{ form.lamp_ids.length }} 盏
          </div>
        </el-form-item>
      </template>

      <el-row :gutter="16">
        <el-col :span="12">
          <el-form-item label="故障成因" prop="cause">
            <el-select v-model="form.cause" style="width: 100%">
              <el-option v-for="(item, key) in REGION_CAUSE" :key="key" :label="item.label" :value="key" />
            </el-select>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="紧急程度" prop="fault_level">
            <el-select v-model="form.fault_level" style="width: 100%">
              <el-option v-for="(item, key) in FAULT_LEVEL" :key="key" :label="item.label" :value="key" />
            </el-select>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="故障来源" prop="source">
            <el-select v-model="form.source" style="width: 100%">
              <el-option v-for="(item, key) in FAULT_SOURCE" :key="key" :label="item.label" :value="key" />
            </el-select>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="发生时间" prop="reported_at">
            <el-date-picker
              v-model="form.reported_at"
              type="datetime"
              value-format="YYYY-MM-DD HH:mm:ss"
              placeholder="默认取当前时间"
              style="width: 100%"
            />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="上报人" prop="reporter">
            <el-input v-model="form.reporter" placeholder="例如 监控中心 / 巡检员" />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="联系电话" prop="reporter_phone">
            <el-input v-model="form.reporter_phone" placeholder="选填" />
          </el-form-item>
        </el-col>
        <el-col :span="24">
          <el-form-item label="故障描述" prop="description">
            <el-input
              v-model="form.description"
              type="textarea"
              :rows="3"
              maxlength="512"
              show-word-limit
              placeholder="请描述线路/控制箱故障现象与影响范围"
            />
          </el-form-item>
        </el-col>
      </el-row>
    </el-form>

    <template #footer>
      <el-button @click="$emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">建立并关联路灯</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { lampApi } from '@/api/lamp'
import { regionApi } from '@/api/region'
import { FAULT_LEVEL, FAULT_SOURCE, REGION_CAUSE } from '@/constants/dict'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
})

const emit = defineEmits(['update:modelValue', 'saved'])

const formRef = ref(null)
const submitting = ref(false)
const mode = ref('circuit')
const lampCandidates = ref([])

const createForm = () => ({
  circuit_code: '',
  lamp_ids: [],
  cause: '线路故障',
  fault_level: 'high',
  source: 'monitoring',
  description: '',
  reporter: '',
  reporter_phone: '',
  reported_at: '',
})
const form = reactive(createForm())

const circuitOptions = computed(() => {
  const values = new Set()
  lampCandidates.value.forEach((item) => item.circuit_code && values.add(item.circuit_code))
  return [...values].sort()
})

const selectedCircuit = computed(() => {
  const first = lampCandidates.value.find((item) => item.id === form.lamp_ids[0])
  return first?.circuit_code || ''
})

const rules = {
  cause: [{ required: true, message: '请选择故障成因', trigger: 'change' }],
  description: [{ required: true, message: '请填写故障描述', trigger: 'blur' }],
}

function lampLabel(item) {
  return `${item.code} · ${item.circuit_code || '未登记回路'} · ${item.road_name} · ${item.name || '未命名'}`
}

async function loadLamps() {
  const data = await lampApi.list({ page: 1, page_size: 200 }, { silent: true })
  lampCandidates.value = data?.items ?? []
}

async function syncForm() {
  Object.assign(form, createForm())
  mode.value = 'circuit'
  await loadLamps()
}

function validateTarget() {
  if (mode.value === 'circuit') {
    if (!form.circuit_code) {
      ElMessage.warning('请选择受影响回路')
      return false
    }
  } else if (form.lamp_ids.length === 0) {
    ElMessage.warning('请至少勾选一盏受影响路灯')
    return false
  }
  return true
}

async function handleSubmit() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid || !validateTarget()) return

  submitting.value = true
  try {
    const payload = { ...form }
    if (!payload.reported_at) delete payload.reported_at
    if (mode.value === 'circuit') {
      delete payload.lamp_ids
    } else {
      delete payload.circuit_code
    }
    await regionApi.create(payload)
    ElMessage.success('区域故障已建立, 同回路受影响路灯已关联')
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
