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
import { useTranslation } from 'react-i18next'

import { SystemInstancesPanel } from '@/features/system-info/components/system-instances-panel'
import { SystemTasksPanel } from '@/features/system-info/components/system-tasks-panel'
import { useStatus } from '@/hooks/use-status'

import { SettingsSection } from '../components/settings-section'
import { UpdateCheckerSection } from './update-checker-section'

/**
 * Instances, background tasks and version updates, shown at the bottom of the
 * advanced configuration page.
 */
export function SystemInfoSection() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const version = (status?.version as string | undefined) ?? null
  const startTime = (status?.start_time as number | null | undefined) ?? null

  return (
    <SettingsSection title={t('System Info')}>
      <div className='space-y-4'>
        <SystemInstancesPanel />
        <SystemTasksPanel />
        <UpdateCheckerSection currentVersion={version} startTime={startTime} />
      </div>
    </SettingsSection>
  )
}

