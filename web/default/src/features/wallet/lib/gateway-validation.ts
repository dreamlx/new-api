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
import type { TopupInfo } from '../types'

/**
 * Validate whether a payment gateway is enabled before allowing selection.
 * Returns the i18n error key if the gateway is not enabled, or null if valid.
 * Matches classic's preTopUp validation logic.
 *
 * Key distinction from previous broken logic:
 * - type 'alipay'/'wxpay' = Epay-mediated (from pay_methods list), validated
 *   against enable_online_topup
 * - type 'direct-alipay'/'direct-wxpay' = direct API (StandaloneGatewayButton),
 *   validated against enable_alipay_topup / enable_wxpay_topup
 */
export function validatePaymentGateway(
  methodType: string,
  topupInfo: TopupInfo | null
): string | null {
  // ── Standalone gateways ─────────────────────────────────────────────
  if (methodType === PAYMENT_TYPES.PAYPAL && !topupInfo?.enable_paypal_topup) {
    return 'PayPal top-up is not enabled by the administrator'
  }
  if (methodType === PAYMENT_TYPES.STRIPE && !topupInfo?.enable_stripe_topup) {
    return 'Stripe top-up is not enabled by the administrator'
  }

  // ── Epay-mediated alipay/wxpay (from pay_methods list) ─────────────
  // These are validated against enable_online_topup because they go
  // through the Epay gateway (/api/user/pay), NOT the direct API.
  if (methodType === PAYMENT_TYPES.ALIPAY && !topupInfo?.enable_online_topup) {
    return 'Online top-up is not enabled by the administrator'
  }
  if (methodType === PAYMENT_TYPES.WECHAT && !topupInfo?.enable_online_topup) {
    return 'Online top-up is not enabled by the administrator'
  }

  // ── Direct alipay/wxpay (StandaloneGatewayButton) ──────────────────
  // These bypass Epay and call their own API endpoints directly.
  if (
    methodType === PAYMENT_TYPES.DIRECT_ALIPAY &&
    !topupInfo?.enable_alipay_topup
  ) {
    return 'Alipay top-up is not enabled by the administrator'
  }
  if (
    methodType === PAYMENT_TYPES.DIRECT_WECHAT &&
    !topupInfo?.enable_wxpay_topup
  ) {
    return 'WeChat Pay top-up is not enabled by the administrator'
  }

  // ── Waffo / Waffo Pancake ──────────────────────────────────────────
  if (
    methodType === PAYMENT_TYPES.WAFFO_PANCAKE &&
    !topupInfo?.enable_waffo_pancake_topup
  ) {
    return 'Waffo Pancake top-up is not enabled by the administrator'
  }
  if (methodType.startsWith('waffo:') && !topupInfo?.enable_waffo_topup) {
    return 'Waffo top-up is not enabled by the administrator'
  }

  // ── Catch-all: any other Epay-mediated method ──────────────────────
  if (
    !topupInfo?.enable_online_topup &&
    methodType !== PAYMENT_TYPES.STRIPE &&
    methodType !== PAYMENT_TYPES.PAYPAL &&
    methodType !== PAYMENT_TYPES.ALIPAY &&
    methodType !== PAYMENT_TYPES.WECHAT &&
    methodType !== PAYMENT_TYPES.DIRECT_ALIPAY &&
    methodType !== PAYMENT_TYPES.DIRECT_WECHAT &&
    !methodType.startsWith('waffo')
  ) {
    return 'Online top-up is not enabled by the administrator'
  }
  return null
}
