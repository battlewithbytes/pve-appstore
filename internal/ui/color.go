package ui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Renderer is the lipgloss renderer bound to stdout.
// lipgloss v1.x auto-detects TrueColor but doesn't apply it without
// an explicit SetColorProfile call on some terminals.
var Renderer = newRenderer()

func newRenderer() *lipgloss.Renderer {
	r := lipgloss.NewRenderer(os.Stdout)
	r.SetColorProfile(termenv.TrueColor)
	return r
}

// Predefined styles for consistent CLI output.
var (
	Green = Renderer.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	Cyan  = Renderer.NewStyle().Foreground(lipgloss.Color("14"))
	Red   = Renderer.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	White = Renderer.NewStyle().Foreground(lipgloss.Color("15"))
	Dim   = Renderer.NewStyle().Foreground(lipgloss.Color("245"))
)
