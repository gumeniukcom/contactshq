import client from './client'
import type { Pipeline, CreatePipelineInput, SyncRun } from '@/types'

export function listPipelines() {
  return client.get<{ pipelines: Pipeline[] }>('/pipelines')
}

export function getPipeline(id: string) {
  return client.get<Pipeline>(`/pipelines/${id}`)
}

export function createPipeline(data: CreatePipelineInput) {
  return client.post<Pipeline>('/pipelines', data)
}

export function updatePipeline(id: string, data: Partial<CreatePipelineInput>) {
  return client.put<Pipeline>(`/pipelines/${id}`, data)
}

export function deletePipeline(id: string) {
  return client.delete(`/pipelines/${id}`)
}

export function triggerPipeline(id: string) {
  return client.post(`/pipelines/${id}/trigger`)
}

export function listPipelineRuns(id: string, limit = 50) {
  return client.get<{ runs: SyncRun[] }>(`/pipelines/${id}/runs`, { params: { limit } })
}
