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
  isApiSuccess,
} from '../api'
import { PAYMENT_TYPES } from '../constants'
import {
  isStripePayment,
  isPayPalPayment,
  isWaffoPancakePayment,
  submitPaymentForm,
  buildAmountRequest,
} from '../lib'
import {
  type PaymentResult,
  processPayPalBranch,
  processAlipayBranch,
  processWxpayBranch,
  processCreemBranch,
  extractPaymentError,
} from '../lib/payment-result'

// Re-export types for backward compatibility — consumers import from this hook
export type { AlipayResult } from '../lib/payment-result'
export type { PaymentResult } from '../lib/payment-result'

// ============================================================================
// Payment Hook
// ============================================================================

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
            : await calculateAmount(
                buildAmountRequest(topupAmount, paymentType)
              )

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
              extractPaymentError(response as Record<string, unknown>)
            )
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

        // ── PayPal ─────────────────────────────────────────────
        if (isPayPalPayment(paymentType))
          return processPayPalBranch(topupAmountInt)

        // ── Alipay (direct) — only for the direct-alipay standalone button ──
        if (paymentType === PAYMENT_TYPES.DIRECT_ALIPAY)
          return processAlipayBranch(topupAmountInt)

        // ── Wxpay (direct) — only for the direct-wxpay standalone button ──
        if (paymentType === PAYMENT_TYPES.DIRECT_WECHAT)
          return processWxpayBranch(topupAmountInt)

        // ── Creem ───────────────────────────────────────────────
        if (paymentType === PAYMENT_TYPES.CREEM) return processCreemBranch()

        // ── Epay (generic) — form submission ────────────────────
        const response = await requestPayment({
          amount: topupAmountInt,
          payment_method: paymentType,
        })

        if (!isApiSuccess(response)) {
          toast.error(extractPaymentError(response as Record<string, unknown>))
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
