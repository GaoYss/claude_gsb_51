<template>
  <el-drawer
    :model-value="modelValue"
    title="区域故障处置详情"
    size="760px"
    @update:model-value="$emit('update:modelValue', $event)"
    @open="load"
  >
    <div v-loading="loading">
      <template v-if="detail.id">
        <div class="drawer-actions">
          <el-button
            v-if="detail.status === 'pending'"
            type="primary"
            :icon="Promotion"
            @click="dispatchVisible = true"
          >统一派工</el-button>
          <el-button
            v-if="detail.status === 'restored'"
            type="success"
            :icon="CircleCheck"
            @click="handleClose"
          >整体闭环</el-button>
          <el-tag v-if="detail.status !== 'restored' && detail.status !== 'closed'" type="warning" effect="plain">
            全部恢复后才能整体闭环, 当前遗留 {{ detail.pending_count }} 盏
          </el-tag>
          <el-tag v-if="detail.status === 'closed'" type="info" effect="plain">已闭环</el-tag>
        </div>

        <el-descriptions :column="2" border size="small" title="区域故障信息">
          <el-descriptions-item label="区域单号">{{ detail.area_no }}</el-descriptions-item>
          <el-descriptions-item label="处置状态">
            <StatusTag :dict="AREA_STATUS" :value="detail.status" />
          </el-descriptions-item>
          <el-descriptions-item label="故障诱因">
            <StatusTag :dict="AREA_CAUSE" :value="detail.cause" />
          </el-descriptions-item>
          <el-descriptions-item label="紧急程度">
            <StatusTag :dict="FAULT_LEVEL" :value="detail.fault_level" />
          </el-descriptions-item>
          <el-descriptions-item label="回路名称">{{ detail.circuit_name }}</el-descriptions-item>
          <el-descriptions-item label="所属道路">{{ detail.road_name || '-' }}</el-descriptions-item>
          <el-descriptions-item label="故障类型">{{ detail.fault_type }}</el-descriptions-item>
          <el-descriptions-item label="上报人">{{ detail.reporter || '-' }}</el-descriptions-item>
          <el-descriptions-item label="上报时间">{{ formatDateTime(detail.reported_at) }}</el-descriptions-item>
          <el-descriptions-item label="派工时间">{{ formatDateTime(detail.dispatched_at) }}</el-descriptions-item>
          <el-descriptions-item v-if="detail.all_restored_at" label="全部恢复时间">
            {{ formatDateTime(detail.all_restored_at) }}
          </el-descriptions-item>
          <el-descriptions-item v-if="detail.all_restored_at" label="恢复耗时">
            {{ formatDuration(detail.restore_minutes) }}
          </el-descriptions-item>
          <el-descriptions-item label="负责人">{{ detail.assignee || '-' }}</el-descriptions-item>
          <el-descriptions-item label="维修班组">{{ detail.repair_team || '-' }}</el-descriptions-item>
          <el-descriptions-item label="故障描述" :span="2">{{ detail.description || '-' }}</el-descriptions-item>
          <el-descriptions-item v-if="detail.closed_at" label="闭环时间">
            {{ formatDateTime(detail.closed_at) }}
          </el-descriptions-item>
          <el-descriptions-item v-if="detail.closed_at" label="闭环说明" :span="1">
            {{ detail.close_remark || '-' }}
          </el-descriptions-item>
        </el-descriptions>

        <div class="drawer-block progress-box">
          <el-progress
            :percentage="recoverPercent"
            :status="detail.pending_count === 0 ? 'success' : ''"
            :stroke-width="16"
            text-inside
          />
          <div class="progress-text text-muted">
            共 {{ detail.total_count }} 盏 · 已恢复 {{ detail.recovered_count }} 盏 · 遗留 {{ detail.pending_count }} 盏
          </div>
        </div>

        <div class="section-title drawer-block">逐盏处置明细</div>
        <el-table :data="detail.items" size="small" border>
          <el-table-column prop="lamp_code" label="路灯编号" width="100" />
          <el-table-column prop="road_name" label="道路" min-width="100" show-overflow-tooltip />
          <el-table-column label="处置结果" width="100">
            <template #default="{ row }">
              <StatusTag :dict="AREA_ITEM_RESULT" :value="row.result" />
            </template>
          </el-table-column>
          <el-table-column prop="handler" label="处置人" width="90" />
          <el-table-column label="处置时间" width="135">
            <template #default="{ row }">{{ formatDateTime(row.handled_at) }}</template>
          </el-table-column>
          <el-table-column prop="handle_remark" label="处置说明" min-width="150" show-overflow-tooltip />
          <el-table-column label="操作" width="90" fixed="right">
            <template #default="{ row }">
              <el-button
                v-if="detail.status === 'dispatched' || detail.status === 'restored'"
                link
                type="primary"
                @click="openHandle(row)"
              >{{ row.result === 'pending' ? '登记' : '改判' }}</el-button>
              <span v-else class="text-muted">-</span>
            </template>
          </el-table-column>
        </el-table>
      </template>
      <el-empty v-else description="暂无区域故障数据" />
    </div>

    <AreaDispatchDialog v-model="dispatchVisible" :area="detail" @saved="load" />
    <ItemHandleDialog v-model="handleVisible" :item="activeItem" @saved="load" />
  </el-drawer>
</template>

<script setup>
import { computed, ref } from 'vue'
import { CircleCheck, Promotion } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import StatusTag from '@/components/common/StatusTag.vue'
import { areaFaultApi } from '@/api/areaFault'
import { AREA_CAUSE, AREA_ITEM_RESULT, AREA_STATUS, FAULT_LEVEL } from '@/constants/dict'
import { formatDateTime, formatDuration } from '@/utils/format'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  areaFaultId: { type: [Number, String], default: null },
})

const emit = defineEmits(['update:modelValue', 'changed'])

const loading = ref(false)
const detail = ref({})
const dispatchVisible = ref(false)
const handleVisible = ref(false)
const activeItem = ref(null)

const recoverPercent = computed(() => {
  if (!detail.value.total_count) return 0
  return Math.round((detail.value.recovered_count / detail.value.total_count) * 100)
})

async function load() {
  if (!props.areaFaultId) return
  loading.value = true
  try {
    detail.value = await areaFaultApi.detail(props.areaFaultId)
  } catch (error) {
    detail.value = {}
  } finally {
    loading.value = false
  }
}

function openHandle(row) {
  activeItem.value = { ...row }
  handleVisible.value = true
}

async function handleClose() {
  try {
    const { value } = await ElMessageBox.prompt(
      `区域故障 ${detail.value.area_no} 已全部恢复, 请输入闭环说明`,
      '区域故障整体闭环',
      { confirmButtonText: '确认闭环', cancelButtonText: '取消', inputPlaceholder: '闭环说明' },
    )
    await areaFaultApi.close(detail.value.id, { remark: value ?? '' })
    ElMessage.success('区域故障已整体闭环')
    load()
    emit('changed')
  } catch (error) {
    // 用户取消或请求失败, 提示由拦截器处理
  }
}
</script>

<style scoped>
.drawer-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}

.drawer-block {
  margin-top: 20px;
}

.progress-box {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.progress-text {
  font-size: 13px;
}
</style>
