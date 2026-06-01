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
import { Form, FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage, } from '@/components/ui/form';
import { Switch } from '@/components/ui/switch';
import { Button } from '@/components/ui/button';
import { useTranslation } from 'react-i18next';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';

import { SettingsSection } from '../components/settings-section';
import { useUpdateOption } from '../hooks/use-update-option';
import { useQueryClient } from '@tanstack/react-query';
import { isMaskedSecret } from './paypal-settings-section';


export interface AlipaySettingsValues {
  AlipayEnabled: boolean
  AlipayAppId: string
  AlipayPrivateKey: string
  AlipayPublicKey: string
  AlipaySellerId: string
  AlipayIsSandbox: boolean
  AlipayMinTopUp: number
}

interface Props {
  defaultValues: AlipaySettingsValues
  serverAddress?: string
}

export function AlipaySettingsSection({ defaultValues, serverAddress }: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const queryClient = useQueryClient()
  const [loading, setLoading] = useState(false)

  const form = useForm<AlipaySettingsValues>({
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const notificationUrl = serverAddress
    ? `${serverAddress.replace(/\/+$/, '')}/api/user/alipay/notify`
    : `${t('Server Address')}/api/user/alipay/notify`

  const handleSave = async () => {
    setLoading(true)
    try {
      const values = form.getValues()
      const options: { key: string; value: string }[] = []

      // AppId — plain field, only push when changed
      if (values.AlipayAppId !== defaultValues.AlipayAppId) {
        options.push({ key: 'AlipayAppId', value: values.AlipayAppId || '' })
      }

      // Sensitive fields — only push if not masked (i.e., actually edited)
      if (values.AlipayPrivateKey && !isMaskedSecret(values.AlipayPrivateKey)) {
        options.push({ key: 'AlipayPrivateKey', value: values.AlipayPrivateKey })
      }
      if (values.AlipayPublicKey && !isMaskedSecret(values.AlipayPublicKey)) {
        options.push({ key: 'AlipayPublicKey', value: values.AlipayPublicKey })
      }

      // SellerId — plain field, only push when changed
      if (values.AlipaySellerId !== defaultValues.AlipaySellerId) {
        options.push({ key: 'AlipaySellerId', value: values.AlipaySellerId || '' })
      }

      if (defaultValues.AlipayIsSandbox !== values.AlipayIsSandbox) {
        options.push({
          key: 'AlipayIsSandbox',
          value: String(values.AlipayIsSandbox),
        })
      }

      if (
        values.AlipayMinTopUp !== undefined &&
        values.AlipayMinTopUp !== null &&
        values.AlipayMinTopUp !== defaultValues.AlipayMinTopUp
      ) {
        options.push({
          key: 'AlipayMinTopUp',
          value: String(values.AlipayMinTopUp || 1),
        })
      }

      // Enabled switch — saved AFTER credentials, because backend validation
      // rejects enable=true when credentials are still empty
      let enabledOption: { key: string; value: string } | null = null
      if (defaultValues.AlipayEnabled !== values.AlipayEnabled) {
        enabledOption = {
          key: 'AlipayEnabled',
          value: String(values.AlipayEnabled),
        }
      }

      if (options.length === 0 && !enabledOption) {
        toast.success(t('Updated successfully'))
        setLoading(false)
        return
      }

      for (const opt of options) {
        await updateOption.mutateAsync(opt)
      }
      if (enabledOption) {
        await updateOption.mutateAsync(enabledOption)
      }
      // Re-fetch options from the API. The backend strips sensitive fields
      // (suffix Key/Secret/Token), so AlipayPrivateKey and AlipayPublicKey
      // will come back empty. Non-sensitive fields like AlipayAppId,
      // AlipaySellerId, AlipayIsSandbox, AlipayMinTopUp, AlipayEnabled
      // will be re-populated with their saved values.
      await queryClient.invalidateQueries({ queryKey: ['system-options'] })
      toast.success(t('Updated successfully'))
    } catch {
      toast.error(t('Update failed'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <SettingsSection
      title={t('Alipay Settings')}
      description={t('Configure Alipay payment gateway for top-ups')}
    >
      <Form {...form}>
        <form className='space-y-4'>
          {/* Blue info box */}
          <div className='rounded-lg border border-blue-200 bg-blue-50 p-4 text-sm text-blue-900 dark:border-blue-800 dark:bg-blue-950 dark:text-blue-100'>
            <p className='font-medium'>{t('Alipay Webhook Configuration:')}</p>
            <p className='mt-1'>
              {t('Alipay credentials can be obtained from the')}{' '}
              <a
                href='https://open.alipay.com/develop/manage'
                target='_blank'
                rel='noreferrer'
                className='underline hover:opacity-80'
              >
                {t('Alipay Open Platform')}
              </a>
              {t('. It is recommended to test in the')}{' '}
              <a
                href='https://openhome.alipay.com/develop/sandbox/app'
                target='_blank'
                rel='noreferrer'
                className='underline hover:opacity-80'
              >
                {t('Sandbox environment')}
              </a>
              {t(' first.')}
            </p>
            <p className='mt-2'>
              <span className='font-medium'>{t('Notification URL:')}</span>{' '}
              <code className='rounded bg-blue-100 px-1.5 py-0.5 text-xs dark:bg-blue-900'>
                {notificationUrl}
              </code>
            </p>
          </div>

          {/* Amber warning box */}
          <div className='rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-100'>
            <p>
              {t('Private key / Public key are sensitive information. Only fill them when initially configuring or changing. Leave blank to use the saved value.')}
            </p>
          </div>

          {/* AppId + SellerId + MinTopUp: same row */}
          <div className='grid grid-cols-1 gap-4 md:grid-cols-3'>
            <FormField
              control={form.control}
              name='AlipayAppId'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Alipay App ID')}</FormLabel>
                  <FormControl>
                    <Input placeholder={t('Alipay application AppId')} {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='AlipaySellerId'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Seller ID (optional)')}</FormLabel>
                  <FormControl>
                    <Input placeholder={t('PID, leave blank to skip seller_id verification')} {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='AlipayMinTopUp'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Alipay Min TopUp')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      placeholder={t('e.g., 1')}
                      {...field}
                      onChange={(e) => field.onChange(Number(e.target.value) || 1)}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          {/* PrivateKey + PublicKey: same row, Textarea with rows={6} */}
          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='AlipayPrivateKey'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Application Private Key')}</FormLabel>
                  <FormControl>
                    <Textarea
                      rows={6}
                      className='font-mono text-xs'
                      placeholder={t('Alipay private key PEM content, sensitive information, masked when saved')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='AlipayPublicKey'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Alipay Public Key')}</FormLabel>
                  <FormControl>
                    <Textarea
                      rows={6}
                      className='font-mono text-xs'
                      placeholder={t('Alipay public key PEM content, sensitive information, masked when saved')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          {/* Enabled + Sandbox switches: same row */}
          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='AlipayEnabled'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between rounded-lg border p-3'>
                  <div className='space-y-0.5'>
                    <FormLabel>{t('Enable Alipay')}</FormLabel>
                    <FormDescription>
                      {t('When enabled, the Alipay button will appear on the user top-up page')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='AlipayIsSandbox'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between rounded-lg border p-3'>
                  <div className='space-y-0.5'>
                    <FormLabel>{t('Sandbox Mode')}</FormLabel>
                    <FormDescription>
                      {t('When enabled, the Alipay sandbox gateway will be used')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
          </div>

          <Button
            type='button'
            onClick={handleSave}
            disabled={loading}
          >
            {loading ? t('Saving...') : t('Save Alipay settings')}
          </Button>
        </form>
      </Form>
    </SettingsSection>
  )
}
