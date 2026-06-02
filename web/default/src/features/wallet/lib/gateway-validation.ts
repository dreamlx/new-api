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
import type { TopupInfo } from '../types'

/**
 * Validate whether a payment gateway is enabled before allowing selection.
 * Returns the i18n error key if the gateway is not enabled, or null if valid.
 * Matches classic's preTopUp validation logic.
 */
export function validatePaymentGateway(
  methodType: string,
  topupInfo: TopupInfo | null
): string | null {
  if (methodType === 'paypal' && !topupInfo?.enable_paypal_topup) {
    return 'PayPal top-up is not enabled by the administrator'
  }
  if (methodType === 'stripe' && !topupInfo?.enable_stripe_topup) {
    return 'Stripe top-up is not enabled by the administrator'
  }
  if (methodType === 'alipay' && !topupInfo?.enable_alipay_topup) {
    return 'Alipay top-up is not enabled by the administrator'
  }
  if (methodType === 'wxpay' && !topupInfo?.enable_wxpay_topup) {
    return 'WeChat Pay top-up is not enabled by the administrator'
  }
  if (methodType === 'waffo_pancake' && !topupInfo?.enable_waffo_pancake_topup) {
    return 'Waffo Pancake top-up is not enabled by the administrator'
  }
  if (methodType.startsWith('waffo:') && !topupInfo?.enable_waffo_topup) {
    return 'Waffo top-up is not enabled by the administrator'
  }
  if (
    !topupInfo?.enable_online_topup &&
    methodType !== 'stripe' &&
    methodType !== 'paypal' &&
    methodType !== 'alipay' &&
    methodType !== 'wxpay' &&
    !methodType.startsWith('waffo')
  ) {
    return 'Online top-up is not enabled by the administrator'
  }
  return null
}
