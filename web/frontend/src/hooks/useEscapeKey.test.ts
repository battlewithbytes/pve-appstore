import { describe, it, expect, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { fireEvent } from '@testing-library/react'
import { useEscapeKey } from './useEscapeKey'

describe('useEscapeKey', () => {
  it('calls callback on Escape keypress', () => {
    const callback = vi.fn()
    renderHook(() => useEscapeKey(callback))

    fireEvent.keyDown(window, { key: 'Escape' })
    expect(callback).toHaveBeenCalledTimes(1)
  })

  it('does not call callback on other keys', () => {
    const callback = vi.fn()
    renderHook(() => useEscapeKey(callback))

    fireEvent.keyDown(window, { key: 'Enter' })
    fireEvent.keyDown(window, { key: 'a' })
    fireEvent.keyDown(window, { key: 'Tab' })
    fireEvent.keyDown(window, { key: 'ArrowUp' })
    expect(callback).not.toHaveBeenCalled()
  })

  it('calls callback multiple times on multiple Escape presses', () => {
    const callback = vi.fn()
    renderHook(() => useEscapeKey(callback))

    fireEvent.keyDown(window, { key: 'Escape' })
    fireEvent.keyDown(window, { key: 'Escape' })
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(callback).toHaveBeenCalledTimes(3)
  })

  it('removes event listener on cleanup', () => {
    const callback = vi.fn()
    const { unmount } = renderHook(() => useEscapeKey(callback))

    // Escape works before unmount
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(callback).toHaveBeenCalledTimes(1)

    // After unmount, listener should be removed
    unmount()
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(callback).toHaveBeenCalledTimes(1) // still 1, not 2
  })

  it('updates callback reference when it changes', () => {
    const callback1 = vi.fn()
    const callback2 = vi.fn()

    const { rerender } = renderHook(
      ({ cb }) => useEscapeKey(cb),
      { initialProps: { cb: callback1 } }
    )

    fireEvent.keyDown(window, { key: 'Escape' })
    expect(callback1).toHaveBeenCalledTimes(1)
    expect(callback2).toHaveBeenCalledTimes(0)

    rerender({ cb: callback2 })

    fireEvent.keyDown(window, { key: 'Escape' })
    expect(callback1).toHaveBeenCalledTimes(1) // not called again
    expect(callback2).toHaveBeenCalledTimes(1)
  })
})
