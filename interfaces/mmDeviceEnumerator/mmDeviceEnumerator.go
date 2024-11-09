package mmDeviceEnumerator

import (
	"fmt"

	"github.com/moutend/go-wca/pkg/wca"
)

// . AudioDevice represents an audio device with its name, ID, and default status
type AudioDevice struct {
	Name      string // Friendly name of the device (e.g., "Speakers", "Headphones")
	Id        string // Unique identifier for the device
	IsDefault bool   // Flag indicating if this is the default audio device
}

// . GetDevices retrieves a list of active audio devices and identifies the default device
func GetDevices() ([]AudioDevice, error) {
	//* Create a COM instance of the multimedia device enumerator
	var mmde *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &mmde); err != nil {
		//! Failed to initialize the device enumerator
		return nil, fmt.Errorf("failed to create device enumerator: %w", err)
	}
	defer mmde.Release()

	//* Retrieve the default audio endpoint (render device for console output)
	var defaultDevice *wca.IMMDevice
	if err := mmde.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &defaultDevice); err != nil {
		//! Failed to get the default audio device
		return nil, fmt.Errorf("failed to get default audio endpoint: %w", err)
	}
	defer defaultDevice.Release()

	//* Get the ID of the default audio device for later comparison
	var defaultId string
	if err := defaultDevice.GetId(&defaultId); err != nil {
		//! Failed to retrieve the default device ID
		return nil, fmt.Errorf("failed to get ID of default device: %w", err)
	}

	//* Enumerate all active audio devices (render devices)
	var mmdc *wca.IMMDeviceCollection
	if err := mmde.EnumAudioEndpoints(wca.ERender, wca.DEVICE_STATE_ACTIVE, &mmdc); err != nil {
		//! Failed to get the collection of active audio devices
		return nil, fmt.Errorf("failed to enumerate audio endpoints: %w", err)
	}
	defer mmdc.Release()

	//* Get the total count of active audio devices
	var count uint32
	if err := mmdc.GetCount(&count); err != nil {
		//! Failed to retrieve the device count
		return nil, fmt.Errorf("failed to get count of devices: %w", err)
	}

	//* Initialize the slice to hold device information
	audioDevices := make([]AudioDevice, count)
	for i := uint32(0); i < count; i++ {
		//* Access the device at the current index in the collection
		var device *wca.IMMDevice
		if err := mmdc.Item(i, &device); err != nil {
			//! Failed to access device at index i
			return nil, fmt.Errorf("failed to get device at index %d: %w", i, err)
		}
		defer device.Release()

		//* Retrieve the unique identifier for this device
		var id string
		if err := device.GetId(&id); err != nil {
			//! Failed to get device ID
			return nil, fmt.Errorf("failed to get ID for device at index %d: %w", i, err)
		}

		//* Open the property store to access device properties like its friendly name
		var propStore *wca.IPropertyStore
		if err := device.OpenPropertyStore(wca.STGM_READ, &propStore); err != nil {
			//! Failed to access the property store for the device
			return nil, fmt.Errorf("failed to open property store for device at index %d: %w", i, err)
		}
		defer propStore.Release()

		//* Get the friendly name of the device from the property store
		var name wca.PROPVARIANT
		if err := propStore.GetValue(&wca.PKEY_Device_FriendlyName, &name); err != nil {
			//! Failed to retrieve the friendly name of the device
			return nil, fmt.Errorf("failed to get device name for device at index %d: %w", i, err)
		}

		//* Determine if this device is the default device
		isDefault := id == defaultId
		audioDevices[i] = AudioDevice{Name: name.String(), Id: id, IsDefault: isDefault}
	}

	//* Return the list of active audio devices
	return audioDevices, nil
}
