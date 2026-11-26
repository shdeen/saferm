package output

import (
	"io"
	"os"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/shdeen/saferm/internal/appstrings"
	"golang.org/x/term"
)

// formatConfigurationField returns the terminal output for one setting: the
// label in a fixed-width column with the value beside it, each wrapped onto
// further lines as needed. A list value is joined with commas, an unset value
// shows the unset marker, and an empty value shows the empty marker.
func formatConfigurationField(fieldLabel string, configuredValue any) string {
	displayedValue := displayValue(configuredValue)
	if patterns, isList := configuredValue.([]string); isList && patterns != nil {
		displayedValue = strings.Join(patterns, ", ")
	}

	if displayedValue == "" {
		displayedValue = appstrings.Text("config.empty")
	}

	labelLines := strings.Split(runewidth.Wrap(fieldLabel, configurationLabelWidth-configurationColumnGap), "\n")
	valueLines := strings.Split(runewidth.Wrap(displayedValue, configurationWidth-configurationLabelWidth), "\n")

	var formattedField strings.Builder

	for lineIndex := range max(len(labelLines), len(valueLines)) {
		labelLine, valueLine := "", ""
		if lineIndex < len(labelLines) {
			labelLine = labelLines[lineIndex]
		}

		if lineIndex < len(valueLines) {
			valueLine = valueLines[lineIndex]
		}

		formattedField.WriteString(runewidth.FillRight(labelLine, configurationLabelWidth))
		formattedField.WriteString(valueLine)
		formattedField.WriteByte('\n')
	}

	return formattedField.String()
}

// invalidValueFormatter formats the error shown below a setting whose value in
// the configuration file could not be applied.
//   - colored: whether the error is written to a terminal, which shows it in red
type invalidValueFormatter struct {
	colored bool
}

// format returns the catalog error that names the value from the file, wrapped
// between words in the value column below the setting, or an empty string when
// the value is empty.
func (formatter invalidValueFormatter) format(invalidValue string) (string, error) {
	if invalidValue == "" {
		return "", nil
	}

	errorText, err := RenderString(appstrings.Text("config.invalidValue"), invalidValue)
	if err != nil {
		return "", err
	}

	var formattedError strings.Builder

	for _, errorLine := range wrapWords(errorText, configurationWidth-configurationLabelWidth) {
		formattedError.WriteString(strings.Repeat(" ", configurationLabelWidth))

		if formatter.colored {
			errorLine = terminalRed + errorLine + terminalColorReset
		}

		formattedError.WriteString(errorLine)
		formattedError.WriteByte('\n')
	}

	return formattedError.String(), nil
}

// wrapWords splits the text into lines no wider than the width, breaking
// between words. A word wider than the width is broken across lines.
func wrapWords(text string, width int) []string {
	var (
		wrappedLines []string
		currentLine  string
	)

	for word := range strings.FieldsSeq(text) {
		for wordPart := range strings.SplitSeq(runewidth.Wrap(word, width), "\n") {
			if currentLine != "" && runewidth.StringWidth(currentLine)+1+runewidth.StringWidth(wordPart) > width {
				wrappedLines = append(wrappedLines, currentLine)
				currentLine = ""
			}

			if currentLine != "" {
				currentLine += " "
			}

			currentLine += wordPart
		}
	}

	if currentLine != "" {
		wrappedLines = append(wrappedLines, currentLine)
	}

	return wrappedLines
}

// isTerminal reports whether the writer is a terminal.
func isTerminal(writer io.Writer) bool {
	file, isFile := writer.(*os.File)

	return isFile && term.IsTerminal(int(file.Fd()))
}

// Terminal escape sequences that color the error below a setting.
//   - terminalRed: starts red text
//   - terminalColorReset: returns to the terminal's own color
const (
	terminalRed        = "\x1b[31m"
	terminalColorReset = "\x1b[0m"
)

// Layout of the configuration display, in terminal columns.
//   - configurationWidth: the width of the whole display
//   - configurationLabelWidth: the width of the label column, including the gap after a label
//   - configurationColumnGap: the least space between a label and its value
const (
	configurationWidth      = 60
	configurationLabelWidth = 18
	configurationColumnGap  = 2
)
