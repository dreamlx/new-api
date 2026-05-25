/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import React, { useState } from 'react'
import useDialogState from '@/hooks/use-dialog'
import type { Platform, PlatformCreated, PlatformsDialogType } from '../types'

type PlatformsContextType = {
  open: PlatformsDialogType | null
  setOpen: (str: PlatformsDialogType | null) => void
  currentRow: Platform | null
  setCurrentRow: React.Dispatch<React.SetStateAction<Platform | null>>
  /**
   * skReveal holds the plaintext platform_sk returned by the Create endpoint.
   * It is intentionally NOT persisted anywhere; cleared on dialog dismiss.
   */
  skReveal: PlatformCreated | null
  setSkReveal: React.Dispatch<React.SetStateAction<PlatformCreated | null>>
  refreshTrigger: number
  triggerRefresh: () => void
}

const PlatformsContext = React.createContext<PlatformsContextType | null>(null)

export function PlatformsProvider({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = useDialogState<PlatformsDialogType>(null)
  const [currentRow, setCurrentRow] = useState<Platform | null>(null)
  const [skReveal, setSkReveal] = useState<PlatformCreated | null>(null)
  const [refreshTrigger, setRefreshTrigger] = useState(0)

  const triggerRefresh = () => setRefreshTrigger((prev) => prev + 1)

  return (
    <PlatformsContext
      value={{
        open,
        setOpen,
        currentRow,
        setCurrentRow,
        skReveal,
        setSkReveal,
        refreshTrigger,
        triggerRefresh,
      }}
    >
      {children}
    </PlatformsContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const usePlatforms = () => {
  const ctx = React.useContext(PlatformsContext)
  if (!ctx) {
    throw new Error('usePlatforms must be used within <PlatformsProvider>')
  }
  return ctx
}
