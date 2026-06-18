import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { buildSearchParams } from './filter'

describe('buildSearchParams', () => {
  test('keeps WiseModel URL filters in route schema casing', () => {
    assert.deepEqual(
      buildSearchParams(
        {
          isWisemodel: true,
          wisemodelPackageId: 'pkg_123',
          upstreamRequestId: 'up_req_123',
        },
        'common'
      ),
      {
        isWisemodel: true,
        wisemodelPackageId: 'pkg_123',
        upstreamRequestId: 'up_req_123',
      }
    )
  })
})
