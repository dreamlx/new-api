import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { assertOptionUpdateSuccess } from './option-update'

describe('assertOptionUpdateSuccess', () => {
  test('throws backend rejection message', () => {
    assert.throws(
      () =>
        assertOptionUpdateSuccess({
          success: false,
          message: 'backend rejected',
        }),
      /backend rejected/
    )
  })

  test('returns successful responses unchanged', () => {
    const response = { success: true, message: 'ok' }

    assert.equal(assertOptionUpdateSuccess(response), response)
  })
})
