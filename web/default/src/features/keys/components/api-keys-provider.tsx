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
import React, { useState, useCallback, useRef, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import useDialogState from '@/hooks/use-dialog'
import { isVerificationRequiredError } from '@/lib/secure-verification'
import { fetchTokenKey, fetchTokenKeysBatch } from '../api'
import { ERROR_MESSAGES } from '../constants'
import { type ApiKey, type ApiKeysDialogType } from '../types'

type ApiKeysContextType = {
  open: ApiKeysDialogType | null
  setOpen: (str: ApiKeysDialogType | null) => void
  currentRow: ApiKey | null
  setCurrentRow: React.Dispatch<React.SetStateAction<ApiKey | null>>
  refreshTrigger: number
  triggerRefresh: () => void
  resolvedKey: string
  setResolvedKey: React.Dispatch<React.SetStateAction<string>>
  resolveRealKey: (id: number) => Promise<string | null>
  resolveRealKeysBatch: (ids: number[]) => Promise<Record<number, string>>
  resolvedKeys: Record<number, string>
  loadingKeys: Record<number, boolean>
  copiedKeyId: number | null
  markKeyCopied: (id: number) => void
}

const ApiKeysContext = React.createContext<ApiKeysContextType | null>(null)

export function ApiKeysProvider({ children }: { children: React.ReactNode }) {
  const { t } = useTranslation()
  const [open, setOpen] = useDialogState<ApiKeysDialogType>(null)
  const [currentRow, setCurrentRow] = useState<ApiKey | null>(null)
  const [refreshTrigger, setRefreshTrigger] = useState(0)
  const [resolvedKey, setResolvedKey] = useState('')

  const [resolvedKeys, setResolvedKeys] = useState<Record<number, string>>({})
  const [loadingKeys, setLoadingKeys] = useState<Record<number, boolean>>({})
  const pendingRequests = useRef<Record<number, Promise<string | null>>>({})
  const pendingKeyExportRef = useRef<{
    resolve: (value: unknown) => void
    reject: (error: unknown) => void
    parseResult: (result: unknown) => unknown
  } | null>(null)

  const [copiedKeyId, setCopiedKeyId] = useState<number | null>(null)
  const copiedTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  useEffect(() => {
    return () => clearTimeout(copiedTimerRef.current)
  }, [])

  const markKeyCopied = useCallback((id: number) => {
    setCopiedKeyId(id)
    clearTimeout(copiedTimerRef.current)
    copiedTimerRef.current = setTimeout(() => setCopiedKeyId(null), 2000)
  }, [])

  const resolvePendingKeyExport = useCallback((result: unknown) => {
    const pending = pendingKeyExportRef.current
    if (!pending) return

    try {
      pending.resolve(pending.parseResult(result))
    } catch (error) {
      pending.reject(error)
    } finally {
      pendingKeyExportRef.current = null
    }
  }, [])

  const rejectPendingKeyExport = useCallback((error: unknown) => {
    const pending = pendingKeyExportRef.current
    if (!pending) return
    pending.reject(error)
    pendingKeyExportRef.current = null
  }, [])

  const {
    open: verificationOpen,
    methods: verificationMethods,
    state: verificationState,
    startVerification,
    executeVerification,
    cancel: cancelVerificationBase,
    setCode: setVerificationCode,
    switchMethod: switchVerificationMethod,
  } = useSecureVerification({
    onSuccess: resolvePendingKeyExport,
    onError: rejectPendingKeyExport,
  })

  const cancelVerification = useCallback(() => {
    rejectPendingKeyExport(new Error(t('Secure verification cancelled')))
    cancelVerificationBase()
  }, [cancelVerificationBase, rejectPendingKeyExport, t])

  const requestKeyExportVerification = useCallback(
    async ({
      apiCall,
      parseResult,
      title,
      description,
    }: {
      apiCall: () => Promise<unknown>
      parseResult: (result: unknown) => unknown
      title: string
      description: string
    }) => {
      if (pendingKeyExportRef.current) {
        pendingKeyExportRef.current.reject(
          new Error(t('Another secure verification is already running'))
        )
        pendingKeyExportRef.current = null
      }

      return new Promise<unknown>((resolve, reject) => {
        pendingKeyExportRef.current = { resolve, reject, parseResult }
        startVerification(apiCall, {
          preferredMethod: 'passkey',
          title,
          description,
        })
          .then((started) => {
            if (!started && pendingKeyExportRef.current) {
              pendingKeyExportRef.current = null
              reject(new Error(t('Secure verification was not started')))
            }
          })
          .catch((error) => {
            if (pendingKeyExportRef.current) {
              pendingKeyExportRef.current = null
            }
            reject(error)
          })
      })
    },
    [startVerification, t]
  )

  const triggerRefresh = useCallback(() => {
    setRefreshTrigger((prev) => prev + 1)
  }, [])

  const resolveRealKey = useCallback(
    async (id: number): Promise<string | null> => {
      if (resolvedKeys[id]) return resolvedKeys[id]
      if (id in pendingRequests.current) return pendingRequests.current[id]

      const request = (async () => {
        setLoadingKeys((prev) => ({ ...prev, [id]: true }))
        try {
          let res
          try {
            res = await fetchTokenKey(id)
          } catch (error) {
            if (!isVerificationRequiredError(error)) {
              throw error
            }
            res = (await requestKeyExportVerification({
              apiCall: () => fetchTokenKey(id),
              parseResult: (result) => result,
              title: t('Verify to reveal API key'),
              description: t(
                'Confirm your identity before viewing, copying, or exporting this API key.'
              ),
            })) as Awaited<ReturnType<typeof fetchTokenKey>>
          }

          if (res.success && res.data?.key) {
            const fullKey = `sk-${res.data.key}`
            setResolvedKeys((prev) => ({ ...prev, [id]: fullKey }))
            return fullKey
          }
          toast.error(res.message || t(ERROR_MESSAGES.UNEXPECTED))
          return null
        } catch (error) {
          toast.error(
            error instanceof Error ? error.message : t(ERROR_MESSAGES.UNEXPECTED)
          )
          return null
        } finally {
          delete pendingRequests.current[id]
          setLoadingKeys((prev) => {
            const next = { ...prev }
            delete next[id]
            return next
          })
        }
      })()

      pendingRequests.current[id] = request
      return request
    },
    [resolvedKeys, requestKeyExportVerification, t]
  )

  const resolveRealKeysBatch = useCallback(
    async (ids: number[]): Promise<Record<number, string>> => {
      const uncachedIds = ids.filter((id) => !resolvedKeys[id])
      if (uncachedIds.length === 0) {
        const result: Record<number, string> = {}
        for (const id of ids) result[id] = resolvedKeys[id]
        return result
      }

      for (const id of uncachedIds) {
        setLoadingKeys((prev) => ({ ...prev, [id]: true }))
      }

      try {
        let res
        try {
          res = await fetchTokenKeysBatch(uncachedIds)
        } catch (error) {
          if (!isVerificationRequiredError(error)) {
            throw error
          }
          res = (await requestKeyExportVerification({
            apiCall: () => fetchTokenKeysBatch(uncachedIds),
            parseResult: (result) => result,
            title: t('Verify to export API keys'),
            description: t(
              'Confirm your identity before batch copying or exporting API keys.'
            ),
          })) as Awaited<ReturnType<typeof fetchTokenKeysBatch>>
        }

        if (res.success && res.data?.keys) {
          const newKeys: Record<number, string> = {}
          for (const [idStr, key] of Object.entries(res.data.keys)) {
            newKeys[Number(idStr)] = `sk-${key}`
          }
          setResolvedKeys((prev) => ({ ...prev, ...newKeys }))

          const result: Record<number, string> = { ...newKeys }
          for (const id of ids) {
            if (resolvedKeys[id]) result[id] = resolvedKeys[id]
          }
          return result
        }
        toast.error(res.message || t(ERROR_MESSAGES.UNEXPECTED))
        return {}
      } catch (error) {
        toast.error(
          error instanceof Error ? error.message : t(ERROR_MESSAGES.UNEXPECTED)
        )
        return {}
      } finally {
        for (const id of uncachedIds) {
          setLoadingKeys((prev) => {
            const next = { ...prev }
            delete next[id]
            return next
          })
        }
      }
    },
    [resolvedKeys, requestKeyExportVerification, t]
  )

  return (
    <ApiKeysContext
      value={{
        open,
        setOpen,
        currentRow,
        setCurrentRow,
        refreshTrigger,
        triggerRefresh,
        resolvedKey,
        setResolvedKey,
        resolveRealKey,
        resolveRealKeysBatch,
        resolvedKeys,
        loadingKeys,
        copiedKeyId,
        markKeyCopied,
      }}
    >
      {children}
      <SecureVerificationDialog
        open={verificationOpen}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) cancelVerification()
        }}
        methods={verificationMethods}
        state={verificationState}
        onVerify={async (method, code) => {
          await executeVerification(method, code)
        }}
        onCancel={cancelVerification}
        onCodeChange={setVerificationCode}
        onMethodChange={switchVerificationMethod}
      />
    </ApiKeysContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useApiKeys = () => {
  const apiKeysContext = React.useContext(ApiKeysContext)

  if (!apiKeysContext) {
    throw new Error('useApiKeys has to be used within <ApiKeysContext>')
  }

  return apiKeysContext
}
