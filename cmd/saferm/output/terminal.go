// Package output renders operational results and owns terminal confirmation input.
package output

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/saferm"
)

// Terminal carries out a request on the streams of one invocation.
//   - Input: the source of confirmation answers
//   - Output: the destination for results
//   - Errors: the destination for failures and confirmation prompts
//   - JSON: whether to write JSON instead of plain text
type Terminal struct {
	Input  io.Reader
	Output io.Writer
	Errors io.Writer
	JSON   bool
}

// Execute carries out the request through saferm.Run and writes its result.
// When the operation needs confirmation, Execute prompts for it, then either
// reruns the request as confirmed or writes a cancellation notice. It writes a
// result that accompanies an error and returns that error.
func (terminal Terminal) Execute(ctx context.Context, request *saferm.Request) error {
	result, err := saferm.Run(ctx, request)
	if errors.Is(err, errs.ErrConfirmation) && result.Confirmation != nil {
		approved, confirmErr := terminal.confirm(ctx, *result.Confirmation)
		if confirmErr != nil {
			return confirmErr
		}

		if !approved {
			return render(terminal.Output, terminal.JSON, request.Command, saferm.Result{Notice: appstrings.Text("notice.canceled")})
		}

		confirmedRequest := *request
		confirmedRequest.Confirmed = true
		result, err = saferm.Run(ctx, &confirmedRequest)
	}

	if err != nil && result.Actions == nil && result.Config == nil && result.Removed == nil && result.Problems == nil {
		return err
	}

	return errors.Join(err, render(terminal.Output, terminal.JSON, request.Command, result))
}

// Failure writes the error to the Errors stream, as a JSON object when JSON is
// set.
func (terminal Terminal) Failure(err error) error {
	if terminal.JSON {
		return writeJSON(terminal.Errors, struct {
			Error string `json:"error"`
		}{err.Error()})
	}

	return renderTemplate(terminal.Errors, "failure", err)
}

// confirm writes the confirmation's prompt to the Errors stream, reads one line
// from Input, and reports whether that line confirms. The line confirms when it
// is y or yes in any letter case, or when it is empty and the confirmation
// defaults to yes. Input that ends before a full line does not confirm.
func (terminal Terminal) confirm(ctx context.Context, confirmation saferm.Confirmation) (bool, error) {
	prompt, err := RenderString(confirmation.Prompt, confirmation)
	if err != nil {
		return false, err
	}

	hintKey := "prompt.defaultNoHint"
	if confirmation.DefaultYes {
		hintKey = "prompt.defaultYesHint"
	}

	prompt += " " + appstrings.Text(hintKey)

	if terminal.JSON {
		confirmation.Prompt = prompt
		err = writeJSON(terminal.Errors, confirmation)
	} else {
		_, err = fmt.Fprintln(terminal.Errors, prompt)
	}

	if err != nil {
		return false, fmt.Errorf("%w: %w", errs.ErrOutputWrite, err)
	}

	answers := make(chan answer, 1)
	go readAnswer(terminal.Input, answers)

	select {
	case <-ctx.Done():
		return false, fmt.Errorf("%w: %w", errs.ErrOperationCanceled, ctx.Err())
	case response := <-answers:
		if errors.Is(response.err, io.EOF) {
			return false, nil
		}

		if response.err != nil {
			return false, fmt.Errorf("%w: %w", errs.ErrInputRead, response.err)
		}

		input := strings.TrimSpace(response.text)

		return (input == "" && confirmation.DefaultYes) || strings.EqualFold(input, "y") || strings.EqualFold(input, "yes"), nil
	}
}

// answer is one line read from the confirmation input, with the error that
// ended the read, if any.
type answer struct {
	text string
	err  error
}

// readAnswer reads one line from the reader and sends it on the channel with
// any read error.
func readAnswer(reader io.Reader, answers chan<- answer) {
	response, err := bufio.NewReader(reader).ReadString('\n')
	answers <- answer{text: response, err: err}
}
