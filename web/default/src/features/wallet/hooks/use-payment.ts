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
import { useState, useCallback } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'
import {
  calculateAmount,
  calculateStripeAmount,
  calculateWaffoPancakeAmount,
  requestPayment,
  requestStripePayment,
  requestPayPalPayment,
  requestAlipayPayment,
  requestWxpayPayment,
  requestCreemPayment,
  requestWaffoPayment,
  isApiSuccess,
} from '../api'
import {
  isStripePayment,
  isPayPalPayment,
  isAlipayPayment,
  isWxpayPayment,
  isWaffoPancakePayment,
  submitPaymentForm,
} from '../lib'
import type { WxpayResult } from '../types'

// ============================================================================
// Payment Hook
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

export function usePayment() {
  const [amount, setAmount] = useState<number>(0)
  const [calculating, setCalculating] = useState(false)
  const [processing, setProcessing] = useState(false)

  // Calculate payment amount
  const calculatePaymentAmount = useCallback(
    async (topupAmount: number, paymentType: string) => {
      try {
        setCalculating(true)

        const isStripe = isStripePayment(paymentType)
        const isPancake = isWaffoPancakePayment(paymentType)
        const response = isStripe
          ? await calculateStripeAmount({ amount: topupAmount })
          : isPancake
            ? await calculateWaffoPancakeAmount({ amount: topupAmount })
            : await calculateAmount({ amount: topupAmount })

        if (isApiSuccess(response) && response.data) {
          const calculatedAmount = parseFloat(response.data)
          setAmount(calculatedAmount)
          return calculatedAmount
        }

        // Don't show error for calculation, just set to 0
        setAmount(0)
        return 0
      } catch (_error) {
        setAmount(0)
        return 0
      } finally {
        setCalculating(false)
      }
    },
    []
  )

  // Process payment — dispatches to the correct API endpoint per payment type,
  // matching the routing in classic's onlineTopUp function.
  const processPayment = useCallback(
    async (
      topupAmount: number,
      paymentType: string
    ): Promise<PaymentResult> => {
      try {
        setProcessing(true)

        const topupAmountInt = Math.floor(topupAmount)

        // ── Stripe ──────────────────────────────────────────────
        if (isStripePayment(paymentType)) {
          const response = await requestStripePayment({
            amount: topupAmountInt,
            payment_method: 'stripe',
          })
          if (!isApiSuccess(response)) {
            toast.error(
              (response as unknown as { message?: string }).message ||
                i18next.t('Payment request failed')
            )
            return { type: 'redirect', success: false }
          }
          if (response.data?.pay_link) {
            window.open(response.data.pay_link as string, '_blank')
            toast.success(i18next.t('Redirecting to payment page...'))
            return { type: 'redirect', success: true }
          }
          return { type: 'redirect', success: false }
        }

        // ── PayPal ─────────────────────────────────────────────
        if (isPayPalPayment(paymentType)) {
          const response = await requestPayPalPayment({
            amount: topupAmountInt,
          })
          if (!isApiSuccess(response)) {
            toast.error(
              (response as unknown as { message?: string }).message ||
                i18next.t('Payment request failed')
            )
            return { type: 'redirect', success: false }
          }
          if (response.data?.pay_link) {
            window.open(response.data.pay_link as string, '_blank')
            toast.success(i18next.t('Redirecting to payment page...'))
            return { type: 'redirect', success: true }
          }
          return { type: 'redirect', success: false }
        }

        // ── Alipay (direct) ─────────────────────────────────────
        if (isAlipayPayment(paymentType)) {
          const response = await requestAlipayPayment({
            amount: topupAmountInt,
          })
          if (!isApiSuccess(response)) {
            toast.error(
              (response as unknown as { message?: string }).message ||
                i18next.t('Payment request failed')
            )
            return { type: 'alipay', success: false, data: { pay_link: '', trade_no: '' } }
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
          return { type: 'alipay', success: false, data: { pay_link: '', trade_no: '' } }
        }

        // ── Wxpay (direct) — returns QR code data ───────────────
        if (isWxpayPayment(paymentType)) {
          const response = await requestWxpayPayment({
            amount: topupAmountInt,
          })
          if (!isApiSuccess(response)) {
            toast.error(
              (response as unknown as { message?: string }).message ||
                i18next.t('Payment request failed')
            )
            return { type: 'wxpay', success: false, data: { code_url: '', trade_no: '' } }
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
          return { type: 'wxpay', success: false, data: { code_url: '', trade_no: '' } }
        }

        // ── Creem ───────────────────────────────────────────────
        // Note: Creem is typically handled via a separate UI path
        // (product cards), but we support it here as a fallback.
        if (paymentType === 'creem') {
          toast.error(
            i18next.t(
              'Please select a Creem product below to proceed with payment.'
            )
          )
          return { type: 'redirect', success: false }
        }

        // ── Waffo (legacy) ─────────────────────────────────────
        // Note: Waffo is typically handled via dedicated Waffo payment
        // method buttons, but we support it here as a fallback.
        if (paymentType === 'waffo') {
          const response = await requestWaffoPayment({
            amount: topupAmountInt,
          })
          if (!isApiSuccess(response)) {
            toast.error(
              (response as unknown as { message?: string }).message ||
                i18next.t('Payment request failed')
            )
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
          return { type: 'redirect', success: false }
        }

        // ── Epay (generic) — form submission ────────────────────
        const response = await requestPayment({
          amount: topupAmountInt,
          payment_method: paymentType,
        })

        if (!isApiSuccess(response)) {
          toast.error(
            (response as unknown as { message?: string }).message ||
              i18next.t('Payment request failed')
          )
          return { type: 'form', success: false }
        }

        // Handle Epay form submission
        const url = (response as unknown as { url?: string }).url
        if (url && response.data) {
          submitPaymentForm(url, response.data)
          toast.success(i18next.t('Redirecting to payment page...'))
          return { type: 'form', success: true }
        }

        return { type: 'form', success: false }
      } catch (_error) {
        toast.error(i18next.t('Payment request failed'))
        return { type: 'form', success: false }
      } finally {
        setProcessing(false)
      }
    },
    []
  )

  return {
    amount,
    calculating,
    processing,
    calculatePaymentAmount,
    processPayment,
    setAmount,
  }
}
