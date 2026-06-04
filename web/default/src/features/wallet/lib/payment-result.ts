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
import i18next from 'i18next'
import { toast } from 'sonner'
import {
  requestPayPalPayment,
  requestAlipayPayment,
  requestWxpayPayment,
  requestWaffoPayment,
  isApiSuccess,
} from '../api'
import type { WxpayResult } from '../types'

// ============================================================================
// Payment Result Types
// ============================================================================

/** Result from Alipay payment — needs polling to auto-refresh balance */
export interface AlipayResult {
  pay_link: string
  trade_no: string
}

/** Discriminated return type for processPayment */
export type PaymentResult =
  | { type: 'redirect'; success: boolean }
  | { type: 'form'; success: boolean }
  | { type: 'alipay'; success: boolean; data: AlipayResult }
  | { type: 'wxpay'; success: boolean; data: WxpayResult }

// ============================================================================
// Error Extraction
// ============================================================================

/**
 * Extract a human-readable error message from a payment API response,
 * matching Classic's `typeof data === 'string' ? data : message || t('支付失败')`.
 *
 * The backend returns two incompatible error schemas:
 *   1. { message: "error", data: "<plain string>" }  — Alipay/Wxpay/some PayPal
 *   2. { success: false, message: "<actual error>" } — PayPal via ApiErrorMsg
 *
 * This function handles both: if `data` is a string it takes priority (case 1),
 * otherwise falls back to `message` (case 2), and finally to a generic i18n key.
 */
export function extractPaymentError(response: Record<string, unknown>): string {
  const data = response.data
  const message = typeof response.message === 'string' ? response.message : ''
  if (typeof data === 'string' && data) return data
  if (message && message !== 'error') return message
  return i18next.t('Payment request failed')
}

// ============================================================================
// Payment Branch Processors
// ============================================================================

/** Process PayPal payment — redirect to PayPal via pay_link */
export async function processPayPalBranch(
  topupAmountInt: number
): Promise<PaymentResult> {
  const response = await requestPayPalPayment({
    amount: topupAmountInt,
  })
  if (!isApiSuccess(response)) {
    toast.error(extractPaymentError(response as Record<string, unknown>))
    return { type: 'redirect', success: false }
  }
  if (response.data?.pay_link) {
    window.open(response.data.pay_link as string, '_blank')
    toast.success(i18next.t('Redirecting to payment page...'))
    return { type: 'redirect', success: true }
  }
  toast.error(i18next.t('Payment failed'))
  return { type: 'redirect', success: false }
}

/** Process Alipay (direct) — open pay_link and return trade_no for polling */
export async function processAlipayBranch(
  topupAmountInt: number
): Promise<PaymentResult> {
  const response = await requestAlipayPayment({
    amount: topupAmountInt,
  })
  if (!isApiSuccess(response)) {
    toast.error(extractPaymentError(response as Record<string, unknown>))
    return {
      type: 'alipay',
      success: false,
      data: { pay_link: '', trade_no: '' },
    }
  }
  if (response.data?.pay_link) {
    window.open(response.data.pay_link as string, '_blank')
    toast.success(i18next.t('Redirecting to payment page...'))
    return {
      type: 'alipay',
      success: true,
      data: {
        pay_link: response.data.pay_link,
        trade_no: response.data.trade_no || '',
      },
    }
  }
  toast.error(i18next.t('Payment failed'))
  return {
    type: 'alipay',
    success: false,
    data: { pay_link: '', trade_no: '' },
  }
}

/** Process Wxpay (direct) — returns QR code data for scanning */
export async function processWxpayBranch(
  topupAmountInt: number
): Promise<PaymentResult> {
  const response = await requestWxpayPayment({
    amount: topupAmountInt,
  })
  if (!isApiSuccess(response)) {
    toast.error(extractPaymentError(response as Record<string, unknown>))
    return {
      type: 'wxpay',
      success: false,
      data: { code_url: '', trade_no: '' },
    }
  }
  if (response.data?.code_url) {
    toast.success(i18next.t('Please scan QR code to pay'))
    return {
      type: 'wxpay',
      success: true,
      data: {
        code_url: response.data.code_url,
        trade_no: response.data.trade_no || '',
      },
    }
  }
  toast.error(i18next.t('Payment failed'))
  return { type: 'wxpay', success: false, data: { code_url: '', trade_no: '' } }
}

/** Process Creem — typically handled via separate UI path, fallback only */
export async function processCreemBranch(): Promise<PaymentResult> {
  toast.error(
    i18next.t('Please select a Creem product below to proceed with payment.')
  )
  return { type: 'redirect', success: false }
}

/** Process Waffo (legacy) — typically handled via dedicated Waffo buttons, fallback only */
export async function processWaffoBranch(
  topupAmountInt: number
): Promise<PaymentResult> {
  const response = await requestWaffoPayment({
    amount: topupAmountInt,
  })
  if (!isApiSuccess(response)) {
    toast.error(extractPaymentError(response as Record<string, unknown>))
    return { type: 'redirect', success: false }
  }
  const paymentUrl =
    response.data &&
    typeof response.data === 'object' &&
    'payment_url' in response.data
      ? (response.data as { payment_url: string }).payment_url
      : null
  if (paymentUrl) {
    window.open(paymentUrl, '_blank')
    toast.success(i18next.t('Redirecting to payment page...'))
    return { type: 'redirect', success: true }
  }
  toast.error(i18next.t('Payment failed'))
  return { type: 'redirect', success: false }
}
