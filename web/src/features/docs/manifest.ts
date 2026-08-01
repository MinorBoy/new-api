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
import { BookOpen, Cable, Code2, Terminal } from 'lucide-react'

import billingRulesEn from './content/en/billing-rules.md'
import curlEn from './content/en/clients/curl.md'
import pythonEn from './content/en/clients/python.md'
import endpointsEn from './content/en/endpoints.md'
import errorCodesEn from './content/en/error-codes.md'
import overviewEn from './content/en/overview.md'
import pricingEn from './content/en/pricing.md'
import quickstartEn from './content/en/quickstart.md'
import billingRulesZh from './content/zh/billing-rules.md'
import curlZh from './content/zh/clients/curl.md'
import pythonZh from './content/zh/clients/python.md'
import endpointsZh from './content/zh/endpoints.md'
import errorCodesZh from './content/zh/error-codes.md'
// Markdown sources are inlined as raw strings by the rsbuild `asset/source`
// rule (see rsbuild.config.ts). Keep these imports grouped per category so the
// tree reads top-to-bottom in sidebar order.
import overviewZh from './content/zh/overview.md'
import pricingZh from './content/zh/pricing.md'
import quickstartZh from './content/zh/quickstart.md'
import type { DocNavGroup } from './types'

/**
 * The documentation tree. This is the single source of truth for the sidebar,
 * the top-level category tabs, the search index, and prev/next navigation.
 *
 * To add a page:
 *   1. Create `content/{zh,en}/<slug>.md`.
 *   2. Import both language strings above.
 *   3. Add a `DocPage` entry under the right group below.
 */
export const docsNavGroups: DocNavGroup[] = [
  {
    id: 'getting-started',
    title: { en: 'Getting Started', zh: '入门' },
    icon: BookOpen,
    pages: [
      {
        slug: 'overview',
        title: { en: 'Overview', zh: '使用概览' },
        content: { en: overviewEn, zh: overviewZh },
        order: 1,
      },
      {
        slug: 'quickstart',
        title: { en: 'Quickstart', zh: '快速接入' },
        content: { en: quickstartEn, zh: quickstartZh },
        order: 2,
      },
    ],
  },
  {
    id: 'api-reference',
    title: { en: 'API Reference', zh: '接口参考' },
    icon: Cable,
    pages: [
      {
        slug: 'endpoints',
        title: { en: 'Endpoints', zh: '接口地址' },
        content: { en: endpointsEn, zh: endpointsZh },
        order: 1,
      },
      {
        slug: 'pricing',
        title: { en: 'Models & Pricing', zh: '模型与计费' },
        content: { en: pricingEn, zh: pricingZh },
        order: 2,
      },
      {
        slug: 'error-codes',
        title: { en: 'Error Codes', zh: '错误码参考' },
        content: { en: errorCodesEn, zh: errorCodesZh },
        order: 3,
      },
    ],
  },
  {
    id: 'clients',
    title: { en: 'Clients', zh: '客户端接入' },
    icon: Code2,
    pages: [
      {
        slug: 'clients/curl',
        title: { en: 'cURL', zh: 'cURL' },
        content: { en: curlEn, zh: curlZh },
        order: 1,
      },
      {
        slug: 'clients/python',
        title: { en: 'Python', zh: 'Python' },
        content: { en: pythonEn, zh: pythonZh },
        order: 2,
      },
    ],
  },
  {
    id: 'operations',
    title: { en: 'Operations', zh: '运维' },
    icon: Terminal,
    pages: [
      {
        slug: 'billing-rules',
        title: { en: 'Billing Rules', zh: '计费规则' },
        content: { en: billingRulesEn, zh: billingRulesZh },
        order: 1,
      },
    ],
  },
]

/** Flat list of all pages in sidebar order, with group indices attached. */
export type FlatDocEntry = {
  page: DocNavGroup['pages'][number]
  group: DocNavGroup
  groupIndex: number
  pageIndex: number
}

export const flatDocEntries: FlatDocEntry[] = docsNavGroups.flatMap(
  (group, groupIndex) =>
    [...group.pages]
      .sort((a, b) => a.order - b.order)
      .map((page, pageIndex) => ({ page, group, groupIndex, pageIndex }))
)

/** The default page shown at `/docs` with no slug. */
export const docsHomeSlug = 'overview'
