import type { UpdateOptionResponse } from '../types'

export function assertOptionUpdateSuccess(
  response: UpdateOptionResponse,
  fallbackMessage = 'Failed to update setting'
): UpdateOptionResponse {
  if (!response.success) {
    throw new Error(response.message || fallbackMessage)
  }
  return response
}
