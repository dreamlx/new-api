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
import { PAYMENT_TYPES } from '../constants'
import type { PaymentMethod, TopupInfo } from '../types'

/**
 * Check if a payment method type is a standalone gateway (doesn't depend on Epay).
 * Stripe and PayPal operate independently — they are always visible regardless
 * of the Epay (enable_online_topup) toggle.
 */
export function isStandaloneGateway(type: string): boolean {
  return type === PAYMENT_TYPES.STRIPE || type === PAYMENT_TYPES.PAYPAL
}

/**
 * Filter payment methods from the pay_methods list (matching classic logic):
 * - Remove 'waffo' (handled by dedicated Waffo section)
 * - Remove 'alipay' when direct Alipay is enabled (replaced by dedicated button)
 * - Remove 'wxpay' when direct Wxpay is enabled (replaced by dedicated button)
 * - When enableOnlineTopUp is false, only keep standalone gateways (stripe, paypal)
 */
export function filterVisiblePayMethods(
  payMethods: PaymentMethod[],
  options: {
    enableAlipayTopup?: boolean
    enableWxpayTopup?: boolean
    enableOnlineTopUp: boolean
  }
): PaymentMethod[] {
  return payMethods.filter((method) => {
    if (!method || !method.type) return false
    if (method.type === 'waffo') return false
    if (method.type === PAYMENT_TYPES.ALIPAY && options.enableAlipayTopup)
      return false
    if (method.type === PAYMENT_TYPES.WECHAT && options.enableWxpayTopup)
      return false
    // When online topup (Epay) is disabled, only show standalone gateway methods
    if (!options.enableOnlineTopUp && !isStandaloneGateway(method.type))
      return false
    return true
  })
}

/**
 * Check if any configurable topup method is available.
 * This determines whether the topup form section should be shown at all.
 */
export function hasAnyConfigurableTopup(
  topupInfo: TopupInfo | null,
  extra: {
    enableWaffoTopup?: boolean
    enableWaffoPancakeTopup?: boolean
    enablePayPalTopup?: boolean
    enableAlipayTopup?: boolean
    enableWxpayTopup?: boolean
  }
): boolean {
  return !!(
    topupInfo?.enable_online_topup ||
    topupInfo?.enable_stripe_topup ||
    extra.enableWaffoTopup ||
    extra.enableWaffoPancakeTopup ||
    extra.enablePayPalTopup ||
    extra.enableAlipayTopup ||
    extra.enableWxpayTopup
  )
}
