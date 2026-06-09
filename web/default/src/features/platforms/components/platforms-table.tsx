/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { Pencil, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { listPlatforms } from '../api'
import { PLATFORM_STATUS, type Platform } from '../types'
import { usePlatforms } from './platforms-provider'

export function PlatformsTable() {
  const { t } = useTranslation()
  const { setOpen, setCurrentRow, refreshTrigger } = usePlatforms()

  const { data, isLoading } = useQuery({
    queryKey: ['platforms', refreshTrigger],
    queryFn: async () => {
      const res = await listPlatforms(1, 100)
      if (!res.success) {
        toast.error(res.message || t('Failed to load platforms'))
        return { items: [] as Platform[], total: 0 }
      }
      return res.data ?? { items: [] as Platform[], total: 0 }
    },
  })

  const items = data?.items ?? []

  return (
    <div className='rounded-md border'>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Platform ID')}</TableHead>
            <TableHead>{t('Platform Name')}</TableHead>
            <TableHead>{t('Status')}</TableHead>
            <TableHead>{t('Shadow User ID')}</TableHead>
            <TableHead>{t('Created At')}</TableHead>
            <TableHead className='text-right'>{t('Actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {isLoading && (
            <TableRow>
              <TableCell
                colSpan={6}
                className='text-muted-foreground text-center'
              >
                {t('Loading...')}
              </TableCell>
            </TableRow>
          )}
          {!isLoading && items.length === 0 && (
            <TableRow>
              <TableCell
                colSpan={6}
                className='text-muted-foreground text-center'
              >
                {t('No platforms yet. Click "Create Platform" to add one.')}
              </TableCell>
            </TableRow>
          )}
          {items.map((p) => (
            <TableRow key={p.id}>
              <TableCell className='font-mono'>{p.platform_id}</TableCell>
              <TableCell>{p.name || '—'}</TableCell>
              <TableCell>
                {p.status === PLATFORM_STATUS.ENABLED ? (
                  <Badge variant='default'>{t('Enabled')}</Badge>
                ) : (
                  <Badge variant='secondary'>{t('Disabled')}</Badge>
                )}
              </TableCell>
              <TableCell>{p.shadow_user_id}</TableCell>
              <TableCell className='text-muted-foreground text-xs'>
                {p.created_at}
              </TableCell>
              <TableCell className='text-right'>
                <Button
                  variant='ghost'
                  size='icon'
                  aria-label={t('Edit Platform')}
                  onClick={() => {
                    setCurrentRow(p)
                    setOpen('update')
                  }}
                >
                  <Pencil className='h-4 w-4' />
                </Button>
                <Button
                  variant='ghost'
                  size='icon'
                  aria-label={t('Delete Platform')}
                  onClick={() => {
                    setCurrentRow(p)
                    setOpen('delete')
                  }}
                >
                  <Trash2 className='text-destructive h-4 w-4' />
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
