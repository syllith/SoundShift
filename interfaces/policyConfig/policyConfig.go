package policyConfig

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

// . IPolicyConfig defines the interface for managing audio policy configurations
type IPolicyConfig struct {
	ole.IUnknown
}

// . IPolicyConfigVtbl contains pointers to methods in the IPolicyConfig interface for audio policy operations
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

// . VTable retrieves the virtual method table (VTable) for the IPolicyConfig interface
func (v *IPolicyConfig) VTable() *IPolicyConfigVtbl {
	return (*IPolicyConfigVtbl)(unsafe.Pointer(v.RawVTable))
}

// . SetDefaultEndpoint sets the specified audio endpoint as the default for a given role
func (v *IPolicyConfig) SetDefaultEndpoint(deviceID string, eRole wca.ERole) error {
	//* Attempt to set the default audio endpoint for the specified device and role
	err := pcvSetDefaultEndpoint(v, deviceID, eRole)
	if err != nil {
		//! Return a wrapped error if setting the default endpoint fails
		return fmt.Errorf("failed to set default endpoint: %w", err)
	}
	//* Default endpoint successfully set
	return nil
}

// . SetDefaultEndPoint sets the given device as the default endpoint for multimedia and communication roles
func SetDefaultEndPoint(deviceId string) error {
	//* Validate the provided device ID
	if deviceId == "" {
		return fmt.Errorf("invalid device ID provided")
	}

	//* Define GUIDs for the IPolicyConfig COM interface and the client instance
	CPolicyConfigClientUID := ole.NewGUID("870AF99C-171D-4F9E-AF0D-E63DF40C2BC9")
	IPolicyConfigUID := ole.NewGUID("F8679F50-850A-41CF-9C72-430F290290C8")

	//* Create an instance of IPolicyConfig to access audio policy configuration methods
	var pcv *IPolicyConfig
	if err := wca.CoCreateInstance(CPolicyConfigClientUID, 0, wca.CLSCTX_ALL, IPolicyConfigUID, &pcv); err != nil {
		//! Return an error if the COM instance creation fails
		return fmt.Errorf("failed to create IPolicyConfig instance: %w", err)
	}
	defer pcv.Release() // Ensure the COM object is released after usage

	//* Set the specified device as the default endpoint for both multimedia and communication roles
	roles := []wca.ERole{wca.EMultimedia, wca.ECommunications}
	for _, role := range roles {
		if err := pcv.SetDefaultEndpoint(deviceId, role); err != nil {
			//! Return an error if setting the default endpoint fails for any role
			return fmt.Errorf("failed to set default endpoint for role %v: %w", role, err)
		}
	}

	//* Default endpoint successfully set for all roles
	return nil
}

// . pcvSetDefaultEndpoint makes a syscall to set the default audio endpoint for a specific role
func pcvSetDefaultEndpoint(pcv *IPolicyConfig, deviceID string, eRole wca.ERole) error {
	//* Validate that the IPolicyConfig reference is not nil
	if pcv == nil {
		return errors.New("IPolicyConfig reference cannot be nil")
	}

	//* Convert the device ID string to a UTF-16 pointer for use in the syscall
	ptr, err := syscall.UTF16PtrFromString(deviceID)
	if err != nil {
		//! Return an error if the conversion to UTF-16 fails
		return fmt.Errorf("failed to convert deviceID to UTF16 pointer: %w", err)
	}

	//* Execute the syscall to set the default endpoint
	hr, _, e := syscall.SyscallN(
		pcv.VTable().SetDefaultEndpoint, // Address of the SetDefaultEndpoint function
		uintptr(unsafe.Pointer(pcv)),    // Pointer to IPolicyConfig instance
		uintptr(unsafe.Pointer(ptr)),    // Pointer to UTF-16 device ID
		uintptr(uint32(eRole)),          // Role (e.g., multimedia or communications)
	)
	if e != 0 {
		//! Syscall encountered an error
		return fmt.Errorf("syscall failed: %v", e)
	}
	if hr != 0 {
		//! COM call returned an error code (HRESULT)
		return ole.NewError(hr)
	}

	//* Default endpoint set successfully for the specified role
	return nil
}

