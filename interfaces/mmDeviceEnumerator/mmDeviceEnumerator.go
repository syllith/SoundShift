package mmDeviceEnumerator

import (
	"fmt"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

func Init() {
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		return
	}
	defer ole.CoUninitialize()

	var mmde *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &mmde); err != nil {
		fmt.Println(err)
		return
	}
	defer mmde.Release()

	var mmdc *wca.IMMDeviceCollection
	mmde.EnumAudioEndpoints(wca.ERender, wca.DEVICE_STATE_ACTIVE, &mmdc)

	var count uint32
	mmdc.GetCount(&count)

	for i := 0; i < int(count); i++ {
		var device *wca.IMMDevice
		mmdc.Item(uint32(i), &device)

		var id string
		device.GetId(&id)

		fmt.Println(id)
	}

	// if err := mmde.EnumAudioEndpoints(wca.EAll, wca.DEVICE_STATE_ACTIVE, &mmdc); err != nil {
	// 	fmt.Println(err)
	// 	return
	// }

}
