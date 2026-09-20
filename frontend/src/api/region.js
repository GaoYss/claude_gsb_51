import request from './request'

// 区域故障处置接口。
export const regionApi = {
  list: (params) => request.get('/region-faults', { params }),
  detail: (id) => request.get(`/region-faults/${id}`),
  create: (data) => request.post('/region-faults', data),
  addLamps: (id, lampIds) => request.post(`/region-faults/${id}/lamps`, { lamp_ids: lampIds }),
  removeLamp: (id, faultId) => request.delete(`/region-faults/${id}/lamps/${faultId}`),
  dispatch: (id, data) => request.post(`/region-faults/${id}/dispatch`, data),
  close: (id, data) => request.post(`/region-faults/${id}/close`, data),
  resolveLamp: (id, faultId, data) => request.post(`/region-faults/${id}/lamps/${faultId}/resolve`, data),
  redispatchLamp: (id, faultId, data) => request.post(`/region-faults/${id}/lamps/${faultId}/redispatch`, data),
  circuitOverview: () => request.get('/region-faults/circuits/overview'),
  meta: () => request.get('/region-faults/meta'),
}
