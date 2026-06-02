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
import { useState, useCallback, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/lib/api'
import { toast } from 'sonner'

// Maximum number of Alipay polling attempts before giving up
const ALIPAY_MAX_POLLS = 100 // ~5 minutes at 3-second intervals

/**
 * Hook for polling Alipay payment status after opening pay_link.
 * After the user completes payment on the Alipay page, the backend receives
 * an async Alipay notify callback. This hook polls /api/user/topup/status
 * until a terminal state is reached, then auto-refreshes the balance.
 */
export function useAlipayPolling(
  fetchUserRef: React.MutableRefObject<(() => Promise<void>) | undefined>
) {
  const { t } = useTranslation()
  const [alipayPolling, setAlipayPolling] = useState(false)
  const alipayPollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Poll /api/user/topup/status for an Alipay trade_no until terminal state
  const pollAlipayStatus = useCallback(
    (tradeNo: string) => {
      if (!tradeNo) return
      setAlipayPolling(true)
      let cancelled = false
      let attempt = 0

      const tick = async () => {
        if (cancelled) return
        attempt++
        try {
          const res = await api.get('/api/user/topup/status', {
            params: { trade_no: tradeNo },
          })
          const { message, data } = res.data || {}
          if (message === 'success' && data) {
            const next = data.status
            if (
              next === 'success' ||
              next === 'failed' ||
              next === 'expired' ||
              next === 'anomaly'
            ) {
              if (!cancelled) {
                setAlipayPolling(false)
                if (next === 'success') {
                  toast.success(t('Payment successful'))
                  await fetchUserRef.current?.()
                } else {
                  toast.error(
                    next === 'expired'
                      ? t('Order expired')
                      : next === 'anomaly'
                        ? t('Order anomaly')
                        : t('Payment failed')
                  )
                }
              }
              return
            }
          }
        } catch {
          // Network errors are transient — keep polling
        }
        if (cancelled) return
        if (attempt >= ALIPAY_MAX_POLLS) {
          setAlipayPolling(false)
          toast.warning(t('Payment status check timed out. Please refresh the page to verify.'))
          return
        }
        alipayPollTimerRef.current = setTimeout(tick, 3000)
      }

      alipayPollTimerRef.current = setTimeout(tick, 3000)

      // Return cleanup function
      return () => {
        cancelled = true
        if (alipayPollTimerRef.current) {
          clearTimeout(alipayPollTimerRef.current)
          alipayPollTimerRef.current = null
        }
        setAlipayPolling(false)
      }
    },
    [t, fetchUserRef]
  )

  // Cleanup Alipay polling on unmount
  useEffect(() => {
    return () => {
      if (alipayPollTimerRef.current) {
        clearTimeout(alipayPollTimerRef.current)
      }
    }
  }, [])

  return { alipayPolling, pollAlipayStatus }
}
