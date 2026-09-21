import dayjs from '@/lib/dayjs'

export function formatDurationSeconds(
  seconds: unknown,
  t: (key: string) => string
): string {
  const s = Number(seconds)
  if (!Number.isFinite(s) || s <= 0) {
    return '-'
  }

  const total = Math.floor(s)
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const secs = total % 60

  if (hours > 0) {
    return `${hours}${t('h')} ${minutes}${t('m')}`
  }
  if (minutes > 0) {
    return `${minutes}${t('m')} ${secs}${t('s')}`
  }
  return `${secs}${t('s')}`
}

export function formatTimeLeftUntil(
  value: unknown,
  t: (key: string) => string
): string {
  if (typeof value !== 'string' || value.trim() === '') {
    return '-'
  }
  const expiresAt = dayjs(value)
  if (!expiresAt.isValid()) {
    return '-'
  }

  const secondsLeft = expiresAt.diff(dayjs(), 'second')
  if (secondsLeft <= 0) {
    return t('Expired')
  }

  const days = Math.floor(secondsLeft / (24 * 60 * 60))
  const remainingSeconds = secondsLeft % (24 * 60 * 60)
  if (days > 0) {
    const hours = Math.floor(remainingSeconds / 3600)
    return `${days} ${t('days')} ${hours}${t('h')}`
  }

  return formatDurationSeconds(secondsLeft, t)
}
