/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useTranslation } from 'react-i18next'
import { SectionPageLayout } from '@/components/layout'

import { PlatformsDeleteDialog } from './components/platforms-delete-dialog'
import { PlatformsMutateDrawer } from './components/platforms-mutate-drawer'
import { PlatformsPrimaryButtons } from './components/platforms-primary-buttons'
import { PlatformsProvider, usePlatforms } from './components/platforms-provider'
import { PlatformsSkRevealDialog } from './components/platforms-sk-reveal-dialog'
import { PlatformsTable } from './components/platforms-table'

function PlatformsContent() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow } = usePlatforms()

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Platforms')}</SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t('Manage downstream platforms and their API credentials')}
        </SectionPageLayout.Description>
        <SectionPageLayout.Actions>
          <PlatformsPrimaryButtons />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <PlatformsTable />
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <PlatformsMutateDrawer
        open={open === 'create' || open === 'update'}
        onOpenChange={(isOpen) => !isOpen && setOpen(null)}
        currentRow={open === 'update' ? currentRow : null}
      />
      <PlatformsDeleteDialog />
      <PlatformsSkRevealDialog />
    </>
  )
}

export function Platforms() {
  return (
    <PlatformsProvider>
      <PlatformsContent />
    </PlatformsProvider>
  )
}
