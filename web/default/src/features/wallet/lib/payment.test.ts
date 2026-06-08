import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { PAYMENT_TYPES } from '../constants'
import {
  buildAmountRequest,
  getDefaultPaymentType,
  getMinTopupAmount,
} from './payment'

describe('buildAmountRequest', () => {
  test('passes direct payment method through to amount preview', () => {
    assert.deepEqual(buildAmountRequest(10, PAYMENT_TYPES.DIRECT_ALIPAY), {
      amount: 10,
      payment_method: PAYMENT_TYPES.DIRECT_ALIPAY,
    })
  })
})

describe('getDefaultPaymentType', () => {
  test('uses direct Alipay type when only direct Alipay is enabled', () => {
    assert.equal(
      getDefaultPaymentType({
        enable_online_topup: false,
        enable_stripe_topup: false,
        pay_methods: [],
        min_topup: 1,
        stripe_min_topup: 1,
        amount_options: [],
        discount: {},
        enable_alipay_topup: true,
        alipay_min_topup: 5,
      }),
      PAYMENT_TYPES.DIRECT_ALIPAY
    )
  })

  test('uses direct WeChat type when only direct WeChat Pay is enabled', () => {
    assert.equal(
      getDefaultPaymentType({
        enable_online_topup: false,
        enable_stripe_topup: false,
        pay_methods: [],
        min_topup: 1,
        stripe_min_topup: 1,
        amount_options: [],
        discount: {},
        enable_wxpay_topup: true,
        wxpay_min_topup: 5,
      }),
      PAYMENT_TYPES.DIRECT_WECHAT
    )
  })
})

describe('getMinTopupAmount', () => {
  test('uses direct Alipay minimum when Epay Alipay is hidden by direct Alipay', () => {
    assert.equal(
      getMinTopupAmount({
        enable_online_topup: true,
        enable_stripe_topup: false,
        pay_methods: [{ name: 'Alipay', type: PAYMENT_TYPES.ALIPAY }],
        min_topup: 1,
        stripe_min_topup: 1,
        amount_options: [],
        discount: {},
        enable_alipay_topup: true,
        alipay_min_topup: 5,
      }),
      5
    )
  })
})
