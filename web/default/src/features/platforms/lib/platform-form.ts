import type { CreatePlatformPayload, PlatformStatus } from '../types'

type CreatePlatformFormValues = {
  platformId: string
  name: string
  status: PlatformStatus
  shadowUserId?: number
}

export function buildCreatePlatformPayload({
  platformId,
  name,
  status,
  shadowUserId,
}: CreatePlatformFormValues): CreatePlatformPayload {
  return {
    platform_id: platformId,
    name,
    status,
    ...(shadowUserId !== undefined ? { shadow_user_id: shadowUserId } : {}),
  }
}
