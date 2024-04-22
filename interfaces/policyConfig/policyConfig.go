package policyConfig

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

// * Interface
type IPolicyConfig struct {
	ole.IUnknown
}

type IPolicyConfigVtbl struct {
	ole.IUnknownVtbl
	GetMixFormat          uintptr
	GetDeviceFormat       uintptr
	ResetDeviceFormat     uintptr
	SetDeviceFormat       uintptr
	GetProcessingPeriod   uintptr
	SetProcessingPeriod   uintptr
	GetShareMode          uintptr
	SetShareMode          uintptr
	GetPropertyValue      uintptr
	SetPropertyValue      uintptr
	SetDefaultEndpoint    uintptr
	SetEndpointVisibility uintptr
}

func (v *IPolicyConfig) VTable() *IPolicyConfigVtbl {
	return (*IPolicyConfigVtbl)(unsafe.Pointer(v.RawVTable))
}

func (v *IPolicyConfig) SetDefaultEndpoint(deviceID string, eRole wca.ERole) error {
	err := pcvSetDefaultEndpoint(v, deviceID, eRole)
	if err != nil {
		return fmt.Errorf("failed to set default endpoint: %w", err)
	}
	return nil
}

// * Exports
func SetDefaultEndPoint(deviceId string) error {
	if deviceId == "" {
		return fmt.Errorf("invalid device ID provided")
	}

	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		return fmt.Errorf("failed to initialize COM library: %w", err)
	}
	defer ole.CoUninitialize()

	CPolicyConfigClientUID := ole.NewGUID("870AF99C-171D-4F9E-AF0D-E63DF40C2BC9")
	IPolicyConfigUID := ole.NewGUID("F8679F50-850A-41CF-9C72-430F290290C8")

	var pcv *IPolicyConfig
	if err := wca.CoCreateInstance(CPolicyConfigClientUID, 0, wca.CLSCTX_ALL, IPolicyConfigUID, &pcv); err != nil {
		return fmt.Errorf("failed to create IPolicyConfig instance: %w", err)
	}
	defer pcv.Release()

	roles := []wca.ERole{wca.EMultimedia, wca.ECommunications}
	for _, role := range roles {
		if err := pcv.SetDefaultEndpoint(deviceId, role); err != nil {
			return fmt.Errorf("failed to set default endpoint for role %v: %w", role, err)
		}
	}

	return nil
}

// * Backend
func pcvSetDefaultEndpoint(pcv *IPolicyConfig, deviceID string, eRole wca.ERole) error {
	if pcv == nil {
		return errors.New("IPolicyConfig reference cannot be nil")
	}

	ptr, err := syscall.UTF16PtrFromString(deviceID)
	if err != nil {
		return fmt.Errorf("failed to convert deviceID to UTF16 pointer: %w", err)
	}

	hr, _, e := syscall.SyscallN(
		pcv.VTable().SetDefaultEndpoint,
		uintptr(unsafe.Pointer(pcv)),
		uintptr(unsafe.Pointer(ptr)),
		uintptr(uint32(eRole)),
	)
	if e != 0 {
		return fmt.Errorf("syscall failed: %v", e)
	}
	if hr != 0 {
		return ole.NewError(hr)
	}
	return nil
}
