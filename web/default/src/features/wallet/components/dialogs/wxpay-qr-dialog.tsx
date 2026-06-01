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
import { useEffect, useRef, useState } from 'react'
import { QRCodeSVG } from 'qrcode.react'
import { CheckCircle, XCircle, AlertTriangle, Clock, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { api } from '@/lib/api'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

// Polling interval (ms) for /api/user/topup/status
const POLL_INTERVAL_MS = 3000
// Auto-close delay after success (ms)
const AUTO_CLOSE_AFTER_SUCCESS_MS = 2000

type WxpayStatus = 'pending' | 'success' | 'failed' | 'expired' | 'anomaly'

interface WxpayQRDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  codeUrl: string
  tradeNo: string
  onSuccess?: () => void
}

export function WxpayQRDialog({
  open,
  onOpenChange,
  codeUrl,
  tradeNo,
  onSuccess,
}: WxpayQRDialogProps) {
  const { t } = useTranslation()
  const [status, setStatus] = useState<WxpayStatus>('pending')
  const cancelledRef = useRef(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const autoCloseTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Reset state when dialog opens for a new order
  useEffect(() => {
    if (open) {
      cancelledRef.current = false
      setStatus('pending')
    }
  }, [open])

  // Polling logic
  useEffect(() => {
    if (!open || !tradeNo) return

    cancelledRef.current = false
    setStatus('pending')

    const poll = async () => {
      if (cancelledRef.current) return
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
            if (cancelledRef.current) return
            setStatus(next as WxpayStatus)
            if (next === 'success') {
              onSuccess?.()
              autoCloseTimerRef.current = setTimeout(() => {
                if (!cancelledRef.current) onOpenChange(false)
              }, AUTO_CLOSE_AFTER_SUCCESS_MS)
            }
            return
          }
        }
      } catch {
        // Network errors are transient — keep polling
      }
      if (!cancelledRef.current) {
        timerRef.current = setTimeout(poll, POLL_INTERVAL_MS)
      }
    }

    timerRef.current = setTimeout(poll, POLL_INTERVAL_MS)

    return () => {
      cancelledRef.current = true
      if (timerRef.current) {
        clearTimeout(timerRef.current)
        timerRef.current = null
      }
      if (autoCloseTimerRef.current) {
        clearTimeout(autoCloseTimerRef.current)
        autoCloseTimerRef.current = null
      }
    }
    // We intentionally don't include onOpenChange/onSuccess in deps
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, tradeNo])

  const renderBody = () => {
    if (status === 'success') {
      return (
        <div className='flex flex-col items-center py-6'>
          <CheckCircle className='h-16 w-16 text-green-600' />
          <p className='mt-3 text-lg font-medium'>{t('Payment successful')}</p>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('Balance updated, closing...')}
          </p>
        </div>
      )
    }

    if (status === 'failed' || status === 'expired' || status === 'anomaly') {
      const isExpired = status === 'expired'
      const isAnomaly = status === 'anomaly'
      const Icon = isExpired ? Clock : isAnomaly ? AlertTriangle : XCircle
      const colorClass = isExpired
        ? 'text-orange-500'
        : isAnomaly
          ? 'text-yellow-500'
          : 'text-red-500'
      const title = isExpired
        ? t('Order expired')
        : isAnomaly
          ? t('Order anomaly')
          : t('Payment failed')
      const description = isAnomaly
        ? t('Amount anomaly detected, please contact administrator')
        : isExpired
          ? t('Please initiate a new top-up')
          : t('Please try again later or contact administrator')

      return (
        <div className='flex flex-col items-center py-6'>
          <Icon className={`h-16 w-16 ${colorClass}`} />
          <p className='mt-3 text-lg font-medium'>{title}</p>
          <p className='text-muted-foreground mt-1 text-sm'>{description}</p>
        </div>
      )
    }

    return (
      <div className='flex flex-col items-center py-2'>
        <p className='text-muted-foreground mb-3 text-sm'>
          {t('Please scan QR code with WeChat to pay')}
        </p>
        {codeUrl ? (
          <div className='rounded-lg bg-white p-3'>
            <QRCodeSVG value={codeUrl} size={200} includeMargin={false} />
          </div>
        ) : (
          <Loader2 className='h-8 w-8 animate-spin text-muted-foreground' />
        )}
        <div className='mt-3 flex items-center gap-2'>
          <Loader2 className='h-4 w-4 animate-spin text-muted-foreground' />
          <span className='text-muted-foreground text-sm'>
            {t('Waiting for payment...')}
          </span>
        </div>
        {tradeNo && (
          <p className='text-muted-foreground mt-2 text-xs'>
            {t('Order No.')}: {tradeNo}
          </p>
        )}
      </div>
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-w-sm'>
        <DialogHeader>
          <DialogTitle>{t('WeChat Pay')}</DialogTitle>
          <DialogDescription>
            {t('Scan the QR code with WeChat to complete payment')}
          </DialogDescription>
        </DialogHeader>
        {renderBody()}
      </DialogContent>
    </Dialog>
  )
}
