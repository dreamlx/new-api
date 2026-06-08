/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  PAYMENT_TYPES,
  DEFAULT_PRESET_MULTIPLIERS,
  DEFAULT_PAYMENT_TYPE,
  DEFAULT_MIN_TOPUP,
} from '../constants'
import type { AmountRequest, PresetAmount, TopupInfo } from '../types'

// ============================================================================
// Payment Processing Functions
// ============================================================================

/**
 * Check if browser is Safari
 */
function isSafariBrowser(): boolean {
  return (
    navigator.userAgent.indexOf('Safari') > -1 &&
    navigator.userAgent.indexOf('Chrome') < 1
  )
}

/**
 * Submit payment form (for non-Stripe payments)
 */
export function submitPaymentForm(
  url: string,
  params: Record<string, unknown>
): void {
  const form = document.createElement('form')
  form.action = url
  form.method = 'POST'

  // Don't open in new tab for Safari
  if (!isSafariBrowser()) {
    form.target = '_blank'
  }

  // Add form parameters
  Object.entries(params).forEach(([key, value]) => {
    const input = document.createElement('input')
    input.type = 'hidden'
    input.name = key
    input.value = String(value)
    form.appendChild(input)
  })

  document.body.appendChild(form)
  form.submit()
  document.body.removeChild(form)
}

/**
 * Check if payment method is Stripe
 */
export function isStripePayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.STRIPE
}

/**
 * Check if payment method is PayPal
 */
export function isPayPalPayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.PAYPAL
}

/**
 * Check if payment method is Waffo Pancake
 *
 * Pancake is a metered-style payment that goes through a dedicated checkout
 * URL flow rather than the generic epay form submission, so it must be
 * special-cased in payment dispatch logic.
 */
export function isWaffoPancakePayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.WAFFO_PANCAKE
}

export function buildAmountRequest(
  amount: number,
  paymentType: string
): AmountRequest {
  return {
    amount,
    payment_method: paymentType,
  }
}

/**
 * Get default payment type from topup info
 */
export function getDefaultPaymentType(topupInfo: TopupInfo | null): string {
  if (!topupInfo) {
    return DEFAULT_PAYMENT_TYPE
  }

  // Return the first visible pay method; direct integrations replace the
  // same-named Epay methods in the wallet UI.
  const firstVisiblePayMethod = topupInfo.pay_methods?.find((method) => {
    if (method.type === PAYMENT_TYPES.ALIPAY && topupInfo.enable_alipay_topup) {
      return false
    }
    if (method.type === PAYMENT_TYPES.WECHAT && topupInfo.enable_wxpay_topup) {
      return false
    }
    return true
  })
  if (firstVisiblePayMethod) {
    return firstVisiblePayMethod.type
  }

  if (topupInfo.enable_stripe_topup) {
    return PAYMENT_TYPES.STRIPE
  }

  if (topupInfo.enable_paypal_topup) {
    return PAYMENT_TYPES.PAYPAL
  }

  if (topupInfo.enable_alipay_topup) {
    return PAYMENT_TYPES.DIRECT_ALIPAY
  }

  if (topupInfo.enable_wxpay_topup) {
    return PAYMENT_TYPES.DIRECT_WECHAT
  }

  if (topupInfo.enable_waffo_topup) {
    return PAYMENT_TYPES.WAFFO
  }

  if (topupInfo.enable_waffo_pancake_topup) {
    return PAYMENT_TYPES.WAFFO_PANCAKE
  }

  return DEFAULT_PAYMENT_TYPE
}

/**
 * Get minimum topup amount from topup info
 */
export function getMinTopupAmount(topupInfo: TopupInfo | null): number {
  if (!topupInfo) {
    return DEFAULT_MIN_TOPUP
  }

  const defaultPaymentType = getDefaultPaymentType(topupInfo)
  const defaultPayMethod = topupInfo.pay_methods?.find(
    (method) => method.type === defaultPaymentType
  )
  if (defaultPayMethod?.min_topup) {
    return defaultPayMethod.min_topup
  }

  if (defaultPaymentType === PAYMENT_TYPES.STRIPE) {
    return topupInfo.stripe_min_topup || DEFAULT_MIN_TOPUP
  }

  if (defaultPaymentType === PAYMENT_TYPES.PAYPAL) {
    return topupInfo.paypal_min_topup || DEFAULT_MIN_TOPUP
  }

  if (defaultPaymentType === PAYMENT_TYPES.DIRECT_ALIPAY) {
    return topupInfo.alipay_min_topup || DEFAULT_MIN_TOPUP
  }

  if (defaultPaymentType === PAYMENT_TYPES.DIRECT_WECHAT) {
    return topupInfo.wxpay_min_topup || DEFAULT_MIN_TOPUP
  }

  if (defaultPaymentType === PAYMENT_TYPES.WAFFO) {
    return topupInfo.waffo_min_topup || DEFAULT_MIN_TOPUP
  }

  if (defaultPaymentType === PAYMENT_TYPES.WAFFO_PANCAKE) {
    return topupInfo.waffo_pancake_min_topup || DEFAULT_MIN_TOPUP
  }

  if (topupInfo.enable_online_topup) {
    return topupInfo.min_topup || DEFAULT_MIN_TOPUP
  }

  return DEFAULT_MIN_TOPUP
}

/**
 * Generate preset amounts based on minimum topup
 */
export function generatePresetAmounts(minAmount: number): PresetAmount[] {
  return DEFAULT_PRESET_MULTIPLIERS.map((multiplier) => ({
    value: minAmount * multiplier,
  }))
}

/**
 * Merge custom preset amounts with discounts
 */
export function mergePresetAmounts(
  amountOptions: number[],
  discounts: Record<number, number>
): PresetAmount[] {
  if (!amountOptions || amountOptions.length === 0) {
    return []
  }

  return amountOptions.map((amount) => ({
    value: amount,
    discount: discounts[amount] || 1.0,
  }))
}
