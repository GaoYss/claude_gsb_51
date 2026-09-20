<template>
  <el-drawer
    :model-value="modelValue"
    title="区域故障处置详情"
    size="900px"
    @update:model-value="$emit('update:modelValue', $event)"
    @open="load"
  >
    <div v-loading="loading">
      <template v-if="detail.id">
        <el-descriptions :column="3" border size="small" title="区域单信息">
          <el-descriptions-item label="区域单号">{{ detail.region_no }}</el-descriptions-item>
          <el-descriptions-item label="处理状态">
            <StatusTag :dict="REGION_STATUS" :value="detail.status" />
          </el-descriptions-item>
          <el-descriptions-item label="故障成因">
            <StatusTag :dict="REGION_CAUSE" :value="detail.cause" />
          </el-descriptions-item>
          <el-descriptions-item label="受影响回路">{{ detail.circuit_code }}</el-descriptions-item>
          <el-descriptions-item label="紧急程度">
            <StatusTag :dict="FAULT_LEVEL" :value="detail.fault_level" />
          </el-descriptions-item>
          <el-descriptions-item label="发生时间">{{ formatDateTime(detail.reported_at) }}</el-descriptions-item>
          <el-descriptions-item label="处置班组">{{ detail.dispatch_team || '-' }}</el-descriptions-item>
          <el-descriptions-item label="维修负责人">{{ detail.dispatch_repairman || '-' }}</el-descriptions-item>
          <el-descriptions-item label="派工时间">
            {{ detail.dispatched_at ? formatDateTime(detail.dispatched_at) : '未派工' }}
          </el-descriptions-item>
          <el-descriptions-item label="影响 / 恢复 / 遗留" :span="3">
            <el-tag type="info" effect="plain">{{ detail.affected_count }} 盏受影响</el-tag>
            <el-tag type="success" effect="plain" class="gap-tag">{{ detail.restored_count }} 盏已恢复</el-tag>
            <el-tag :type="legacyCount ? 'danger' : 'success'" effect="plain">{{ legacyCount }} 盏遗留</el-tag>
            <span v-if="detail.restored_at" class="text-muted gap-tag">
              全部恢复耗时 {{ recoveryHours }} 小时
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="故障描述" :span="3">{{ detail.description }}</el-descriptions-item>
        </el-descriptions>

        <div class="drawer-toolbar">
          <div class="section-title">逐盏处置进展 ({{ detail.items?.length || 0 }} 盏)</div>
          <div>
            <el-button v-if="detail.status === 'pending'" type="primary" :icon="Promotion" @click="dispatchVisible = true">
              统一派工
            </el-button>
            <el-button
              v-if="detail.status !== 'closed'"
              type="success"
              :disabled="legacyCount > 0"
              @click="handleClose"
            >
              整体闭环
            </el-button>
          </div>
        </div>
        <el-alert
          v-if="detail.status !== 'closed' && legacyCount > 0"
          type="warning"
          :closable="false"
          show-icon
          :title="`仍有 ${legacyCount} 盏路灯未恢复, 未全部恢复前不允许整体闭环`"
          class="close-hint"
        />

        <el-table :data="detail.items" size="small" border>
          <el-table-column prop="lamp_code" label="路灯编号" width="105" />
          <el-table-column label="道路" min-width="90" show-overflow-tooltip>
            <template #default="{ row }">{{ row.road_name }}</template>
          </el-table-column>
          <el-table-column label="子故障状态" width="95">
            <template #default="{ row }"><StatusTag :dict="FAULT_STATUS" :value="row.fault_status" /></template>
          </el-table-column>
          <el-table-column label="路灯状态" width="90">
            <template #default="{ row }"><StatusTag :dict="RUN_STATUS" :value="row.run_status" /></template>
          </el-table-column>
          <el-table-column prop="repairman" label="维修人" width="90" />
          <el-table-column label="最近维修" width="90">
            <template #default="{ row }">
              <el-tag v-if="row.repair_status" :type="row.repair_status === 'finished' ? 'success' : 'warning'" size="small" effect="plain">
                {{ dictLabel(REPAIR_STATUS, row.repair_status) }}
              </el-tag>
              <span v-else>-</span>
            </template>
          </el-table-column>
          <el-table-column label="维修结果" width="90">
            <template #default="{ row }">{{ dictLabel(REPAIR_RESULT, row.repair_result) }}</template>
          </el-table-column>
          <el-table-column label="恢复" width="70" align="center">
            <template #default="{ row }">
              <el-icon v-if="row.restored" color="#67c23a" :size="16"><CircleCheckFilled /></el-icon>
              <el-icon v-else color="#f56c6c" :size="16"><CircleCloseFilled /></el-icon>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="190" fixed="right">
            <template #default="{ row }">
              <el-button
                v-if="canResolve(row)"
                link
                type="primary"
                @click="openResolve(row)"
              >登记处置结果</el-button>
              <el-button
                v-if="canRedispatch(row)"
                link
                type="warning"
                @click="openRedispatch(row)"
              >再次派工</el-button>
              <span v-if="row.restored" class="text-muted">等待闭环</span>
            </template>
          </el-table-column>
        </el-table>
      </template>
      <el-empty v-else description="暂无区域故障数据" />
    </div>

    <RegionDispatchDialog
      v-if="detail.status === 'pending'"
      v-model="dispatchVisible"
      :region-id="detail.id"
      @saved="afterAction"
    />
    <RegionResolveDialog
      v-model="resolveVisible"
      :region-id="detail.id"
      :item="activeItem"
      @saved="afterAction"
    />
    <RegionRedispatchDialog
      v-model="redispatchVisible"
      :region-id="detail.id"
      :item="activeItem"
      @saved="afterAction"
    />
  </el-drawer>
