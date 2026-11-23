// Package clioutput is how gg writes to the terminal: the few line shapes
// every command prints with, so that all of them read alike.
//
// What a command reports goes to standard output: the titles of its
// sections, the entries under them and the line a command that succeeded
// ends on. What it says to the person running it goes to standard error:
// warnings, errors it goes on from, questions and the line a command that
// failed ends on. A pipe or a redirection of the output carries the report
// alone, and the questions and failures still reach the terminal.
//
//	▶ Section                a titled block of entries
//	  ✔ entry                Success
//	  → entry                Item
//	  ✘ entry                Status, an entry under the symbol and in the style the caller names
//	    detail               Detail, a line under an entry
//	  CREATE  path           Change, something written, removed or left
//	  $ command              Command, one to run next
//	  → progress             Progress, on standard error
//	  ⚠ warning              Warn, on standard error
//	  ✘ error                Error, on standard error
//
//	✔ verdict                Done, the last line of a command that succeeded
//	✘ verdict                Fail, on standard error, of one that failed
//	  note                   Note, what to do next
//
//	? question (y/N):        Confirm, on standard error
//
// A line is colored when the stream it goes to is a terminal that takes
// colors, each stream judged on its own.
package clioutput

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

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

// attributes are the terminal attributes of the styles; StylePlain has none.
var attributes = map[Style]color.Attribute{
	StyleSuccess: color.FgHiGreen,
	StyleWarn:    color.FgHiYellow,
	StyleError:   color.FgHiRed,
	StyleInfo:    color.FgHiCyan,
	StyleMuted:   color.FgHiBlack,
	StyleBold:    color.Bold,
	StyleBlue:    color.FgHiBlue,
	StyleMagenta: color.FgHiMagenta,
}

// Symbol identifies the Unicode symbol used by command output primitives.
type Symbol string

const (
	SymbolSection Symbol = "▶"
	SymbolSuccess Symbol = "✔"
	SymbolWarn    Symbol = "⚠"
	SymbolError   Symbol = "✘"
	SymbolItem    Symbol = "→"
	SymbolPrompt  Symbol = "?"
	SymbolCommand Symbol = "$"
)

// Label names what a command did to a file or a registration: the first
// word of the line Change prints.
type Label string

const (
	// LabelCreate and LabelUpdate are a file written for the first time and
	// written over.
	LabelCreate Label = "CREATE"
	LabelUpdate Label = "UPDATE"
	// LabelRemove is a file deleted, or a registration taken out of one.
	LabelRemove Label = "REMOVE"
	// LabelFix is a declaration gg completed in a file the project wrote.
	LabelFix Label = "FIX"
	// LabelAdd is a registration added to a file.
	LabelAdd Label = "ADD"
	// LabelSkip is a file or a registration left as it is, nothing being
	// left to do to it.
	LabelSkip Label = "SKIP"
)

// labelStyles are the styles the labels print in. A label is as wide as the
// widest, so the texts after them line up.
var labelStyles = map[Label]Style{
	LabelCreate: StyleSuccess,
	LabelUpdate: StyleWarn,
	LabelRemove: StyleError,
	LabelFix:    StyleWarn,
	LabelAdd:    StyleSuccess,
	LabelSkip:   StyleMuted,
}

// labelWidth is the width of the widest label.
const labelWidth = len(LabelCreate)

// Colored reports whether standard output takes colors, for a program gg
// runs to print the way gg does.
func Colored() bool { return !color.NoColor }

// Text returns formatted text in the style, colored when standard output
// takes colors.
func Text(style Style, format string, args ...any) string {
	return paint(os.Stdout, style, formatMessage(format, args...))
}

// Section prints the title of a block of entries.
func Section(title string) {
	fmt.Fprintf(os.Stdout, "\n%s %s\n", Text(StyleInfo, "%s", SymbolSection), Text(StyleBold, "%s", title))
}

// Status prints an entry under a symbol in a style.
func Status(style Style, symbol Symbol, format string, args ...any) {
	entry(os.Stdout, style, symbol, format, args...)
}

// Success prints an entry that went well.
func Success(format string, args ...any) {
	entry(os.Stdout, StyleSuccess, SymbolSuccess, format, args...)
}

// Item prints a plain entry.
func Item(format string, args ...any) {
	entry(os.Stdout, StyleMuted, SymbolItem, format, args...)
}

// Detail prints a line under the entry before it.
func Detail(format string, args ...any) {
	fmt.Fprintf(os.Stdout, "    %s\n", formatMessage(format, args...))
}

