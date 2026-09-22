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
import {
  Activity,
  Box,
  FileText,
  Key,
  LayoutDashboard,
  Radio,
  User,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { type SidebarData } from '@/components/layout/types'
import { getModelsSectionNavItems } from '@/features/system-settings/models/section-registry'
import { getOperationsSectionNavItems } from '@/features/system-settings/operations/section-registry'

/**
 * Root navigation groups for the application sidebar. Personal build:
 * single-level sidebar, settings sections are inlined as the last group
 * instead of a separate drill-in view.
 */
export function useSidebarData(): SidebarData {
  const { t } = useTranslation()

  return {
    navGroups: [
      {
        id: 'general',
        title: t('General'),
        items: [
          {
            title: t('Overview'),
            url: '/dashboard/overview',
            icon: Activity,
          },
          {
            title: t('Dashboard'),
            url: '/dashboard/models',
            icon: LayoutDashboard,
          },
          {
            title: t('API Keys'),
            url: '/keys',
            icon: Key,
          },
          {
            title: t('Usage Logs'),
            url: '/usage-logs',
            icon: FileText,
          },
        ],
      },
      {
        id: 'personal',
        title: t('Personal'),
        items: [
          {
            title: t('Profile'),
            url: '/profile',
            icon: User,
          },
        ],
      },
      {
        id: 'admin',
        title: t('Admin'),
        items: [
          {
            title: t('Channels'),
            url: '/channels',
            icon: Radio,
          },
          {
            title: t('Models'),
            url: '/models/metadata',
            icon: Box,
          },
        ],
      },
      {
        id: 'settings',
        title: t('Settings'),
        items: [
          ...getModelsSectionNavItems(t),
          ...getOperationsSectionNavItems(t),
        ],
      },
    ],
  }
}
