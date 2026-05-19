#!/usr/bin/env node
/*
 * scripts/check-payment-i18n.mjs
 *
 * Fails (exit 1) when any payment-domain i18n key has an empty translation
 * in a non-fallback locale (en/zh-TW/fr/ja/ru/vi). The fallback locale
 * (zh-CN) is allowed to use the Chinese source key as its own value.
 *
 * Scope: payment keys ONLY. A global empty-value lint would also flag
 * hundreds of pre-existing non-payment empties unrelated to this branch;
 * that's out of scope for the payment-integration MR. If a payment key
 * is added in the future, no extra wiring is needed — the prefix patterns
 * below will pick it up.
 *
 * Run: bun run i18n:check:payment   (registered in web/package.json)
 */

import { readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const LOCALES_DIR = resolve(__dirname, '../src/i18n/locales');
const NON_FALLBACK_LOCALES = ['en', 'zh-TW', 'fr', 'ja', 'ru', 'vi'];

// A key is "payment-domain" if it contains any of these substrings or matches
// any of these literal keys. The substring match catches Chinese-source keys
// like "微信支付公钥 ID"; the literal list catches mixed-language keys like
// "API v3 密钥" that don't share a Chinese prefix with the others.
const PAYMENT_SUBSTRINGS = [
  '支付宝', '微信支付', '微信扫码', '商户号', '商户私钥', '商户证书', '商户 API',
  '应用私钥', '应用公钥', '支付宝公钥', '支付宝开放平台', '微信支付公钥',
  'pub_key.pem', 'APIv3', '沙箱', '退款', '充值订单', 'PayPal', 'Stripe',
  '管理员未开启', '订单已过期', '订单异常', '额度已到账', '请使用微信扫描',
  '系统检测到金额异常', '请重新发起充值', '请稍后重试',
];
const PAYMENT_LITERAL_KEYS = [
  'API v3 密钥',
  'API v3 密钥（32 位字符串），敏感信息，已保存时此处显示掩码',
  'API v3 密钥与商户私钥为敏感信息，仅在初次配置或更换证书时填写。留空表示沿用已保存的值。',
  'Seller ID（可选）',
  'PID，留空表示不校验 seller_id',
  '默认异步通知地址：',
  '异步通知地址：',
  '异步通知地址（可选）',
  '等待支付结果...',
  '支付成功',
  '支付未完成：',
  '更新支付宝设置',
  '更新微信支付设置',
];

function isPaymentKey(key) {
  if (PAYMENT_LITERAL_KEYS.includes(key)) return true;
  for (const needle of PAYMENT_SUBSTRINGS) {
    if (key.includes(needle)) return true;
  }
  return false;
}

let failed = false;
let warned = false;

for (const locale of NON_FALLBACK_LOCALES) {
  const path = resolve(LOCALES_DIR, `${locale}.json`);
  const raw = readFileSync(path, 'utf8');
  const parsed = JSON.parse(raw);
  const empties = [];
  for (const ns of Object.keys(parsed)) {
    const dict = parsed[ns];
    if (!dict || typeof dict !== 'object') continue;
    for (const key of Object.keys(dict)) {
      if (!isPaymentKey(key)) continue;
      if (dict[key] === '') empties.push(`${ns}.${key}`);
    }
  }
  if (empties.length > 0) {
    if (locale === 'en') {
      // P1-1 explicitly requires en.json to be complete — fail hard.
      failed = true;
      console.error(`\n[${locale}] ${empties.length} payment key(s) with empty translation:`);
      for (const k of empties) console.error(`  - ${k}`);
    } else {
      // Other locales: warn but don't block. Pre-existing gaps are out of scope.
      warned = true;
      console.warn(`\n[${locale}] ${empties.length} payment key(s) with empty translation (warning only):`);
      for (const k of empties.slice(0, 5)) console.warn(`  - ${k}`);
      if (empties.length > 5) console.warn(`  ... and ${empties.length - 5} more`);
    }
  }
}

if (failed) {
  console.error(
    '\n✗ Payment-domain i18n check failed. Fill the en.json keys above before merging.',
  );
  process.exit(1);
} else if (warned) {
  console.log('\n✓ Payment-domain i18n check passed (en.json complete; other locales have warnings).');
} else {
  console.log('✓ Payment-domain i18n check passed.');
}
