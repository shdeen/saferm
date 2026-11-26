// Package notify carries the IDs of actions that await review to the optional
// SafeRM Notify program, over a loopback connection. It has no desktop
// dependencies and never starts SafeRM Notify.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/shdeen/saferm/internal/config"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/filesystem"
)

// Parameters of the connection between Send and a Listener.
//   - endpointFile: the file in the SafeRM directory that publishes the listener's endpoint
//   - connectionTimeout: the time allowed to connect, and to read or write one message
//   - maximumMessageBytes: the largest message that a Listener reads
const (
	endpointFile        = "notifications.json"
	connectionTimeout   = 250 * time.Millisecond
	maximumMessageBytes = 1024
)

// endpoint is the published location of a Listener.
type endpoint struct {
	// Address is the listener's loopback host and port.
	Address string `json:"address"`
	// Token is the secret that a message must repeat for the listener to accept it.
	Token string `json:"token"`
}

// message asks SafeRM Notify to notify the user about one action.
type message struct {
	// ID is the ID of the action that awaits review.
	ID int64 `json:"id"`
	// Token is the token from the listener's endpoint.
	Token string `json:"token"`
}

// Listener receives action IDs from processes of the same user.
type Listener struct {
	// socket accepts the connections that deliver messages.
	socket net.Listener
	// endpoint is the address and token that the listener published.
	endpoint endpoint
	// path is the location of the endpoint file.
	path string
}

// Listen opens a listener on a loopback port and publishes its address, with a
// newly generated token, in the endpoint file under the home directory.
func Listen(ctx context.Context, home string) (*Listener, error) {
	configuration := net.ListenConfig{}

	socket, err := configuration.Listen(ctx, "tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errs.ErrNotificationReceive, err)
	}

	listener := &Listener{socket: socket, endpoint: endpoint{Address: socket.Addr().String(), Token: uuid.NewString()}, path: filepath.Join(home, config.Directory, endpointFile)}

	document, err := json.Marshal(listener.endpoint)
	if err == nil {
		err = os.MkdirAll(filepath.Dir(listener.path), filesystem.PrivateDirectoryMode)
	}

	if err == nil {
		err = os.WriteFile(listener.path, document, filesystem.PrivateFileMode)
	}

	if err != nil {
		return nil, fmt.Errorf("%q: %w: %w", listener.path, errs.ErrNotificationReceive, errors.Join(err, socket.Close()))
	}

	return listener, nil
}

// Receive waits for the next message and returns its action ID. It returns
// ErrInputInvalid for a message with the wrong token or a nonpositive ID, and
// an error that wraps net.ErrClosed once the listener is closed.
func (listener *Listener) Receive() (int64, error) {
	connection, err := listener.socket.Accept()
	if err != nil {
		return 0, fmt.Errorf("%w: %w", errs.ErrNotificationReceive, err)
	}
	defer connection.Close() //nolint:errcheck // Closing a received message's connection cannot change its validity.

	if err = connection.SetDeadline(time.Now().Add(connectionTimeout)); err != nil {
		return 0, fmt.Errorf("%w: %w", errs.ErrNotificationReceive, err)
	}

	var request message
	if err = json.NewDecoder(io.LimitReader(connection, maximumMessageBytes)).Decode(&request); err != nil {
		return 0, fmt.Errorf("%w: %w", errs.ErrNotificationReceive, err)
	}

	if request.Token != listener.endpoint.Token || request.ID <= 0 {
		return 0, errs.ErrInputInvalid
	}

	return request.ID, nil
}

// Close closes the listener and removes its endpoint file.
func (listener *Listener) Close() error {
	closeErr := listener.socket.Close()

	removeErr := os.Remove(listener.path)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}

	if err := errors.Join(closeErr, removeErr); err != nil {
		return fmt.Errorf("%q: %w: %w", listener.path, errs.ErrNotificationReceive, err)
	}

	return nil
}

// Send delivers the action ID to the listener that the endpoint file under the
// home directory publishes. It returns nil without sending when there is no
// endpoint file or when nothing listens at the endpoint. It returns
// ErrInputInvalid for a nonpositive ID and for an endpoint that has no token or
// is not on the loopback address.
func Send(ctx context.Context, home string, actionID int64) error {
	document, err := os.ReadFile(filepath.Join(home, config.Directory, endpointFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	var destination endpoint
	if err == nil {
		err = json.Unmarshal(document, &destination)
	}

	if err != nil {
		return fmt.Errorf("%w: %w", errs.ErrNotificationSend, err)
	}

	host, _, err := net.SplitHostPort(destination.Address)
	if err != nil || host != "127.0.0.1" || destination.Token == "" || actionID <= 0 {
		return errs.ErrInputInvalid
	}

	dialer := net.Dialer{Timeout: connectionTimeout}

	connection, err := dialer.DialContext(ctx, "tcp4", destination.Address)
	if err != nil {
		return nil // A disabled or exited SafeRM Notify leaves no running listener.
	}
	defer connection.Close() //nolint:errcheck // Delivery errors are reported by Encode; closing does not acknowledge delivery.

	if err = connection.SetWriteDeadline(time.Now().Add(connectionTimeout)); err == nil {
		err = json.NewEncoder(connection).Encode(message{ID: actionID, Token: destination.Token})
	}

	if err != nil {
		return fmt.Errorf("%d: %w: %w", actionID, errs.ErrNotificationSend, err)
	}

	return nil
}
