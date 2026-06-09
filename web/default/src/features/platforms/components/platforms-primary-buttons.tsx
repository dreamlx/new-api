/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { usePlatforms } from './platforms-provider'

export function PlatformsPrimaryButtons() {
  const { t } = useTranslation()
  const { setOpen, setCurrentRow } = usePlatforms()

  return (
    <Button
      onClick={() => {
        setCurrentRow(null)
        setOpen('create')
      }}
    >
      <Plus className='h-4 w-4' />
      {t('Create Platform')}
    </Button>
  )
}
