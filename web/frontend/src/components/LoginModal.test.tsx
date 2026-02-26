import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { LoginForm, LoginModal } from './LoginModal'

// Mock the api module
vi.mock('../api', () => ({
  api: {
    login: vi.fn(),
  },
}))

import { api } from '../api'

describe('LoginForm', () => {
  const mockOnSuccess = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders password input and login button', () => {
    render(<LoginForm onSuccess={mockOnSuccess} />)

    expect(screen.getByPlaceholderText('Password')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Login' })).toBeInTheDocument()
  })

  it('disables login button when password is empty', () => {
    render(<LoginForm onSuccess={mockOnSuccess} />)

    const button = screen.getByRole('button', { name: 'Login' })
    expect(button).toBeDisabled()
  })

  it('enables login button when password is entered', async () => {
    const user = userEvent.setup()
    render(<LoginForm onSuccess={mockOnSuccess} />)

    await user.type(screen.getByPlaceholderText('Password'), 'mypassword')

    const button = screen.getByRole('button', { name: 'Login' })
    expect(button).not.toBeDisabled()
  })

  it('calls api.login and onSuccess on successful submit', async () => {
    const user = userEvent.setup()
    vi.mocked(api.login).mockResolvedValueOnce({ status: 'ok' })

    render(<LoginForm onSuccess={mockOnSuccess} />)

    await user.type(screen.getByPlaceholderText('Password'), 'correctpassword')
    await user.click(screen.getByRole('button', { name: 'Login' }))

    await waitFor(() => {
      expect(api.login).toHaveBeenCalledWith('correctpassword')
      expect(mockOnSuccess).toHaveBeenCalled()
    })
  })

  it('displays error on failed login', async () => {
    const user = userEvent.setup()
    vi.mocked(api.login).mockRejectedValueOnce(new Error('Invalid password'))

    render(<LoginForm onSuccess={mockOnSuccess} />)

    await user.type(screen.getByPlaceholderText('Password'), 'wrongpassword')
    await user.click(screen.getByRole('button', { name: 'Login' }))

    await waitFor(() => {
      expect(screen.getByText('Invalid password')).toBeInTheDocument()
    })
    expect(mockOnSuccess).not.toHaveBeenCalled()
  })

  it('shows loading state during submission', async () => {
    const user = userEvent.setup()
    // Create a promise we control to keep login pending
    let resolveLogin: (v: { status: string }) => void
    vi.mocked(api.login).mockReturnValueOnce(
      new Promise((resolve) => { resolveLogin = resolve })
    )

    render(<LoginForm onSuccess={mockOnSuccess} />)

    await user.type(screen.getByPlaceholderText('Password'), 'mypassword')
    await user.click(screen.getByRole('button', { name: 'Login' }))

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Logging in...' })).toBeDisabled()
    })

    // Resolve the login to clean up
    resolveLogin!({ status: 'ok' })
  })
})

describe('LoginModal', () => {
  const mockOnSuccess = vi.fn()
  const mockOnClose = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the modal with title and form', () => {
    render(<LoginModal onSuccess={mockOnSuccess} onClose={mockOnClose} />)

    expect(screen.getByText('Login Required')).toBeInTheDocument()
    expect(screen.getByText('Enter your password to perform this action.')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('Password')).toBeInTheDocument()
  })

  it('renders cancel button that calls onClose', async () => {
    const user = userEvent.setup()
    render(<LoginModal onSuccess={mockOnSuccess} onClose={mockOnClose} />)

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(mockOnClose).toHaveBeenCalled()
  })

  it('calls onClose on Escape key', () => {
    render(<LoginModal onSuccess={mockOnSuccess} onClose={mockOnClose} />)

    fireEvent.keyDown(window, { key: 'Escape' })
    expect(mockOnClose).toHaveBeenCalled()
  })

  it('does not call onClose on non-Escape keys', () => {
    render(<LoginModal onSuccess={mockOnSuccess} onClose={mockOnClose} />)

    fireEvent.keyDown(window, { key: 'Enter' })
    fireEvent.keyDown(window, { key: 'a' })
    expect(mockOnClose).not.toHaveBeenCalled()
  })
})
