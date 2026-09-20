<template>
  <div class="page">
    <PageHeader title="区域故障处置" description="线路或控制箱故障时, 一条区域单关联同一回路多盏路灯, 统一派工、逐盏登记、全部恢复后闭环">
      <el-button :icon="Refresh" @click="load">刷新</el-button>
      <el-button type="primary" :icon="Plus" @click="openCreate">建立区域故障</el-button>
    </PageHeader>

    <el-card shadow="never" class="circuit-card">
      <div class="section-title">
        <span>按回路概览</span>
        <span class="text-muted circuit-summary">
          区域故障 {{ circuit.total_regions }} 单 · 未闭环 {{ circuit.open_regions }} 单 ·
          影响 {{ circuit.affected_lamps }} 盏 · 已恢复 {{ circuit.restored_lamps }} 盏 · 遗留
          <el-tag type="danger" size="small" effect="plain">{{ circuit.legacy_lamps }}</el-tag> 盏
        </span>
      </div>
      <el-table :data="circuit.circuits" size="small" v-loading="circuitLoading">
        <el-table-column prop="circuit_code" label="回路编号" width="150" />
        <el-table-column label="涉及道路" min-width="140">
          <template #default="{ row }">{{ (row.road_names || []).join('、') || '-' }}</template>
        </el-table-column>
        <el-table-column label="区域单(未闭环)" width="120" align="center">
          <template #default="{ row }">
            {{ row.region_total }}
            <el-tag v-if="row.open_total" type="warning" size="small" effect="plain">{{ row.open_total }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="影响范围" width="100" align="center">
          <template #default="{ row }">{{ row.affected_total }} 盏</template>
        </el-table-column>
        <el-table-column label="恢复耗时(均)" width="120" align="center">
          <template #default="{ row }">{{ formatHours(row.avg_recovery_hours) }}</template>
        </el-table-column>
        <el-table-column label="遗留数量" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="row.legacy_total ? 'danger' : 'success'" size="small" effect="plain">
              {{ row.legacy_total }} 盏
            </el-tag>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card shadow="never">
      <div class="filter-bar">
        <el-input v-model="query.keyword" placeholder="区域单号 / 回路编号 / 道路 / 描述" clearable @keyup.enter="handleSearch" />
        <el-select v-model="query.status" placeholder="处理状态" clearable @change="handleSearch">
          <el-option v-for="(item, key) in REGION_STATUS" :key="key" :label="item.label" :value="key" />
        </el-select>
        <el-select v-model="query.cause" placeholder="故障成因" clearable @change="handleSearch">
          <el-option v-for="(item, key) in REGION_CAUSE" :key="key" :label="item.label" :value="key" />
        </el-select>
        <el-date-picker
          v-model="dateRange"
          type="daterange"
          value-format="YYYY-MM-DD"
          range-separator="至"
          start-placeholder="发生开始日期"
          end-placeholder="发生结束日期"
          @change="handleSearch"
        />
        <el-checkbox v-model="query.only_open" @change="handleSearch">仅看未闭环</el-checkbox>
        <el-button type="primary" :icon="Search" @click="handleSearch">查询</el-button>
        <el-button :icon="RefreshLeft" @click="handleReset">重置</el-button>
      </div>
    </el-card>

    <el-card shadow="never">
      <el-table v-loading="loading" :data="rows" stripe>
        <el-table-column prop="region_no" label="区域单号" width="150" fixed="left" />
        <el-table-column label="成因" width="110">
          <template #default="{ row }"><StatusTag :dict="REGION_CAUSE" :value="row.cause" /></template>
        </el-table-column>
        <el-table-column prop="circuit_code" label="受影响回路" width="140" />
        <el-table-column prop="road_name" label="主要道路" min-width="110" show-overflow-tooltip />
        <el-table-column label="紧急程度" width="90">
          <template #default="{ row }"><StatusTag :dict="FAULT_LEVEL" :value="row.fault_level" /></template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }"><StatusTag :dict="REGION_STATUS" :value="row.status" /></template>
        </el-table-column>
        <el-table-column label="影响 / 恢复 / 遗留" width="140" align="center">
          <template #default="{ row }">
            <span>{{ row.affected_count }}</span> /
            <span class="restored-text">{{ row.restored_count }}</span> /
            <el-tag :type="row.affected_count - row.restored_count ? 'danger' : 'success'" size="small" effect="plain">
              {{ row.affected_count - row.restored_count }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="dispatch_team" label="处置班组" width="120" />
        <el-table-column label="发生时间" width="150">
          <template #default="{ row }">{{ formatDateTime(row.reported_at) }}</template>
        </el-table-column>
        <el-table-column label="恢复耗时" width="100">
          <template #default="{ row }">{{ recoveryText(row) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openDetail(row)">处置详情</el-button>
          </template>
        </el-table-column>
      </el-table>
      <DataPagination
        :page="query.page"
        :page-size="query.page_size"
        :total="total"
        @page-change="changePage"
        @size-change="changePageSize"
      />
    </el-card>

    <RegionFormDialog v-model="formVisible" @saved="handleSaved" />
    <RegionDetailDrawer v-model="detailVisible" :region-id="activeRegionId" @changed="handleChanged" />
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { Plus, Refresh, RefreshLeft, Search } from '@element-plus/icons-vue'
import PageHeader from '@/components/common/PageHeader.vue'
import StatusTag from '@/components/common/StatusTag.vue'
import DataPagination from '@/components/common/DataPagination.vue'
import RegionFormDialog from './components/RegionFormDialog.vue'
import RegionDetailDrawer from './components/RegionDetailDrawer.vue'
import { regionApi } from '@/api/region'
import { FAULT_LEVEL, REGION_CAUSE, REGION_STATUS } from '@/constants/dict'
import { formatDateTime, formatHours } from '@/utils/format'
import { useListPage } from '@/composables/useListPage'

const { loading, rows, total, query, load, search, reset, changePage, changePageSize } = useListPage(regionApi.list, {
  keyword: '',
  status: '',
  cause: '',
  start_date: '',
  end_date: '',
  only_open: false,
})

const dateRange = ref([])
const formVisible = ref(false)
const detailVisible = ref(false)
const activeRegionId = ref(null)

const circuitLoading = ref(false)
const emptyCircuit = () => ({
  total_regions: 0,
  open_regions: 0,
  affected_lamps: 0,
  restored_lamps: 0,
  legacy_lamps: 0,
  circuits: [],
})
const circuit = ref(emptyCircuit())

async function loadCircuit() {
  circuitLoading.value = true
  try {
    circuit.value = await regionApi.circuitOverview()
  } catch (error) {
    circuit.value = emptyCircuit()
  } finally {
    circuitLoading.value = false
  }
}

function recoveryText(row) {
  if (!row.restored_at) return '-'
  const hours = (new Date(row.restored_at) - new Date(row.reported_at)) / 3600000
  return formatHours(Math.max(hours, 0))
}

function handleSearch() {
  query.start_date = dateRange.value?.[0] ?? ''
  query.end_date = dateRange.value?.[1] ?? ''
  search()
}

function handleReset() {
  dateRange.value = []
  reset()
}

function openCreate() {
  formVisible.value = true
}

function openDetail(row) {
  activeRegionId.value = row.id
  detailVisible.value = true
}

function handleSaved() {
  load()
  loadCircuit()
}

function handleChanged() {
  load()
  loadCircuit()
}

onMounted(loadCircuit)
</script>

<style scoped>
.circuit-card {
  margin-bottom: 12px;
}

.section-title {
  font-weight: 600;
  margin-bottom: 10px;
  display: flex;
  align-items: center;
  gap: 12px;
}

.circuit-summary {
  font-weight: 400;
  font-size: 13px;
}

.restored-text {
  color: var(--el-color-success);
}
</style>
