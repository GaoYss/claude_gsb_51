<template>
  <div class="page">
    <PageHeader title="区域故障处置" description="线路或控制箱故障导致同一回路多盏路灯受影响时, 统一建单、派工、逐盏登记并整体闭环">
      <el-button :icon="Refresh" @click="load">刷新</el-button>
      <el-button type="primary" :icon="Plus" @click="openCreate">建立区域故障</el-button>
    </PageHeader>

    <el-card shadow="never">
      <div class="filter-bar">
        <el-input v-model="query.keyword" placeholder="区域单号 / 回路 / 道路 / 描述" clearable @keyup.enter="handleSearch" />
        <el-select v-model="query.status" placeholder="处置状态" clearable @change="handleSearch">
          <el-option v-for="(item, key) in AREA_STATUS" :key="key" :label="item.label" :value="key" />
        </el-select>
        <el-select v-model="query.cause" placeholder="故障诱因" clearable @change="handleSearch">
          <el-option v-for="(item, key) in AREA_CAUSE" :key="key" :label="item.label" :value="key" />
        </el-select>
        <el-select v-model="query.fault_level" placeholder="紧急程度" clearable @change="handleSearch">
          <el-option v-for="(item, key) in FAULT_LEVEL" :key="key" :label="item.label" :value="key" />
        </el-select>
        <el-checkbox v-model="query.only_open" @change="handleSearch">仅看未闭环</el-checkbox>
        <el-button type="primary" :icon="Search" @click="handleSearch">查询</el-button>
        <el-button :icon="RefreshLeft" @click="handleReset">重置</el-button>
      </div>
    </el-card>

    <el-card shadow="never">
      <el-table v-loading="loading" :data="rows" stripe>
        <el-table-column prop="area_no" label="区域单号" width="150" fixed="left" />
        <el-table-column label="诱因" width="100">
          <template #default="{ row }"><StatusTag :dict="AREA_CAUSE" :value="row.cause" /></template>
        </el-table-column>
        <el-table-column prop="circuit_name" label="故障回路" min-width="150" show-overflow-tooltip />
        <el-table-column prop="road_name" label="所属道路" min-width="110" show-overflow-tooltip />
        <el-table-column label="紧急程度" width="90">
          <template #default="{ row }"><StatusTag :dict="FAULT_LEVEL" :value="row.fault_level" /></template>
        </el-table-column>
        <el-table-column label="处置状态" width="100">
          <template #default="{ row }"><StatusTag :dict="AREA_STATUS" :value="row.status" /></template>
        </el-table-column>
        <el-table-column label="恢复进度" width="150">
          <template #default="{ row }">
            <el-progress :percentage="percent(row)" :status="row.pending_count === 0 ? 'success' : ''" :stroke-width="12" text-inside />
          </template>
        </el-table-column>
        <el-table-column label="影响/遗留" width="100" align="center">
          <template #default="{ row }">
            <span>{{ row.total_count }} / <b :class="row.pending_count > 0 ? 'text-danger' : ''">{{ row.pending_count }}</b></span>
          </template>
        </el-table-column>
        <el-table-column prop="assignee" label="负责人" width="90" />
        <el-table-column prop="repair_team" label="班组" width="120" show-overflow-tooltip />
        <el-table-column label="上报时间" width="150">
          <template #default="{ row }">{{ formatDateTime(row.reported_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="190" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openDetail(row)">详情</el-button>
            <el-button v-if="row.status === 'pending'" link type="warning" @click="openDispatch(row)">派工</el-button>
            <el-button
              v-if="row.status === 'restored'"
              link
              type="success"
              @click="quickClose(row)"
            >闭环</el-button>
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

    <AreaFaultFormDialog
      v-model="formVisible"
      :fault-type-options="faultTypeOptions"
      @saved="handleSaved"
    />
    <AreaDispatchDialog v-model="dispatchVisible" :area="dispatchTarget" @saved="load" />
    <AreaFaultDetailDrawer v-model="detailVisible" :area-fault-id="activeId" @changed="load" />
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Refresh, RefreshLeft, Search } from '@element-plus/icons-vue'
import PageHeader from '@/components/common/PageHeader.vue'
import StatusTag from '@/components/common/StatusTag.vue'
import DataPagination from '@/components/common/DataPagination.vue'
import AreaFaultFormDialog from './components/AreaFaultFormDialog.vue'
import AreaDispatchDialog from './components/AreaDispatchDialog.vue'
import AreaFaultDetailDrawer from './components/AreaFaultDetailDrawer.vue'
import { areaFaultApi } from '@/api/areaFault'
import { useDictStore } from '@/stores/dict'
import { AREA_CAUSE, AREA_STATUS, FAULT_LEVEL } from '@/constants/dict'
import { formatDateTime } from '@/utils/format'
import { useListPage } from '@/composables/useListPage'

const dictStore = useDictStore()

const { loading, rows, total, query, load, search, reset, changePage, changePageSize } = useListPage(areaFaultApi.list, {
  keyword: '',
  status: '',
  cause: '',
  fault_level: '',
  only_open: false,
})

// 区域故障的线路/控制箱类型复用故障模块的故障类型字典。
const faultTypeOptions = computed(() => {
  const types = dictStore.faultMeta.fault_types ?? []
  return types.length ? types : ['线路故障', '控制箱故障']
})

const formVisible = ref(false)
const dispatchVisible = ref(false)
const detailVisible = ref(false)
const dispatchTarget = ref(null)
const activeId = ref(null)

function percent(row) {
  if (!row.total_count) return 0
  return Math.round((row.recovered_count / row.total_count) * 100)
}

function handleSearch() {
  search()
}

function handleReset() {
  reset()
}

function openCreate() {
  formVisible.value = true
}

function openDetail(row) {
  activeId.value = row.id
  detailVisible.value = true
}

function openDispatch(row) {
  dispatchTarget.value = { ...row }
  dispatchVisible.value = true
}

async function quickClose(row) {
  try {
    const { value } = await ElMessageBox.prompt(`区域故障 ${row.area_no} 已全部恢复, 请输入闭环说明`, '区域故障整体闭环', {
      confirmButtonText: '确认闭环',
      cancelButtonText: '取消',
      inputPlaceholder: '闭环说明',
    })
    await areaFaultApi.close(row.id, { remark: value ?? '' })
    ElMessage.success('区域故障已整体闭环')
    load()
  } catch (error) {
    // 用户取消或请求失败
  }
}

function handleSaved() {
  load()
}

onMounted(() => {
  dictStore.ensureLoaded().catch(() => {})
})
</script>

<style scoped>
.text-danger {
  color: var(--el-color-danger);
}
</style>
