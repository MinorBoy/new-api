import { useTranslation } from 'react-i18next'

import { CodeTabs } from '../lib/code-tabs'
import { resolveDocLocale } from '../lib/resolve-doc'
import { useDocLocale } from '../lib/use-doc-locale'
import type { ApiEndpoint, CodeSample } from '../types'
import { EndpointInfoCard } from './endpoint-info-card'
import { ErrorCodesTable } from './error-codes-table'
import { MethodBadge } from './method-badge'
import { ParamsTable } from './params-table'
import { ProtocolBadge } from './protocol-badge'

function SectionTitle({ children }: { children: React.ReactNode }) {
  return (
    <h3 className='mt-8 mb-3 border-b pb-1 text-base font-semibold'>
      {children}
    </h3>
  )
}

/**
 * The endpoint detail page: a 4stoken-style structured reference rendered from
 * an `ApiEndpoint` record. Sections: header, info card, request params,
 * response params, error codes, code samples.
 */
export function EndpointDetail({ endpoint }: { endpoint: ApiEndpoint }) {
  const { t } = useTranslation()
  const locale = useDocLocale()

  const samples: Array<{
    lang: string
    label: string
    highlight: string
    code: string
  }> = endpoint.codeSamples as CodeSample[]

  return (
    <article className='mx-auto w-full max-w-3xl min-w-0 px-4 py-8 sm:px-6 lg:px-8'>
      {/* Header */}
      <div className='flex flex-wrap items-center gap-3'>
        <MethodBadge method={endpoint.method} />
        <ProtocolBadge protocol={endpoint.protocol} />
        <code className='text-muted-foreground font-mono text-sm'>
          {endpoint.path}
        </code>
      </div>
      <h1 className='mt-3 text-2xl font-bold tracking-tight'>
        {resolveDocLocale(endpoint.title, locale)}
      </h1>
      <p className='text-muted-foreground mt-2'>
        {resolveDocLocale(endpoint.summary, locale)}
      </p>

      {/* Info card */}
      <SectionTitle>{t('Endpoint Info')}</SectionTitle>
      <EndpointInfoCard endpoint={endpoint} />

      {/* Request params */}
      {endpoint.requestParams && endpoint.requestParams.length > 0 && (
        <>
          <SectionTitle>{t('Request Parameters')}</SectionTitle>
          <ParamsTable
            fields={endpoint.requestParams}
            requiredHeader={t('Required')}
          />
        </>
      )}

      {/* Response params */}
      {endpoint.responseParams && endpoint.responseParams.length > 0 && (
        <>
          <SectionTitle>{t('Response Parameters')}</SectionTitle>
          <ParamsTable
            fields={endpoint.responseParams}
            requiredHeader={t('Returns')}
          />
        </>
      )}

      {/* Error codes */}
      {endpoint.errorCodes && endpoint.errorCodes.length > 0 && (
        <>
          <SectionTitle>{t('Error Codes')}</SectionTitle>
          <ErrorCodesTable rows={endpoint.errorCodes} />
        </>
      )}

      {/* Code samples */}
      <SectionTitle>{t('Code Samples')}</SectionTitle>
      <div className='not-prose'>
        <CodeTabs tabs={samples} />
      </div>
    </article>
  )
}
