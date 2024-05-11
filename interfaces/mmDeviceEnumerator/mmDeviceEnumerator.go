package mmDeviceEnumerator

import (
	"fmt"

	"github.com/moutend/go-wca/pkg/wca"
)

// Audio device struct
type AudioDevice struct {
	Name      string
	Id        string
	IsDefault bool
}

func GetDevices() ([]AudioDevice, error) {
	var mmde *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &mmde); err != nil {
		return nil, fmt.Errorf("failed to create device enumerator: %w", err)
	}
	defer mmde.Release()

	var defaultDevice *wca.IMMDevice
	if err := mmde.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &defaultDevice); err != nil {
		return nil, fmt.Errorf("failed to get default audio endpoint: %w", err)
	}
	defer defaultDevice.Release()

	var defaultId string
	if err := defaultDevice.GetId(&defaultId); err != nil {
		return nil, fmt.Errorf("failed to get ID of default device: %w", err)
	}

	var mmdc *wca.IMMDeviceCollection
	if err := mmde.EnumAudioEndpoints(wca.ERender, wca.DEVICE_STATE_ACTIVE, &mmdc); err != nil {
		return nil, fmt.Errorf("failed to enumerate audio endpoints: %w", err)
	}
	defer mmdc.Release()

	var count uint32
	if err := mmdc.GetCount(&count); err != nil {
		return nil, fmt.Errorf("failed to get count of devices: %w", err)
	}

	audioDevices := make([]AudioDevice, count)
	for i := uint32(0); i < count; i++ {
		var device *wca.IMMDevice
		if err := mmdc.Item(i, &device); err != nil {
			return nil, fmt.Errorf("failed to get device at index %d: %w", i, err)
		}
		defer device.Release()

		var id string
		if err := device.GetId(&id); err != nil {
			return nil, fmt.Errorf("failed to get ID for device at index %d: %w", i, err)
		}

		var propStore *wca.IPropertyStore
		if err := device.OpenPropertyStore(wca.STGM_READ, &propStore); err != nil {
			return nil, fmt.Errorf("failed to open property store for device at index %d: %w", i, err)
		}
		defer propStore.Release()

		var name wca.PROPVARIANT
		if err := propStore.GetValue(&wca.PKEY_Device_FriendlyName, &name); err != nil {
			return nil, fmt.Errorf("failed to get device name for device at index %d: %w", i, err)
		}

		isDefault := id == defaultId
		audioDevices[i] = AudioDevice{Name: name.String(), Id: id, IsDefault: isDefault}
	}

	return audioDevices, nil
}
