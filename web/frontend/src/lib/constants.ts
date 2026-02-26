// Container/install status values
export const STATUS = {
  RUNNING: 'running',
  STOPPED: 'stopped',
  INSTALLING: 'installing',
  UNINSTALLED: 'uninstalled',
  FAILED: 'failed',
} as const

// Job state values
export const JOB_STATE = {
  PENDING: 'pending',
  RUNNING: 'running',
  COMPLETED: 'completed',
  FAILED: 'failed',
  CANCELLED: 'cancelled',
} as const

// Polling intervals (ms)
export const POLL_INTERVAL = {
  FAST: 3000,
  NORMAL: 5000,
  SLOW: 10000,
} as const
