import request from './request'

// 区域故障接口: 线路/控制箱故障导致同一回路多盏路灯受影响时统一处置。
export const areaFaultApi = {
  list: (params) => request.get('/area-faults', { params }),
  detail: (id) => request.get(`/area-faults/${id}`),
  create: (data) => request.post('/area-faults', data),
  dispatch: (id, data) => request.post(`/area-faults/${id}/dispatch`, data),
  close: (id, data) => request.post(`/area-faults/${id}/close`, data),
  handleItem: (itemId, data) => request.post(`/area-faults/items/${itemId}/handle`, data),
  meta: () => request.get('/area-faults/meta'),
  overview: () => request.get('/area-faults/overview'),
}
