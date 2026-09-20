<template>
  <div v-loading="loading" class="page">
    <PageHeader title="区域故障回路概览" description="按故障回路汇总影响范围、恢复耗时与遗留数量">
      <el-button :icon="Refresh" @click="load">刷新</el-button>
      <el-button type="primary" @click="$router.push('/area-faults')">去处置区域故障</el-button>
    </PageHeader>

    <div class="card-grid">
      <StatCard label="区域故障" :value="overview.total" suffix="条" icon="Warning" color="#409eff"
        :hint="`未闭环 ${overview.open_total} 条 / 已闭环 ${overview.closed_total} 条`" />
      <StatCard label="累计影响路灯" :value="overview.affected_lamps" suffix="盏" icon="Postcard" color="#e6a23c"
        :hint="`涉及 ${overview.circuits.length} 个回路`" />
      <StatCard label="已恢复路灯" :value="overview.recovered_lamps" suffix="盏" icon="CircleCheck" color="#67c23a"
        hint="逐盏登记为已恢复的路灯" />
      <StatCard label="遗留路灯" :value="overview.pending_lamps" suffix="盏" icon="AlarmClock" color="#f56c6c"
        hint="尚未恢复, 阻断区域故障整体闭环" />
    </div>

    <el-row :gutter="16">
      <el-col :xs="24" :md="12">
        <el-card shadow="never">
          <div class="section-title">按诱因分布</div>
          <BarList :items="causeItems" />
        </el-card>
      </el-col>
      <el-col :xs="24" :md="12">
        <el-card shadow="never">
          <div class="section-title">按处置状态分布</div>
          <BarList :items="statusItems" />
        </el-card>
      </el-col>
    </el-row>

    <el-card shadow="never">
      <div class="section-title">各回路影响与恢复情况</div>
      <el-table :data="overview.circuits" stripe>
        <el-table-column prop="circuit_name" label="故障回路" min-width="160" fixed="left" show-overflow-tooltip />
        <el-table-column prop="road_name" label="所属道路" min-width="110" show-overflow-tooltip />
        <el-table-column label="未闭环/已闭环" width="130" align="center">
          <template #default="{ row }">
            <span><b :class="row.open_total > 0 ? 'text-danger' : ''">{{ row.open_total }}</b> / {{ row.closed_total }}</span>
          </template>
        </el-table-column>
        <el-table-column label="影响范围(盏)" width="110" align="center">
          <template #default="{ row }">{{ row.affected_lamp_count }}</template>
        </el-table-column>
        <el-table-column label="已恢复(盏)" width="100" align="center">
          <template #default="{ row }">
            <span class="text-success">{{ row.recovered_lamp_count }}</span>
          </template>
        </el-table-column>
        <el-table-column label="遗留数量(盏)" width="110" align="center">
          <template #default="{ row }">
            <el-tag :type="row.pending_lamp_count > 0 ? 'danger' : 'info'" size="small" effect="plain">
              {{ row.pending_lamp_count }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="恢复进度" width="160">
          <template #default="{ row }">
            <el-progress :percentage="circuitPercent(row)" :status="row.pending_lamp_count === 0 ? 'success' : ''"
              :stroke-width="12" text-inside />
          </template>
        </el-table-column>
        <el-table-column label="平均恢复耗时" width="140">
          <template #default="{ row }">{{ formatDuration(row.avg_restore_minutes) }}</template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import PageHeader from '@/components/common/PageHeader.vue'
import StatCard from '@/components/common/StatCard.vue'
import BarList from '@/components/common/BarList.vue'
import { areaFaultApi } from '@/api/areaFault'
import { AREA_CAUSE, AREA_STATUS } from '@/constants/dict'
import { formatDuration } from '@/utils/format'

const loading = ref(false)

const emptyOverview = () => ({
  total: 0,
  open_total: 0,
  closed_total: 0,
  affected_lamps: 0,
  recovered_lamps: 0,
  pending_lamps: 0,
  by_cause: [],
  by_status: [],
  circuits: [],
})

const overview = ref(emptyOverview())

const causeItems = computed(() =>
  (overview.value.by_cause ?? []).map((item) => ({
    label: AREA_CAUSE[item.label]?.label ?? item.label,
    count: item.count,
  })),
)

const statusItems = computed(() =>
  (overview.value.by_status ?? []).map((item) => ({
    label: AREA_STATUS[item.label]?.label ?? item.label,
    count: item.count,
  })),
)

function circuitPercent(row) {
  if (!row.affected_lamp_count) return 0
  return Math.round((row.recovered_lamp_count / row.affected_lamp_count) * 100)
}

async function load() {
  loading.value = true
  try {
    overview.value = await areaFaultApi.overview()
  } catch (error) {
    overview.value = emptyOverview()
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.text-danger {
  color: var(--el-color-danger);
}

.text-success {
  color: var(--el-color-success);
}
</style>
