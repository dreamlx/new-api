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
import LanguageDetector from 'i18next-browser-languagedetector';
import { initReactI18next } from 'react-i18next';
import i18n from 'i18next';

import wisemodelZh from './wisemodel-locales/zh.json';
import wisemodelVi from './wisemodel-locales/vi.json';
import wisemodelRu from './wisemodel-locales/ru.json';
import wisemodelJa from './wisemodel-locales/ja.json';
import wisemodelFr from './wisemodel-locales/fr.json';
// WiseModel extension namespace. Living in its own namespace so future
// upstream syncs cannot conflict on these UI strings.
import wisemodelEn from './wisemodel-locales/en.json';
import zh from './locales/zh.json';
import vi from './locales/vi.json';
import ru from './locales/ru.json';
import ja from './locales/ja.json';
import fr from './locales/fr.json';
import en from './locales/en.json';


export const resources = {
  en: { ...en, wisemodel: wisemodelEn },
  zh: { ...zh, wisemodel: wisemodelZh },
  fr: { ...fr, wisemodel: wisemodelFr },
  ru: { ...ru, wisemodel: wisemodelRu },
  ja: { ...ja, wisemodel: wisemodelJa },
  vi: { ...vi, wisemodel: wisemodelVi },
} as const

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    fallbackLng: 'en',
    supportedLngs: ['en', 'zh', 'fr', 'ru', 'ja', 'vi'],
    ns: ['translation', 'wisemodel'],
    defaultNS: 'translation',
    fallbackNS: 'translation',
    load: 'languageOnly', // Convert zh-CN -> zh
    nsSeparator: false, // Allow literal colons in keys (e.g., URLs, labels)
    debug: import.meta.env.DEV,
    interpolation: {
      escapeValue: false, // not needed for react as it escapes by default
    },
    detection: {
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
    },
  })

export default i18n
