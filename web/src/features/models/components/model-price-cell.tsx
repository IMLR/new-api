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

import { Badge } from '@/components/ui/badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { formatPriceUsd, type ModelPriceInfo } from '../lib/model-price'

type ModelPriceCellProps = {
  info?: ModelPriceInfo
}

export function ModelPriceCell({ info }: ModelPriceCellProps) {
  const { t } = useTranslation()

  if (!info) {
    return (
      <Badge variant='outline' className='text-muted-foreground font-normal'>
        {t('Not Set')}
      </Badge>
    )
  }

  if (info.kind === 'per-request') {
    return (
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger
            render={<span className='cursor-help font-mono text-xs' />}
          >
            {formatPriceUsd(info.price)}
          </TooltipTrigger>
          <TooltipContent side='top'>
            {t('Per-request billing (fixed price)')}
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
    )
  }

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger
          render={<span className='cursor-help font-mono text-xs' />}
        >
          {info.output === null
            ? formatPriceUsd(info.input)
            : `${formatPriceUsd(info.input)} / ${formatPriceUsd(info.output)}`}
        </TooltipTrigger>
        <TooltipContent side='top' className='space-y-1'>
          <div>
            {t('Input price ($/1M tokens)')}: {formatPriceUsd(info.input)}
          </div>
          {info.output !== null ? (
            <div>
              {t('Output price ($/1M tokens)')}: {formatPriceUsd(info.output)}
            </div>
          ) : null}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

