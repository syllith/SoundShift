package policyConfig

import (
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

func (v *IPolicyConfig) SetDefaultEndpoint(deviceID string, eRole wca.ERole) (err error) {
	err = pcvSetDefaultEndpoint(v, deviceID, eRole)
	return
}

// * Exports
func SetDefaultEndPoint(deviceId string) {
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		return
	}
	defer ole.CoUninitialize()

	CPolicyConfigClientUID := ole.NewGUID("870AF99C-171D-4F9E-AF0D-E63DF40C2BC9")
	IPolicyConfigUID := ole.NewGUID("F8679F50-850A-41CF-9C72-430F290290C8")

	var pcv *IPolicyConfig
	if err := wca.CoCreateInstance(CPolicyConfigClientUID, 0, wca.CLSCTX_ALL, IPolicyConfigUID, &pcv); err != nil {
		return
	}
	defer pcv.Release()

	pcv.SetDefaultEndpoint(deviceId, wca.EMultimedia)
	pcv.SetDefaultEndpoint(deviceId, wca.ECommunications)
}

// * Backend
func pcvSetDefaultEndpoint(pcv *IPolicyConfig, deviceID string, eRole wca.ERole) (err error) {
	var ptr *uint16
	if ptr, err = syscall.UTF16PtrFromString(deviceID); err != nil {
		return
	}

	hr, _, _ := syscall.SyscallN(
		pcv.VTable().SetDefaultEndpoint,
		uintptr(unsafe.Pointer(pcv)),
		uintptr(unsafe.Pointer(ptr)),
		uintptr(uint32(eRole)))
	if hr != 0 {
		err = ole.NewError(hr)
	}
	return
}
