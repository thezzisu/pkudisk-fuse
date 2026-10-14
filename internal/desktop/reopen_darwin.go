package desktop

import (
	"fmt"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

// LaunchServices normally sends an Apple Event when an already-running .app
// is opened again; it does not start a second process. Register after AppKit
// initialization, on the main thread. Carbon is a system framework, and goffi
// calls it without CGO or an additional executable.
func installReopen(open func()) (func(), error) {
	library, err := ffi.LoadLibrary("/System/Library/Frameworks/Carbon.framework/Carbon")
	if err != nil {
		return nil, fmt.Errorf("load macOS application events: %w", err)
	}
	install, err := ffi.GetSymbol(library, "AEInstallEventHandler")
	if err != nil {
		return nil, err
	}
	remove, err := ffi.GetSymbol(library, "AERemoveEventHandler")
	if err != nil {
		return nil, err
	}
	callback := ffi.NewCallback(func(event, reply, refcon uintptr) uintptr {
		open()
		return 0 // noErr
	})
	eventClass, eventID := uint32(0x61657674), uint32(0x72617070) // aevt / rapp
	var refcon uintptr
	var systemHandler uint8
	var status int32
	cif := &types.CallInterface{}
	err = ffi.PrepareCallInterface(cif, types.DefaultCall, types.SInt32TypeDescriptor,
		[]*types.TypeDescriptor{types.UInt32TypeDescriptor, types.UInt32TypeDescriptor,
			types.PointerTypeDescriptor, types.PointerTypeDescriptor, types.UInt8TypeDescriptor})
	if err != nil {
		return nil, err
	}
	_, err = ffi.CallFunction(cif, install, unsafe.Pointer(&status), []unsafe.Pointer{
		unsafe.Pointer(&eventClass), unsafe.Pointer(&eventID), unsafe.Pointer(&callback),
		unsafe.Pointer(&refcon), unsafe.Pointer(&systemHandler)})
	if err != nil {
		return nil, err
	}
	if status != 0 {
		return nil, fmt.Errorf("install macOS reopen handler: %d", status)
	}
	return func() {
		cleanup := &types.CallInterface{}
		if ffi.PrepareCallInterface(cleanup, types.DefaultCall, types.SInt32TypeDescriptor,
			[]*types.TypeDescriptor{types.UInt32TypeDescriptor, types.UInt32TypeDescriptor,
				types.PointerTypeDescriptor, types.UInt8TypeDescriptor}) != nil {
			return
		}
		_, _ = ffi.CallFunction(cleanup, remove, unsafe.Pointer(&status), []unsafe.Pointer{
			unsafe.Pointer(&eventClass), unsafe.Pointer(&eventID), unsafe.Pointer(&callback), unsafe.Pointer(&systemHandler)})
	}, nil
}
