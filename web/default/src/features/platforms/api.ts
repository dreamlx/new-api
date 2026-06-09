/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { api } from '@/lib/api'
import type {
  ApiEnvelope,
  CreatePlatformPayload,
  ListPlatformsResponse,
  Platform,
  PlatformCreated,
  UpdatePlatformPayload,
} from './types'

const BASE = '/api/admin/v2/platforms'

/** Paginated list. Hash + plaintext sk are never returned by this endpoint. */
export async function listPlatforms(
  page = 1,
  pageSize = 20
): Promise<ApiEnvelope<ListPlatformsResponse>> {
  const res = await api.get(`${BASE}?page=${page}&page_size=${pageSize}`)
  return res.data
}

export async function getPlatform(id: number): Promise<ApiEnvelope<Platform>> {
  const res = await api.get(`${BASE}/${id}`)
  return res.data
}

/**
 * Create a platform. The response includes the plaintext platform_sk EXACTLY
 * ONCE — callers must surface it via the SkRevealDialog and warn the user
 * that it cannot be retrieved later.
 */
export async function createPlatform(
  payload: CreatePlatformPayload
): Promise<ApiEnvelope<PlatformCreated>> {
  const res = await api.post(BASE, payload)
  return res.data
}

/** Update mutable fields only. platform_id and sk are not changeable here. */
export async function updatePlatform(
  payload: UpdatePlatformPayload
): Promise<ApiEnvelope<Platform>> {
  const { id, ...body } = payload
  const res = await api.patch(`${BASE}/${id}`, body)
  return res.data
}

/**
 * Delete a platform. Backend transaction also disables every token owned by
 * the platform's shadow user. Response includes tokens_disabled count.
 */
export async function deletePlatform(
  id: number
): Promise<
  ApiEnvelope<{ id: number; platform_id: string; tokens_disabled: number }>
> {
  const res = await api.delete(`${BASE}/${id}`)
  return res.data
}
