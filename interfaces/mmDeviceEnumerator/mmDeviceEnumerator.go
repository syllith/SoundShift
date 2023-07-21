package mmDeviceEnumerator

import (
	"fmt"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

// Audio device struct
type AudioDevice struct {
	Name      string
	Id        string
	IsDefault bool
}

func GetDevices() []AudioDevice {
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		return nil
	}
	defer ole.CoUninitialize()

	var mmde *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &mmde); err != nil {
		fmt.Println(err)
		return nil
	}
	defer mmde.Release()

	var defaultDevice *wca.IMMDevice
	if err := mmde.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &defaultDevice); err != nil {
		fmt.Println(err)
		return nil
	}
	defer defaultDevice.Release()

	var defaultId string
	defaultDevice.GetId(&defaultId)

	var mmdc *wca.IMMDeviceCollection
	mmde.EnumAudioEndpoints(wca.ERender, wca.DEVICE_STATE_ACTIVE, &mmdc)
	defer mmdc.Release()

	var count uint32
	mmdc.GetCount(&count)

	audioDevices := make([]AudioDevice, count)

	for i := 0; i < int(count); i++ {
		var device *wca.IMMDevice
		mmdc.Item(uint32(i), &device)

		var id string
		device.GetId(&id)

		var propStore *wca.IPropertyStore
		device.OpenPropertyStore(wca.STGM_READ, &propStore)

		var name wca.PROPVARIANT
		propStore.GetValue(&wca.PKEY_Device_FriendlyName, &name)
		propStore.Release() // Release the property store after getting the name.

		isDefault := id == defaultId

		audioDevices[i] = AudioDevice{Name: name.String(), Id: id, IsDefault: isDefault}

		device.Release() // Release the device after processing.
	}

	return audioDevices
}