</template>

<script setup>
import { computed, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { CircleCheckFilled, CircleCloseFilled, Promotion } from '@element-plus/icons-vue'
import StatusTag from '@/components/common/StatusTag.vue'
import { regionApi } from '@/api/region'
import {
  FAULT_LEVEL,
  FAULT_STATUS,
  REGION_CAUSE,
  REGION_STATUS,
  REPAIR_RESULT,
  REPAIR_STATUS,
  RUN_STATUS,
  dictLabel,
} from '@/constants/dict'
import { formatDateTime } from '@/utils/format'
import RegionDispatchDialog from './RegionDispatchDialog.vue'
import RegionResolveDialog from './RegionResolveDialog.vue'
import RegionRedispatchDialog from './RegionRedispatchDialog.vue'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  regionId: { type: [Number, String], default: null },
})

const emit = defineEmits(['update:modelValue', 'changed'])

const loading = ref(false)
const detail = ref({})
const dispatchVisible = ref(false)
const resolveVisible = ref(false)
const redispatchVisible = ref(false)
const activeItem = ref(null)

const legacyCount = computed(() => (detail.value.affected_count || 0) - (detail.value.restored_count || 0))
const recoveryHours = computed(() => {
  if (!detail.value.restored_at) return '-'
  const hours = (new Date(detail.value.restored_at) - new Date(detail.value.reported_at)) / 3600000
  return Math.max(Number(hours.toFixed(1)), 0)
})

async function load() {
  if (!props.regionId) return
  loading.value = true
  try {
    detail.value = await regionApi.detail(props.regionId)
  } catch (error) {
    detail.value = {}
  } finally {
    loading.value = false
  }
}

// 有进行中的维修记录时可逐盏登记处置结果。
function canResolve(row) {
  return detail.value.status === 'processing' && row.repair_status === 'ongoing'
}

// 上一次维修已完工但未修复(待配件/观察中/无法修复)时, 可对该盏再次派工返修。
function canRedispatch(row) {
  return detail.value.status === 'processing' &&
    row.fault_status === 'processing' &&
    row.repair_status === 'finished' &&
    row.repair_result !== 'fixed'
}

function openResolve(row) {
  activeItem.value = row
  resolveVisible.value = true
}

function openRedispatch(row) {
  activeItem.value = row
  redispatchVisible.value = true
}

async function handleClose() {
  try {
    const { value } = await ElMessageBox.prompt(
      `确认区域故障 ${detail.value.region_no} 全部路灯已恢复, 执行整体闭环?`,
      '区域故障整体闭环',
      { confirmButtonText: '确认闭环', cancelButtonText: '取消', inputPlaceholder: '闭环说明(选填)' },
    )
    await regionApi.close(detail.value.id, { remark: value ?? '' })
    ElMessage.success('区域故障已整体闭环')
    afterAction()
  } catch (error) {
    // 用户取消
  }
}

async function afterAction() {
  dispatchVisible.value = false
  resolveVisible.value = false
  redispatchVisible.value = false
  await load()
  emit('changed')
}
</script>

<style scoped>
.drawer-toolbar {
  margin: 20px 0 12px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.section-title {
  font-weight: 600;
}

.gap-tag {
  margin-left: 8px;
}

.close-hint {
  margin-bottom: 12px;
}
</style>
