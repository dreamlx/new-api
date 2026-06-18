export function buildDisablePayPalOptions(): { key: string; value: string }[] {
  return [
    { key: 'PayPalClientId', value: '' },
    { key: 'PayPalClientSecret', value: '' },
    { key: 'PayPalWebhookSecret', value: '' },
  ]
}
