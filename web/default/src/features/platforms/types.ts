/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { z } from 'zod'

// ============================================================================
// Platform Schema & Types
// ============================================================================

/** Platform status mirrors common.UserStatus*: 1 = enabled, 2 = disabled. */
export const PLATFORM_STATUS = {
  ENABLED: 1,
  DISABLED: 2,
} as const

export type PlatformStatus =
  (typeof PLATFORM_STATUS)[keyof typeof PLATFORM_STATUS]

export const platformSchema = z.object({
  id: z.number(),
  platform_id: z.string(),
  name: z.string(),
  status: z.number(),
  shadow_user_id: z.number(),
  created_at: z.string(),
  updated_at: z.string(),
})
export type Platform = z.infer<typeof platformSchema>

// Returned ONLY by Create — includes the one-time plaintext sk + warning.
export const platformCreatedSchema = platformSchema.extend({
  platform_sk: z.string(),
  warning: z.string().optional(),
})
export type PlatformCreated = z.infer<typeof platformCreatedSchema>

export type PlatformsDialogType = 'create' | 'update' | 'delete' | 'sk-reveal'

export interface ApiEnvelope<T> {
  success: boolean
  message?: string
  data?: T
}

export interface ListPlatformsResponse {
  items: Platform[]
  total: number
  page: number
  page_size: number
}

// Form data the mutate drawer collects. platform_id is only writable on create.
export interface CreatePlatformPayload {
  platform_id: string
  name?: string
  status?: PlatformStatus
  shadow_user_id?: number
}

export interface UpdatePlatformPayload {
  id: number
  name?: string
  status?: PlatformStatus
  shadow_user_id?: number
}
