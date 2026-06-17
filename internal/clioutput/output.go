package clioutput

import (
	"fmt"

	"github.com/fatih/color"
)

// Style identifies the terminal style used by command output primitives.
type Style int

const (
	StylePlain Style = iota
	StyleSuccess
	StyleWarn
	StyleError
	StyleInfo
	StyleMuted
	StyleBold
	StyleBlue
	StyleMagenta
)

// Symbol identifies the Unicode symbol used by command output primitives.
type Symbol string

const (
	SymbolSection Symbol = "▶"
	SymbolSuccess Symbol = "✔"
	SymbolInfo    Symbol = "ℹ"
	SymbolWarn    Symbol = "⚠"
	SymbolError   Symbol = "✘"
	SymbolItem    Symbol = "→"
	SymbolPrompt  Symbol = "?"
	SymbolDone    Symbol = "🎉"
	SymbolCommand Symbol = "$"
)

// Text returns formatted text with the requested terminal style.
func Text(style Style, format string, args ...any) string {
	text := formatMessage(format, args...)
	switch style {
	case StyleSuccess:
		return sprint(text, color.FgHiGreen)
	case StyleWarn:
		return sprint(text, color.FgHiYellow)
	case StyleError:
		return sprint(text, color.FgHiRed)
	case StyleInfo:
		return sprint(text, color.FgHiCyan)
	case StyleMuted:
		return sprint(text, color.FgHiBlack)
	case StyleBold:
		return sprint(text, color.Bold)
	case StyleBlue:
		return sprint(text, color.FgHiBlue)
	case StyleMagenta:
		return sprint(text, color.FgHiMagenta)
	default:
		return text
	}
}

func sprint(text string, attributes ...color.Attribute) string {
	c := color.New(attributes...)
	if !color.NoColor {
		c.EnableColor()
	}
	return c.Sprint(text)
}

// Line prints an indented command output line.
func Line(style Style, format string, args ...any) {
	fmt.Printf("  %s\n", Text(style, format, args...))
}

// Status prints an indented command output line with a symbol and optional label.
func Status(style Style, symbol Symbol, label string, format string, args ...any) {
	marker := string(symbol)
	if label != "" {
		marker = fmt.Sprintf("%s %s", marker, label)
	}

	message := formatMessage(format, args...)
	if message == "" {
		fmt.Printf("  %s\n", Text(style, "%s", marker))
		return
	}
	fmt.Printf("  %s %s\n", Text(style, "%s", marker), message)
}

// Success prints a successful command status line.
func Success(label string, format string, args ...any) {
	Status(StyleSuccess, SymbolSuccess, label, format, args...)
}

// Info prints an informational command status line.
func Info(label string, format string, args ...any) {
	Status(StyleInfo, SymbolInfo, label, format, args...)
}

// Warn prints a warning command status line.
func Warn(label string, format string, args ...any) {
	Status(StyleWarn, SymbolWarn, label, format, args...)
}

// Error prints an error command status line.
func Error(label string, format string, args ...any) {
	Status(StyleError, SymbolError, label, format, args...)
}

// Item prints a secondary command status line.
func Item(label string, format string, args ...any) {
	Status(StyleMuted, SymbolItem, label, format, args...)
}

// Command prints a shell command suggestion line.
func Command(format string, args ...any) {
	Status(StyleInfo, SymbolCommand, "", format, args...)
}

// Section prints a titled command output section.
func Section(title string) {
	fmt.Printf("\n%s %s\n", Text(StyleInfo, "%s", SymbolSection), Text(StyleBold, "%s", title))
}

// Prompt prints an interactive prompt without appending a newline.
func Prompt(format string, args ...any) {
	fmt.Printf("\n%s %s", Text(StyleInfo, "%s", SymbolPrompt), formatMessage(format, args...))
}

// Done prints a command completion line.
func Done(format string, args ...any) {
	fmt.Printf("\n%s %s\n", Text(StyleSuccess, "%s", SymbolDone), formatMessage(format, args...))
}

func formatMessage(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}
