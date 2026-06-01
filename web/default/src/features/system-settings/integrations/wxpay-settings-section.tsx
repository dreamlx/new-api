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


export interface WxpaySettingsValues {
  WxpayEnabled: boolean
  WxpayAppId: string
  WxpayMchId: string
  WxpayMchSerialNo: string
  WxpayApiV3Key: string
  WxpayPrivateKey: string
  WxpayPublicKeyId: string
  WxpayPublicKey: string
  WxpayMinTopUp: number
}

interface Props {
  defaultValues: WxpaySettingsValues
  serverAddress?: string
}

export function WxpaySettingsSection({ defaultValues, serverAddress }: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const queryClient = useQueryClient()
  const [loading, setLoading] = useState(false)

  const form = useForm<WxpaySettingsValues>({
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const notifyUrl = serverAddress
    ? `${serverAddress.replace(/\/+$/, '')}/api/user/wxpay/notify`
    : `<ServerAddress>/api/user/wxpay/notify`

  const handleSave = async () => {
    setLoading(true)
    try {
      const values = form.getValues()
      const options: { key: string; value: string }[] = []

      // Plain (non-secret) fields — push when changed
      const plainFields: (keyof WxpaySettingsValues)[] = [
        'WxpayAppId',
        'WxpayMchId',
        'WxpayMchSerialNo',
        'WxpayPublicKeyId',
      ]
      for (const key of plainFields) {
        if (values[key] !== defaultValues[key]) {
          options.push({ key, value: values[key] || '' })
        }
      }

      // Secret fields — only push if not masked (i.e., actually edited)
      if (values.WxpayApiV3Key && !isMaskedSecret(values.WxpayApiV3Key)) {
        options.push({ key: 'WxpayApiV3Key', value: values.WxpayApiV3Key })
      }
      if (values.WxpayPrivateKey && !isMaskedSecret(values.WxpayPrivateKey)) {
        options.push({ key: 'WxpayPrivateKey', value: values.WxpayPrivateKey })
      }
      if (values.WxpayPublicKey && !isMaskedSecret(values.WxpayPublicKey)) {
        options.push({ key: 'WxpayPublicKey', value: values.WxpayPublicKey })
      }

      if (
        values.WxpayMinTopUp !== undefined &&
        values.WxpayMinTopUp !== null &&
        values.WxpayMinTopUp !== defaultValues.WxpayMinTopUp
      ) {
        options.push({
          key: 'WxpayMinTopUp',
          value: String(values.WxpayMinTopUp || 1),
        })
      }

      // Enabled switch — saved AFTER credentials, because backend validation
      // rejects enable=true when credentials are still empty
      let enabledOption: { key: string; value: string } | null = null
      if (defaultValues.WxpayEnabled !== values.WxpayEnabled) {
        enabledOption = {
          key: 'WxpayEnabled',
          value: String(values.WxpayEnabled),
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
      // (suffix Key/Secret/Token), so WxpayApiV3Key, WxpayPrivateKey, and
      // WxpayPublicKey will come back empty. Non-sensitive fields like
      // WxpayAppId, WxpayMchId, WxpayMchSerialNo, WxpayPublicKeyId,
      // WxpayMinTopUp, WxpayEnabled will be re-populated with saved values.
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
      title={t('WeChat Pay Settings')}
      description={t('Configure WeChat Pay payment gateway for top-ups')}
    >
      <Form {...form}>
        <form className='space-y-4'>
          {/* Blue info box */}
          <div className='rounded-lg border border-blue-200 bg-blue-50 p-4 dark:border-blue-800 dark:bg-blue-950'>
            <p className='text-sm text-blue-900 dark:text-blue-100'>
              {t('WeChat Pay merchant credentials can be obtained from the')}{' '}
              <a
                href='https://pay.weixin.qq.com/'
                target='_blank'
                rel='noreferrer'
                className='font-medium underline hover:text-blue-700 dark:hover:text-blue-300'
              >
                {t('WeChat Pay Merchant Platform')}
              </a>
              {t('. The system uses Native (QR code) payment mode.')}
            </p>
            <p className='mt-2 text-sm text-blue-900 dark:text-blue-100'>
              <span className='font-medium'>{t('Notification URL:')}</span>{' '}
              <code className='rounded bg-blue-100 px-1.5 py-0.5 text-xs dark:bg-blue-900'>
                {notifyUrl}
              </code>
            </p>
          </div>

          {/* Amber warning box */}
          <div className='rounded-lg border border-amber-200 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-950'>
            <p className='text-sm text-amber-900 dark:text-amber-100'>
              {t('API v3 Key, Merchant Private Key and WeChat Pay Public Key are sensitive information. Only fill them when initially configuring or changing. Leave blank to use the saved value.')}
            </p>
          </div>

          {/* AppId + MchId + MchSerialNo — same row */}
          <div className='grid grid-cols-1 gap-4 md:grid-cols-3'>
            <FormField
              control={form.control}
              name='WxpayAppId'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('WeChat Pay App ID')}</FormLabel>
                  <FormControl>
                    <Input placeholder={t('Official account / Mini program / Mobile app AppId')} {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='WxpayMchId'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Merchant ID MchId')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('WeChat Pay merchant ID')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='WxpayMchSerialNo'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Merchant Certificate Serial No')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('Merchant API certificate serial number')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          {/* PublicKeyId (1/3) + PublicKey (2/3) — same row */}
          <div className='grid grid-cols-1 gap-4 md:grid-cols-3'>
            <FormField
              control={form.control}
              name='WxpayPublicKeyId'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('WeChat Pay Public Key ID')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder='PUB_KEY_ID_...'
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='WxpayPublicKey'
              render={({ field }) => (
                <FormItem className='md:col-span-2'>
                  <FormLabel>{t('WeChat Pay Public Key')}</FormLabel>
                  <FormControl>
                    <Textarea
                      rows={4}
                      className='font-mono text-xs'
                      placeholder={t('Full contents of pub_key.pem, must include BEGIN/END PUBLIC KEY')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          {/* ApiV3Key + PrivateKey — same row, Textarea for PEM keys */}
          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='WxpayApiV3Key'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('WeChat Pay API V3 Key')}</FormLabel>
                  <FormControl>
                    <Textarea
                      rows={4}
                      className='font-mono text-xs'
                      placeholder={t('API v3 key (32-character string), sensitive information, masked when saved')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='WxpayPrivateKey'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('WeChat Pay Private Key')}</FormLabel>
                  <FormControl>
                    <Textarea
                      rows={4}
                      className='font-mono text-xs'
                      placeholder={t('Merchant apiclient_key.pem content, sensitive information, masked when saved')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          {/* MinTopUp + Enabled switch — same row */}
          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='WxpayMinTopUp'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('WeChat Pay Min TopUp')}</FormLabel>
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

            <FormField
              control={form.control}
              name='WxpayEnabled'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between rounded-lg border p-3'>
                  <div className='space-y-0.5'>
                    <FormLabel>{t('Enable WeChat Pay')}</FormLabel>
                    <FormDescription>
                      {t('When enabled, the WeChat Pay QR code button will appear on the user top-up page')}
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
            {loading ? t('Saving...') : t('Save WeChat Pay settings')}
          </Button>
        </form>
      </Form>
    </SettingsSection>
  )
}
