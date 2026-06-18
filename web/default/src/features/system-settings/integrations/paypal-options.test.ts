import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { buildDisablePayPalOptions } from './paypal-options'

describe('buildDisablePayPalOptions', () => {
  it('clears all credentials required for PayPal availability', () => {
    assert.deepEqual(buildDisablePayPalOptions(), [
      { key: 'PayPalClientId', value: '' },
      { key: 'PayPalClientSecret', value: '' },
      { key: 'PayPalWebhookSecret', value: '' },
    ])
  })
})
