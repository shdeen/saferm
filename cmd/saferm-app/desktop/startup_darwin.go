//go:build darwin

package desktop

/*
#cgo CFLAGS: -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework Foundation -framework ServiceManagement
#include <stdlib.h>
char *saferm_register_notifier(const char *identifier);
*/
import "C" //nolint:gocritic // C and unsafe are distinct imports required by cgo, not duplicate aliases.

import (
	"fmt"
	"unsafe" //nolint:gocritic // C and unsafe are distinct imports required by cgo, not duplicate aliases.

	"github.com/shdeen/saferm/internal/errs"
)

// registerNotifier registers SafeRM Notify as a login item through
// saferm_register_notifier. On failure, it returns ErrNotifierSetup with the
// system's description of the failure.
func registerNotifier(notifierPath string) error {
	identifier := C.CString(NotifierID)
	defer C.free(unsafe.Pointer(identifier)) //nolint:nlreturn // cgo generates a wrapper with no editable return spacing.

	failure := C.saferm_register_notifier(identifier)
	if failure == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(failure)) //nolint:nlreturn // cgo generates a wrapper with no editable return spacing.

	return fmt.Errorf("%q, %s: %w", notifierPath, C.GoString(failure), errs.ErrNotifierSetup)
}
