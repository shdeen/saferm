package output

import (
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/shdeen/saferm/internal/appstrings"
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

// Layout of the configuration display, in terminal columns.
//   - configurationWidth: the width of the whole display
//   - configurationLabelWidth: the width of the label column, including the gap after a label
//   - configurationColumnGap: the least space between a label and its value
const (
	configurationWidth      = 60
	configurationLabelWidth = 18
	configurationColumnGap  = 2
)
