import { SettingsPage } from '../components/settings-page'
import type { SiteSettings } from '../types'
import {
  SITE_DEFAULT_SECTION,
  getSiteSectionContent,
  getSiteSectionMeta,
} from './section-registry.tsx'

const defaultSiteSettings: SiteSettings = {
  // home.style selects the public landing page composition consumed via
  // GET /api/home_page_style without switching frontend products.
  'home.style': 'default',
  Notice: '',
  SystemName: 'New API',
  Logo: '',
  Footer: '',
  About: '',
  HomePageContent: '',
  ServerAddress: '',
  'legal.user_agreement': '',
  'legal.privacy_policy': '',
  HeaderNavModules: '',
  SidebarModulesAdmin: '',
}

export function SiteSettings() {
  return (
    <SettingsPage
      routePath='/_authenticated/system-settings/site/$section'
      defaultSettings={defaultSiteSettings}
      defaultSection={SITE_DEFAULT_SECTION}
      getSectionContent={getSiteSectionContent}
      getSectionMeta={getSiteSectionMeta}
    />
  )
}
