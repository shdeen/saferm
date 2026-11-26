package desktop

// Invariants tested:
//  1. Windows activation: An existing application receives the launching process's arguments through its message-only window.

import (
	"encoding/json"
	"os"
	"reflect"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"
	"golang.org/x/sys/windows"
)

// TestWindowsActivation verifies invariant #1: Windows activation.
// Test class: Expanded.
// Test layer: Coverage.
func TestWindowsActivation(t *testing.T) {
	runtime.LockOSThread()

	defer runtime.UnlockOSThread()

	applicationID := "saferm-test-" + uuid.NewString()
	if forwarded, err := ForwardActivation(applicationID); err != nil || forwarded {
		t.Errorf("✗ activation forwarded without a running application: %v, %v", forwarded, err)
	}

	receiver := activationReceiver{deliveries: make(chan string, 1)}
	className := windows.StringToUTF16Ptr("wails-app-" + applicationID + "-sic")
	windowClass := w32.WNDCLASSEX{Instance: w32.GetModuleHandle(""), ClassName: className}
	windowClass.Size = uint32(unsafe.Sizeof(windowClass))

	windowClass.WndProc = syscall.NewCallback(receiver.handleMessage)
	if w32.RegisterClassEx(&windowClass) == 0 {
		t.Fatal("💣 register activation test window")
	}

	unregisterClass := windows.NewLazySystemDLL("user32.dll").NewProc("UnregisterClassW")
	// #nosec G103 -- The UTF-16 class name stays alive until native class cleanup completes.
	defer unregisterClass.Call(uintptr(unsafe.Pointer(className)), windowClass.Instance)

	window := w32.CreateWindowEx(0, className, windows.StringToUTF16Ptr("wails-app-"+applicationID+"-siw"), 0, 0, 0, 0, 0, w32.HWND_MESSAGE, 0, windowClass.Instance, nil)
	if window == 0 {
		t.Fatal("💣 create activation test window")
	}
	defer w32.DestroyWindow(window)

	if forwarded, err := ForwardActivation(applicationID); err != nil || !forwarded {
		t.Errorf("✗ existing application did not receive activation: %v, %v", forwarded, err)
	}

	select {
	case document := <-receiver.deliveries:
		var activation application.SecondInstanceData
		if err := json.Unmarshal([]byte(document), &activation); err != nil || !reflect.DeepEqual(activation.Args, os.Args) {
			t.Errorf("✗ activation arguments changed: %+v, %v", activation, err)
		}
	default:
		t.Error("✗ activation was not delivered")
	}

	if !t.Failed() {
		t.Log("✓ existing Windows application receives activation")
	}
}

type activationReceiver struct {
	deliveries chan string
}

// handleMessage copies the OS-owned message while WM_COPYDATA guarantees its lifetime.
func (receiver *activationReceiver) handleMessage(windowHandle w32.HWND, windowMessage uint32, parameter w32.WPARAM, dataPointer w32.LPARAM) w32.LRESULT {
	if windowMessage != w32.WM_COPYDATA {
		return w32.DefWindowProc(windowHandle, windowMessage, parameter, dataPointer)
	}

	var message w32.COPYDATASTRUCT
	// ReadProcessMemory copies native memory without converting an OS integer
	// address into a Go pointer. The destination is an allocated Go struct.
	// #nosec G103 -- The destination size is exactly the allocated struct size.
	if err := windows.ReadProcessMemory(windows.CurrentProcess(), dataPointer, (*byte)(unsafe.Pointer(&message)), unsafe.Sizeof(message), nil); err != nil {
		return 0
	}

	if message.DwData != w32.WMCOPYDATA_SINGLE_INSTANCE_DATA || message.CbData < 2 {
		return 0
	}

	characters := make([]uint16, message.CbData/2)
	// #nosec G103 -- The read is bounded by the allocated slice length in bytes.
	if err := windows.ReadProcessMemory(windows.CurrentProcess(), message.LpData, (*byte)(unsafe.Pointer(&characters[0])), uintptr(len(characters)*2), nil); err == nil {
		receiver.deliveries <- windows.UTF16ToString(characters)
	}

	return 0
}
