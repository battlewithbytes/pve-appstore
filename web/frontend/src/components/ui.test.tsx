import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  Center,
  BackLink,
  Badge,
  StatusDot,
  StateBadge,
  ResourceCard,
  InfoCard,
  InfoRow,
  Linkify,
  SectionTitle,
  FormRow,
  ActionButton,
  DevStatusBadge,
} from './ui'

describe('Center', () => {
  it('renders children', () => {
    render(<Center>Hello World</Center>)
    expect(screen.getByText('Hello World')).toBeInTheDocument()
  })

  it('applies custom className', () => {
    const { container } = render(<Center className="custom">Content</Center>)
    expect(container.firstChild).toHaveClass('custom')
  })
})

describe('BackLink', () => {
  it('renders default back link', () => {
    render(<BackLink />)
    const link = screen.getByRole('link')
    expect(link).toHaveAttribute('href', '#/')
    expect(link).toHaveTextContent('Back to apps')
  })

  it('renders with custom href and label', () => {
    render(<BackLink href="#/settings" label="Back to settings" />)
    const link = screen.getByRole('link')
    expect(link).toHaveAttribute('href', '#/settings')
    expect(link).toHaveTextContent('Back to settings')
  })
})

describe('Badge', () => {
  it('renders children', () => {
    render(<Badge>v1.0</Badge>)
    expect(screen.getByText('v1.0')).toBeInTheDocument()
  })

  it('applies custom className', () => {
    render(<Badge className="bg-red-500">Alert</Badge>)
    expect(screen.getByText('Alert')).toHaveClass('bg-red-500')
  })
})

describe('StatusDot', () => {
  it('renders running dot with pulse animation class', () => {
    const { container } = render(<StatusDot running={true} />)
    const dot = container.firstChild as HTMLElement
    expect(dot).toHaveClass('bg-status-running')
  })

  it('renders stopped dot', () => {
    const { container } = render(<StatusDot running={false} />)
    const dot = container.firstChild as HTMLElement
    expect(dot).toHaveClass('bg-status-stopped')
  })
})

describe('StateBadge', () => {
  it('renders completed state', () => {
    render(<StateBadge state="completed" />)
    expect(screen.getByText('completed')).toBeInTheDocument()
  })

  it('renders failed state', () => {
    render(<StateBadge state="failed" />)
    expect(screen.getByText('failed')).toBeInTheDocument()
  })

  it('renders cancelled state', () => {
    render(<StateBadge state="cancelled" />)
    expect(screen.getByText('cancelled')).toBeInTheDocument()
  })

  it('renders running state with warning styling', () => {
    render(<StateBadge state="running" />)
    expect(screen.getByText('running')).toBeInTheDocument()
  })
})

describe('ResourceCard', () => {
  it('renders label and value', () => {
    render(<ResourceCard label="CPU" value="4 cores" />)
    expect(screen.getByText('CPU')).toBeInTheDocument()
    expect(screen.getByText('4 cores')).toBeInTheDocument()
  })

  it('renders sub text when provided', () => {
    render(<ResourceCard label="Memory" value="2 GB" sub="8 GB" />)
    expect(screen.getByText('/ 8 GB')).toBeInTheDocument()
  })

  it('renders progress bar when pct provided', () => {
    const { container } = render(<ResourceCard label="Disk" value="50 GB" pct={60} />)
    const progressBar = container.querySelector('[style]')
    expect(progressBar).toHaveStyle({ width: '60%' })
  })

  it('caps progress bar at 100%', () => {
    const { container } = render(<ResourceCard label="Disk" value="100 GB" pct={150} />)
    const progressBar = container.querySelector('[style]')
    expect(progressBar).toHaveStyle({ width: '100%' })
  })
})

describe('InfoCard', () => {
  it('renders title and children', () => {
    render(<InfoCard title="Details"><p>Some info</p></InfoCard>)
    expect(screen.getByText('Details')).toBeInTheDocument()
    expect(screen.getByText('Some info')).toBeInTheDocument()
  })
})

describe('Linkify', () => {
  it('renders plain text without links', () => {
    render(<Linkify text="Hello world" />)
    expect(screen.getByText('Hello world')).toBeInTheDocument()
  })

  it('converts URLs to clickable links', () => {
    render(<Linkify text="Visit https://example.com for more" />)
    const link = screen.getByRole('link')
    expect(link).toHaveAttribute('href', 'https://example.com')
    expect(link).toHaveAttribute('target', '_blank')
  })
})

describe('InfoRow', () => {
  it('renders label and value', () => {
    render(<InfoRow label="Version" value="1.0.0" />)
    expect(screen.getByText('Version')).toBeInTheDocument()
    expect(screen.getByText('1.0.0')).toBeInTheDocument()
  })
})

describe('SectionTitle', () => {
  it('renders children', () => {
    render(<SectionTitle>Configuration</SectionTitle>)
    expect(screen.getByText('Configuration')).toBeInTheDocument()
  })
})

describe('FormRow', () => {
  it('renders label and children', () => {
    render(<FormRow label="Name"><input /></FormRow>)
    expect(screen.getByText('Name')).toBeInTheDocument()
  })

  it('renders help text when provided', () => {
    render(<FormRow label="Name" help="Enter a unique name"><input /></FormRow>)
    expect(screen.getByText('Enter a unique name')).toBeInTheDocument()
  })

  it('renders description when provided', () => {
    render(<FormRow label="Name" description="App identifier"><input /></FormRow>)
    expect(screen.getByText('App identifier')).toBeInTheDocument()
  })
})

describe('ActionButton', () => {
  it('renders label and handles click', async () => {
    const user = userEvent.setup()
    const onClick = vi.fn()

    render(<ActionButton label="Start" onClick={onClick} />)
    await user.click(screen.getByText('Start'))
    expect(onClick).toHaveBeenCalledTimes(1)
  })
})

describe('DevStatusBadge', () => {
  it('renders status text', () => {
    render(<DevStatusBadge status="draft" />)
    expect(screen.getByText('draft')).toBeInTheDocument()
  })

  it('renders deployed status', () => {
    render(<DevStatusBadge status="deployed" />)
    expect(screen.getByText('deployed')).toBeInTheDocument()
  })

  it('renders unknown status with draft styling', () => {
    render(<DevStatusBadge status="unknown" />)
    expect(screen.getByText('unknown')).toBeInTheDocument()
  })
})
