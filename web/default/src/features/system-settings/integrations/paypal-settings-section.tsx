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
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage, } from '@/components/ui/form';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue, } from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import { useTranslation } from 'react-i18next';
import { Input } from '@/components/ui/input';
import { useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';

import { SettingsSection } from '../components/settings-section';
import { useUpdateOption } from '../hooks/use-update-option';
import { useQueryClient } from '@tanstack/react-query';


/**
 * Heuristic to detect a masked secret returned from the server.
 * The backend masks sensitive option fields as "***xxxx" (asterisks + last 4
 * chars), so we treat any value containing this pattern as "not edited" and
 * skip the PUT for that field. Only values that the admin re-types are sent
 * upstream.
 */
export function isMaskedSecret(value: string): boolean {
  return typeof value === 'string' && value.includes('***')
}

export interface PayPalSettingsValues {
  PayPalClientId: string
  PayPalClientSecret: string
  PayPalWebhookSecret: string
  PayPalMode: string
  PayPalMinTopUp: number
}

interface Props {
  defaultValues: PayPalSettingsValues
  serverAddress?: string
}

export function PayPalSettingsSection({ defaultValues, serverAddress }: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const queryClient = useQueryClient()
  const [loading, setLoading] = useState(false)

  const form = useForm<PayPalSettingsValues>({
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const handleSave = async () => {
    setLoading(true)
    try {
      const values = form.getValues()
      const options: { key: string; value: string }[] = []

      // ClientId — only push if not masked (sensitive field)
      if (values.PayPalClientId && !isMaskedSecret(values.PayPalClientId)) {
        options.push({ key: 'PayPalClientId', value: values.PayPalClientId })
      }

      // Only push sensitive fields if they were actually edited
      // (i.e., not still masked from the server response)
      if (values.PayPalClientSecret && !isMaskedSecret(values.PayPalClientSecret)) {
        options.push({
          key: 'PayPalClientSecret',
          value: values.PayPalClientSecret,
        })
      }
      if (values.PayPalWebhookSecret && !isMaskedSecret(values.PayPalWebhookSecret)) {
        options.push({
          key: 'PayPalWebhookSecret',
          value: values.PayPalWebhookSecret,
        })
      }

      options.push({
        key: 'PayPalMode',
        value: values.PayPalMode || 'sandbox',
      })
      options.push({
        key: 'PayPalMinTopUp',
        value: String(values.PayPalMinTopUp || 1),
      })

      for (const opt of options) {
        await updateOption.mutateAsync(opt)
      }
      // Re-fetch options from the API. The backend strips sensitive fields
      // (suffix Secret/Key/Token), so the refreshed defaultValues will have
      // those fields as empty — non-sensitive fields like PayPalMode and
      // PayPalMinTopUp will be re-populated with their saved values.
      await queryClient.invalidateQueries({ queryKey: ['system-options'] })
      toast.success(t('Updated successfully'))
    } catch {
      toast.error(t('Update failed'))
    } finally {
      setLoading(false)
    }
  }

  const webhookUrl = serverAddress
    ? `${serverAddress}/api/paypal/webhook`
    : '<ServerAddress>/api/paypal/webhook'

  return (
    <SettingsSection
      title={t('PayPal Settings')}
      description={t('Configure PayPal payment gateway for top-ups')}
    >
      <Form {...form}>
        <form className='space-y-4'>
          <div className='rounded-md bg-blue-50 p-4 text-sm text-blue-900 dark:bg-blue-950 dark:text-blue-100'>
            <p className='mb-2 font-medium'>{t('PayPal Webhook Configuration:')}</p>
            <ul className='list-inside list-disc space-y-1'>
              <li>
                {t('PayPal credentials can be obtained from the')}{' '}
                <a
                  href='https://developer.paypal.com/dashboard'
                  target='_blank'
                  rel='noreferrer'
                  className='underline hover:no-underline'
                >
                  {t('PayPal Developer Dashboard')}
                </a>
                {t('. It is recommended to test in the')}{' '}
                <a
                  href='https://sandbox.paypal.com'
                  target='_blank'
                  rel='noreferrer'
                  className='underline hover:no-underline'
                >
                  {t('Sandbox environment')}
                </a>
                {t('first.')}
              </li>
              <li>
                {t('Webhook URL:')}{' '}
                <code className='rounded bg-blue-100 px-1 py-0.5 text-xs dark:bg-blue-900'>
                  {webhookUrl}
                </code>
              </li>
              <li>
                {t('Required events:')}{' '}
                <code className='rounded bg-blue-100 px-1 py-0.5 text-xs dark:bg-blue-900'>
                  CHECKOUT.ORDER.APPROVED
                </code>{' '}
                {t('and')}{' '}
                <code className='rounded bg-blue-100 px-1 py-0.5 text-xs dark:bg-blue-900'>
                  PAYMENT.CAPTURE.COMPLETED
                </code>
              </li>
            </ul>
          </div>

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='PayPalClientId'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('PayPal Client ID')}</FormLabel>
                  <FormControl>
                    <Input
                      type='password'
                      placeholder={t('PayPal application Client ID, sensitive information not displayed')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='PayPalClientSecret'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('PayPal Client Secret')}</FormLabel>
                  <FormControl>
                    <Input
                      type='password'
                      placeholder={t('PayPal application Client Secret, sensitive information not displayed')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='PayPalWebhookSecret'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('PayPal Webhook Secret')}</FormLabel>
                  <FormControl>
                    <Input
                      type='password'
                      placeholder={t('PayPal Webhook signing secret, sensitive information not displayed')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='PayPalMode'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('PayPal Mode')}</FormLabel>
                  <Select
                    onValueChange={field.onChange}
                    defaultValue={field.value || 'sandbox'}
                    value={field.value || 'sandbox'}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value='sandbox'>{t('Sandbox (Test Environment)')}</SelectItem>
                      <SelectItem value='live'>{t('Live (Production Environment)')}</SelectItem>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='PayPalMinTopUp'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('PayPal Min TopUp')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={1}
                    placeholder={t('Unit: USD')}
                    {...field}
                    onChange={(e) => field.onChange(Number(e.target.value) || 1)}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <Button
            type='button'
            onClick={handleSave}
            disabled={loading}
          >
            {loading ? t('Saving...') : t('Save PayPal settings')}
          </Button>
        </form>
      </Form>
    </SettingsSection>
  )
}
