package output

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/saferm"
)

// resultTemplate holds the templates for plain-text results.
//
//go:embed result.tmpl
var resultTemplate string

// render writes an operation's result to the writer, either as JSON or through
// the plain-text template that fits the command and the contents of the result.
func render(writer io.Writer, asJSON bool, command string, result saferm.Result) error {
	if asJSON {
		return renderJSON(writer, command, result)
	}

	if result.Notice != "" {
		return renderTemplate(writer, "notice", result.Notice)
	}

	if result.Document != "" {
		if _, err := io.WriteString(writer, result.Document); err != nil {
			return fmt.Errorf("%w: %w", errs.ErrOutputWrite, err)
		}

		return nil
	}

	if result.Config != nil {
		return renderTemplate(writer, "config", result.Config)
	}

	if command == saferm.Cleanup {
		return renderTemplate(writer, "cleanup", result)
	}

	if command == saferm.List || command == saferm.View {
		return renderTemplate(writer, command, result)
	}

	return renderTemplate(writer, "summary", result)
}

// renderJSON writes the result as JSON:
//   - the settings, for a result that has settings and no notice
//   - the list of actions, for the list command
//   - the action, for the view command
//
// For every other command it writes the whole result. A delete, move, dispose,
// restore, or purge result holds its actions in a list, also when it holds one
// action, with any problems beside them.
func renderJSON(writer io.Writer, command string, result saferm.Result) error {
	if result.Config != nil && result.Notice == "" {
		return writeJSON(writer, result.Config)
	}

	if command == saferm.List {
		return writeJSON(writer, result.Actions)
	}

	if command == saferm.View && len(result.Actions) == 1 {
		return writeJSON(writer, result.Actions[0])
	}

	return writeJSON(writer, result)
}

// writeJSON writes the value to the writer as indented JSON.
func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("%w: %w", errs.ErrOutputWrite, err)
	}

	return nil
}

// renderTemplate executes the named result template with the data and writes
// the output to the writer with its tab-separated columns aligned.
func renderTemplate(writer io.Writer, name string, data any) error {
	invalidValues := invalidValueFormatter{colored: isTerminal(writer)}
	functions := template.FuncMap{"appstring": appstrings.Text, "date": displayDate, "status": displayStatus, "operation": displayOperation, "value": displayValue, "configField": formatConfigurationField, "invalidValue": invalidValues.format}

	parsed, err := template.New("result").Funcs(functions).Parse(resultTemplate)
	if err != nil {
		return fmt.Errorf("%q: %w: %w", name, errs.ErrTemplateRender, err)
	}

	var buffer bytes.Buffer
	if err = parsed.ExecuteTemplate(&buffer, name, data); err != nil {
		return fmt.Errorf("%q: %w: %w", name, errs.ErrTemplateRender, err)
	}

	table := tabwriter.NewWriter(writer, tableMinimumWidth, tableTabWidth, tablePadding, tablePaddingCharacter, tableFlags)
	if _, err = buffer.WriteTo(table); err != nil {
		return fmt.Errorf("%w: %w", errs.ErrOutputWrite, err)
	}

	if err = table.Flush(); err != nil {
		return fmt.Errorf("%w: %w", errs.ErrOutputWrite, err)
	}

	return nil
}

// RenderString executes a template document with the data and returns the
// output.
func RenderString(document string, data any) (string, error) {
	parsed, err := template.New("prompt").Parse(document)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errs.ErrTemplateRender, err)
	}

	var buffer bytes.Buffer
	if err = parsed.Execute(&buffer, data); err != nil {
		return "", fmt.Errorf("%w: %w", errs.ErrTemplateRender, err)
	}

	return buffer.String(), nil
}

// displayDate formats a timestamp in local time, or returns the unset marker
// for the zero time.
func displayDate(timestamp time.Time) string {
	if timestamp.IsZero() {
		return appstrings.Text("display.unset")
	}

	return timestamp.Local().Format(appstrings.Text("display.date"))
}

// displayStatus returns the name of a status code.
func displayStatus(status int) string { return appstrings.Text("status." + strconv.Itoa(status)) }

// displayOperation returns the display name of an action type.
func displayOperation(operation string) string { return appstrings.Text("operation." + operation) }

// displayValue returns the text of a string, integer, or list value, with a
// list in brackets. It returns the unset marker for a nil pointer, a nil list,
// and an empty string.
func displayValue(value any) string {
	switch setting := value.(type) {
	case *string:
		if setting != nil {
			return *setting
		}
	case *int:
		if setting != nil {
			return strconv.Itoa(*setting)
		}
	case []string:
		if setting != nil {
			return "[" + strings.Join(setting, ", ") + "]"
		}
	case string:
		if setting != "" {
			return setting
		}
	}

	return appstrings.Text("display.unset")
}

// Settings for aligning the columns of plain-text results.
const (
	tableMinimumWidth     = 0
	tableTabWidth         = 4
	tablePadding          = 2
	tablePaddingCharacter = ' '
	tableFlags            = 0
)
