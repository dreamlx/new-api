/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { createPlatform, updatePlatform } from '../api'
import { buildCreatePlatformPayload } from '../lib/platform-form'
import { PLATFORM_STATUS, type Platform, type PlatformStatus } from '../types'
import { usePlatforms } from './platforms-provider'

// Same regex as the backend admin handler. Keep them in sync.
const PLATFORM_ID_PATTERN = /^[a-z0-9_-]{1,80}$/

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: Platform | null
}

export function PlatformsMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: Props) {
  const { t } = useTranslation()
  const { setSkReveal, setOpen: setDialog, triggerRefresh } = usePlatforms()
  const isUpdate = Boolean(currentRow)

  const [platformId, setPlatformId] = useState('')
  const [name, setName] = useState('')
  const [shadowUserId, setShadowUserId] = useState('')
  const [status, setStatus] = useState<PlatformStatus>(PLATFORM_STATUS.ENABLED)
  const [submitting, setSubmitting] = useState(false)

  // Reset form whenever the drawer re-opens or switches mode.
  useEffect(() => {
    if (!open) return
    if (currentRow) {
      setPlatformId(currentRow.platform_id)
      setName(currentRow.name ?? '')
      setShadowUserId(String(currentRow.shadow_user_id))
      setStatus(currentRow.status as PlatformStatus)
    } else {
      setPlatformId('')
      setName('')
      setShadowUserId('')
      setStatus(PLATFORM_STATUS.ENABLED)
    }
  }, [open, currentRow])

  const parseShadowUserId = () => {
    const raw = shadowUserId.trim()
    if (!raw) return undefined
    const parsed = Number(raw)
    if (!Number.isInteger(parsed) || parsed <= 0) {
      toast.error(t('shadow_user_id must be a positive integer'))
      return null
    }
    return parsed
  }

  const validate = () => {
    if (!isUpdate && !PLATFORM_ID_PATTERN.test(platformId)) {
      toast.error(t('platform_id must match ^[a-z0-9_-]{1,80}$'))
      return false
    }
    return true
  }

  const onSubmit = async () => {
    if (!validate()) return
    const parsedShadowUserId = parseShadowUserId()
    if (parsedShadowUserId === null) return
    setSubmitting(true)
    try {
      if (isUpdate && currentRow) {
        const res = await updatePlatform({
          id: currentRow.id,
          name,
          status,
          ...(parsedShadowUserId ? { shadow_user_id: parsedShadowUserId } : {}),
        })
        if (!res.success) {
          toast.error(res.message || t('Failed to update platform'))
          return
        }
        toast.success(t('Platform updated'))
        onOpenChange(false)
        triggerRefresh()
        return
      }

      const res = await createPlatform(
        buildCreatePlatformPayload({
          platformId,
          name,
          status,
          shadowUserId: parsedShadowUserId,
        })
      )
      if (!res.success || !res.data) {
        toast.error(res.message || t('Failed to create platform'))
        return
      }
      toast.success(t('Platform created'))
      onOpenChange(false)
      // Hand the plaintext sk to the reveal dialog — the only place it can
      // ever be displayed. The provider also stores it so the dialog can pick it up.
      setSkReveal(res.data)
      setDialog('sk-reveal')
      triggerRefresh()
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>
            {isUpdate ? t('Edit Platform') : t('Create Platform')}
          </SheetTitle>
          <SheetDescription>
            {isUpdate
              ? t(
                  'Update the platform name, status, or shadow user. platform_id and sk cannot be changed here.'
                )
              : t(
                  'Provision a new downstream platform credential. The plaintext sk is returned exactly once.'
                )}
          </SheetDescription>
        </SheetHeader>

        <div className='flex flex-col gap-4 px-4 py-4'>
          <div className='flex flex-col gap-2'>
            <Label htmlFor='platform_id'>{t('Platform ID')}</Label>
            <Input
              id='platform_id'
              value={platformId}
              disabled={isUpdate}
              onChange={(e) => setPlatformId(e.target.value)}
              placeholder='e.g. acme_corp'
            />
            <p className='text-muted-foreground text-xs'>
              {t('Lowercase letters, digits, underscore, hyphen; 1–80 chars.')}
            </p>
          </div>

          <div className='flex flex-col gap-2'>
            <Label htmlFor='name'>{t('Platform Name')}</Label>
            <Input
              id='name'
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('Display name (optional)')}
            />
          </div>

          <div className='flex flex-col gap-2'>
            <Label htmlFor='shadow_user_id'>{t('Shadow User ID')}</Label>
            <Input
              id='shadow_user_id'
              inputMode='numeric'
              value={shadowUserId}
              onChange={(e) => setShadowUserId(e.target.value)}
              placeholder={t('Leave empty to auto-create a shadow user')}
            />
            <p className='text-muted-foreground text-xs'>
              {t('When provided, this must be an existing user ID.')}
            </p>
          </div>

          <div className='flex flex-col gap-2'>
            <Label>{t('Status')}</Label>
            <Select
              value={String(status)}
              onValueChange={(v) => setStatus(Number(v) as PlatformStatus)}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={String(PLATFORM_STATUS.ENABLED)}>
                  {t('Enabled')}
                </SelectItem>
                <SelectItem value={String(PLATFORM_STATUS.DISABLED)}>
                  {t('Disabled')}
                </SelectItem>
              </SelectContent>
            </Select>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Disabling will reject all token operations from this platform.'
              )}
            </p>
          </div>
        </div>

        <SheetFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={onSubmit} disabled={submitting}>
            {submitting
              ? t('Saving...')
              : isUpdate
                ? t('Save Changes')
                : t('Create')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
