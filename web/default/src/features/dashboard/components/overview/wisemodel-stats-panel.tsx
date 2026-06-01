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
import { StaggerContainer, StaggerItem } from '@/components/page-transition';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { useIsAdmin } from '@/hooks/use-admin';
import { computeTimeRange } from '@/lib/time';
import { formatNumber } from '@/lib/format';
import { formatQuota } from '@/lib/format';
import { Coins, Send } from 'lucide-react';
import { useMemo } from 'react';
import { api } from '@/lib/api';

import { StatCard } from '../ui/stat-card';


// ---------------------------------------------------------------------------
// API
// ---------------------------------------------------------------------------

interface WiseModelStatResponse {
  success: boolean
  data?: {
    quota?: number
    count?: number
  }
  message?: string
}

async function getWiseModelStats(params: {
  start_timestamp: number
  end_timestamp: number
}): Promise<WiseModelStatResponse> {
  const res = await api.get<WiseModelStatResponse>('/api/log/stat', {
    params: {
      is_wisemodel: true,
      type: 2,
      start_timestamp: params.start_timestamp,
      end_timestamp: params.end_timestamp,
    },
  })
  return res.data
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function WiseModelStatsPanel() {
  const { t } = useTranslation()
  const isAdmin = useIsAdmin()

  const timeRange = useMemo(() => computeTimeRange(30), [])

  const { data, isLoading } = useQuery({
    queryKey: [
      'dashboard',
      'overview',
      'wisemodel-stats',
      timeRange.start_timestamp,
      timeRange.end_timestamp,
    ],
    queryFn: () =>
      getWiseModelStats({
        start_timestamp: timeRange.start_timestamp,
        end_timestamp: timeRange.end_timestamp,
      }),
    enabled: isAdmin,
    staleTime: 60 * 1000,
  })

  if (!isAdmin) return null

  const quota = data?.data?.quota ?? 0
  const count = data?.data?.count ?? 0

  const cards = [
    {
      key: 'wisemodel-quota',
      title: t('quota_consumed', { ns: 'wisemodel' }),
      value: formatQuota(quota),
      description: t('WiseModel quota consumed in the selected period'),
      icon: Coins,
      tone: 'teal' as const,
    },
    {
      key: 'wisemodel-requests',
      title: t('request_count', { ns: 'wisemodel' }),
      value: formatNumber(count),
      description: t('WiseModel request count in the selected period'),
      icon: Send,
      tone: 'teal' as const,
    },
  ]

  return (
    <div className='bg-card overflow-hidden rounded-2xl border shadow-xs'>
      <div className='flex flex-col gap-3 p-4 sm:p-5'>
        <div className='flex flex-col gap-1'>
          <h3 className='text-base font-semibold'>
            {t('External Platform Integration')}
          </h3>
          <p className='text-muted-foreground text-sm'>
            {t('WiseModel usage statistics')}
          </p>
        </div>
        <StaggerContainer className='grid gap-3 md:grid-cols-2'>
          {cards.map((card) => (
            <StaggerItem
              key={card.key}
              className='bg-background/60 rounded-xl border p-3'
            >
              <StatCard
                title={card.title}
                value={card.value}
                description={card.description}
                icon={card.icon}
                tone={card.tone}
                loading={isLoading}
              />
            </StaggerItem>
          ))}
        </StaggerContainer>
      </div>
    </div>
  )
}
