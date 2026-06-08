/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useState } from 'react'
import { Copy, Check, AlertTriangle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { usePlatforms } from './platforms-provider'

/**
 * PlatformsSkRevealDialog is the ONLY surface in the UI where a plaintext
 * platform_sk is ever displayed. It opens immediately after a successful
 * Create, holds the value in-memory only (provider state), and clears it
 * the moment the dialog closes — there's no way to retrieve it afterwards.
 */
export function PlatformsSkRevealDialog() {
  const { t } = useTranslation()
  const { open, setOpen, skReveal, setSkReveal } = usePlatforms()
  const [copied, setCopied] = useState(false)

  const isOpen = open === 'sk-reveal' && Boolean(skReveal)

  const handleClose = () => {
    setOpen(null)
    setSkReveal(null) // Clear plaintext sk from React state.
    setCopied(false)
  }

  const handleCopy = async () => {
    if (!skReveal?.platform_sk) return
    try {
      // Copy both headers in the same format as the API expects them
      // (matching Classic's "复制凭证" behavior)
      const text = `X-Platform-Id: ${skReveal.platform_id}\nX-Platform-Sk: ${skReveal.platform_sk}`
      await navigator.clipboard.writeText(text)
      setCopied(true)
      toast.success(t('Copied'))
    } catch {
      toast.error(t('Copy failed — please select and copy manually'))
    }
  }

  return (
    <AlertDialog open={isOpen} onOpenChange={(o) => !o && handleClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle className='flex items-center gap-2'>
            <AlertTriangle className='h-5 w-5 text-amber-500' />
            {t('Save this secret now — it will not be shown again')}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t(
              "This is the only time the plaintext platform_sk is returned. Store it securely in your platform's secret manager. If lost, this platform must be deleted and recreated."
            )}
          </AlertDialogDescription>
        </AlertDialogHeader>

        <div className='flex flex-col gap-3 pt-2'>
          <div className='text-muted-foreground text-xs'>
            {t('Platform ID')}:{' '}
            <span className='text-foreground font-mono font-medium'>
              {skReveal?.platform_id}
            </span>
          </div>
          <div className='bg-muted/40 rounded-md border p-3 font-mono text-sm break-all select-all'>
            {skReveal?.platform_sk}
          </div>
          <Button
            type='button'
            variant='outline'
            onClick={handleCopy}
            className='self-start'
          >
            {copied ? (
              <>
                <Check className='h-4 w-4' /> {t('Copied')}
              </>
            ) : (
              <>
                <Copy className='h-4 w-4' /> {t('Copy to clipboard')}
              </>
            )}
          </Button>
        </div>

        <AlertDialogFooter>
          <AlertDialogAction onClick={handleClose}>
            {t('I have copied the secret')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
