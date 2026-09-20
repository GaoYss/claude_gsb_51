<template>
  <el-dialog
    :model-value="modelValue"
    title="建立区域故障"
    width="720px"
    @update:model-value="$emit('update:modelValue', $event)"
    @open="syncForm"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
      <el-row :gutter="16">
        <el-col :span="12">
          <el-form-item label="故障诱因" prop="cause">
            <el-radio-group v-model="form.cause">
              <el-radio-button v-for="(item, key) in AREA_CAUSE" :key="key" :value="key">
                {{ item.label }}
              </el-radio-button>
            </el-radio-group>
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
          <el-form-item label="回路名称" prop="circuit_name">
            <el-input v-model="form.circuit_name" placeholder="如: 滨江路东段回路 / 解放路控制箱-A" />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="所属道路" prop="road_name">
            <el-input v-model="form.road_name" placeholder="选填, 默认取首盏路灯道路" />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="故障类型" prop="fault_type">
            <el-select v-model="form.fault_type" style="width: 100%">
              <el-option v-for="item in faultTypeOptions" :key="item" :label="item" :value="item" />
            </el-select>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="上报时间" prop="reported_at">
            <el-date-picker
              v-model="form.reported_at"
              type="datetime"
              value-format="YYYY-MM-DD HH:mm:ss"
              placeholder="默认取当前时间"
              style="width: 100%"
            />
          </el-form-item>
        </el-col>
      </el-row>

      <el-form-item label="受影响路灯" prop="lamp_ids">
        <el-select
          v-model="form.lamp_ids"
          multiple
          filterable
          remote
          reserve-keyword
          :remote-method="searchLamps"
          :loading="lampLoading"
          :max-collapse-tags="4"
          collapse-tags
          collapse-tags-tooltip
          placeholder="按路灯编号 / 道路搜索, 可多选同一回路的多盏路灯"
          style="width: 100%"
          @change="handleLampChange"
        >
          <el-option
            v-for="item in lampCandidates"
            :key="item.id"
            :label="`${item.code} · ${item.road_name} · ${item.name || '未命名'} · ${dictLabel(RUN_STATUS, item.run_status)}`"
            :value="item.id"
          />
        </el-select>
        <div class="form-hint text-muted">
          已选择 <b>{{ form.lamp_ids.length }}</b> 盏路灯, 建单后将为每盏路灯各生成一条故障单并逐盏登记处置结果
        </div>
      </el-form-item>

      <el-form-item label="上报人" prop="reporter">
        <el-input v-model="form.reporter" placeholder="默认 监控中心" />
      </el-form-item>
      <el-form-item label="故障描述" prop="description">
        <el-input
          v-model="form.description"
          type="textarea"
          :rows="3"
          maxlength="512"
          show-word-limit
          placeholder="请描述线路或控制箱故障现象与影响范围"
        />
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="$emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">建立区域故障</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { areaFaultApi } from '@/api/areaFault'
import { lampApi } from '@/api/lamp'
import { AREA_CAUSE, FAULT_LEVEL, RUN_STATUS, dictLabel } from '@/constants/dict'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  faultTypeOptions: { type: Array, default: () => [] },
})

const emit = defineEmits(['update:modelValue', 'saved'])

const formRef = ref(null)
const submitting = ref(false)
const lampLoading = ref(false)
const lampCandidates = ref([])

const createForm = () => ({
  cause: 'line',
  circuit_name: '',
  road_name: '',
  lamp_ids: [],
  fault_type: '线路故障',
  fault_level: 'high',
  description: '',
  reporter: '',
  reported_at: '',
})

const form = reactive(createForm())

const rules = {
  cause: [{ required: true, message: '请选择故障诱因', trigger: 'change' }],
  circuit_name: [{ required: true, message: '请填写回路名称', trigger: 'blur' }],
  fault_type: [{ required: true, message: '请选择故障类型', trigger: 'change' }],
  lamp_ids: [{ type: 'array', required: true, min: 1, message: '至少选择一盏受影响路灯', trigger: 'change' }],
  description: [{ required: true, message: '请填写故障描述', trigger: 'blur' }],
}

async function searchLamps(keyword = '') {
  lampLoading.value = true
  try {
    const data = await lampApi.list({ keyword, page: 1, page_size: 50 }, { silent: true })
    lampCandidates.value = data?.items ?? []
  } catch (error) {
    lampCandidates.value = []
  } finally {
    lampLoading.value = false
  }
}

// 首次选择路灯时, 用其所在道路兜底填充道路字段。
function handleLampChange(ids) {
  if (form.road_name || ids.length === 0) return
  const first = lampCandidates.value.find((item) => item.id === ids[0])
  if (first) form.road_name = first.road_name
}

function syncForm() {
  Object.assign(form, createForm())
  searchLamps('')
}

async function handleSubmit() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    const payload = { ...form }
    if (!payload.reported_at) delete payload.reported_at
    if (!payload.road_name) delete payload.road_name
    if (!payload.reporter) delete payload.reporter
    await areaFaultApi.create(payload)
    ElMessage.success('区域故障已建立, 并为受影响路灯逐盏生成故障单')
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