// Change prints what the command did to a file or a registration, under the
// label naming it: "CREATE  service/sample/create.go".
func Change(label Label, format string, args ...any) {
	fmt.Fprintf(os.Stdout, "  %s  %s\n", Text(labelStyles[label], "%-*s", labelWidth, label), formatMessage(format, args...))
}

// Command prints a command to run next.
func Command(format string, args ...any) {
	entry(os.Stdout, StyleInfo, SymbolCommand, format, args...)
}

// Done prints the line a command that succeeded ends on.
func Done(format string, args ...any) {
	fmt.Fprintf(os.Stdout, "\n%s %s\n", Text(StyleSuccess, "%s", SymbolSuccess), formatMessage(format, args...))
}

// Note prints what to do next, under the line a command ends on.
func Note(format string, args ...any) {
	fmt.Fprintf(os.Stdout, "  %s\n", Text(StyleMuted, format, args...))
}

// Progress says what the command is doing while a step takes its time, on
// standard error: it is no part of what the command reports.
func Progress(format string, args ...any) {
	entry(os.Stderr, StyleMuted, SymbolItem, format, args...)
}

// Warn prints a warning, on standard error.
func Warn(format string, args ...any) {
	entry(os.Stderr, StyleWarn, SymbolWarn, format, args...)
}

// Error prints an error the command goes on from, on standard error.
func Error(format string, args ...any) {
	entry(os.Stderr, StyleError, SymbolError, format, args...)
}

// Fail prints the line a command that failed ends on, on standard error.
func Fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "\n%s %s\n", paint(os.Stderr, StyleError, string(SymbolError)), formatMessage(format, args...))
}

// Confirm asks a question to answer with yes or no, on standard error, and
// reports whether the answer read from standard input is y or yes, in
// either case; any other answer, an empty one included, is a no. An answer
// that does not come from a terminal is not echoed on it, so the line the
// question stands on is ended for what the command prints next.
func Confirm(format string, args ...any) bool {
	fmt.Fprintf(os.Stderr, "\n%s %s (y/N): ", paint(os.Stderr, StyleInfo, string(SymbolPrompt)), formatMessage(format, args...))
	if answersOf != os.Stdin {
		answers, answersOf = bufio.NewReader(os.Stdin), os.Stdin
	}
	asked := time.Now()
	answer, _ := answers.ReadString('\n')
	waited += time.Since(asked)
	if !isTerminal(os.Stdin) {
		fmt.Fprintln(os.Stderr)
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// answers reads what is typed at the questions, off the standard input
// answersOf: one reader serves every question asked of it, since a reader of
// its own for each would take, with the answer it was asked for, the answers
// typed ahead for the questions after it. A standard input put in the place
// of that one gets a reader of its own.
var (
	answers   *bufio.Reader
	answersOf *os.File
)

// waited is how long the command has waited for answers so far.
var waited time.Duration

// Waited returns how long the command has waited for the answers to its
// questions, which the time it took does not count (see Elapsed).
func Waited() time.Duration { return waited }

// Elapsed formats how long a command took for the line it ends on: 240ms
// under a second, 1.3s under a minute, 1m12s from there on.
func Elapsed(d time.Duration) string {
	switch {
	case d < time.Second:
		return d.Round(time.Millisecond).String()
	case d < time.Minute:
		return d.Round(100 * time.Millisecond).String()
	default:
		return d.Round(time.Second).String()
	}
}

// entry prints one indented line to stream: the symbol in the style, then
// the message.
func entry(stream *os.File, style Style, symbol Symbol, format string, args ...any) {
	fmt.Fprintf(stream, "  %s %s\n", paint(stream, style, string(symbol)), formatMessage(format, args...))
}

// paint returns text in the style when stream takes colors, and as it is
// otherwise. Standard output takes them when the color library says so,
// which reads NO_COLOR, TERM and whether standard output is a terminal;
// standard error takes them when it is a terminal as well, which the library
// does not look at: a redirected standard error must not carry the escape
// sequences a terminal on standard output asked for.
func paint(stream *os.File, style Style, text string) string {
	attribute, styled := attributes[style]
	if !styled || color.NoColor {
		return text
	}
	if stream != os.Stdout && !isTerminal(stream) {
		return text
	}
	painter := color.New(attribute)
	painter.EnableColor()
	return painter.Sprint(text)
}

// isTerminal reports whether file is a terminal rather than a pipe or a
// regular file.
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func formatMessage(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}