// . SetVolume sets the volume level for the specified audio device
func SetVolume(deviceID string, volumeLevel float32) error {
	var deviceEnumerator *wca.IMMDeviceEnumerator
	var deviceCollection *wca.IMMDeviceCollection
	var audioEndpointVolume *wca.IAudioEndpointVolume

	//* Create an instance of the device enumerator to access audio devices
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &deviceEnumerator); err != nil {
		//! Return an error if unable to create the device enumerator instance
		return fmt.Errorf("failed to create device enumerator instance: %w", err)
	}
	defer deviceEnumerator.Release()

	//* Enumerate active audio rendering devices
	if err := deviceEnumerator.EnumAudioEndpoints(wca.ERender, wca.DEVICE_STATE_ACTIVE, &deviceCollection); err != nil {
		//! Return an error if unable to enumerate audio endpoints
		return fmt.Errorf("failed to enumerate audio endpoints: %w", err)
	}
	defer deviceCollection.Release()

	//* Get the count of available audio devices
	var count uint32
	if err := deviceCollection.GetCount(&count); err != nil {
		//! Return an error if unable to retrieve the count of devices
		return fmt.Errorf("failed to get count of devices: %w", err)
	}

	//* Iterate over each device to find the one matching the specified deviceID
	for i := uint32(0); i < count; i++ {
		var device *wca.IMMDevice
		if err := deviceCollection.Item(i, &device); err != nil {
			//. Skip to next device if unable to retrieve this one
			continue
		}
		defer device.Release()

		//* Retrieve the unique identifier for the device
		var id string
		if err := device.GetId(&id); err != nil {
			//. Skip to next device if unable to get ID
			continue
		}

		//* Check if the current device matches the specified deviceID
		if id == deviceID {
			//* Activate the audio endpoint volume interface for volume control
			if err := device.Activate(wca.IID_IAudioEndpointVolume, wca.CLSCTX_ALL, nil, &audioEndpointVolume); err != nil {
				//! Return an error if unable to activate the endpoint volume interface
				return fmt.Errorf("failed to activate endpoint volume interface for device %s: %w", deviceID, err)
			}
			defer audioEndpointVolume.Release()

			//* Set the master volume level for the device
			if err := audioEndpointVolume.SetMasterVolumeLevelScalar(volumeLevel, nil); err != nil {
				//! Return an error if setting the volume level fails
				return fmt.Errorf("failed to set volume level for device %s: %w", deviceID, err)
			}
			break // Exit loop once the volume has been set for the specified device
		}
	}

	//* Volume level successfully set for the specified device
	return nil
}

// . GetVolume retrieves the current volume level for the specified audio device
func GetVolume(deviceID string) (float32, error) {
	var deviceEnumerator *wca.IMMDeviceEnumerator
	var deviceCollection *wca.IMMDeviceCollection
	var audioEndpointVolume *wca.IAudioEndpointVolume

	//* Create an instance of the device enumerator to access audio devices
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &deviceEnumerator); err != nil {
		//! Return an error if unable to create the device enumerator instance
		return 0, fmt.Errorf("failed to create device enumerator instance: %w", err)
	}
	defer deviceEnumerator.Release()

	//* Enumerate active audio rendering devices
	if err := deviceEnumerator.EnumAudioEndpoints(wca.ERender, wca.DEVICE_STATE_ACTIVE, &deviceCollection); err != nil {
		//! Return an error if unable to enumerate audio endpoints
		return 0, fmt.Errorf("failed to enumerate audio endpoints: %w", err)
	}
	defer deviceCollection.Release()

	//* Get the count of available audio devices
	var count uint32
	if err := deviceCollection.GetCount(&count); err != nil {
		//! Return an error if unable to retrieve the count of devices
		return 0, fmt.Errorf("failed to get count of devices: %w", err)
	}

	//* Iterate over each device to find the one matching the specified deviceID
	for i := uint32(0); i < count; i++ {
		var device *wca.IMMDevice
		if err := deviceCollection.Item(i, &device); err != nil {
			//. Skip to next device if unable to retrieve this one
			continue
		}
		defer device.Release()

		//* Retrieve the unique identifier for the device
		var id string
		if err := device.GetId(&id); err != nil {
			//. Skip to next device if unable to get ID
			continue
		}

		//* Check if the current device matches the specified deviceID
		if id == deviceID {
			//* Activate the audio endpoint volume interface to access the volume level
			if err := device.Activate(wca.IID_IAudioEndpointVolume, wca.CLSCTX_ALL, nil, &audioEndpointVolume); err != nil {
				//! Return an error if unable to activate the endpoint volume interface
				return 0, fmt.Errorf("failed to activate endpoint volume interface for device %s: %w", deviceID, err)
			}
			defer audioEndpointVolume.Release()

			//* Retrieve the current volume level for the device
			var currentVolume float32
			if err := audioEndpointVolume.GetMasterVolumeLevelScalar(&currentVolume); err != nil {
				//! Return an error if unable to get the volume level
				return 0, fmt.Errorf("failed to get volume level for device %s: %w", deviceID, err)
			}
			//* Return the retrieved volume level
			return currentVolume, nil
		}
	}

	//! Return an error if the specified deviceID is not found
	return 0, fmt.Errorf("device not found")
}
