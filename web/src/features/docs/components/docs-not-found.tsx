import { Link } from '@tanstack/react-router'
import { FileQuestion } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

export function DocsNotFound({ slug }: { slug: string }) {
  const { t } = useTranslation()
  return (
    <div className='mx-auto flex max-w-2xl flex-col items-center gap-6 py-24 text-center'>
      <FileQuestion className='text-muted-foreground size-16' />
      <div className='space-y-2'>
        <h1 className='text-2xl font-semibold'>{t('Page not found')}</h1>
        <p className='text-muted-foreground text-sm'>
          {t('We could not find a documentation page for')}
          {': '}
          <code className='bg-muted rounded px-1.5 py-0.5 font-mono text-xs'>
            {slug}
          </code>
        </p>
      </div>
      <Button render={<Link to='/docs/$' params={{ _splat: 'overview' }} />}>
        {t('Back to documentation')}
      </Button>
    </div>
  )
}
