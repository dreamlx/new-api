import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { PLATFORM_STATUS } from '../types'
import { buildCreatePlatformPayload } from './platform-form'

describe('buildCreatePlatformPayload', () => {
  test('includes status when creating a disabled platform', () => {
    assert.deepEqual(
      buildCreatePlatformPayload({
        platformId: 'acme',
        name: 'Acme',
        status: PLATFORM_STATUS.DISABLED,
        shadowUserId: 12,
      }),
      {
        platform_id: 'acme',
        name: 'Acme',
        status: PLATFORM_STATUS.DISABLED,
        shadow_user_id: 12,
      }
    )
  })
})
