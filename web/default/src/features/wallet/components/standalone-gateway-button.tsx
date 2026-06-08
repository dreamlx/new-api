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
import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { getPaymentIcon } from '../lib'
import type { PaymentMethod } from '../types'

interface StandaloneGatewayButtonProps {
  paymentType: string
  label: string
  minTopup: number
  topupAmount: number
  paymentLoading: string | null
  onSelect: (method: PaymentMethod) => void
}

/**
 * Reusable button component for standalone payment gateways (Alipay, WeChat Pay, Waffo Pancake).
 * Handles min-topup validation, loading state, and tooltip display when disabled.
 */
export function StandaloneGatewayButton({
  paymentType,
  label,
  minTopup,
  topupAmount,
  paymentLoading,
  onSelect,
}: StandaloneGatewayButtonProps) {
  const { t } = useTranslation()
  const disabled = minTopup > topupAmount

  const method: PaymentMethod = {
    name: label,
    type: paymentType,
    min_topup: minTopup,
  }

  const button = (
    <Button
      key={`direct-${paymentType}`}
      variant='outline'
      onClick={() => onSelect(method)}
      disabled={disabled || !!paymentLoading}
      className='h-9 min-w-0 justify-start gap-2 rounded-lg px-3'
    >
      {paymentLoading === paymentType ? (
        <Loader2 className='h-4 w-4 animate-spin' />
      ) : (
        getPaymentIcon(paymentType, 'h-4 w-4', undefined, label)
      )}
      <span className='truncate'>{label}</span>
    </Button>
  )

  return disabled ? (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger render={button}></TooltipTrigger>
        <TooltipContent>
          {t('Minimum topup amount: {{amount}}', {
            amount: minTopup,
          })}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  ) : (
    button
  )
}
