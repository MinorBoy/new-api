import { createFileRoute, useNavigate, useSearch } from '@tanstack/react-router'
import i18next from 'i18next'
import { useEffect } from 'react'
import { toast } from 'sonner'

import { wechatLoginByCode } from '@/features/auth/api'
import { sanitizeAuthRedirect } from '@/features/auth/lib/auth-redirect'
import { applyAuthBundle, isAuthBundle } from '@/lib/api'
import { getServerErrorMessageKey } from '@/lib/server-error-message'

function OAuthComponent() {
  const navigate = useNavigate()
  const search = useSearch({ from: '/(auth)/oauth' }) as {
    redirect?: string
    provider?: 'github' | 'discord' | 'oidc' | 'linuxdo' | 'telegram' | 'wechat'
    code?: string
    state?: string
  }

  useEffect(() => {
    ;(async () => {
      try {
        if (search?.provider === 'wechat' && search.code) {
          const res = await wechatLoginByCode(search.code)
          if (res?.success && isAuthBundle(res.data)) {
            applyAuthBundle(res.data)
            const target =
              sanitizeAuthRedirect(search?.redirect, window.location.origin) ??
              '/dashboard'
            navigate({ href: target, replace: true })
            return
          }
          if (getServerErrorMessageKey(res)) {
            navigate({ to: '/sign-in', replace: true })
            return
          }
        }
      } catch (error: unknown) {
        if (getServerErrorMessageKey(error)) {
          navigate({ to: '/sign-in', replace: true })
          return
        }
      }
      toast.error(i18next.t('OAuth failed'))
      navigate({ to: '/sign-in', replace: true })
    })()
  }, [navigate, search])

  return null
}

export const Route = createFileRoute('/(auth)/oauth')({
  component: OAuthComponent,
})
